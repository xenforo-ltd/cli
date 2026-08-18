package main

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/xenforo-ltd/cli/internal/cache"
	"github.com/xenforo-ltd/cli/internal/config"
	"github.com/xenforo-ltd/cli/internal/customerapi"
	"github.com/xenforo-ltd/cli/internal/dockercompose"
	"github.com/xenforo-ltd/cli/internal/downloads"
	"github.com/xenforo-ltd/cli/internal/extract"
	"github.com/xenforo-ltd/cli/internal/ui"
	"github.com/xenforo-ltd/cli/internal/xf"
	"github.com/xenforo-ltd/cli/internal/xfcmd"
)

// plannedInitSteps returns the number of steps that will be printed for a
// fresh install run with the given options. Every step that runs, or that is
// reachable but skipped (and therefore printed via printSkippedStep), counts
// toward the total; steps that are entirely unreachable (composer with no
// composer.json, or anything gated behind a skipped "Starting Docker
// environment" step) do not.
func plannedInitSteps(opts InitOptions, hasComposer bool) int {
	// Preparing target directory, Downloading XenForo files, Extracting
	// XenForo files, Setting up Docker configuration, Configuring
	// environment, Starting Docker environment.
	total := 6

	if opts.SkipUp {
		return total
	}

	if hasComposer {
		total++
	}

	// Installing XenForo always occupies a slot once containers can start,
	// whether it runs or is printed as skipped.
	total++

	return total
}

