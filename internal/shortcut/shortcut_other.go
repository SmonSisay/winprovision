//go:build !windows

package shortcut

import (
	"fmt"
	"time"

	"github.com/SmonSisay/winprovision/internal/models"
)

// CreateDesktopShortcut creates a desktop shortcut when configured and missing.
func CreateDesktopShortcut(app models.AppDefinition) models.TaskResult {
	start := time.Now()
	return models.TaskResult{
		Name:     app.Name + " Shortcut",
		Module:   "shortcut",
		Status:   models.TaskStatusFailed,
		Message:  "Shortcut creation is only supported on Windows",
		Duration: time.Since(start),
		Err:      fmt.Errorf("shortcut creation is only supported on Windows"),
	}
}
