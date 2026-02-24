package cmd

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/xenforo-ltd/cli/internal/api"
	"github.com/xenforo-ltd/cli/internal/cache"
	"github.com/xenforo-ltd/cli/internal/config"
	"github.com/xenforo-ltd/cli/internal/dockercompose"
	"github.com/xenforo-ltd/cli/internal/downloads"
	"github.com/xenforo-ltd/cli/internal/errors"
	"github.com/xenforo-ltd/cli/internal/extract"
	"github.com/xenforo-ltd/cli/internal/ui"
	"github.com/xenforo-ltd/cli/internal/xf"
	"github.com/xenforo-ltd/cli/internal/xfcmd"
)

func executeInit(ctx context.Context, opts *InitOptions) error {
	if opts.InstanceName == "" {
		opts.InstanceName = xf.GenerateInstanceName(filepath.Base(opts.TargetPath))
	}

	if opts.SiteTitle == "" {
		opts.SiteTitle = "XenForo"
	}

	client, err := api.NewClient()
	if err != nil {
		return err
	}
	titleMap := getProductTitleMap(ctx, client, opts.LicenseKey)

	totalSteps := 7

	fmt.Println()
	ui.PrintStep(1, totalSteps, "Preparing target directory")
	ui.PrintDetail(opts.TargetPath)
	if err := prepareTargetDirectory(opts.TargetPath); err != nil {
		return err
	}

	fmt.Println()
	ui.PrintStep(2, totalSteps, "Downloading XenForo files")
	cachedFiles, err := downloadProducts(ctx, client, opts)
	if err != nil {
		return err
	}

	fmt.Println()
	ui.PrintStep(3, totalSteps, "Extracting XenForo files")
	if err := extractProducts(cachedFiles, opts.TargetPath, titleMap); err != nil {
		return err
	}

	fmt.Println()
	ui.PrintStep(4, totalSteps, "Setting up Docker configuration")
	xfcmdOpts := xfcmd.InitOptions{
		OverwriteExisting: true,
		Contexts:          opts.Contexts,
	}
	if err := xfcmd.Init(opts.TargetPath, xfcmdOpts); err != nil {
		return err
	}
	ui.PrintSuccess("Docker configuration ready")

	meta := &xf.Metadata{
		LicenseKey:         opts.LicenseKey,
		InstanceName:       opts.InstanceName,
		InstalledProducts:  opts.Products,
		InstalledVersion:   opts.VersionString,
		InstalledVersionID: opts.VersionID,
	}
	if err := xf.WriteMetadata(opts.TargetPath, meta); err != nil {
		// Non-fatal - warn but continue
		ui.PrintWarning(fmt.Sprintf("Could not write metadata: %v", err))
	}

	fmt.Println()
	ui.PrintStep(5, totalSteps, "Configuring environment")
	if err := configureEnvironment(opts); err != nil {
		return err
	}
	ui.PrintSuccess("Environment configured")

	fmt.Println()
	ui.PrintStep(6, totalSteps, "Starting Docker environment")
	runner, err := dockercompose.NewRunner(opts.TargetPath)
	if err != nil {
		return err
	}
	siteURL := fallbackBoardURL(opts.InstanceName)

	if !opts.SkipUp {
		if config.IsVerbose() {
			ui.PrintSubstep("Running docker compose up...")
			if err := runner.Up(true); err != nil {
				return err
			}
		} else {
			spinner := ui.NewSpinner("Starting Docker environment...")
			spinner.Start()
			tracker := newPhaseTrackerWriter(spinner, "Starting Docker environment", dockerStartPhaseRules())
			if err := runner.UpWithOutput(true, tracker, tracker); err != nil {
				spinner.StopWithMessage("error", "Failed to start containers")
				printHiddenOutputTail("Docker output", tracker.TailLines())
				return err
			}
			spinner.StopWithMessage("success", "Docker containers started")
		}

		detectedURL, detectedErr := runner.GetURL()
		var detected bool
		siteURL, detected = chooseBoardURL(opts.InstanceName, detectedURL, detectedErr)
		if !detected && config.IsVerbose() && detectedErr != nil {
			ui.PrintWarning(fmt.Sprintf("Could not auto-detect site URL, using fallback %s: %v", siteURL, detectedErr))
		}

		fmt.Println()
		ui.PrintStep(7, totalSteps, "Installing XenForo")
		if !opts.SkipInstall {
			ui.PrintSubstep("Waiting for database to be ready...")
			if err := runner.WaitForDatabase(ctx, 2*time.Second); err != nil {
				return err
			}

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
			installArgs = append(installArgs, "--password=$(printenv XF_INSTALL_PASSWORD)")
			shellCmd := shellJoinArgs(append([]string{"php", "cmd.php"}, installArgs...))
			shellInstallArgs := []string{"sh", "-c", shellCmd}

			if config.IsVerbose() {
				ui.PrintSubstep("Running XenForo installation...")
				if err := runner.ExecOrRunWithEnv("xf", true, installEnv, shellInstallArgs...); err != nil {
					ui.PrintWarning(fmt.Sprintf("xf:install failed: %v", err))
					fmt.Println("    You can run it manually:")
					fmt.Printf("    %s\n", ui.Command.Render(fmt.Sprintf("cd %s && xf xf:install", opts.TargetPath)))
				}
			} else {
				spinner := ui.NewSpinner("Installing XenForo...")
				spinner.Start()
				tracker := newPhaseTrackerWriter(spinner, "Installing XenForo", installPhaseRules())
				if err := runner.ExecOrRunWithEnvAndOutput("xf", true, installEnv, tracker, tracker, shellInstallArgs...); err != nil {
					spinner.Stop()
					printHiddenOutputTail("Installer output", tracker.TailLines())
					ui.PrintWarning(fmt.Sprintf("xf:install failed: %v", err))
					fmt.Println("    You can run it manually:")
					fmt.Printf("    %s\n", ui.Command.Render(fmt.Sprintf("cd %s && xf xf:install", opts.TargetPath)))
				} else {
					spinner.StopWithMessage("success", "XenForo installed")
				}
			}
		} else {
			ui.PrintSubstep("Skipped (--skip-install flag set)")
		}
	} else {
		ui.PrintSubstep("Skipped (--skip-up flag set)")
	}

	fmt.Println()
	ui.SuccessBox("XenForo development environment initialized!", []ui.KVPair{
		ui.KV("Location", ui.Path.Render(opts.TargetPath)),
		ui.KV("Instance", opts.InstanceName),
		ui.KV("Products", formatProductNames(opts.Products, titleMap)),
	})

	if !opts.SkipUp {
		fmt.Println()
		fmt.Printf("%s Access your site at: %s\n", ui.StatusIcon("success"), ui.URL.Render(siteURL))
	} else {
		fmt.Println()
		fmt.Println("To start the environment:")
		fmt.Printf("%s%s\n", ui.Indent1, ui.Command.Render(fmt.Sprintf("cd %s", opts.TargetPath)))
		fmt.Printf("%s%s\n", ui.Indent1, ui.Command.Render("xf up"))
	}

	fmt.Println()
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

		parts := strings.SplitN(inside, ":", 2)
		name := strings.ToLower(strings.TrimSpace(parts[0]))
		if name == "" {
			return "importing data"
		}
		if len(parts) == 2 {
			percent := strings.TrimSpace(parts[1])
			if percent != "" {
				return fmt.Sprintf("importing %s (%s)", name, percent)
			}
		}
		return fmt.Sprintf("importing %s", name)
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

	ui.PrintSubstep(title + " (last lines):")
	for _, line := range lines {
		fmt.Printf("%s%s\n", ui.Indent2, ui.Dim.Render(line))
	}
}

