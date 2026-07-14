// Package progress renders console progress and final provisioning reports.
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

// Display renders provisioning progress to the console.
type Display struct {
	startTime time.Time
	results   []models.TaskResult
	total     int
	completed int
}

// NewDisplay creates a new progress display.
func NewDisplay(totalTasks int) *Display {
	return &Display{
		startTime: time.Now(),
		total:     totalTasks,
	}
}

// ShowBanner prints the application startup banner.
func (d *Display) ShowBanner(version, windowsVersion, username string) {
	cyan := color.New(color.FgCyan, color.Bold)
	cyan.Println("============================================================")
	cyan.Println("           Windows Provisioning Tool")
	cyan.Println("============================================================")
	fmt.Printf("Version:         %s\n", version)
	fmt.Printf("Windows:         %s\n", windowsVersion)
	fmt.Printf("Logged-in User:  %s\n", username)
	fmt.Println()
}

// ShowDestination prints the detected destination summary.
func (d *Display) ShowDestination(destination string) {
	fmt.Printf("Destination:     %s\n\n", destination)
}

// ShowActionSummary prints the list of planned actions.
func (d *Display) ShowActionSummary(actions []string) {
	fmt.Println("Planned Actions:")
	for _, action := range actions {
		fmt.Printf("  - %s\n", action)
	}
	fmt.Println()
}

// Confirm prompts the user to confirm provisioning.
func (d *Display) Confirm() (bool, error) {
	reader := bufio.NewReader(os.Stdin)
	yellow := color.New(color.FgYellow)
	yellow.Print("Proceed with provisioning? [y/N]: ")
	line, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("read confirmation: %w", err)
	}
	answer := strings.TrimSpace(strings.ToLower(line))
	return answer == "y" || answer == "yes", nil
}

// TaskStart prints the start of a task.
func (d *Display) TaskStart(module, task string) {
	elapsed := time.Since(d.startTime).Round(time.Second)
	fmt.Printf("[%s] %s > %s\n", elapsed, module, task)
}

// TaskComplete prints a completed task result and stores it for the final report.
func (d *Display) TaskComplete(result models.TaskResult) {
	d.completed++
	d.results = append(d.results, result)

	percent := 0
	if d.total > 0 {
		percent = (d.completed * 100) / d.total
	}

	elapsed := time.Since(d.startTime).Round(time.Second)
	statusColor := color.New(color.FgGreen)
	switch result.Status {
	case models.TaskStatusSkipped:
		statusColor = color.New(color.FgYellow)
	case models.TaskStatusFailed:
		statusColor = color.New(color.FgRed)
	}

	statusText := statusColor.Sprint(string(result.Status))
	fmt.Printf(
		"  elapsed=%s progress=%d%% status=%s message=%s\n",
		elapsed,
		percent,
		statusText,
		result.Message,
	)
}

// ShowFinalReport prints the provisioning summary.
func (d *Display) ShowFinalReport() {
	fmt.Println()
	color.New(color.FgCyan, color.Bold).Println("Provisioning Summary")
	fmt.Println(strings.Repeat("-", 60))

	var errors, skipped int
	for _, result := range d.results {
		statusColor := color.New(color.FgGreen)
		switch result.Status {
		case models.TaskStatusSkipped:
			statusColor = color.New(color.FgYellow)
			skipped++
		case models.TaskStatusFailed:
			statusColor = color.New(color.FgRed)
			errors++
		}
		fmt.Printf("%-28s %s\n", result.Name, statusColor.Sprint(string(result.Status)))
		if result.Status == models.TaskStatusFailed && result.Err != nil {
			fmt.Printf("  error: %s\n", result.Err.Error())
		}
	}

	fmt.Println(strings.Repeat("-", 60))
	fmt.Printf("Total Time:  %s\n", time.Since(d.startTime).Round(time.Second))
	fmt.Printf("Errors:      %d\n", errors)
	fmt.Printf("Skipped:     %d\n", skipped)
}

// Results returns all recorded task results.
func (d *Display) Results() []models.TaskResult {
	return append([]models.TaskResult(nil), d.results...)
}

// HasFailures reports whether any task failed.
func (d *Display) HasFailures() bool {
	for _, result := range d.results {
		if result.Status == models.TaskStatusFailed {
			return true
		}
	}
	return false
}
