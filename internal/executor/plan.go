// The provisioning plan: the single ordered list of work.
//
// Planning and execution deliberately share one list. Previously the task
// count shown to the operator and the tasks actually run were built by two
// separate functions, so adding a step meant editing both and a mismatch
// produced a progress bar that lied.
package executor

import (
	"context"
	"fmt"
	"time"

	"github.com/SmonSisay/winprovision/internal/dism"
	"github.com/SmonSisay/winprovision/internal/installer"
	"github.com/SmonSisay/winprovision/internal/logging"
	"github.com/SmonSisay/winprovision/internal/models"
	"github.com/SmonSisay/winprovision/internal/progress"
	"github.com/SmonSisay/winprovision/internal/shortcut"
	winconfig "github.com/SmonSisay/winprovision/internal/windows"
)

// plannedTask is one unit of provisioning work: how it is reported, how it is
// summarised to the operator, and what it does.
type plannedTask struct {
	module  string
	label   string
	summary string
	run     func(ctx context.Context) models.TaskResult

	// after, when set, is called with the result as soon as this task
	// finishes. It exists so a task can highlight its own outcome immediately
	// rather than only at the end of the run.
	after func(models.TaskResult)
}

// taskPlan is an ordered list of tasks, in execution order.
type taskPlan struct {
	tasks []plannedTask
}

// buildTaskPlan turns the loaded configuration into the ordered task list.
//
// The order below is the provisioning contract and is preserved exactly:
//  1. Windows configuration
//  2. .NET Framework 3.5
//  3. each application: install, optional deploy fallback, optional shortcut
//
// Auto-discovery is absent from the plan, because the work it finds is only
// known once the destination has been scanned.
func buildTaskPlan(env *environment) *taskPlan {
	plan := &taskPlan{}
	plan.addCopyTask(env)
	plan.addWindowsTasks(env.settings)
	plan.addDotNetTask(env)
	plan.addAppTasks(env.apps, env.destination)
	return plan
}

// addCopyTask adds the software copy as the first task. It is planned rather
// than run ad hoc so the progress total covers every task that is reported.
// Its failure is still highlighted separately, since a copy failure is the
// likely cause of any installer failures that follow (see environment.execute).
func (p *taskPlan) addCopyTask(env *environment) {
	p.add(plannedTask{
		module:  "copy",
		label:   "Copying Software",
		summary: "Copy software payloads to destination drive",
		run: func(context.Context) models.TaskResult {
			return runCopyPhase(env.rootDir, env.destination, env.logger)
		},
		after: warnIfCopyFailed,
	})
}

func (p *taskPlan) add(task plannedTask) {
	p.tasks = append(p.tasks, task)
}

func (p *taskPlan) TotalTasks() int {
	return len(p.tasks)
}

// ActionSummary returns one line per task for the pre-flight confirmation
// screen.
func (p *taskPlan) ActionSummary() []string {
	summary := make([]string, 0, len(p.tasks))
	for _, t := range p.tasks {
		summary = append(summary, t.summary)
	}
	return summary
}

// execute runs every task in order, reporting each to the display and the log.
func (p *taskPlan) execute(ctx context.Context, display *progress.Display, logger logging.Logger) {
	for _, task := range p.tasks {
		display.TaskStart(task.module, task.label)
		result := safeRunTask(func() models.TaskResult { return task.run(ctx) })
		display.TaskComplete(result)
		logger.WithModule(task.module).Info(
			task.label,
			string(result.Status),
			result.Message,
			result.Duration,
			result.Err,
		)
		if task.after != nil {
			task.after(result)
		}
	}
}

// warnIfCopyFailed explains that a failed copy is the likely cause of the
// installer failures that follow, so the operator looks in the right place
// instead of at a cascade of unrelated-looking errors.
func warnIfCopyFailed(result models.TaskResult) {
	if result.Status != models.TaskStatusFailed {
		return
	}
	fmt.Println()
	fmt.Println("WARNING: Software copy encountered failures.")
	fmt.Println("         Installer tasks may fail because files are missing from the destination.")
	fmt.Println()
}

