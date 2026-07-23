// Package executor orchestrates the full provisioning workflow.
package executor

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/SmonSisay/winprovision/internal/config"
	"github.com/SmonSisay/winprovision/internal/copy"
	"github.com/SmonSisay/winprovision/internal/dism"
	"github.com/SmonSisay/winprovision/internal/installer"
	"github.com/SmonSisay/winprovision/internal/logging"
	"github.com/SmonSisay/winprovision/internal/models"
	"github.com/SmonSisay/winprovision/internal/progress"
	"github.com/SmonSisay/winprovision/internal/shortcut"
	"github.com/SmonSisay/winprovision/internal/utils"
	winconfig "github.com/SmonSisay/winprovision/internal/windows"
)

// Default timeout for individual tasks (installers, DISM, etc.).
const defaultTaskTimeout = 10 * time.Minute

// Options configures provisioning execution.
type Options struct {
	Version string
	Confirm func() (bool, error)
}

// Run executes the full provisioning workflow.
func Run(ctx context.Context, opts Options) int {
	if opts.Version == "" {
		opts.Version = "dev"
	}
	if opts.Confirm == nil {
		opts.Confirm = func() (bool, error) { return true, nil }
	}

	rootDir, err := utils.GetExecutableDir()
	if err != nil {
		fmt.Printf("FATAL: %v\n", err)
		return models.ExitFatal
	}

	isAdmin, err := utils.IsAdmin()
	if err != nil {
		fmt.Printf("FATAL: administrator check failed: %v\n", err)
		return models.ExitFatal
	}
	if !isAdmin {
		fmt.Println("FATAL: Setup.exe must be run as Administrator.")
		return models.ExitFatal
	}

	settings, err := config.LoadSettings(rootDir)
	if err != nil {
		fmt.Printf("FATAL: %v\n", err)
		return models.ExitFatal
	}

	apps, err := config.LoadApps(rootDir)
	if err != nil {
		fmt.Printf("FATAL: %v\n", err)
		return models.ExitFatal
	}

	logger, err := logging.NewFileLogger(rootDir, settings.Logging.File, settings.Logging.Level)
	if err != nil {
		fmt.Printf("FATAL: %v\n", err)
		return models.ExitFatal
	}
	defer logger.Close()

	windowsVersion, err := utils.GetWindowsVersion()
	if err != nil {
		windowsVersion = "Windows 11"
	}
	username, err := utils.GetLoggedInUser()
	if err != nil {
		username = "Unknown"
	}

	// Create D: partition first so resolveDestination can auto-detect it.
	fmt.Println()
	fmt.Println("  ─── Preparing Disk ───")
	fmt.Println()
	partResult := winconfig.EnsureSecondaryPartition(ctx)
	if partResult.Status == models.TaskStatusFailed {
		fmt.Printf("  WARNING: %s\n", partResult.Message)
	} else if partResult.Status == models.TaskStatusSkipped {
		fmt.Printf("  %s\n", partResult.Message)
	} else {
		fmt.Printf("  D: partition created successfully\n")
	}
	fmt.Println()

	destinationRoot, err := resolveDestination(settings)
	if err != nil {
		fmt.Printf("FATAL: %v\n", err)
		return models.ExitFatal
	}
	softwareDestination := utils.ResolveSoftwareDestination(destinationRoot, settings.Destination.FolderName)

	// Build the task plan once — derived from the same data used for the
	// progress display and action summary, eliminating the DRY violation.
	plan := buildTaskPlan(settings, apps)

	display := progress.NewDisplay(plan.TotalTasks())
	display.ShowBanner(opts.Version, windowsVersion, username)
	display.ShowDestination(softwareDestination)
	display.ShowActionSummary(plan.ActionSummary())

	confirmed, err := opts.Confirm()
	if err != nil {
		fmt.Printf("FATAL: %v\n", err)
		return models.ExitFatal
	}
	if !confirmed {
		fmt.Println("Provisioning cancelled by user.")
		return models.ExitSuccess
	}

	start := time.Now()
	logger.Info("startup", string(models.TaskStatusSuccess), "Application started", 0, nil)
	logger.Info("admin-check", string(models.TaskStatusSuccess), "Administrator check passed", 0, nil)

	runTask := func(module, task string, fn func() models.TaskResult) models.TaskResult {
		display.TaskStart(module, task)
		result := safeRunTask(fn)
		display.TaskComplete(result)
		logger.WithModule(module).Info(
			task,
			string(result.Status),
			result.Message,
			result.Duration,
			result.Err,
		)
		return result
	}

	copyResult := runTask("copy", "Copying Software", func() models.TaskResult {
		return runCopyPhase(rootDir, softwareDestination, logger)
	})

	if copyResult.Status == models.TaskStatusFailed {
		fmt.Println()
		fmt.Println("WARNING: Software copy encountered failures.")
		fmt.Println("         Installer tasks may fail because files are missing from the destination.")
		fmt.Println()
	}

	runWindowsTasks(ctx, settings, runTask)
	runDotNetTask(ctx, settings, rootDir, logger, runTask)
	runInstallerTasks(ctx, apps, softwareDestination, runTask)
	runDiscoveryPhase(ctx, softwareDestination, apps, display, logger)

	display.ShowFinalReport()
	logger.Info(
		"complete",
		string(models.TaskStatusSuccess),
		fmt.Sprintf("Provisioning completed in %s", time.Since(start).Round(time.Second)),
		time.Since(start),
		nil,
	)

	if display.HasFailures() {
		return models.ExitTaskFailures
	}
	return models.ExitSuccess
}