func executeInit(ctx context.Context, opts *InitOptions) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	if opts.InstanceName == "" {
		opts.InstanceName = xf.GenerateInstanceName(filepath.Base(opts.TargetPath))
	}

	if opts.SiteTitle == "" {
		opts.SiteTitle = "XenForo"
	}

	client, err := customerapi.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create customer API client: %w", err)
	}

	titleMap := getProductTitleMap(ctx, client, opts.LicenseKey)

	// The composer-dependencies step only exists for packages that ship a
	// composer.json (repository checkouts; see shouldRunComposer). Whether
	// that's true can only be known once the xenforo package itself is
	// available, so it's resolved (using the cache - free if this exact
	// version was already downloaded) before the first step total is
	// committed to output. Without this, "Preparing target directory" would
	// print a provisional [1/n] that a later-discovered composer.json could
	// invalidate, leaving an earlier line on screen that never matches the
	// final [n/n].
	hasComposer, err := detectComposerBeforeDownload(ctx, client, opts)
	if err != nil {
		return err
	}

	totalSteps := plannedInitSteps(*opts, hasComposer)
	step := 1

	ui.PrintStep(step, totalSteps, "Preparing target directory")
	ui.Printf("%s%s\n", ui.Indent2, ui.Path.Render(ui.ShortHome(opts.TargetPath)))

	step++

	if err := prepareTargetDirectory(opts.TargetPath); err != nil {
		return err
	}

	ui.Println()
	ui.PrintStep(step, totalSteps, "Downloading XenForo files")
	step++

	cachedFiles, err := downloadProducts(ctx, client, opts)
	if err != nil {
		return err
	}

	ui.Println()
	ui.PrintStep(step, totalSteps, "Extracting XenForo files")
	step++

	if err := extractProducts(cachedFiles, opts.TargetPath, titleMap); err != nil {
		return err
	}

	ui.Println()
	ui.PrintStep(step, totalSteps, "Setting up Docker configuration")
	step++

	xfcmdOpts := xfcmd.InitOptions{
		OverwriteExisting: true,
		Contexts:          opts.Contexts,
	}
	writtenDefaults, err := xfcmd.Init(opts.TargetPath, xfcmdOpts)
	if err != nil {
		return fmt.Errorf("failed to initialize Docker configuration: %w", err)
	}

	ui.PrintSuccess("Docker configuration ready")

	for _, p := range writtenDefaults {
		ui.PrintInfo("Updated defaults written to " + ui.Path.Render(p))
	}

	meta := &xf.Metadata{
		LicenseKey:         opts.LicenseKey,
		InstanceName:       opts.InstanceName,
		InstalledProducts:  opts.Products,
		InstalledVersion:   opts.VersionString,
		InstalledVersionID: opts.VersionID,
	}
	if err := xf.WriteMetadata(opts.TargetPath, meta); err != nil {
		// Non-fatal - warn but continue
		ui.PrintWarning("Could not write metadata")
	}

	ui.Println()
	ui.PrintStep(step, totalSteps, "Configuring environment")
	step++

	if err := configureEnvironment(opts); err != nil {
		return err
	}

	ui.PrintSuccess("Environment configured")

	ui.Println()

	runner, err := dockercompose.NewRunner(opts.TargetPath)
	if err != nil {
		return fmt.Errorf("failed to initialize Docker Compose runner: %w", err)
	}

	siteURL := fallbackBoardURL(opts.InstanceName)

	if opts.SkipUp {
		printSkippedStep(step, totalSteps, "Starting Docker environment", "use --up to start containers")
	} else {
		ui.PrintStep(step, totalSteps, "Starting Docker environment")

		if cfg.Verbose {
			ui.PrintSubstep("Running docker compose up...")

			if err := runner.Up(ctx, true); err != nil {
				return fmt.Errorf("failed to start Docker environment: %w", err)
			}
		} else {
			spinner := ui.NewSpinner("Starting Docker environment")
			spinner.Start()

			tracker := newPhaseTrackerWriter(spinner, "Starting Docker environment", dockerStartPhaseRules())
			if err := runner.UpWithOutput(ctx, true, tracker, tracker); err != nil {
				spinner.StopWithMessage("error", "Failed to start containers")
				printHiddenOutputTail("Docker output", tracker.TailLines())

				return fmt.Errorf("failed to start Docker environment: %w", err)
			}

			spinner.StopWithMessage("success", "Docker containers started")
		}

		detectedURL, detectedErr := runner.GetURL(ctx)

		var detected bool

		siteURL, detected = chooseBoardURL(opts.InstanceName, detectedURL, detectedErr)
		if !detected && detectedErr != nil {
			ui.PrintWarning("Could not detect the site URL; using " + ui.URL.Render(siteURL))
		}

		// Uses the same hasComposer captured before download (not a fresh
		// shouldRunComposer(opts.TargetPath) filesystem check) so this gate
		// can never disagree with the total plannedInitSteps already
		// printed above.
		if hasComposer {
			if opts.SkipComposer {
				ui.Println()
				printSkippedStep(step, totalSteps, "Installing Composer dependencies", "--skip-composer")
				step++
			} else {
				ui.Println()
				ui.PrintStep(step, totalSteps, "Installing Composer dependencies")
				step++

				if err := runComposerInstall(ctx, runner, cfg.Verbose); err != nil {
					return err
				}
			}
		}

		if opts.SkipInstall {
			ui.Println()
			printSkippedStep(step, totalSteps, "Installing XenForo", "--skip-install")
		} else {
			ui.Println()
			ui.PrintStep(step, totalSteps, "Installing XenForo")

			installArgs := make([]string, 0, 8)
			installArgs = append(installArgs, "xf:install")
			installArgs = append(installArgs, "--no-interaction")
			installArgs = append(installArgs, "--clear")
			installArgs = append(installArgs, "--user="+opts.AdminUser)
			installArgs = append(installArgs, "--email="+opts.AdminEmail)
			installArgs = append(installArgs, "--title="+opts.SiteTitle)
			installArgs = append(installArgs, "--url="+siteURL)

			installEnv := map[string]string{
				"XF_INSTALL_PASSWORD": opts.AdminPassword,
			}

			shellInstallArgs := []string{"sh", "-c", installShellCommand(installArgs)}

			if cfg.Verbose {
				ui.PrintSubstep("Waiting for the database to be ready...")

				if err := runner.WaitForDatabase(ctx, 2*time.Second); err != nil {
					return fmt.Errorf("failed waiting for database to become ready: %w", err)
				}

				ui.PrintSubstep("Running XenForo installation...")

				if err := runner.ExecOrRunWithEnv(ctx, "xf", true, installEnv, shellInstallArgs...); err != nil {
					return printInstallFailure(err)
				}
			} else {
				spinner := ui.NewSpinner("Waiting for the database")
				spinner.Start()

				if err := runner.WaitForDatabase(ctx, 2*time.Second); err != nil {
					spinner.Stop()

					return fmt.Errorf("failed waiting for database to become ready: %w", err)
				}

				spinner.UpdateMessage("Installing XenForo")

				tracker := newPhaseTrackerWriter(spinner, "Installing XenForo", installPhaseRules())
				if err := runner.ExecOrRunWithEnvAndOutput(ctx, "xf", true, installEnv, tracker, tracker, shellInstallArgs...); err != nil {
					spinner.Stop()
					printHiddenOutputTail("Installer output", tracker.TailLines())

					return printInstallFailure(err)
				}

				spinner.StopWithMessage("success", "XenForo installed")
			}
		}
	}

	successDetails := []ui.KVPair{
		ui.KV("Location", ui.Path.Render(ui.ShortHome(opts.TargetPath))),
		ui.KV("Instance", opts.InstanceName),
		ui.KV("Products", formatProductNames(opts.Products, titleMap)),
	}
	if !opts.SkipUp {
		successDetails = append(successDetails, ui.KV("URL", ui.URL.Render(siteURL)))
	}

	ui.Println()
	ui.SuccessBox("XenForo development environment initialized", successDetails)

	if opts.SkipUp {
		ui.Println()
		printStartHint(opts.TargetPath)
	}

	ui.Println()
	printUsefulCommands()

	return nil
}

