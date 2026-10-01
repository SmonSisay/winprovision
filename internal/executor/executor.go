// Package executor orchestrates the full provisioning workflow.
package executor

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/SmonSisay/winprovision/internal/config"
	"github.com/SmonSisay/winprovision/internal/copy"
	"github.com/SmonSisay/winprovision/internal/installer"
	"github.com/SmonSisay/winprovision/internal/logging"
	"github.com/SmonSisay/winprovision/internal/models"
	"github.com/SmonSisay/winprovision/internal/progress"
	"github.com/SmonSisay/winprovision/internal/utils"
)

// Options configures provisioning execution.
type Options struct {
	Version string

	// Confirm asks the operator to approve the plan. A nil Confirm means
	// "proceed without asking", which is what unattended runs want.
	Confirm func() (bool, error)
}

// Run executes the full provisioning workflow and returns a process exit code.
func Run(ctx context.Context, opts Options) int {
	opts = opts.withDefaults()

	env, err := newEnvironment(ctx, opts)
	if err != nil {
		return reportFatal(err)
	}
	defer env.logger.Close()

	// The destination is resolved, and then space-checked, before anything is
	// planned or shown. An undersized destination must stop the run while the
	// machine is still untouched rather than part-way through the copy.
	env.destination, err = env.resolveDestination()
	if err != nil {
		return reportFatal(err)
	}
	if err := env.checkDestinationSpace(); err != nil {
		return reportFatal(err)
	}

	plan := buildTaskPlan(env)
	switch err := env.confirmOrAbort(plan); {
	case err == nil:
		// Approved: carry on.
	case errors.Is(err, errCancelled):
		// Declining is a clean outcome, not a failure.
		fmt.Println("Provisioning cancelled by user.")
		return models.ExitSuccess
	default:
		return reportFatal(err)
	}

	// The clock starts once the work is approved, so a run that sat waiting on
	// the operator is not reported as taking that long.
	env.startedAt = time.Now()
	env.execute(plan)

	return env.finish()
}

// reportFatal prints a startup failure and returns the fatal exit code.
func reportFatal(err error) int {
	fmt.Printf("FATAL: %v\n", err)
	return models.ExitFatal
}

// environment holds everything the provisioning phases need: resolved
// configuration, the logger, and the chosen destination. Building it up front
// keeps Run readable and makes each phase's dependencies explicit.
type environment struct {
	opts      Options
	ctx       context.Context
	rootDir   string
	settings  *models.Settings
	apps      []models.AppDefinition
	logger    logging.Logger
	display   *progress.Display
	windows   string
	username  string
	startedAt time.Time

	// destination is the folder the software payload is copied to, e.g.
	// D:\Softwares. Its parent volume root is kept for the space check.
	destination     string
	destinationRoot string
}

// newEnvironment performs startup validation and loads configuration. Every
// failure here is fatal and reported before any machine state is changed.
func newEnvironment(ctx context.Context, opts Options) (*environment, error) {
	rootDir, err := utils.GetExecutableDir()
	if err != nil {
		return nil, err
	}

	// Provisioning writes to HKLM, System32 and Program Files, so elevation
	// is checked before anything else.
	elevated, err := utils.IsAdmin()
	if err != nil {
		return nil, fmt.Errorf("administrator check failed: %w", err)
	}
	if !elevated {
		return nil, errNotElevated
	}

	settings, err := config.LoadSettings(rootDir)
	if err != nil {
		return nil, err
	}
	apps, err := config.LoadApps(rootDir)
	if err != nil {
		return nil, err
	}

	logger, err := logging.NewFileLogger(rootDir, settings.Logging.File, settings.Logging.Level)
	if err != nil {
		return nil, err
	}

	// These two are cosmetic banner details. An unknown value is a cosmetic
	// problem, not a reason to refuse to provision.
	windowsVersion, err := utils.GetWindowsVersion()
	if err != nil {
		windowsVersion = "Windows"
	}
	username, err := utils.GetLoggedInUser()
	if err != nil {
		username = "Unknown"
	}

	env := &environment{
		opts:      opts,
		ctx:       ctx,
		rootDir:   rootDir,
		settings:  settings,
		apps:      apps,
		logger:    logger,
		windows:   windowsVersion,
		username:  username,
		startedAt: time.Now(),
	}
	return env, nil
}

