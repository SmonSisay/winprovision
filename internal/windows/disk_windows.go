//go:build windows

package windows

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/SmonSisay/winprovision/internal/models"
)

// EnsureSecondaryPartition checks whether a non-system fixed drive (D:, E:, etc.)
// exists. If only C: exists, it shrinks C: by 50% and creates D: from the freed space.
func EnsureSecondaryPartition(ctx context.Context) models.TaskResult {
	start := time.Now()
	result := models.TaskResult{
		Name:   "Create D: Partition",
		Module: "windows.disk",
	}

	if hasSecondaryPartition() {
		result.Status = models.TaskStatusSkipped
		result.Message = "Secondary partition already exists"
		result.Duration = time.Since(start)
		return result
	}

	ps := `
$part = Get-Partition -DriveLetter C -ErrorAction Stop
$totalSize = $part.Size
$shrinkSize = [math]::Floor($totalSize * 0.5)
Resize-Partition -DriveLetter C -Size $shrinkSize -ErrorAction Stop
$newPart = New-Partition -DiskNumber $part.DiskNumber -UseMaximumSize -AssignDriveLetter -ErrorAction Stop
Format-Volume -Partition $newPart -FileSystem NTFS -NewFileSystemLabel "Data" -Confirm:$false -ErrorAction Stop
Write-Output "OK:$($newPart.DriveLetter)"
`
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", ps)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		result.Status = models.TaskStatusFailed
		result.Message = fmt.Sprintf("Failed to create partition: %s", strings.TrimSpace(stderr.String()))
		result.Err = fmt.Errorf("create secondary partition: %w", err)
		result.Duration = time.Since(start)
		return result
	}

	time.Sleep(2 * time.Second)

	if !hasSecondaryPartition() {
		result.Status = models.TaskStatusFailed
		result.Message = "Partition created but secondary drive not detected"
		result.Err = fmt.Errorf("secondary drive not found after partitioning")
		result.Duration = time.Since(start)
		return result
	}

	result.Status = models.TaskStatusSuccess
	result.Message = "Created D: partition (50% of disk)"
	result.Duration = time.Since(start)
	return result
}

func hasSecondaryPartition() bool {
	ps := `Get-Partition | Where-Object { $_.DriveLetter -ne 'C' -and $_.DriveLetter -ne '' -and $_.Type -eq 'Basic' -and (Get-Volume -Partition $_).DriveType -eq 'Fixed' } | Select-Object -First 1 -ExpandProperty DriveLetter`
	cmd := exec.Command("powershell", "-NoProfile", "-Command", ps)
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}
