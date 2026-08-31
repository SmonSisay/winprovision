// Package installer detects and installs applications defined in apps.json.
package installer

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/SmonSisay/winprovision/internal/copy"
	"github.com/SmonSisay/winprovision/internal/logging"
	"github.com/SmonSisay/winprovision/internal/models"
	"github.com/SmonSisay/winprovision/internal/registry"
	"github.com/SmonSisay/winprovision/internal/utils"
)

var fallbackSilentFlags = [][]string{
	{"/S"},
	{"/SILENT"},
	{"/VERYSILENT"},
	{"/silent"},
	{"/quiet"},
	{"/qn"},
	{"/passive"},
	{"/quiet", "/norestart"},
	{"/qn", "/norestart"},
	{"/passive", "/norestart"},
	{"/s", "/v", "/qn"},
	{"/s", "/v", "/passive"},
	{"/s", "/v", "/quiet", "/norestart"},
	{"/SILENT", "/SUPPRESSMSGBOXES"},
	{"/VERYSILENT", "/SUPPRESSMSGBOXES"},
	{"--silent"},
	{"--quiet"},
	{"--SILENT"},
	{"--VERYSILENT"},
	{"-s"},
	{"-silent"},
	{"-quiet"},
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
			if !e.IsDir() {
				ext := strings.ToLower(filepath.Ext(e.Name()))
				if ext == ".exe" || ext == ".msi" {
					return filepath.Join(dir, e.Name())
				}
			}
		}
	}
	return ""
}

// runInstaller tries to run the installer with the given args.
func runInstaller(ctx context.Context, exePath string, args []string) error {
	ext := strings.ToLower(filepath.Ext(exePath))
	var cmd *exec.Cmd
	if ext == ".msi" {
		msiArgs := append([]string{"/i", exePath}, args...)
		cmd = exec.CommandContext(ctx, "msiexec.exe", msiArgs...)
	} else {
		cmd = exec.CommandContext(ctx, exePath, args...)
	}
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
	if installed && !app.AlwaysInstall {
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

	// Build list of flag sets to try. Explicit args are always tried first.
	// If all explicit args fail, fallback flags are tried as a last resort
	// to handle installers whose correct silent flags weren't known at
	// config time (e.g. old InstallShield, InnoSetup, NSIS variants).
	var flagSets [][]string
	explicitArgs := SplitArgs(app.SilentArgs)
	if len(explicitArgs) > 0 {
		flagSets = append(flagSets, explicitArgs)
	} else {
		flagSets = append(flagSets, fallbackSilentFlags...)
	}

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
		// Try the attended wizard fallback last: some installers (e.g.
		// Power Geez's ADVINSTSFX bootstrapper) cannot run silently at all,
		// so the wizard is shown for the operator to complete manually.
		if app.AttendedFallback {
			wizardErr := runInstallerAttended(ctx, installerPath)
			if wizardErr == nil {
				result.Status = models.TaskStatusSuccess
				result.Message = "Installed successfully (attended wizard)"
				result.Duration = time.Since(start)
				return result
			}
			lastErr = wizardErr
		}

		result.Status = models.TaskStatusFailed
		result.Message = fmt.Sprintf("All install attempts failed: %v", lastErr)
		result.Err = fmt.Errorf("install %s: %w", installerPath, lastErr)
		result.Duration = time.Since(start)
		return result
	}

	result.Message = "Installed successfully"
	result.Duration = time.Since(start)
	return result
}

