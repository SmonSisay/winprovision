//go:build windows

// Package dism wraps DISM operations for enabling Windows features.
package dism

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/SmonSisay/winprovision/internal/models"
)

const (
	moduleName       = "dism"
	// dismRebootRequired is the DISM exit code meaning "success, but a system
	// reboot is required to complete the operation".
	dismRebootRequired = 3010
)

// IsDotNet35Enabled reports whether .NET Framework 3.5 is already enabled.
func IsDotNet35Enabled(ctx context.Context) (bool, error) {
	cmd := exec.CommandContext(
		ctx,
		"dism.exe",
		"/Online",
		"/Get-FeatureInfo",
		"/FeatureName:NetFx3",
	)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stdout
	if err := cmd.Run(); err != nil {
		return false, fmt.Errorf("query NetFx3 feature state: %w", err)
	}

	output := strings.ToLower(stdout.String())
	return strings.Contains(output, "state : enabled"), nil
}

// EnableDotNet35 enables .NET Framework 3.5 using the provided SXS source path.
func EnableDotNet35(ctx context.Context, sxsPath string) models.TaskResult {
	start := time.Now()
	result := models.TaskResult{
		Name:   ".NET Framework 3.5",
		Module: moduleName,
	}

	enabled, err := IsDotNet35Enabled(ctx)
	if err == nil && enabled {
		result.Status = models.TaskStatusSkipped
		result.Message = ".NET Framework 3.5 already enabled"
		result.Duration = time.Since(start)
		return result
	}

	cleanPath := filepath.Clean(sxsPath)
	cmd := exec.CommandContext(
		ctx,
		"dism.exe",
		"/Online",
		"/Enable-Feature",
		"/FeatureName:NetFx3",
		"/All",
		"/LimitAccess",
		"/Source:"+cleanPath,
		"/NoRestart",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	duration := time.Since(start)
	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}

	if runErr != nil {
		// Exit 3010 = the feature was enabled, DISM just notes a pending
		// reboot. We never restart and do not ask for one, so this is reported
		// as an ordinary success.
		if exitCode == dismRebootRequired {
			result.Status = models.TaskStatusSuccess
			result.Message = fmt.Sprintf(
				".NET Framework 3.5 enabled (exit=%d, duration=%s, source=%s)",
				exitCode,
				duration.Round(time.Millisecond),
				cleanPath,
			)
			result.Duration = duration
			return result
		}
		// DISM reports servicing errors on stdout, not stderr, so a
		// stderr-only message comes out empty and hides the real cause
		// (e.g. "0x800f0912 The source files could not be found").
		// Fall back to stdout so the operator sees why it failed.
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
		result.Status = models.TaskStatusFailed
		result.Message = fmt.Sprintf(
			"DISM failed (exit=%d, duration=%s, source=%s): %s",
			exitCode,
			duration.Round(time.Millisecond),
			cleanPath,
			detail,
		)
		result.Err = fmt.Errorf("enable NetFx3 (source=%s): %w; stdout=%s; stderr=%s", cleanPath, runErr, stdout.String(), stderr.String())
		result.Duration = duration
		return result
	}

	result.Status = models.TaskStatusSuccess
	result.Message = fmt.Sprintf(
		".NET Framework 3.5 enabled (exit=%d, duration=%s, source=%s)",
		exitCode,
		duration.Round(time.Millisecond),
		cleanPath,
	)
	result.Duration = duration
	return result
}
