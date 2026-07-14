//go:build !windows

package windows

import (
	"context"
	"errors"

	"github.com/SmonSisay/winprovision/internal/models"
)

// DisableFirewall disables Windows Firewall for all profiles when not already disabled.
func DisableFirewall(ctx context.Context) models.TaskResult {
	return failedResult("Firewall", "windows.firewall", "firewall configuration is only supported on Windows")
}

// EnableRemoteDesktop enables Remote Desktop when not already enabled.
func EnableRemoteDesktop(ctx context.Context) models.TaskResult {
	return failedResult("Remote Desktop", "windows.rdp", "remote desktop configuration is only supported on Windows")
}

// EnableAdministrator enables the built-in Administrator account.
func EnableAdministrator(ctx context.Context) models.TaskResult {
	return failedResult("Administrator", "windows.administrator", "administrator configuration is only supported on Windows")
}

// SetAdministratorPassword sets the built-in Administrator password.
func SetAdministratorPassword(ctx context.Context, password string) models.TaskResult {
	_ = password
	return failedResult("Administrator Password", "windows.administrator", "administrator configuration is only supported on Windows")
}

// ShowFileExtensions configures Explorer to show file extensions.
func ShowFileExtensions() models.TaskResult {
	return failedResult("Show File Extensions", "windows.explorer", "explorer configuration is only supported on Windows")
}

// ShowHiddenFiles configures Explorer to show hidden files.
func ShowHiddenFiles() models.TaskResult {
	return failedResult("Show Hidden Files", "windows.explorer", "explorer configuration is only supported on Windows")
}

func failedResult(name, module, message string) models.TaskResult {
	return models.TaskResult{
		Name:     name,
		Module:   module,
		Status:   models.TaskStatusFailed,
		Message:  message,
		Duration: 0,
		// errors.New is correct here: no wrapping, no format string — just a static message.
		Err: errors.New(message),
	}
}
