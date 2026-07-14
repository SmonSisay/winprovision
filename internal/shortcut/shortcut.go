//go:build windows

// Package shortcut creates Windows desktop shortcuts.
package shortcut

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"github.com/SmonSisay/winprovision/internal/models"
	"github.com/SmonSisay/winprovision/internal/utils"
)

const moduleName = "shortcut"

// CreateDesktopShortcut creates a desktop shortcut when configured and missing.
func CreateDesktopShortcut(app models.AppDefinition) models.TaskResult {
	start := time.Now()
	result := models.TaskResult{
		Name:   app.Name + " Shortcut",
		Module: moduleName,
	}

	if !app.DesktopShortcut.Enabled {
		result.Status = models.TaskStatusSkipped
		result.Message = "Desktop shortcut not configured"
		result.Duration = time.Since(start)
		return result
	}

	shortcutPath, err := desktopShortcutPath(app.DesktopShortcut.Name)
	if err != nil {
		result.Status = models.TaskStatusFailed
		result.Message = "Failed to resolve desktop path"
		result.Err = err
		result.Duration = time.Since(start)
		return result
	}

	if utils.FileExists(shortcutPath) {
		result.Status = models.TaskStatusSkipped
		result.Message = "Shortcut already exists"
		result.Duration = time.Since(start)
		return result
	}

	targetPath := utils.ExpandEnv(app.DesktopShortcut.TargetPath)
	if !utils.FileExists(targetPath) {
		result.Status = models.TaskStatusFailed
		result.Message = fmt.Sprintf("Shortcut target not found: %s", targetPath)
		result.Err = fmt.Errorf("shortcut target not found: %s", targetPath)
		result.Duration = time.Since(start)
		return result
	}

	if err := createShortcut(shortcutPath, targetPath, filepath.Dir(targetPath)); err != nil {
		result.Status = models.TaskStatusFailed
		result.Message = "Failed to create desktop shortcut"
		result.Err = err
		result.Duration = time.Since(start)
		return result
	}

	result.Status = models.TaskStatusSuccess
	result.Message = "Desktop shortcut created"
	result.Duration = time.Since(start)
	return result
}

func desktopShortcutPath(name string) (string, error) {
	publicDesktop := os.Getenv("PUBLIC")
	if publicDesktop == "" {
		// %PUBLIC% is unset — fall back to the current user's home directory.
		// Never hardcode C:\Users\Public: Windows may be installed on any drive.
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve user home directory: %w", err)
		}
		publicDesktop = home
	}
	desktop := filepath.Join(publicDesktop, "Desktop")
	if err := utils.EnsureDir(desktop); err != nil {
		return "", err
	}
	fileName := name
	if filepath.Ext(fileName) != ".lnk" {
		fileName += ".lnk"
	}
	return filepath.Join(desktop, fileName), nil
}

func createShortcut(shortcutPath, targetPath, workingDir string) error {
	ole.CoInitialize(0)
	defer ole.CoUninitialize()

	unknown, err := oleutil.CreateObject("WScript.Shell")
	if err != nil {
		return fmt.Errorf("create WScript.Shell: %w", err)
	}
	defer unknown.Release()

	shell, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return fmt.Errorf("query WScript.Shell interface: %w", err)
	}
	defer shell.Release()

	shortcutVariant, err := oleutil.CallMethod(shell, "CreateShortcut", shortcutPath)
	if err != nil {
		return fmt.Errorf("create shortcut object: %w", err)
	}
	shortcut := shortcutVariant.ToIDispatch()
	defer shortcut.Release()

	if _, err := oleutil.PutProperty(shortcut, "TargetPath", targetPath); err != nil {
		return fmt.Errorf("set shortcut target: %w", err)
	}
	if _, err := oleutil.PutProperty(shortcut, "WorkingDirectory", workingDir); err != nil {
		return fmt.Errorf("set shortcut working directory: %w", err)
	}
	if _, err := oleutil.CallMethod(shortcut, "Save"); err != nil {
		return fmt.Errorf("save shortcut: %w", err)
	}
	return nil
}
