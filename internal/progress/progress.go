package progress

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/SmonSisay/winprovision/internal/models"
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
	white := color.New(color.FgWhite, color.Bold)

	green.Println("  ╔══════════════════════════════════════════════════════╗")
	green.Println("  ║           WINDOWS PROVISION TOOL                    ║")
	green.Println("  ╚══════════════════════════════════════════════════════╝")
	white.Printf("  Version        :  ")
	fmt.Println(version)
	white.Printf("  Windows        :  ")
	fmt.Println(windowsVersion)
	white.Printf("  User           :  ")
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

	percent := 0
	if d.total > 0 {
		percent = (d.completed * 100) / d.total
	}

	bar := progressBar(percent, 25)

	statusColor := color.New(color.FgGreen, color.Bold)
	switch result.Status {
	case models.TaskStatusSkipped:
		statusColor = color.New(color.FgYellow, color.Bold)
	case models.TaskStatusFailed:
		statusColor = color.New(color.FgRed, color.Bold)
	}

	statusText := statusColor.Sprint(string(result.Status))
	fmt.Printf("%s\n", statusText)
	if result.Status == models.TaskStatusFailed {
		fmt.Printf("           error: %s\n", result.Message)
	}
	fmt.Printf("           %s %3d%% %s\n", bar, percent, statusText)
}

func (d *Display) ShowFinalReport() {
	fmt.Println()
	green := color.New(color.FgGreen, color.Bold)
	red := color.New(color.FgRed, color.Bold)
	cyan := color.New(color.FgCyan, color.Bold)
	cyan.Println("  ╔══════════════════════════════════════════════════════╗")
	cyan.Println("  ║                    COMPLETED 100%                    ║")
	cyan.Println("  ╚══════════════════════════════════════════════════════╝")
	fmt.Println()

	grey := color.New(color.Faint)
	grey.Println("  ─── Summary ───")

	var errCount, skipCount int
	for _, r := range d.results {
		if r.Name == "" {
			continue
		}
		nameColor := color.New(color.FgWhite, color.Bold)
		statusColor := color.New(color.FgGreen)
		icon := "✓"
		switch r.Status {
		case models.TaskStatusSkipped:
			statusColor = color.New(color.FgYellow)
			icon = "-"
			skipCount++
		case models.TaskStatusFailed:
			statusColor = color.New(color.FgRed)
			icon = "✗"
			errCount++
		}
		nameColor.Printf("  %s  %-30s", icon, r.Name)
		statusText := statusColor.Sprintf("%s", r.Status)
		// Align status text at column 42 (30 name + tab)
		if len(r.Name) > 34 {
			fmt.Printf("\t%s\n", statusText)
		} else {
			fmt.Printf("\t%s\n", statusText)
		}
	}

	grey.Println()
	grey.Println("  ─── Stats ───")
	fmt.Printf("  %-25s:  %d/%d\n", "Total tasks", d.completed, d.total)
	fmt.Printf("  %-25s:  %d\n", "Passed", (d.completed - errCount - skipCount))
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
	completed := color.New(color.FgGreen, color.Bold)
	completed.Println(`     ________  ___  _____  __    __`)
	completed.Println(`    /  _/ __ \/ _ \/ ___/ / /   / /`)
	completed.Println(`   / // / / / // / __ \_/ /   / /  `)
	completed.Println(` _/ / /_/ / __ \ /_/ / /___/ /___ `)
	completed.Println(`/___/_____/_/ |_\____/_____/_____/ `)
	completed.Println(`        C O M P L E T E D        `)
	completed.Println()
}

func (d *Display) Results() []models.TaskResult {
	return append([]models.TaskResult(nil), d.results...)
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