func printUsefulCommands() {
	fmt.Println(ui.Bold.Render("Useful commands:"))
	ui.PrintKeyValuePadded([]ui.KVPair{
		ui.KV("xf up", "Start the environment"),
		ui.KV("xf down", "Stop the environment"),
		ui.KV("xf reboot", "Restart the environment"),
		ui.KV("xf logs", "View container logs"),
		ui.KV("xf ps", "List running services"),
		ui.KV("xf composer", "Run Composer"),
		ui.KV("xf php", "Run PHP"),
	})
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
		if err := os.MkdirAll(targetPath, 0755); err != nil {
			return errors.Wrap(errors.CodeDirCreateFailed, "failed to create target directory", err)
		}
		ui.PrintSubstep(fmt.Sprintf("Created directory: %s", ui.Path.Render(targetPath)))
		return nil
	}

	if err != nil {
		return errors.Wrap(errors.CodeFileReadFailed, "failed to check target directory", err)
	}

	if !info.IsDir() {
		return errors.New(errors.CodeInvalidInput, "target path exists but is not a directory")
	}

	entries, err := os.ReadDir(targetPath)
	if err != nil {
		return errors.Wrap(errors.CodeFileReadFailed, "failed to read target directory", err)
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
			return errors.Newf(
				errors.CodeInvalidInput,
				"target directory is not empty (%d visible items); use an empty directory or an existing XenForo directory",
				nonHiddenCount,
			)
		}
	} else {
		ui.PrintSubstep("Directory is empty and ready")
	}

	return nil
}

