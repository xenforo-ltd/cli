package setup

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xenforo-ltd/cli/internal/clierror"
	"github.com/xenforo-ltd/cli/internal/config"
	"github.com/xenforo-ltd/cli/internal/environment"
	"github.com/xenforo-ltd/cli/internal/ui"
)

type Options struct {
	TargetPath      string
	InstanceName    string
	SiteTitle       string
	AdminUser       string
	AdminPassword   string
	AdminEmail      string
	Contexts        []string
	EnvResolved     map[string]string
	StartContainers bool
	SkipComposer    bool
	SkipBuildJS     bool
	SkipInstall     bool
}

func Existing(ctx context.Context, opts *Options) error {
	ui.Println(ui.Header.Render("Initializing Docker environment"))
	ui.Println()
	if err := CheckPrerequisites(ctx); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}
	step := 1
	totalSteps := 4
	ui.PrintStep(step, totalSteps, "Setting up Docker configuration")
	step++
	environmentOpts := environment.InitOptions{OverwriteExisting: true, Contexts: opts.Contexts}
	writtenDefaults, err := environment.Init(opts.TargetPath, environmentOpts)
	if err != nil {
		return fmt.Errorf("failed to initialize Docker files in existing XenForo directory: %w", err)
	}
	ui.PrintSuccess("Docker configuration files extracted")
	for _, p := range writtenDefaults {
		ui.PrintInfo("Updated defaults written to " + ui.Path.Render(p))
	}
	ui.Println()
	ui.PrintStep(step, totalSteps, "Configuring environment")
	step++
	if err := configureExistingEnv(opts); err != nil {
		return err
	}
	ui.PrintSubstep("Configured instance: " + ui.Bold.Render(opts.InstanceName))
	ui.Println()
	if err := RunJavaScript(ctx, opts, step, totalSteps); err != nil {
		return err
	}
	step++
	ui.Println()
	siteURL := FallbackBoardURL(opts.InstanceName)
	urlDetected := false
	if opts.StartContainers {
		ui.PrintStep(step, totalSteps, "Starting environment")
		siteURL, urlDetected, err = startAndInstall(ctx, opts, cfg.Verbose)
		if err != nil {
			return err
		}
	} else {
		PrintSkippedStep(step, totalSteps, "Starting environment", "use --up to start containers")
	}
	details := []ui.KVPair{ui.KV("Location", ui.Path.Render(ui.ShortHome(opts.TargetPath))), ui.KV("Instance", opts.InstanceName)}
	if urlDetected {
		details = append(details, ui.KV("URL", ui.URL.Render(siteURL)))
	}
	ui.Println()
	ui.SuccessBox("Docker environment initialized", details)
	if !opts.StartContainers {
		ui.Println()
		PrintStartHint(opts.TargetPath)
	}
	ui.Println()
	PrintUsefulCommands()
	return nil
}

func startAndInstall(ctx context.Context, opts *Options, verbose bool) (string, bool, error) {
	runner, err := environment.NewRunner(opts.TargetPath)
	if err != nil {
		return "", false, fmt.Errorf("failed to initialize Docker Compose runner: %w", err)
	}
	if err := runner.Compose(ctx, os.Stdin, os.Stdout, os.Stderr, "up", "--detach"); err != nil {
		return "", false, fmt.Errorf("failed to start Docker environment: %w", err)
	}
	detectedURL, detectedErr := runner.GetURL(ctx)
	siteURL, urlDetected := ChooseBoardURL(opts.InstanceName, detectedURL, detectedErr)
	if ShouldRunComposer(opts.TargetPath) && !opts.SkipComposer {
		ui.Println()
		if err := RunComposerInstall(ctx, runner, opts.TargetPath, verbose); err != nil {
			return "", false, err
		}
	}
	if !opts.SkipInstall && opts.AdminUser != "" {
		if opts.SiteTitle == "" {
			opts.SiteTitle = opts.EnvResolved["XF_TITLE"]
		}
		if opts.SiteTitle == "" {
			opts.SiteTitle = InstanceTitle("", opts.InstanceName)
		}
		if err := validateAdminDetails(opts); err != nil {
			return "", false, err
		}
		ui.Println()
		if err := installExistingXenForo(ctx, runner, opts, siteURL, verbose); err != nil {
			return "", false, err
		}
	}
	return siteURL, urlDetected, nil
}

func configureExistingEnv(opts *Options) error {
	envPath := filepath.Join(opts.TargetPath, ".env")
	if opts.InstanceName == "" {
		dirName := filepath.Base(opts.TargetPath)
		opts.InstanceName = environment.GenerateInstanceName(dirName)
	}
	contexts := opts.Contexts
	if len(contexts) == 0 {
		contexts = []string{"caddy", "mysql", "development", "caddy-development", "redis", "mailpit"}
	}

	adminEmail := opts.AdminEmail
	if adminEmail == "" {
		adminEmail = "admin@example.com"
	}

	updates := map[string]string{
		"XF_INSTANCE":    opts.InstanceName,
		"XF_CONTEXTS":    strings.Join(contexts, ":"),
		"XF_TITLE":       InstanceTitle("", opts.InstanceName),
		"XF_EMAIL":       adminEmail,
		"XF_DEBUG":       "1",
		"XF_DEVELOPMENT": "1",
	}
	maps.Copy(updates, opts.EnvResolved)
	if err := environment.WriteEnvFile(envPath, updates); err != nil {
		return fmt.Errorf("failed to write environment configuration: %w", err)
	}
	return nil
}

func validateAdminDetails(opts *Options) error {
	var missing []string
	if opts.AdminUser == "" {
		missing = append(missing, "--admin-user")
	}
	if opts.AdminPassword == "" {
		missing = append(missing, "--admin-password")
	}
	if opts.AdminEmail == "" {
		missing = append(missing, "--admin-email")
	}
	if len(missing) > 0 {
		return clierror.Usage(fmt.Errorf("missing required flags for the XenForo installation: %s: %w", strings.Join(missing, ", "), clierror.ErrInvalidInput))
	}
	return nil
}

func installExistingXenForo(ctx context.Context, runner *environment.Runner, opts *Options, siteURL string, verbose bool) error {
	if err := runner.WaitForDatabase(ctx, 2*time.Second); err != nil {
		return fmt.Errorf("failed waiting for database to become ready: %w", err)
	}
	shellInstallArgs, installEnv := InstallInvocation(opts.AdminUser, opts.AdminEmail, opts.SiteTitle, siteURL, opts.AdminPassword)
	if verbose {
		ui.PrintSubstep("Running XenForo installation...")
		if err := runner.ExecOrRun(ctx, "xf", installEnv, os.Stdin, os.Stdout, os.Stderr, shellInstallArgs...); err != nil {
			return fmt.Errorf("failed to install XenForo: %w", err)
		}
		return nil
	}
	spinner := ui.NewSpinner("Installing XenForo...")
	spinner.Start()
	tracker := ui.NewPhaseTrackerWriter(spinner, "Installing XenForo", ui.InstallPhaseRules())
	if err := runner.ExecOrRun(ctx, "xf", installEnv, os.Stdin, tracker, tracker, shellInstallArgs...); err != nil {
		spinner.Stop()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		ui.PrintHiddenOutputTail("Installer output", tracker.TailLines())
		return fmt.Errorf("failed to install XenForo: %w", err)
	}
	spinner.StopWithMessage("success", "XenForo installed")
	return nil
}
