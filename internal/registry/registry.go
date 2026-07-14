//go:build windows

// Package registry provides thin helpers around the Windows registry.
package registry

import (
	"fmt"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// OpenKey opens a registry key from a full path like HKLM\SOFTWARE\Example.
func OpenKey(path string, access uint32) (registry.Key, error) {
	hive, subKey, err := splitPath(path)
	if err != nil {
		return 0, err
	}
	key, err := registry.OpenKey(hive, subKey, access)
	if err != nil {
		return 0, fmt.Errorf("open registry key %s: %w", path, err)
	}
	return key, nil
}

// KeyExists reports whether a registry key exists.
func KeyExists(path string) bool {
	key, err := OpenKey(path, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	_ = key.Close()
	return true
}

// GetString reads a string value from a registry key path.
func GetString(path, valueName string) (string, error) {
	key, err := OpenKey(path, registry.QUERY_VALUE)
	if err != nil {
		return "", err
	}
	defer key.Close()

	value, _, err := key.GetStringValue(valueName)
	if err != nil {
		return "", fmt.Errorf("read string value %s from %s: %w", valueName, path, err)
	}
	return value, nil
}

// GetDWORD reads a DWORD value from a registry key path.
func GetDWORD(path, valueName string) (uint32, error) {
	key, err := OpenKey(path, registry.QUERY_VALUE)
	if err != nil {
		return 0, err
	}
	defer key.Close()

	value, _, err := key.GetIntegerValue(valueName)
	if err != nil {
		return 0, fmt.Errorf("read dword value %s from %s: %w", valueName, path, err)
	}
	return uint32(value), nil
}

// SetDWORD writes a DWORD value to a registry key path.
func SetDWORD(path, valueName string, value uint32) error {
	key, err := OpenKey(path, registry.SET_VALUE)
	if err != nil {
		key, err = createKey(path)
		if err != nil {
			return fmt.Errorf("set dword value %s on %s: %w", valueName, path, err)
		}
	}
	defer key.Close()

	if err := key.SetDWordValue(valueName, value); err != nil {
		return fmt.Errorf("set dword value %s on %s: %w", valueName, path, err)
	}
	return nil
}

// SetString writes a string value to a registry key path.
func SetString(path, valueName, value string) error {
	key, err := OpenKey(path, registry.SET_VALUE)
	if err != nil {
		key, err = createKey(path)
		if err != nil {
			return fmt.Errorf("set string value %s on %s: %w", valueName, path, err)
		}
	}
	defer key.Close()

	if err := key.SetStringValue(valueName, value); err != nil {
		return fmt.Errorf("set string value %s on %s: %w", valueName, path, err)
	}
	return nil
}

func createKey(path string) (registry.Key, error) {
	hive, subKey, err := splitPath(path)
	if err != nil {
		return 0, err
	}
	key, _, err := registry.CreateKey(hive, subKey, registry.SET_VALUE)
	if err != nil {
		return 0, fmt.Errorf("create registry key %s: %w", path, err)
	}
	return key, nil
}

func splitPath(path string) (registry.Key, string, error) {
	normalized := strings.TrimSpace(path)
	normalized = strings.ReplaceAll(normalized, "/", `\`)
	parts := strings.SplitN(normalized, `\`, 2)
	if len(parts) != 2 {
		return 0, "", fmt.Errorf("invalid registry path: %s", path)
	}

	switch strings.ToUpper(parts[0]) {
	case "HKLM", "HKEY_LOCAL_MACHINE":
		return registry.LOCAL_MACHINE, parts[1], nil
	case "HKCU", "HKEY_CURRENT_USER":
		return registry.CURRENT_USER, parts[1], nil
	default:
		return 0, "", fmt.Errorf("unsupported registry hive: %s", parts[0])
	}
}
