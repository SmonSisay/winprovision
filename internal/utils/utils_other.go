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

// DetectDestinationDrive returns the first available non-system drive letter.
func DetectDestinationDrive() (string, error) {
	return "", fmt.Errorf("drive detection is only supported on Windows")
}

// GetOSBuildNumber returns the Windows build number.
func GetOSBuildNumber() (uint32, error) {
	return 0, fmt.Errorf("build number detection is only supported on Windows")
}
