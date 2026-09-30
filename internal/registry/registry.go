//go:build windows

// Package registry provides thin helpers around the Windows registry.
package registry

import (
	"fmt"

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

// splitPath resolves a configured path to a hive key and subkey.
func splitPath(path string) (registry.Key, string, error) {
	hive, subkey, err := splitRegistryPath(path)
	if err != nil {
		return 0, "", err
	}
	switch hive {
	case HiveLocalMachine:
		return registry.LOCAL_MACHINE, subkey, nil
	case HiveCurrentUser:
		return registry.CURRENT_USER, subkey, nil
	default:
		return 0, "", fmt.Errorf("unsupported registry hive: %s", hive)
	}
}