type phaseRule struct {
	contains []string
	message  string
}

type phaseTrackerWriter struct {
	mu          sync.Mutex
	spinner     *ui.Spinner
	baseMessage string
	rules       []phaseRule
	pending     string
	lastMessage string
	tail        []string
	tailMax     int
}

func newPhaseTrackerWriter(spinner *ui.Spinner, baseMessage string, rules []phaseRule) *phaseTrackerWriter {
	return &phaseTrackerWriter{
		spinner:     spinner,
		baseMessage: baseMessage,
		rules:       rules,
		tailMax:     25,
	}
}

func (w *phaseTrackerWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, b := range p {
		if b == '\n' || b == '\r' {
			w.processLine(w.pending)
			w.pending = ""

			continue
		}

		w.pending += string(b)
	}

	return len(p), nil
}

func (w *phaseTrackerWriter) TailLines() []string {
	w.mu.Lock()
	defer w.mu.Unlock()

	out := make([]string, len(w.tail))
	copy(out, w.tail)

	return out
}

func (w *phaseTrackerWriter) processLine(line string) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return
	}

	w.tail = append(w.tail, trimmed)
	if len(w.tail) > w.tailMax {
		w.tail = w.tail[len(w.tail)-w.tailMax:]
	}

	if strings.HasPrefix(w.baseMessage, "Installing XenForo") {
		if importMessage := parseInstallImportMessage(trimmed); importMessage != "" && importMessage != w.lastMessage {
			w.lastMessage = importMessage
			w.spinner.UpdateMessage(fmt.Sprintf("%s (%s)", w.baseMessage, importMessage))

			return
		}
	}

	lower := strings.ToLower(trimmed)
	for _, rule := range w.rules {
		if containsAny(lower, rule.contains) {
			if rule.message != "" && rule.message != w.lastMessage {
				w.lastMessage = rule.message
				w.spinner.UpdateMessage(fmt.Sprintf("%s (%s)", w.baseMessage, rule.message))
			}

			return
		}
	}
}

func parseInstallImportMessage(line string) string {
	lower := strings.ToLower(line)
	if !strings.Contains(lower, "import") && !strings.Contains(lower, "master data") {
		return ""
	}

	const marker = "master data ("

	idx := strings.Index(lower, marker)
	if idx >= 0 {
		after := line[idx+len(marker):]

		end := strings.Index(after, ")")
		if end < 0 {
			end = len(after)
		}

		inside := strings.TrimSpace(after[:end])
		if inside == "" {
			return "importing data"
		}

		partsCount := 2
		parts := strings.SplitN(inside, ":", partsCount)

		name := strings.ToLower(strings.TrimSpace(parts[0]))
		if name == "" {
			return "importing data"
		}

		if len(parts) == partsCount {
			percent := strings.TrimSpace(parts[1])
			if percent != "" {
				return fmt.Sprintf("importing %s (%s)", name, percent)
			}
		}

		return "importing " + name
	}

	return "importing data"
}

func containsAny(s string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(s, n) {
			return true
		}
	}

	return false
}

func dockerStartPhaseRules() []phaseRule {
	return []phaseRule{
		{contains: []string{"pulling", "pull complete", "downloaded", "extracting"}, message: "pulling images"},
		{contains: []string{"building", "load build", "cached", "exporting", "writing image"}, message: "building services"},
		{contains: []string{"creating", "recreating", "starting", "started", "running"}, message: "starting containers"},
	}
}

func composerPhaseRules() []phaseRule {
	return []phaseRule{
		{contains: []string{"loading composer repositories"}, message: "loading repositories"},
		{contains: []string{"updating dependencies"}, message: "updating dependencies"},
		{contains: []string{"installing dependencies"}, message: "installing dependencies"},
		{contains: []string{"generating autoload"}, message: "finalizing"},
	}
}