// safeRunTask executes a task function with panic recovery. If the task panics,
// it is recorded as a FAILED result and execution continues to the next task.
func safeRunTask(fn func() (result models.TaskResult)) (result models.TaskResult) {
	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			result = models.TaskResult{
				Status:  models.TaskStatusFailed,
				Message: fmt.Sprintf("panic recovered: %v", r),
				Err:     fmt.Errorf("panic: %v\nstack: %s", r, string(stack)),
			}
		}
	}()
	return fn()
}

// runCopyPhase copies the software directory to the destination drive.
func runCopyPhase(rootDir, softwareDestination string, logger logging.Logger) models.TaskResult {
	start := time.Now()
	src := filepath.Join(rootDir, "software")
	stats, err := copy.SyncDirectory(src, softwareDestination, logger.WithModule("copy"))
	if err != nil {
		return models.TaskResult{
			Name:     "Copy Software",
			Module:   "copy",
			Status:   models.TaskStatusFailed,
			Message:  err.Error(),
			Duration: time.Since(start),
			Err:      err,
		}
	}
	if stats.Failed > 0 {
		return models.TaskResult{
			Name:     "Copy Software",
			Module:   "copy",
			Status:   models.TaskStatusFailed,
			Message:  fmt.Sprintf("Copied=%d Skipped=%d Failed=%d", stats.Copied, stats.Skipped, stats.Failed),
			Duration: time.Since(start),
			Err:      fmt.Errorf("%d file copy operations failed", stats.Failed),
		}
	}
	return models.TaskResult{
		Name:     "Copy Software",
		Module:   "copy",
		Status:   models.TaskStatusSuccess,
		Message:  fmt.Sprintf("Copied=%d Skipped=%d Failed=%d", stats.Copied, stats.Skipped, stats.Failed),
		Duration: time.Since(start),
	}
}

// runWindowsTasks runs all enabled Windows configuration tasks.
func runWindowsTasks(ctx context.Context, settings *models.Settings, runTask func(string, string, func() models.TaskResult) models.TaskResult) {
	if settings.Windows.DisableFirewall {
		runTask("windows", "Disable Firewall", func() models.TaskResult {
			return winconfig.DisableFirewall(ctx)
		})
	}
	if settings.Windows.EnableRemoteDesktop {
		runTask("windows", "Enable Remote Desktop", func() models.TaskResult {
			return winconfig.EnableRemoteDesktop(ctx)
		})
	}
	if settings.Windows.EnableAdministrator {
		runTask("windows", "Set Administrator Password", func() models.TaskResult {
			return winconfig.SetAdministratorPassword(ctx, settings.Windows.AdministratorPassword)
		})
		runTask("windows", "Enable Administrator", func() models.TaskResult {
			return winconfig.EnableAdministrator(ctx)
		})
	}
	if settings.Windows.DisableWindowsUpdate {
		runTask("windows", "Disable Windows Update", func() models.TaskResult {
			return winconfig.DisableWindowsUpdate(ctx)
		})
	}
	if settings.Windows.ShowFileExtensions {
		runTask("windows", "Show File Extensions", func() models.TaskResult {
			return winconfig.ShowFileExtensions()
		})
	}
	if settings.Windows.ShowHiddenFiles {
		runTask("windows", "Show Hidden Files", func() models.TaskResult {
			return winconfig.ShowHiddenFiles()
		})
	}
}

