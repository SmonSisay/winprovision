//go:build !windows

package registry

import "fmt"

// KeyExists reports whether a registry key exists.
func KeyExists(path string) bool {
	_ = path
	return false
}

// GetString reads a string value from a registry key path.
func GetString(path, valueName string) (string, error) {
	return "", fmt.Errorf("registry access is only supported on Windows")
}

// GetDWORD reads a DWORD value from a registry key path.
func GetDWORD(path, valueName string) (uint32, error) {
	return 0, fmt.Errorf("registry access is only supported on Windows")
}

// SetDWORD writes a DWORD value to a registry key path.
func SetDWORD(path, valueName string, value uint32) error {
	return fmt.Errorf("registry access is only supported on Windows")
}

// SetString writes a string value to a registry key path.
func SetString(path, valueName, value string) error {
	return fmt.Errorf("registry access is only supported on Windows")
}