func installPhaseRules() []phaseRule {
	return []phaseRule{
		{contains: []string{"installing", "initializing"}, message: "initializing"},
		{contains: []string{"importing", "master data", "phrases", "templates"}, message: "importing data"},
		{contains: []string{"rebuilding", "caches"}, message: "rebuilding caches"},
		{contains: []string{"installation complete", "install complete", "completed successfully", "setup complete"}, message: "finalizing"},
	}
}

func printHiddenOutputTail(title string, lines []string) {
	if len(lines) == 0 {
		return
	}

	ui.Println(ui.Indent2 + ui.Dim.Render("── "+title+" (last "+ui.Plural(len(lines), "line", "lines")+") ──"))

	for _, line := range lines {
		ui.Printf("%s%s\n", ui.Indent2, ui.Dim.Render(line))
	}
}

func printUsefulCommands() {
	commands := []ui.KVPair{
		ui.KV("xf up", "Start the environment"),
		ui.KV("xf down", "Stop the environment"),
		ui.KV("xf reboot", "Restart the environment"),
		ui.KV("xf ps", "Container status"),
		ui.KV("xf logs", "Show logs"),
		ui.KV("xf composer", "Run Composer"),
		ui.KV("xf php", "Run PHP"),
	}

	width := 0
	for _, c := range commands {
		if w := lipgloss.Width(c.Key); w > width {
			width = w
		}
	}

	ui.Println(ui.Bold.Render("Useful commands:"))

	for _, c := range commands {
		ui.Printf("%s%s  %s\n", ui.Indent1, ui.Command.Render(padRight(c.Key, width)), ui.Muted.Render(c.Value))
	}
}

// padRight pads s with trailing spaces to the given display width.
func padRight(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}

	return s + strings.Repeat(" ", width-w)
}

func formatProductNames(products []string, titleMap map[string]string) string {
	if len(products) == 0 {
		return ""
	}

	names := make([]string, 0, len(products))
	for _, product := range products {
		if name := strings.TrimSpace(titleMap[product]); name != "" {
			names = append(names, name)
			continue
		}

		names = append(names, product)
	}

	return strings.Join(names, ", ")
}

func prepareTargetDirectory(targetPath string) error {
	info, err := os.Stat(targetPath)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(targetPath, 0o750); err != nil {
			return fmt.Errorf("failed to create target directory: %w", err)
		}

		ui.PrintSubstep("Created directory: " + ui.Path.Render(targetPath))

		return nil
	}

	if err != nil {
		return fmt.Errorf("failed to check target directory: %w", err)
	}

	if !info.IsDir() {
		return fmt.Errorf("target path exists but is not a directory: %w", ErrInvalidInput)
	}

	entries, err := os.ReadDir(targetPath)
	if err != nil {
		return fmt.Errorf("failed to read target directory: %w", err)
	}

	nonHiddenCount := 0

	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".") {
			nonHiddenCount++
		}
	}

	if nonHiddenCount > 0 {
		hasXenForo, err := detectXenForo(targetPath)
		if err != nil {
			return err
		}

		if hasXenForo {
			ui.PrintWarning("Directory already contains a XenForo installation")
			ui.PrintDetail("Only Docker configuration files will be updated")
		} else {
			return fmt.Errorf(
				"target directory is not empty (%d visible items); use an empty directory or an existing XenForo directory: %w",
				nonHiddenCount,
				ErrInvalidInput,
			)
		}
	} else {
		ui.PrintSubstep("Directory is empty and ready")
	}

	return nil
}

