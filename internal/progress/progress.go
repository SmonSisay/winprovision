package progress

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/SmonSisay/winprovision/internal/models"
	"github.com/fatih/color"
)

type Display struct {
	startTime time.Time
	results   []models.TaskResult
	total     int
	completed int
}

func NewDisplay(totalTasks int) *Display {
	return &Display{
		startTime: time.Now(),
		total:     totalTasks,
	}
}

func (d *Display) ShowBanner(version, windowsVersion, username string) {
	green := color.New(color.FgGreen, color.Bold)

	green.Println("  ╔══════════════════════════════════════════════════════╗")
	green.Println("  ║      Welcome to Windows Provision Tool              ║")
	green.Println("  ╚══════════════════════════════════════════════════════╝")
	green.Printf("  Version        :  ")
	fmt.Println(version)
	green.Printf("  Windows        :  ")
	fmt.Println(windowsVersion)
	green.Printf("  User           :  ")
	fmt.Println(username)
	fmt.Println()
}

func (d *Display) ShowDestination(destination string) {
	color.New(color.FgWhite, color.Bold).Print("  Destination    :  ")
	fmt.Println(destination)
	fmt.Println()
}

func (d *Display) ShowActionSummary(actions []string) {
	yellow := color.New(color.FgYellow, color.Bold)
	white := color.New(color.FgWhite)
	yellow.Println("  ─── Planned Actions ───")
	for _, action := range actions {
		white.Printf("    ✓ %s\n", action)
	}
	fmt.Println()
}

func (d *Display) Confirm() (bool, error) {
	reader := bufio.NewReader(os.Stdin)
	color.New(color.FgYellow, color.Bold).Print("  ▸ Proceed with provisioning? [y/N]: ")
	line, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("read confirmation: %w", err)
	}
	answer := strings.TrimSpace(strings.ToLower(line))
	return answer == "y" || answer == "yes", nil
}

func (d *Display) TaskStart(module, task string) {
	elapsed := time.Since(d.startTime).Round(time.Second)
	fmt.Printf("  [%s] %s > %s ... ", elapsed, module, task)
}

func (d *Display) TaskComplete(result models.TaskResult) {
	d.completed++
	d.results = append(d.results, result)

	statusColor := color.New(color.FgGreen, color.Bold)
	taskPercent := 100
	switch result.Status {
	case models.TaskStatusSkipped:
		statusColor = color.New(color.FgYellow, color.Bold)
		taskPercent = 100
	case models.TaskStatusFailed:
		statusColor = color.New(color.FgRed, color.Bold)
		taskPercent = 0
	}

	statusText := statusColor.Sprint(string(result.Status))
	fmt.Printf("%s\n", statusText)

	if result.Status == models.TaskStatusSkipped && result.Message != "" {
		fmt.Printf("           └─ %s\n", result.Message)
	}
	if result.Status == models.TaskStatusFailed {
		fmt.Printf("           └─ %s\n", result.Message)
	}

	bar := progressBar(taskPercent, 25)
	fmt.Printf("           %s %3d%% %s\n", bar, taskPercent, statusText)
}

// unattendedEnv is the environment variable autounattend.xml sets when it
// launches Setup.exe with no operator present. Its presence means nobody is
// watching the screen, so the tool must never block waiting for a keypress.
const unattendedEnv = "WINPROVISION_UNATTENDED"

// Unattended reports whether Setup.exe was launched without an operator.
// autounattend.xml sets this because its SynchronousCommand blocks Windows
// Setup until the command returns: a prompt waiting on Enter there hangs the
// whole installation with nobody to answer it.
func Unattended() bool {
	v := strings.TrimSpace(os.Getenv(unattendedEnv))
	return v != "" && v != "0" && !strings.EqualFold(v, "false")
}