// addWindowsTasks plans the Windows configuration steps. Each is guarded by its
// own settings flag, so a disabled step never appears in the plan or runs.
func (p *taskPlan) addWindowsTasks(settings *models.Settings) {
	w := settings.Windows

	if w.DisableFirewall {
		p.add(plannedTask{
			module:  "windows",
			label:   "Disable Firewall",
			summary: "Disable Windows Firewall",
			run:     func(ctx context.Context) models.TaskResult { return winconfig.DisableFirewall(ctx) },
		})
	}

	if w.EnableRemoteDesktop {
		p.add(plannedTask{
			module:  "windows",
			label:   "Enable Remote Desktop",
			summary: "Enable Remote Desktop",
			run:     func(ctx context.Context) models.TaskResult { return winconfig.EnableRemoteDesktop(ctx) },
		})
	}

	// Setting the password and enabling the account are two tasks on purpose:
	// the password cannot be set on a disabled account, so order matters.
	if w.EnableAdministrator {
		p.add(plannedTask{
			module:  "windows",
			label:   "Set Administrator Password",
			summary: "Set built-in Administrator password",
			run: func(ctx context.Context) models.TaskResult {
				return winconfig.SetAdministratorPassword(ctx, w.AdministratorPassword)
			},
		})
		p.add(plannedTask{
			module:  "windows",
			label:   "Enable Administrator",
			summary: "Enable built-in Administrator account",
			run:     func(ctx context.Context) models.TaskResult { return winconfig.EnableAdministrator(ctx) },
		})
	}

	if w.DisableWindowsUpdate {
		p.add(plannedTask{
			module:  "windows",
			label:   "Disable Windows Update",
			summary: "Disable Windows Update",
			run:     func(ctx context.Context) models.TaskResult { return winconfig.DisableWindowsUpdate(ctx) },
		})
	}

	if w.ShowFileExtensions {
		p.add(plannedTask{
			module:  "windows",
			label:   "Show File Extensions",
			summary: "Show file extensions",
			run:     func(context.Context) models.TaskResult { return winconfig.ShowFileExtensions() },
		})
	}

	if w.ShowHiddenFiles {
		p.add(plannedTask{
			module:  "windows",
			label:   "Show Hidden Files",
			summary: "Show hidden files",
			run:     func(context.Context) models.TaskResult { return winconfig.ShowHiddenFiles() },
		})
	}
}

// addDotNetTask plans the .NET Framework 3.5 enablement.
//
// The sources\sxs path is resolved inside run rather than while planning,
// because resolving it can prompt the operator, and prompting must happen
// only after the confirmation screen.
func (p *taskPlan) addDotNetTask(env *environment) {
	if !env.settings.Windows.InstallDotNet35 {
		return
	}

	dismLog := env.logger.WithModule("dism")
	p.add(plannedTask{
		module:  "dism",
		label:   "Enable .NET Framework 3.5",
		summary: "Install .NET Framework 3.5",
		run: func(ctx context.Context) models.TaskResult {
			start := time.Now()
			sxsPath := resolveSxSPath(env.rootDir, dismLog)
			if sxsPath == "" {
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
		},
	})
}

// addAppTasks plans each configured application: the install itself, the
// optional deploy fallback, and the optional desktop shortcut.
func (p *taskPlan) addAppTasks(apps []models.AppDefinition, softwareDestination string) {
	for _, app := range apps {
		app := app // capture per iteration for the closures below

		// Copy-only apps are never executed: the copy phase already placed the
		// folder on the destination, so the only work is to report that.
		if app.CopyOnly {
			p.add(plannedTask{
				module:  "installer",
				label:   app.Name,
				summary: fmt.Sprintf("Copy %s (not installed)", app.Name),
				run: func(context.Context) models.TaskResult {
					return models.TaskResult{
						Name:     app.Name,
						Module:   "installer",
						Status:   models.TaskStatusSkipped,
						Message:  "Copy-only — folder copied to destination, installer not executed",
						Duration: 0,
					}
				},
			})
			continue
		}

		p.add(plannedTask{
			module:  "installer",
			label:   app.Name,
			summary: fmt.Sprintf("Install %s", app.Name),
			run: func(ctx context.Context) models.TaskResult {
				return installer.Install(ctx, app, softwareDestination)
			},
		})

		// The deploy fallback is skipped when the installer already worked;
		// runInstallerTasks checks IsInstalled at run time for exactly that.
		if app.Deploy != nil {
			p.add(plannedTask{
				module:  "installer",
				label:   app.Name + " (deploy)",
				summary: fmt.Sprintf("Deploy %s (fallback)", app.Name),
				run: func(ctx context.Context) models.TaskResult {
					if installed, reason, _ := installer.IsInstalled(app); installed {
						return models.TaskResult{
							Name:     app.Name + " (deploy)",
							Module:   "installer",
							Status:   models.TaskStatusSkipped,
							Message:  fmt.Sprintf("Installer confirmed install (%s)", reason),
							Duration: 0,
						}
					}
					return installer.Deploy(ctx, app, softwareDestination)
				},
			})
		}

		if app.DesktopShortcut.Enabled {
			p.add(plannedTask{
				module:  "shortcut",
				label:   app.Name + " Shortcut",
				summary: fmt.Sprintf("Create desktop shortcut for %s", app.Name),
				run:     func(context.Context) models.TaskResult { return shortcut.CreateDesktopShortcut(app) },
			})
		}
	}
}