// detectComposerBeforeDownload determines whether the xenforo core package
// for this run ships a composer.json, which decides whether the plan has a
// "Installing Composer dependencies" step. It fetches (or reuses a cache hit
// for) just the xenforo package - the same download downloadProducts will
// need shortly, so a fresh run pays for it once and a cached run pays
// nothing extra - and peeks inside the archive without extracting it.
//
// This has to happen before the first step total is printed: the answer
// can't be known from the filesystem yet (nothing has been extracted), and
// discovering it only after "Preparing target directory" and "Downloading
// XenForo files" have already committed a total to the screen would leave
// those lines showing a total the run later contradicts.
func detectComposerBeforeDownload(ctx context.Context, client *customerapi.Client, opts *InitOptions) (bool, error) {
	if opts.VersionID == 0 {
		// Non-interactive validation guarantees this is set before
		// executeInit runs; guard defensively rather than assume.
		return false, nil
	}

	cacheManager, err := cache.NewManager()
	if err != nil {
		return false, fmt.Errorf("failed to initialize cache manager: %w", err)
	}

	selection := downloads.Selection{
		Product:       "xenforo",
		VersionID:     opts.VersionID,
		VersionString: opts.VersionString,
	}

	var (
		spinner    *ui.Spinner
		lastUpdate int64
	)

	progress := func(current, total int64) {
		if current-lastUpdate < 102400 && lastUpdate != 0 {
			return
		}

		lastUpdate = current

		msg := fmt.Sprintf("Checking XenForo package (%s)", ui.FormatBytes(current))
		if spinner == nil {
			spinner = ui.NewSpinner(msg)
			spinner.Start()
		} else {
			spinner.UpdateMessage(msg)
		}
	}

	entry, versionStr, err := downloads.DownloadSelection(ctx, client, cacheManager, opts.LicenseKey, selection, false, progress)

	if spinner != nil {
		spinner.Stop()
	}

	if err != nil {
		return false, fmt.Errorf("failed to download xenforo: %w", err)
	}

	if opts.VersionString == "" {
		opts.VersionString = versionStr
	}

	hasComposer, err := extract.ContainsUploadFile(entry.FilePath, "composer.json")
	if err != nil {
		return false, fmt.Errorf("failed to inspect xenforo package: %w", err)
	}

	return hasComposer, nil
}

func downloadProducts(ctx context.Context, client *customerapi.Client, opts *InitOptions) (map[string]*cache.Entry, error) {
	cacheManager, err := cache.NewManager()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize cache manager: %w", err)
	}

	titleMap := getProductTitleMap(ctx, client, opts.LicenseKey)

	cachedFiles := make(map[string]*cache.Entry)

	selections, err := downloads.ResolveSelections(ctx, client, opts.LicenseKey, opts.Products, opts.VersionID, opts.VersionString, opts.ProductOverrides, func(product string) {
		name := titleMap[product]
		if name == "" {
			name = product
		}

		ui.PrintWarning("No versions available for " + name + ", skipping")
	})
	if err != nil {
		return nil, fmt.Errorf("failed to resolve product selections for license %s: %w", opts.LicenseKey, err)
	}

	for _, selection := range selections {
		productName := titleMap[selection.Product]
		if productName == "" {
			productName = selection.Product
		}

		var (
			progressBar *ui.ProgressBar
			spinner     *ui.Spinner
			lastUpdate  int64
		)

		spinner = ui.NewSpinner(fmt.Sprintf("Downloading %s %s", productName, selection.VersionString))
		spinner.Start()

		progress := func(current, total int64) {
			if total > 0 {
				if spinner != nil {
					spinner.Stop()
					spinner = nil
				}

				if progressBar == nil {
					label := fmt.Sprintf("%s %s", productName, selection.VersionString)
					progressBar = ui.NewProgressBar(total, label)
				}

				progressBar.Update(current)
			} else if current-lastUpdate >= 102400 || lastUpdate == 0 {
				lastUpdate = current

				msg := fmt.Sprintf("Downloading %s %s (%s)", productName, selection.VersionString, ui.FormatBytes(current))
				if spinner == nil {
					spinner = ui.NewSpinner(msg)
					spinner.Start()
				} else {
					spinner.UpdateMessage(msg)
				}
			}
		}

		entry, versionStr, err := downloads.DownloadSelection(ctx, client, cacheManager, opts.LicenseKey, selection, false, progress)

		if progressBar != nil {
			progressBar.Finish()
		}

		if err != nil {
			if spinner != nil {
				spinner.Stop()
			}

			return nil, fmt.Errorf("failed to download %s: %w", selection.Product, err)
		}

		successMsg := fmt.Sprintf("Downloaded %s %s (%s)", productName, selection.VersionString, ui.FormatBytes(entry.Metadata.Size))
		if spinner != nil {
			spinner.StopWithMessage("success", successMsg)
		} else {
			ui.PrintSuccess(successMsg)
		}

		if selection.Product == "xenforo" && opts.VersionString == "" {
			opts.VersionString = versionStr
		}

		cachedFiles[selection.Product] = entry
	}

	return cachedFiles, nil
}

func extractProducts(cachedFiles map[string]*cache.Entry, targetPath string, titleMap map[string]string) error {
	return extractCachedFiles(cachedFiles, targetPath, titleMap, "Extracted")
}

