//go:build !windows

package utils

import "fmt"

// IsAdmin reports whether the current process has administrator privileges.
func IsAdmin() (bool, error) {
	return false, fmt.Errorf("admin check is only supported on Windows")
}

// GetWindowsVersion returns a human-readable Windows version string.
func GetWindowsVersion() (string, error) {
	return "", fmt.Errorf("windows version detection is only supported on Windows")
}

// DetectBootableDrive detects a bootable Windows drive.
func DetectBootableDrive() (string, error) {
	return "", fmt.Errorf("bootable drive detection is only supported on Windows")
}

// FreeSpaceBytes reports the free space on the volume containing path.
func FreeSpaceBytes(path string) (uint64, error) {
	_ = path
	return 0, fmt.Errorf("free space detection is only supported on Windows")
}
