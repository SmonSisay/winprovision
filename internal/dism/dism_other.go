//go:build !windows

package dism

import (
	"context"
	"fmt"

	"github.com/SmonSisay/winprovision/internal/models"
)

// IsDotNet35Enabled reports whether .NET Framework 3.5 is already enabled.
func IsDotNet35Enabled(ctx context.Context) (bool, error) {
	_ = ctx
	return false, fmt.Errorf("DISM is only supported on Windows")
}

// EnableDotNet35 enables .NET Framework 3.5 using the provided SXS source path.
func EnableDotNet35(ctx context.Context, sxsPath string) models.TaskResult {
	_ = ctx
	_ = sxsPath
	return models.TaskResult{
		Name:     ".NET Framework 3.5",
		Module:   "dism",
		Status:   models.TaskStatusFailed,
		Message:  "DISM is only supported on Windows",
		Duration: 0,
		Err:      fmt.Errorf("DISM is only supported on Windows"),
	}
}
