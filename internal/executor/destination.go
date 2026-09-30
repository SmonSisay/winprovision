// Destination resolution: choosing which volume receives the software payload.
//
// This file, plan.go and free_space.go are the three parts of the destination
// contract; executor.go orchestrates them.
package executor

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/SmonSisay/winprovision/internal/models"
	"github.com/SmonSisay/winprovision/internal/utils"
)

// selectDestinationVolumeScript picks the volume the software payload is copied
// to. Windows Setup lays down the data partition next to C: (see
// autounattend.xml), but the drive letter it ends up with depends on whatever
// else is attached during the install, so the volume is chosen by size rather
// than by a hardcoded letter.
//
// The filters matter:
//   - DriveType 'Fixed' excludes the removable USB that Setup.exe is running
//     from — the payload must never be copied back onto its own source media.
//   - A drive letter is required because the payload is addressed by letter.
//   - A non-empty FileSystem excludes unformatted OEM/recovery volumes, which
//     would otherwise be a candidate on machines that have them.
const selectDestinationVolumeScript = `Get-Volume | ` +
	`Where-Object { $_.DriveType -eq 'Fixed' -and $_.DriveLetter -and $_.DriveLetter -ne 'C' -and $_.FileSystem -ne '' } | ` +
	`Sort-Object Size -Descending | ` +
	`Select-Object -First 1 -ExpandProperty DriveLetter`

// resolveDestination determines the root that the software payload is copied
// into. Every branch returns a root — a drive or folder, never the finished
// software folder — so ResolveSoftwareDestination appends
// settings.Destination.FolderName exactly once regardless of which branch ran.
func resolveDestination(ctx context.Context, settings *models.Settings) (string, error) {
	folderName := settings.Destination.FolderName
	if folderName == "" {
		folderName = "Softwares"
	}

	root, found, err := detectDestinationVolume(ctx)
	if err != nil {
		return "", err
	}
	if found {
		target := ensureDestinationFolder(root, folderName)
		fmt.Printf("  Using: %s (largest volume other than the system drive)\n", target)
		return root, nil
	}

	if !settings.Destination.PromptIfNoSecondaryDrive {
		return "", fmt.Errorf(
			"no fixed volume other than the system drive was found, and destination.promptIfNoSecondaryDrive is disabled",
		)
	}

	// Nothing suitable detected — ask the operator for a destination.
	userPath, err := utils.PromptDestinationFolder(folderName)
	if err != nil {
		return "", fmt.Errorf("destination folder: %w", err)
	}
	return userPath, nil
}

// detectDestinationVolume returns the root of the largest qualifying volume.
//
// A cancelled or timed-out context is returned as an error, not as "nothing
// suitable found": without that distinction Ctrl+C during the probe would be
// mistaken for an empty machine and drop the operator into a folder prompt.
func detectDestinationVolume(ctx context.Context) (root string, found bool, err error) {
	out, cmdErr := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", selectDestinationVolumeScript).Output()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", false, fmt.Errorf("volume detection cancelled: %w", ctxErr)
	}

	// A PowerShell failure is not fatal here; the prompt below covers it.
	if cmdErr != nil {
		return "", false, nil
	}

	letter := strings.TrimSpace(string(out))
	if letter == "" {
		return "", false, nil
	}
	return letter + `:\`, true, nil
}

// ensureDestinationFolder creates the software folder inside root if it is
// missing and returns its full path.
func ensureDestinationFolder(root, folderName string) string {
	target := filepath.Join(root, folderName)
	if !utils.DirExists(target) {
		_ = utils.EnsureDir(target)
	}
	return target
}