func extractCachedFiles(cachedFiles map[string]*cache.Entry, targetPath string, titleMap map[string]string, verb string) error {
	if entry, ok := cachedFiles["xenforo"]; ok {
		spinner := ui.NewSpinner("Extracting XenForo core")
		spinner.Start()

		fileCount := 0
		progress := func(current, total int, filename string) {
			fileCount = current
			if total > 0 {
				spinner.UpdateMessage(fmt.Sprintf("Extracting files (%d/%d)", current, total))
			}
		}

		if err := extract.XenForoZip(entry.FilePath, targetPath, progress); err != nil {
			spinner.Stop()

			return fmt.Errorf("failed to extract XenForo: %w", err)
		}

		spinner.StopWithMessage("success", fmt.Sprintf("%s XenForo core (%s)", verb, ui.Plural(fileCount, "file", "files")))
	}

	for product, entry := range cachedFiles {
		if product == "xenforo" {
			continue
		}

		productName := product
		if titleMap != nil {
			if name := titleMap[product]; name != "" {
				productName = name
			}
		}

		spinner := ui.NewSpinner("Extracting " + productName)
		spinner.Start()

		fileCount := 0
		progress := func(current, total int, filename string) {
			fileCount = current
			if total > 0 {
				spinner.UpdateMessage(fmt.Sprintf("Extracting %s (%d/%d)", productName, current, total))
			}
		}

		if err := extract.XenForoZip(entry.FilePath, targetPath, progress); err != nil {
			spinner.Stop()

			return fmt.Errorf("failed to extract %s: %w", product, err)
		}

		spinner.StopWithMessage("success", fmt.Sprintf("%s %s (%s)", verb, productName, ui.Plural(fileCount, "file", "files")))
	}

	return nil
}

func configureEnvironment(opts *InitOptions) error {
	envPath := xf.GetEnvPath(opts.TargetPath)

	if _, err := xf.ReadEnvFile(envPath); err != nil {
		return fmt.Errorf(".env file not found after xf init: %w", err)
	}

	updates := map[string]string{
		"XF_INSTANCE":    opts.InstanceName,
		"XF_EMAIL":       opts.AdminEmail,
		"XF_DEBUG":       "1",
		"XF_DEVELOPMENT": "1",
	}

	if opts.SiteTitle != "" {
		updates["XF_TITLE"] = fmt.Sprintf("%s [%s]", opts.SiteTitle, opts.InstanceName)
	}

	if len(opts.Contexts) > 0 {
		updates["XF_CONTEXTS"] = strings.Join(opts.Contexts, ":")
	}

	updates["XF_COOKIE_PREFIX"] = opts.InstanceName + "_"
	maps.Copy(updates, opts.EnvResolved)

	if err := xf.WriteEnvFile(envPath, updates); err != nil {
		return fmt.Errorf("failed to write environment configuration: %w", err)
	}

	ui.PrintSubstep("Configured instance: " + opts.InstanceName)
	ui.PrintDetail("Admin email: " + opts.AdminEmail)

	return nil
}

// shouldRunComposer reports whether a directory is a Composer project.
//
// Repository checkouts track composer.json, so a fresh worktree has one and its
// dependencies must be installed. Release packages ship vendor/ prebuilt and
// have no manifest, so they are skipped automatically.
func shouldRunComposer(targetPath string) bool {
	info, err := os.Stat(filepath.Join(targetPath, "composer.json"))

	return err == nil && !info.IsDir()
}

// runComposerInstall installs Composer dependencies inside the container.
func runComposerInstall(ctx context.Context, runner *dockercompose.Runner, verbose bool) error {
	args := []string{"install", "--no-interaction"}

	if verbose {
		ui.PrintSubstep("Running composer install...")

		if err := runner.Composer(ctx, args...); err != nil {
			return fmt.Errorf("failed to install Composer dependencies: %w", err)
		}

		return nil
	}

	spinner := ui.NewSpinner("Installing Composer dependencies")
	spinner.Start()

	tracker := newPhaseTrackerWriter(spinner, "Installing Composer dependencies", composerPhaseRules())

	composerArgs := append([]string{"composer"}, args...)
	if err := runner.ExecOrRunWithOutput(ctx, "xf", true, tracker, tracker, composerArgs...); err != nil {
		spinner.StopWithMessage("error", "Failed to install Composer dependencies")
		printHiddenOutputTail("Composer output", tracker.TailLines())

		return fmt.Errorf("failed to install Composer dependencies: %w", err)
	}

	spinner.StopWithMessage("success", "Composer dependencies installed")

	return nil
}