func (o Options) withDefaults() Options {
	if o.Version == "" {
		o.Version = "dev"
	}
	if o.Confirm == nil {
		o.Confirm = func() (bool, error) { return true, nil }
	}
	return o
}

// resolveDestination picks the volume the payload is copied to and resolves
// the full software folder beneath it.
func (e *environment) resolveDestination() (string, error) {
	root, err := resolveDestination(e.ctx, e.settings)
	if err != nil {
		return "", err
	}
	e.destinationRoot = root
	e.destination = utils.ResolveSoftwareDestination(root, e.settings.Destination.FolderName)
	return e.destination, nil
}

// checkDestinationSpace refuses to start unless the destination volume can
// hold the payload.
func (e *environment) checkDestinationSpace() error {
	payloadRoot := filepath.Join(e.rootDir, "software")
	return checkDestinationSpace(e.destinationRoot, payloadRoot)
}

// confirmOrAbort shows the plan and asks the operator to approve it. It returns
// normally when provisioning should continue; a refusal is reported to the
// caller as a clean cancellation.
// errCancelled marks a run the operator declined. It is not a failure.
var errCancelled = errors.New("provisioning cancelled by user")

// errNotElevated is returned when Setup.exe is not running elevated.
var errNotElevated = errors.New("setup.exe must be run as Administrator")

// confirmOrAbort shows the plan and asks the operator to approve it. It
// returns a non-nil error when provisioning must not start, either because the
// operator said no or because the confirmation itself failed.
func (e *environment) confirmOrAbort(plan *taskPlan) error {
	e.display = progress.NewDisplay(plan.TotalTasks())
	e.display.ShowBanner(e.opts.Version, e.windows, e.username)
	e.display.ShowDestination(e.destination)
	e.display.ShowActionSummary(plan.ActionSummary())

	// Nobody is at the keyboard during an unattended install, so asking would
	// either read EOF and abort or wait forever. The plan is still printed and
	// written to the log, so it remains reviewable after the fact.
	if progress.Unattended() {
		fmt.Println("  Unattended run — proceeding without confirmation.")
		return nil
	}

	confirmed, err := e.opts.Confirm()
	if err != nil {
		return err
	}
	if !confirmed {
		return errCancelled
	}
	return nil
}

// execute runs the planned tasks, then the auto-discovery phase, then prints
// the final report.
func (e *environment) execute(plan *taskPlan) {
	e.logger.Info("startup", string(models.TaskStatusSuccess), "Application started", 0, nil)
	e.logger.Info("admin-check", string(models.TaskStatusSuccess), "Administrator check passed", 0, nil)

	plan.execute(e.ctx, e.display, e.logger)
	runDiscoveryPhase(e.ctx, e.destination, e.apps, e.display, e.logger)

	e.display.ShowFinalReport()
}

// finish reports the outcome and returns the process exit code.
func (e *environment) finish() int {
	elapsed := time.Since(e.startedAt)
	e.logger.Info(
		"complete",
		string(models.TaskStatusSuccess),
		fmt.Sprintf("Provisioning completed in %s", elapsed.Round(time.Second)),
		elapsed,
		nil,
	)

	if e.display.HasFailures() {
		return models.ExitTaskFailures
	}
	return models.ExitSuccess
}

// safeRunTask executes a task with panic recovery. A panicking task is recorded
// as FAILED and the run continues, so one broken installer cannot leave the
// machine half-provisioned with no report.
func safeRunTask(fn func() (result models.TaskResult)) (result models.TaskResult) {
	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			result = models.TaskResult{
				Status:  models.TaskStatusFailed,
				Message: fmt.Sprintf("panic recovered: %v", r),
				Err:     fmt.Errorf("panic: %v\nstack: %s", r, stack),
			}
		}
	}()
	return fn()
}

// runCopyPhase copies the software directory to the destination drive.
func runCopyPhase(rootDir, softwareDestination string, logger logging.Logger) models.TaskResult {
	start := time.Now()
	result := models.TaskResult{Name: "Copy Software", Module: "copy"}
	result.Duration = time.Since(start)

	stats, err := copy.SyncDirectory(filepath.Join(rootDir, "software"), softwareDestination, logger.WithModule("copy"))
	if err != nil {
		return failedCopy(result, err)
	}
	if stats.Failed > 0 {
		return failedCopy(result, fmt.Errorf("%d file copy operations failed", stats.Failed))
	}

	result.Status = models.TaskStatusSuccess
	result.Message = copyStatsMessage(stats)
	return result
}