func (d *Display) ShowFinalReport() {
	fmt.Println()
	green := color.New(color.FgGreen, color.Bold)
	red := color.New(color.FgRed, color.Bold)
	cyan := color.New(color.FgCyan, color.Bold)

	// d.total counts the planned tasks, but the auto-discovery phase adds
	// results that were not in the plan, so completed can legitimately exceed
	// it. Reporting "5/4" would look like a bug, so the denominator grows to
	// fit whatever actually ran.
	taskTotal := d.total
	if d.completed > taskTotal {
		taskTotal = d.completed
	}
	overallPercent := 0
	if taskTotal > 0 {
		overallPercent = (d.completed * 100) / taskTotal
	}

	cyan.Println("  ╔══════════════════════════════════════════════════════╗")
	cyan.Printf("  ║                 COMPLETED  %3d%%                   ║\n", overallPercent)
	cyan.Println("  ╚══════════════════════════════════════════════════════╝")
	fmt.Println()

	grey := color.New(color.Faint)
	grey.Println("  ─── Summary ───")

	// Counts come from every recorded result, not just the ones with a name.
	// Deriving "Passed" as completed-minus-failures instead would silently
	// inflate it whenever a task finished without a name (a recovered panic),
	// and the report would then disagree with itself.
	var passCount, skipCount, errCount int
	for _, r := range d.results {
		switch r.Status {
		case models.TaskStatusSuccess:
			passCount++
		case models.TaskStatusSkipped:
			skipCount++
		case models.TaskStatusFailed:
			errCount++
		}
	}

	for _, r := range d.results {
		// A task that failed without recording a name (a recovered panic) is
		// counted in Failed, so it has to appear here too. Hiding it would
		// leave the operator with a failure they cannot identify.
		name := r.Name
		if name == "" {
			name = "(unidentified task)"
		}
		nameColor := color.New(color.FgWhite, color.Bold)
		statusColor := color.New(color.FgGreen)
		icon := "✓"
		switch r.Status {
		case models.TaskStatusSkipped:
			statusColor = color.New(color.FgYellow)
			icon = "-"
		case models.TaskStatusFailed:
			statusColor = color.New(color.FgRed)
			icon = "✗"
		}
		nameColor.Printf("  %s  %-30s", icon, name)
		statusColor.Printf("%s\n", r.Status)
		if (r.Status == models.TaskStatusFailed || r.Status == models.TaskStatusSkipped) && r.Message != "" {
			fmt.Printf("      └─ %s\n", r.Message)
		}
	}

	grey.Println()
	grey.Println("  ─── Stats ───")
	fmt.Printf("  %-25s:  %d/%d (%d%%)\n", "Total tasks", d.completed, taskTotal, overallPercent)
	fmt.Printf("  %-25s:  %d\n", "Passed", passCount)
	fmt.Printf("  %-25s:  %d\n", "Skipped", skipCount)
	errColor := green
	if errCount > 0 {
		errColor = red
	}
	errColor.Printf("  %-25s:  %d\n", "Failed", errCount)
	fmt.Printf("  %-25s:  %s\n", "Finished in", time.Since(d.startTime).Round(time.Second))

	fmt.Println()
	if errCount == 0 {
		green.Println("  ✓ All tasks completed successfully!")
	} else {
		fmt.Println()
		red.Println("  ✗ Some tasks failed. Check the log for details.")
	}
	fmt.Println()
	completedArt := color.New(color.FgGreen, color.Bold)
	completedArt.Println(`     ________  ___  _____  __    __`)
	completedArt.Println(`    /  _/ __ \/ _ \/ ___/ / /   / /`)
	completedArt.Println(`   / // / / / // / __ \_/ /   / /  `)
	completedArt.Println(` _/ / /_/ / __ \ /_/ / /___/ /___ `)
	completedArt.Println(`/___/_____/_/ |_\____/_____/_____/ `)
	completedArt.Println(`        C O M P L E T E D        `)
	completedArt.Println()
	info := color.New(color.FgCyan, color.Bold)
	info.Println("  ─────────────────────────────────────────────")
	info.Println("  IT Officers — please verify:")
	info.Println()
	info.Println("  • All applications are installed and working")
	info.Println("  • Run the slave for your branch")
	info.Println("  • Add the computer to the domain")
	info.Println("  • Activate Kaspersky")
	info.Println()
	info.Println("  GitHub: https://github.com/SmonSisay/winprovision")
	info.Println("  Open Source — Contributions & Collaborations are Welcome!")
	info.Println()
	info.Println("  Enjoy your day!")
	info.Println("  July 2026 G.C")
	info.Println("  ─────────────────────────────────────────────")
	fmt.Println()

	// An unattended run has no operator to answer this prompt, and the caller
	// is a blocking autounattend command, so waiting here would hang Windows
	// Setup indefinitely. Everything the operator needs is in the log either way.
	if Unattended() {
		fmt.Println("  Unattended run — closing automatically.")
		return
	}

	fmt.Print("  Press Enter to close this window...")
	fmt.Scanln()
}

func (d *Display) HasFailures() bool {
	for _, result := range d.results {
		if result.Status == models.TaskStatusFailed {
			return true
		}
	}
	return false
}

func progressBar(percent, width int) string {
	filled := percent * width / 100
	bar := "["
	for i := 0; i < width; i++ {
		if i < filled {
			bar += "#"
		} else {
			bar += "-"
		}
	}
	bar += "]"
	return bar
}
