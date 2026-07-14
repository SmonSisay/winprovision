// Package installer detects and installs applications defined in apps.json.
package installer

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/SmonSisay/winprovision/internal/models"
	"github.com/SmonSisay/winprovision/internal/registry"
	"github.com/SmonSisay/winprovision/internal/utils"
)

var fallbackSilentFlags = [][]string{
	{"/S"},
	{"/silent"},
	{"/quiet"},
	{"/qn"},
	{"/quiet", "/norestart"},
	{"/S", "/v", "/qn"},
	{"--silent"},
	{"-silent"},
	{},
}


const moduleName = "installer"

// defaultSilentArgs are tried in order when an auto-discovered installer has
// no explicit silent arguments. The list covers the most common conventions.
var defaultSilentArgs = []string{"/S", "/silent", "/quiet", "/qn"}

// IsInstalled reports whether the application is already installed.
func IsInstalled(app models.AppDefinition) (bool, string, error) {
	detection := app.Detection

	if detection.Registry != nil {
		if registry.KeyExists(detection.Registry.Key) {
			if detection.Registry.ValueName == "" {
				return true, "registry key exists", nil
			}
			value, err := registry.GetString(detection.Registry.Key, detection.Registry.ValueName)
			if err == nil && strings.TrimSpace(value) != "" {
				return true, "registry value found", nil
			}
		}
	}

	if detection.ExecutablePath != "" {
		path := utils.ExpandEnv(detection.ExecutablePath)
		if utils.FileExists(path) {
			return true, "executable exists", nil
		}
	}

	if detection.InstallDir != "" {
		path := utils.ExpandEnv(detection.InstallDir)
		if utils.DirExists(path) {
			return true, "install directory exists", nil
		}
	}

	if detection.ProductVersion != "" {
		if detection.Registry != nil && detection.Registry.ValueName != "" {
			value, err := registry.GetString(detection.Registry.Key, detection.Registry.ValueName)
			if err == nil && value == detection.ProductVersion {
				return true, "product version matches", nil
			}
		}
	}

	return false, "", nil
}

// appFolderName guesses the folder name for an app based on its name or path.
func appFolderName(app models.AppDefinition) string {
	path := filepath.ToSlash(strings.TrimSpace(app.InstallerPath))
	if idx := strings.Index(path, "/"); idx > 0 {
		return path[:idx]
	}
	return app.Name
}

// resolveInstallerPath finds the installer executable. It tries the exact
// path from apps.json first, then searches the app folder for any .exe.
func resolveInstallerPath(app models.AppDefinition, softwareRoot string) string {
	base := filepath.Join(softwareRoot, filepath.FromSlash(app.InstallerPath))
	if utils.FileExists(base) {
		return base
	}

	// Search app folder by trying different possible directories
	candidates := []string{
		filepath.Dir(filepath.Join(softwareRoot, filepath.FromSlash(app.InstallerPath))),
		filepath.Join(softwareRoot, appFolderName(app)),
	}

	for _, dir := range candidates {
		if !utils.DirExists(dir) {
			continue
		}
		for _, name := range []string{"setup.exe", "install.exe"} {
			p := filepath.Join(dir, name)
			if utils.FileExists(p) {
				return p
			}
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".exe") {
				return filepath.Join(dir, e.Name())
			}
		}
	}
	return ""
}

// runInstaller tries to run the installer with the given args.
func runInstaller(ctx context.Context, exePath string, args []string) error {
	cmd := exec.CommandContext(ctx, exePath, args...)
	cmd.Dir = filepath.Dir(exePath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Install runs the application installer from the copied software directory.
func Install(ctx context.Context, app models.AppDefinition, softwareRoot string) models.TaskResult {
	start := time.Now()
	result := models.TaskResult{
		Name:   app.Name,
		Module: moduleName,
	}

	installed, reason, err := IsInstalled(app)
	if err != nil {
		result.Status = models.TaskStatusFailed
		result.Message = "Failed to evaluate installation state"
		result.Err = err
		result.Duration = time.Since(start)
		return result
	}
	if installed {
		result.Status = models.TaskStatusSkipped
		result.Message = fmt.Sprintf("Already installed (%s)", reason)
		result.Duration = time.Since(start)
		return result
	}

	installerPath := resolveInstallerPath(app, softwareRoot)
	if installerPath == "" {
		folderName := appFolderName(app)
		result.Status = models.TaskStatusFailed
		result.Message = fmt.Sprintf("No installer (.exe) found in '%s' folder under software/", folderName)
		result.Err = fmt.Errorf("no installer found for %s", app.Name)
		result.Duration = time.Since(start)
		return result
	}

	// Build list of flag sets to try
	var flagSets [][]string
	explicitArgs := SplitArgs(app.SilentArgs)
	if len(explicitArgs) > 0 {
		flagSets = append(flagSets, explicitArgs)
	}
	flagSets = append(flagSets, fallbackSilentFlags...)

	var lastErr error
	for _, flags := range flagSets {
		runErr := runInstaller(ctx, installerPath, flags)
		if runErr == nil {
			result.Status = models.TaskStatusSuccess
			break
		}
		lastErr = runErr
	}

	if result.Status != models.TaskStatusSuccess {
		result.Message = fmt.Sprintf("All install attempts failed: %v", lastErr)
		result.Err = fmt.Errorf("install %s: %w", installerPath, lastErr)
		result.Duration = time.Since(start)
		return result
	}

	result.Message = "Installed successfully"
	result.Duration = time.Since(start)
	return result
}

// SplitArgs parses a shell-style argument string, respecting double-quoted
// tokens that may contain spaces and backslash-escaped quotes.
//
//	"/key:value" "/path:C:\Program Files\app" → two args, not four.
//	`"C:\Program Files\app"` → single arg with quotes stripped.
func SplitArgs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var args []string
	var current strings.Builder
	inQuote := false
	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		switch {
		case ch == '\\' && i+1 < len(raw) && raw[i+1] == '"':
			// Escaped quote — emit literal quote.
			current.WriteByte('"')
			i++
		case ch == '"':
			inQuote = !inQuote
		case ch == ' ' && !inQuote:
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
		default:
			current.WriteByte(ch)
		}
	}
	if current.Len() > 0 {
		args = append(args, current.String())
	}
	return args
}