func downloadProducts(ctx context.Context, client *api.Client, opts *InitOptions) (map[string]*cache.Entry, error) {
	cacheManager, err := cache.NewManager()
	if err != nil {
		return nil, err
	}
	titleMap := getProductTitleMap(ctx, client, opts.LicenseKey)

	cachedFiles := make(map[string]*cache.Entry)

	selections, err := downloads.ResolveSelections(ctx, client, opts.LicenseKey, opts.Products, opts.VersionID, opts.VersionString, opts.ProductOverrides, func(product string) {
		ui.PrintWarning(fmt.Sprintf("No versions available for %s, skipping", product))
	})
	if err != nil {
		return nil, err
	}

	for _, selection := range selections {
		productName := titleMap[selection.Product]
		if productName == "" {
			productName = selection.Product
		}
		ui.PrintSubstep(fmt.Sprintf("Downloading %s...", productName))

		var progressBar *ui.ProgressBar
		var spinner *ui.Spinner
		var lastUpdate int64
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
				msg := fmt.Sprintf("Downloading %s %s... %s", productName, selection.VersionString, ui.FormatBytes(current))
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
		if spinner != nil {
			spinner.StopWithMessage("success", fmt.Sprintf("Downloaded %s %s", selection.Product, selection.VersionString))
		}
		if err != nil {
			return nil, err
		}

		if selection.Product == "xenforo" && opts.VersionString == "" {
			opts.VersionString = versionStr
		}

		ui.PrintDetail(fmt.Sprintf("Downloaded: %s (%s)", entry.Metadata.Filename, ui.FormatBytes(entry.Metadata.Size)))
		cachedFiles[selection.Product] = entry
	}

	return cachedFiles, nil
}

func extractProducts(cachedFiles map[string]*cache.Entry, targetPath string, titleMap map[string]string) error {
	return extractCachedFiles(cachedFiles, targetPath, titleMap, "Extracted")
}

func extractCachedFiles(cachedFiles map[string]*cache.Entry, targetPath string, titleMap map[string]string, verb string) error {
	if entry, ok := cachedFiles["xenforo"]; ok {
		ui.PrintSubstep("Extracting XenForo core...")

		fileCount := 0
		progress := func(current, total int, filename string) {
			fileCount = current
		}

		if err := extract.ExtractXenForoZip(entry.FilePath, targetPath, progress); err != nil {
			return errors.Wrap(errors.CodeFileWriteFailed, "failed to extract XenForo", err)
		}
		ui.PrintDetail(fmt.Sprintf("%s %d files", verb, fileCount))
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
		ui.PrintSubstep(fmt.Sprintf("Extracting %s...", productName))

		fileCount := 0
		progress := func(current, total int, filename string) {
			fileCount = current
		}

		if err := extract.ExtractXenForoZip(entry.FilePath, targetPath, progress); err != nil {
			return errors.Wrapf(errors.CodeFileWriteFailed, err, "failed to extract %s", product)
		}
		ui.PrintDetail(fmt.Sprintf("%s %d files", verb, fileCount))
	}

	return nil
}

func configureEnvironment(opts *InitOptions) error {
	envPath := xf.GetEnvPath(opts.TargetPath)

	if _, err := xf.ReadEnvFile(envPath); err != nil {
		return errors.Wrap(errors.CodeFileNotFound, ".env file not found after xf init", err)
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
		return err
	}

	ui.PrintSubstep(fmt.Sprintf("Configured instance: %s", opts.InstanceName))
	ui.PrintDetail(fmt.Sprintf("Admin email: %s", opts.AdminEmail))

	return nil
}