// runDotNetTask enables .NET Framework 3.5 if configured.
// It first attempts to find a bootable Windows disk with sources\sxs.
// If not found, it prompts the user to provide the path manually.
func runDotNetTask(ctx context.Context, settings *models.Settings, rootDir string, logger logging.Logger, runTask func(string, string, func() models.TaskResult) models.TaskResult) {
	if settings.Windows.InstallDotNet35 {
		dismLog := logger.WithModule("dism")
		runTask("dism", "Enable .NET Framework 3.5", func() models.TaskResult {
			sxsPath := resolveSxSPath(dismLog)
			if sxsPath == "" {
				start := time.Now()
				dismLog.Warn("resolve-sxs", "FAILED", "Bootable flash not detected and no path provided", 0, nil)
				return models.TaskResult{
					Name:     ".NET Framework 3.5",
					Module:   "dism",
					Status:   models.TaskStatusFailed,
					Message:  "Bootable flash not detected and no path provided. .NET Framework 3.5 cannot be installed.",
					Duration: time.Since(start),
					Err:      fmt.Errorf("no valid sources\\sxs path provided"),
				}
			}
			dismLog.Info("resolve-sxs", "SUCCESS", fmt.Sprintf("Using source path: %s", sxsPath), 0, nil)
			return dism.EnableDotNet35(ctx, sxsPath)
		})
	}
}