// knownAppDirs returns a lowercase set of top-level directory names referenced
// by the provided app definitions, derived from their InstallerPath fields.
// E.g. "Chrome/setup.exe" → "chrome".
func knownAppDirs(apps []models.AppDefinition) map[string]struct{} {
	known := make(map[string]struct{}, len(apps))
	for _, app := range apps {
		normalized := filepath.ToSlash(strings.TrimSpace(app.InstallerPath))
		if idx := strings.Index(normalized, "/"); idx > 0 {
			known[strings.ToLower(normalized[:idx])] = struct{}{}
		}
	}
	return known
}

// findInstallerExe searches dir for a likely installer executable.
// Priority order: setup.exe → install.exe → first *.exe found (alphabetical).
func findInstallerExe(dir string) (string, error) {
	for _, name := range []string{"setup.exe", "install.exe"} {
		p := filepath.Join(dir, name)
		if utils.FileExists(p) {
			return p, nil
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("read directory %s: %w", dir, err)
	}
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".exe") {
			return filepath.Join(dir, e.Name()), nil
		}
	}
	return "", nil // no executable found
}

// DiscoverAndInstall scans softwareRoot for subdirectories not covered by the
// known AppDefinition list. For each uncovered directory that contains an
// executable, the executable is launched with default silent arguments.
// Directories with no executable are skipped. The onStart callback is invoked
// before each discovered task begins, allowing the caller to update the
// progress display.
func DiscoverAndInstall(
	ctx context.Context,
	softwareRoot string,
	known []models.AppDefinition,
	onStart func(name string),
) []models.TaskResult {
	knownDirs := knownAppDirs(known)

	entries, err := os.ReadDir(softwareRoot)
	if err != nil {
		return []models.TaskResult{{
			Name:    "Auto-Discovery",
			Module:  moduleName,
			Status:  models.TaskStatusFailed,
			Message: "failed to scan software directory: " + err.Error(),
			Err:     fmt.Errorf("scan software directory: %w", err),
		}}
	}

	var results []models.TaskResult
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dirName := entry.Name()
		if _, covered := knownDirs[strings.ToLower(dirName)]; covered {
			continue // already handled by a named entry in apps.json
		}

		if onStart != nil {
			onStart(dirName)
		}

		start := time.Now()
		result := models.TaskResult{
			Name:   dirName + " (auto-discovered)",
			Module: moduleName,
		}

		appDir := filepath.Join(softwareRoot, dirName)
		exePath, findErr := findInstallerExe(appDir)
		if findErr != nil {
			result.Status = models.TaskStatusFailed
			result.Message = "Failed to scan directory: " + findErr.Error()
			result.Err = findErr
			result.Duration = time.Since(start)
			results = append(results, result)
			continue
		}
		if exePath == "" {
			result.Status = models.TaskStatusSkipped
			result.Message = "No installer executable found in " + dirName
			result.Duration = time.Since(start)
			results = append(results, result)
			continue
		}

		cmd := exec.CommandContext(ctx, exePath, defaultSilentArgs...)
		cmd.Dir = filepath.Dir(exePath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		runErr := cmd.Run()
		duration := time.Since(start)
		if runErr != nil {
			exitCode := 1
			if cmd.ProcessState != nil {
				exitCode = cmd.ProcessState.ExitCode()
			}
			result.Status = models.TaskStatusFailed
			result.Message = fmt.Sprintf("Installer failed (exit=%d)", exitCode)
			result.Err = fmt.Errorf("run discovered installer %s: %w", exePath, runErr)
		} else {
			result.Status = models.TaskStatusSuccess
			result.Message = "Installed from " + filepath.Base(exePath)
		}
		result.Duration = duration
		results = append(results, result)
	}
	return results
}