// runInstallerAttended launches the installer without any silent flags so the
// operating system / installer wizard UI appears for the operator to complete
// manually. It waits for the process to exit.
func runInstallerAttended(ctx context.Context, exePath string) error {
	ext := strings.ToLower(filepath.Ext(exePath))
	var cmd *exec.Cmd
	if ext == ".msi" {
		cmd = exec.CommandContext(ctx, "msiexec.exe", "/i", exePath)
	} else {
		cmd = exec.CommandContext(ctx, exePath)
	}
	cmd.Dir = filepath.Dir(exePath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Deploy installs an application without running an installer by copying a
// pre-extracted payload into InstallDir and performing post-copy setup
// (font registration, COM self-registration, system file placement). It is
// the deterministic fallback for installers that cannot run silently.
// Issues with fonts or COM registration are reported as warnings in the
// message but do not fail the task: the payload itself is what matters.
func Deploy(ctx context.Context, app models.AppDefinition, softwareRoot string) models.TaskResult {
	start := time.Now()
	result := models.TaskResult{
		Name:   app.Name + " (deploy)",
		Module: moduleName,
	}

	cfg := app.Deploy
	if cfg == nil {
		result.Status = models.TaskStatusSkipped
		result.Message = "No deploy configuration"
		result.Duration = time.Since(start)
		return result
	}

	appDir := filepath.Join(softwareRoot, filepath.FromSlash(appFolderName(app)))
	srcDir := filepath.Join(appDir, filepath.FromSlash(cfg.SourceDir))
	if !utils.DirExists(srcDir) {
		result.Status = models.TaskStatusFailed
		result.Message = fmt.Sprintf("Deploy source not found: %s", srcDir)
		result.Err = fmt.Errorf("deploy source not found: %s", srcDir)
		result.Duration = time.Since(start)
		return result
	}

	destDir := utils.ExpandEnv(cfg.InstallDir)
	if strings.TrimSpace(destDir) == "" {
		result.Status = models.TaskStatusFailed
		result.Message = "Deploy installDir is empty"
		result.Err = fmt.Errorf("deploy installDir is empty")
		result.Duration = time.Since(start)
		return result
	}

	stats, err := copy.SyncDirectory(srcDir, destDir, logging.NopLogger{})
	if err != nil {
		result.Status = models.TaskStatusFailed
		result.Message = fmt.Sprintf("Failed to copy payload: %v", err)
		result.Err = err
		result.Duration = time.Since(start)
		return result
	}
	if stats.Failed > 0 {
		result.Status = models.TaskStatusFailed
		result.Message = fmt.Sprintf("Copy failed: Copied=%d Skipped=%d Failed=%d", stats.Copied, stats.Skipped, stats.Failed)
		result.Err = fmt.Errorf("%d file copy operations failed", stats.Failed)
		result.Duration = time.Since(start)
		return result
	}

	var warnings []string

	if cfg.FontsDir != "" {
		fontDir := filepath.Join(destDir, filepath.FromSlash(cfg.FontsDir))
		if err := installFonts(ctx, fontDir); err != nil {
			warnings = append(warnings, "fonts: "+err.Error())
		}
	}

	systemRoot := windowsSystemRoot()
	for _, rel := range cfg.SystemFiles {
		srcFile := filepath.Join(srcDir, filepath.FromSlash(rel))
		if !utils.FileExists(srcFile) {
			warnings = append(warnings, "missing system file: "+rel)
			continue
		}
		if err := copyToSystemDirs(srcFile, systemRoot); err != nil {
			warnings = append(warnings, "copy "+rel+" to system dirs: "+err.Error())
			continue
		}
		sysCopy := filepath.Join(systemRoot, "System32", filepath.Base(srcFile))
		if err := registerComponent(ctx, sysCopy); err != nil {
			warnings = append(warnings, "register "+rel+": "+err.Error())
		}
	}

	for _, rel := range cfg.RegisterFiles {
		file := filepath.Join(destDir, filepath.FromSlash(rel))
		if !utils.FileExists(file) {
			warnings = append(warnings, "missing component: "+rel)
			continue
		}
		if err := registerComponent(ctx, file); err != nil {
			warnings = append(warnings, "register "+rel+": "+err.Error())
		}
	}

	exeVerified := cfg.Executable == ""
	if cfg.Executable != "" {
		exePath := filepath.Join(destDir, filepath.FromSlash(cfg.Executable))
		exeVerified = utils.FileExists(exePath)
		if !exeVerified {
			warnings = append(warnings, "executable missing: "+cfg.Executable)
		}
	}

	result.Duration = time.Since(start)
	if exeVerified {
		result.Status = models.TaskStatusSuccess
		result.Message = fmt.Sprintf("Deployed %d files to %s", stats.Copied, destDir)
		if len(warnings) > 0 {
			result.Message += " (warnings: " + strings.Join(warnings, "; ") + ")"
		}
		return result
	}

	result.Status = models.TaskStatusFailed
	result.Message = "Deploy incomplete: " + strings.Join(warnings, "; ")
	result.Err = fmt.Errorf("deploy incomplete for %s: %s", app.Name, strings.Join(warnings, "; "))
	return result
}

// windowsSystemRoot returns the Windows directory (C:\Windows by default).
func windowsSystemRoot() string {
	root := os.Getenv("SystemRoot")
	if strings.TrimSpace(root) == "" {
		root = `C:\Windows`
	}
	return root
}

// installFonts registers every *.ttf font in fontDir with Windows: the file
// is copied to C:\Windows\Fonts and a value is added under the fonts
// registry key so the typeface is available system-wide.
func installFonts(ctx context.Context, fontDir string) error {
	if !utils.DirExists(fontDir) {
		return fmt.Errorf("fonts directory not found: %s", fontDir)
	}
	quoted := strings.ReplaceAll(fontDir, "'", "''")
	script := fmt.Sprintf(fontInstallScript, quoted)
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("install fonts via PowerShell: %w", err)
	}
	return nil
}

const fontInstallScript = `
$dir = '%s'
$fonts = Join-Path $env:WINDIR 'Fonts'
$reg = 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Fonts'
Add-Type -AssemblyName System.Drawing
$files = @(Get-ChildItem -Path $dir -File | Where-Object { $_.Extension -imatch '^\.ttf$' })
foreach ($f in $files) {
  try {
    Copy-Item -Path $f.FullName -Destination (Join-Path $fonts $f.Name) -Force
    $family = $f.BaseName
    try {
      $fc = New-Object System.Drawing.Text.PrivateFontCollection
      $fc.AddFontFile($f.FullName)
      if ($fc.Families.Count -gt 0) { $family = $fc.Families[0].Name }
      $fc.Dispose()
    } catch { }
    New-ItemProperty -Path $reg -Name ($family + ' (TrueType)') -Value $f.Name -PropertyType String -Force | Out-Null
  } catch {
    Write-Warning ("Font " + $f.Name + ": " + $_.Exception.Message)
  }
}
Write-Output ("INSTALLED_FONTS=" + $files.Count)
`

// copyToSystemDirs copies a file into both System32 and SysWOW64 so that
// both 64-bit and 32-bit processes can load the component.
func copyToSystemDirs(src, systemRoot string) error {
	base := filepath.Base(src)
	copied := false
	for _, dir := range []string{"System32", "SysWOW64"} {
		dest := filepath.Join(systemRoot, dir, base)
		if err := copySingleFile(src, dest); err != nil {
			return err
		}
		copied = true
	}
	if !copied {
		return fmt.Errorf("no system directory available")
	}
	return nil
}

func copySingleFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source file: %w", err)
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return fmt.Errorf("stat source file: %w", err)
	}
	if err := utils.EnsureDir(filepath.Dir(dst)); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return fmt.Errorf("create destination file: %w", err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy file contents: %w", err)
	}
	if err := out.Sync(); err != nil {
		return fmt.Errorf("sync destination file: %w", err)
	}
	return nil
}

// registerComponent self-registers a COM component (OCX/DLL) with regsvr32.
// 32-bit components require the SysWOW64 regsvr32 on 64-bit Windows, so both
// are attempted and the first success wins.
func registerComponent(ctx context.Context, path string) error {
	if !utils.FileExists(path) {
		return fmt.Errorf("component not found: %s", path)
	}
	root := windowsSystemRoot()
	regsvrs := []string{
		filepath.Join(root, "SysWOW64", "regsvr32.exe"),
		filepath.Join(root, "System32", "regsvr32.exe"),
	}
	var lastErr error
	for _, regsvr := range regsvrs {
		if !utils.FileExists(regsvr) {
			continue
		}
		cmd := exec.CommandContext(ctx, regsvr, "/s", path)
		if err := cmd.Run(); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no regsvr32.exe found under %s", root)
	}
	return lastErr
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
		if !e.IsDir() {
			ext := strings.ToLower(filepath.Ext(e.Name()))
			if ext == ".exe" || ext == ".msi" {
				return filepath.Join(dir, e.Name()), nil
			}
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