// resolveSxSPath tries to find the sources\sxs directory automatically,
// then falls back to asking the user. Returns the validated path or empty string.
func resolveSxSPath(logger logging.Logger) string {
	bootDrive, err := utils.DetectBootableDrive()
	if err == nil {
		sxsPath := bootDrive + `\sources\sxs`
		if utils.DirExists(sxsPath) {
			logger.Info("resolve-sxs", "SUCCESS", fmt.Sprintf("Auto-detected bootable drive: %s (path: %s)", bootDrive, sxsPath), 0, nil)
			return sxsPath
		}
		logger.Warn("resolve-sxs", "WARNING", fmt.Sprintf("Drive %s detected but %s does not exist", bootDrive, sxsPath), 0, nil)
	} else {
		logger.Warn("resolve-sxs", "WARNING", fmt.Sprintf("Auto-detection failed: %v", err), 0, nil)
	}

	input, err := utils.PromptBootableDrive()
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return ""
	}

	// User entered a drive letter like "D:" — build full path
	if len(input) == 2 && input[1] == ':' {
		input = input + `\sources\sxs`
	}
	// User entered a full path like "D:\sources\sxs" or "D:\"
	if strings.HasSuffix(strings.ToLower(input), `\sources\sxs`) {
		// already correct
	} else if strings.HasSuffix(input, `\`) || strings.HasSuffix(input, `/`) {
		input = input + `sources\sxs`
	} else if !strings.Contains(strings.ToLower(input), `\sources`) {
		input = input + `\sources\sxs`
	}

	input = filepath.Clean(input)
	if utils.DirExists(input) {
		logger.Info("resolve-sxs", "SUCCESS", fmt.Sprintf("User-provided path: %s", input), 0, nil)
		return input
	}

	logger.Warn("resolve-sxs", "FAILED", fmt.Sprintf("Path does not exist: %s", input), 0, nil)
	fmt.Printf("ERROR: Path does not exist or is not accessible: %s\n", input)
	return ""
}

// runInstallerTasks installs all configured applications and creates shortcuts.
func runInstallerTasks(ctx context.Context, apps []models.AppDefinition, softwareDestination string, runTask func(string, string, func() models.TaskResult) models.TaskResult) {
	for _, app := range apps {
		app := app
		runTask("installer", app.Name, func() models.TaskResult {
			return installer.Install(ctx, app, softwareDestination)
		})
		if app.DesktopShortcut.Enabled {
			runTask("shortcut", app.Name+" Shortcut", func() models.TaskResult {
				return shortcut.CreateDesktopShortcut(app)
			})
		}
	}
}

// runDiscoveryPhase discovers and installs unlisted software directories.
func runDiscoveryPhase(
	ctx context.Context,
	softwareDestination string,
	apps []models.AppDefinition,
	display *progress.Display,
	logger logging.Logger,
) {
	discovered := installer.DiscoverAndInstall(
		ctx,
		softwareDestination,
		apps,
		func(name string) {
			display.TaskStart("installer", name+" (auto-discovered)")
		},
	)
	for _, result := range discovered {
		display.TaskComplete(result)
		logger.WithModule("installer").Info(
			result.Name,
			string(result.Status),
			result.Message,
			result.Duration,
			result.Err,
		)
	}
}

func resolveDestination(settings *models.Settings) (string, error) {
	folderName := settings.Destination.FolderName
	if folderName == "" {
		folderName = "Softwares"
	}

	// Auto-detect: if a secondary drive (D:) exists with the default folder, use it.
	for _, letter := range []string{"D", "E", "F"} {
		drive := letter + `:\`
		if utils.DirExists(drive) {
			autoPath := letter + `:\` + folderName
			if !utils.DirExists(autoPath) {
				// Drive exists but folder doesn't — create it automatically.
				_ = utils.EnsureDir(autoPath)
			}
			fmt.Printf("  Using: %s\n", autoPath)
			return autoPath, nil
		}
	}

	// No secondary drive found — ask the user.
	userPath, promptErr := utils.PromptDestinationFolder(folderName)
	if promptErr != nil {
		return "", fmt.Errorf("destination folder: %w", promptErr)
	}
	return userPath, nil
}

// taskPlan captures the planned tasks for both progress counting and summary display.
type taskPlan struct {
	actions []taskPlanEntry
}

type taskPlanEntry struct {
	summary string
	count   int
}

func (p *taskPlan) TotalTasks() int {
	total := 0
	for _, a := range p.actions {
		total += a.count
	}
	return total
}

func (p *taskPlan) ActionSummary() []string {
	summary := make([]string, 0, len(p.actions))
	for _, a := range p.actions {
		summary = append(summary, a.summary)
	}
	return summary
}

// buildTaskPlan constructs a single task plan from settings and apps. This
// eliminates the DRY violation between counting tasks and building summaries.
func buildTaskPlan(settings *models.Settings, apps []models.AppDefinition) *taskPlan {
	plan := &taskPlan{}

	plan.actions = append(plan.actions, taskPlanEntry{
		summary: "Copy software payloads to destination drive",
		count:   1,
	})

	if settings.Windows.DisableFirewall {
		plan.actions = append(plan.actions, taskPlanEntry{
			summary: "Disable Windows Firewall",
			count:   1,
		})
	}
	if settings.Windows.EnableRemoteDesktop {
		plan.actions = append(plan.actions, taskPlanEntry{
			summary: "Enable Remote Desktop",
			count:   1,
		})
	}
	if settings.Windows.EnableAdministrator {
		plan.actions = append(plan.actions, taskPlanEntry{
			summary: "Set built-in Administrator password",
			count:   1,
		})
		plan.actions = append(plan.actions, taskPlanEntry{
			summary: "Enable built-in Administrator account",
			count:   1,
		})
	}
	if settings.Windows.ShowFileExtensions {
		plan.actions = append(plan.actions, taskPlanEntry{
			summary: "Show file extensions",
			count:   1,
		})
	}
	if settings.Windows.ShowHiddenFiles {
		plan.actions = append(plan.actions, taskPlanEntry{
			summary: "Show hidden files",
			count:   1,
		})
	}
	if settings.Windows.InstallDotNet35 {
		plan.actions = append(plan.actions, taskPlanEntry{
			summary: "Install .NET Framework 3.5",
			count:   1,
		})
	}
	if settings.Windows.DisableWindowsUpdate {
		plan.actions = append(plan.actions, taskPlanEntry{
			summary: "Disable Windows Update",
			count:   1,
		})
	}

	for _, app := range apps {
		plan.actions = append(plan.actions, taskPlanEntry{
			summary: fmt.Sprintf("Install %s", app.Name),
			count:   1,
		})
		if app.DesktopShortcut.Enabled {
			plan.actions = append(plan.actions, taskPlanEntry{
				summary: fmt.Sprintf("Create desktop shortcut for %s", app.Name),
				count:   1,
			})
		}
	}

	return plan
}