func failedCopy(result models.TaskResult, err error) models.TaskResult {
	result.Status = models.TaskStatusFailed
	result.Err = err
	return result
}

func copyStatsMessage(stats models.CopyStats) string {
	return fmt.Sprintf("Copied=%d Skipped=%d Failed=%d", stats.Copied, stats.Skipped, stats.Failed)
}

// runDiscoveryPhase installs software directories that apps.json does not
// mention. The count is not known until this phase runs, so it is not part of
// the pre-flight plan.
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

// resolveSxSPath returns a validated sources\sxs path for DISM, trying in order:
//  1. the tool's own sources\sxs folder, so a prepared USB is self-sufficient
//     and needs no second flash attached
//  2. a bootable Windows drive, for when the USB has no payload
//  3. a path typed by the operator
//
// Returns an empty string if none of those yields a usable directory.
func resolveSxSPath(rootDir string, logger logging.Logger) string {
	localSxs := filepath.Join(rootDir, "sources", "sxs")
	if utils.IsSxSDirectory(localSxs) {
		logger.Info("resolve-sxs", "SUCCESS", fmt.Sprintf("Using local sources folder: %s", localSxs), 0, nil)
		return localSxs
	}

	logger.Warn("resolve-sxs", "WARNING",
		fmt.Sprintf("No .cab payload in %s, looking for a bootable Windows drive", localSxs), 0, nil)

	if path, ok := detectBootableSxS(logger); ok {
		return path
	}

	return promptSxSPath(logger)
}

// detectBootableSxS looks for Windows installation media with a valid
// sources\sxs payload.
func detectBootableSxS(logger logging.Logger) (string, bool) {
	bootDrive, err := utils.DetectBootableDrive()
	if err != nil {
		logger.Warn("resolve-sxs", "WARNING", fmt.Sprintf("Auto-detection failed: %v", err), 0, nil)
		return "", false
	}

	sxsPath := bootDrive + `\sources\sxs`
	if utils.IsSxSDirectory(sxsPath) {
		logger.Info("resolve-sxs", "SUCCESS",
			fmt.Sprintf("Auto-detected bootable drive: %s (path: %s)", bootDrive, sxsPath), 0, nil)
		return sxsPath, true
	}

	logger.Warn("resolve-sxs", "WARNING",
		fmt.Sprintf("Drive %s detected but %s does not exist", bootDrive, sxsPath), 0, nil)
	return "", false
}

// promptSxSPath asks the operator where the .NET payload is, accepting a bare
// drive letter, a drive root, or a full sources\sxs path.
func promptSxSPath(logger logging.Logger) string {
	input, err := utils.PromptBootableDrive()
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return ""
	}

	input = normalizeSxSInput(input)
	if utils.IsSxSDirectory(input) {
		logger.Info("resolve-sxs", "SUCCESS", fmt.Sprintf("User-provided path: %s", input), 0, nil)
		return input
	}

	logger.Warn("resolve-sxs", "FAILED", fmt.Sprintf("Path does not exist: %s", input), 0, nil)
	fmt.Printf("ERROR: Path does not exist or is not accessible: %s\n", input)
	return ""
}

// normalizeSxSInput expands whatever the operator typed into a full
// sources\sxs path: "D:", "D:\" and "D:\sources\sxs" are all accepted.
func normalizeSxSInput(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return input
	}
	// SxS media is only ever read on Windows, so separators are normalised here
	// instead of relying on filepath, which would follow the host platform.
	input = strings.ReplaceAll(input, "/", `\`)

	lower := strings.ToLower(input)
	switch {
	case strings.HasSuffix(lower, `\sxs`):
		// Already points at an sxs directory; leave the path alone.
	case len(input) == 2 && input[1] == ':':
		input += `\sources\sxs`
	case strings.HasSuffix(input, `\`):
		input += `sources\sxs`
	case !strings.Contains(lower, `\sources`):
		input += `\sources\sxs`
	}
	return filepath.Clean(input)
}
