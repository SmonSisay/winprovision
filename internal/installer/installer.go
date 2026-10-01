// Package installer detects and installs applications defined in apps.json.
package installer

import (
	"context"
	"errors"
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

// successExitCodes are non-zero installer exit codes that still mean the install
// worked. InstallShield's setup.exe returns 1641 and Windows Installer / DISM
// return 3010 when the product is installed but Windows wants a restart before
// every change takes effect. We do not restart and do not report a pending
// restart, so these are counted as ordinary successes; anything else non-zero
// is a real failure.
var successExitCodes = map[int]bool{1641: true, 3010: true}

// installerExitError carries an installer's exit code alongside the exec error
// so the attempt loop can tell a successful-but-restarting installer apart from
// one that genuinely failed.
type installerExitError struct {
	code int
	err  error
}

func (e *installerExitError) Error() string { return e.err.Error() }

func (e *installerExitError) Unwrap() error { return e.err }

// exitCodeOf returns the exit code an installer terminated with, or -1 when the
// code is unavailable (the process was killed by a signal, or the error did not
// come from running an installer at all).
func exitCodeOf(err error) int {
	var ie *installerExitError
	if errors.As(err, &ie) {
		return ie.code
	}
	return -1
}

// isSuccessExit reports whether err is a non-zero exit that still means the
// install succeeded, and returns the exit code that said so.
func isSuccessExit(err error) (code int, ok bool) {
	code = exitCodeOf(err)
	return code, code >= 0 && successExitCodes[code]
}

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

	// The configured path may point at a file that was renamed, so fall back
	// to searching the folder it lives in, then the app folder by name.
	for _, dir := range appSearchDirs(app, softwareRoot) {
		if !utils.DirExists(dir) {
			continue
		}
		if exePath, err := findInstallerExe(dir); err == nil && exePath != "" {
			return exePath
		}
	}
	return ""
}

// appSearchDirs returns the directories to search for an app's installer, in
// priority order: the folder holding the configured installer path, then the
// app folder derived from that path or the app name.
func appSearchDirs(app models.AppDefinition, softwareRoot string) []string {
	configured := filepath.Join(softwareRoot, filepath.FromSlash(app.InstallerPath))
	return []string{
		filepath.Dir(configured),
		filepath.Join(softwareRoot, appFolderName(app)),
	}
}

// Install runs the application installer from the copied software directory.
//
// The strategy is: skip if already installed (unless AlwaysInstall), then try
// silent flags, then optionally fall back to the interactive wizard. Exactly
// which of those apply is decided by the app's flags — see the comment on each.
func Install(ctx context.Context, app models.AppDefinition, softwareRoot string) models.TaskResult {
	start := time.Now()
	result := models.TaskResult{Name: app.Name, Module: moduleName}

	// --- preconditions -------------------------------------------------
	installed, reason, err := IsInstalled(app)
	if err != nil {
		return result.Fail(start, "Failed to evaluate installation state", err)
	}
	if installed && !app.AlwaysInstall {
		return result.Skip(start, fmt.Sprintf("Already installed (%s)", reason))
	}

	installerPath := resolveInstallerPath(app, softwareRoot)
	if installerPath == "" {
		return result.Fail(start,
			fmt.Sprintf("No installer (.exe) found in '%s' folder under software/", appFolderName(app)),
			fmt.Errorf("no installer found for %s", app.Name))
	}

	return attemptInstall(ctx, result, start, installerPath, buildAttempts(app), installerRunner(installerPath))
}

// attempt is one launch of an installer: the arguments to pass, and whether
// this launch is the interactive wizard rather than a silent run.
type attempt struct {
	args     []string
	attended bool
}

// runFunc carries out one attempt and reports whether it succeeded. It is a
// parameter so the attempt sequence and the resulting status can be exercised
// without running a real installer. The whole attempt is passed rather than
// just its arguments, because a wizard attempt and a silent attempt can carry
// the same (empty) argument list.
type runFunc func(ctx context.Context, a attempt) error

// buildAttempts returns the ordered attempts for an app, stopping at the first
// that is expected to work. It is a pure function of the app's flags, so the
// decision can be pinned in tests.
func buildAttempts(app models.AppDefinition) []attempt {
	// AttendedOnly installers reject every silent flag (each one pops an
	// "Invalid command line" dialog before falling back to the wizard anyway),
	// so for those the wizard is the only attempt.
	if app.AttendedOnly {
		return []attempt{{attended: true}}
	}

	attempts := make([]attempt, 0, len(fallbackSilentFlags)+1)
	for _, flags := range silentFlagSets(app) {
		attempts = append(attempts, attempt{args: flags})
	}
	if app.AttendedFallback {
		attempts = append(attempts, attempt{attended: true})
	}
	return attempts
}

// attemptInstall runs the attempts in order and returns the outcome of the
// first one that succeeds. If every attempt fails, the last error is reported:
// it came from the final thing tried, so it is the most relevant cause.
func attemptInstall(
	ctx context.Context,
	result models.TaskResult,
	start time.Time,
	installerPath string,
	attempts []attempt,
	run runFunc,
) models.TaskResult {
	if len(attempts) == 0 {
		return result.Fail(start,
			fmt.Sprintf("No install attempts configured for %s", appFolderNameFor(installerPath)),
			fmt.Errorf("no install attempts for %s", installerPath))
	}

	var lastErr error
	for _, a := range attempts {
		lastErr = run(ctx, a)
		if lastErr == nil {
			if a.attended {
				return result.Succeed(start, "Installed successfully (attended wizard)")
			}
			return result.Succeed(start, "Installed successfully")
		}

		// An installer that exited 1641/3010 has finished the install even though
		// it asked for a restart. Stop here rather than trying the remaining
		// attempts, which would re-run an install that already completed.
		if code, ok := isSuccessExit(lastErr); ok {
			return result.Succeed(start,
				fmt.Sprintf("Installed successfully (exit=%d)", code))
		}
	}

	if ranSilentAttempts(attempts) {
		return result.Fail(start,
			fmt.Sprintf("All install attempts failed: %v", lastErr),
			fmt.Errorf("install %s: %w", installerPath, lastErr))
	}
	return result.Fail(start,
		fmt.Sprintf("All silent install attempts failed: %v", lastErr),
		fmt.Errorf("install %s: %w", installerPath, lastErr))
}

// ranSilentAttempts reports whether any attempt used command-line arguments,
// which decides how the failure is worded: a wizard-only failure and a silent
// failure need different messages to be useful in a log.
func ranSilentAttempts(attempts []attempt) bool {
	for _, a := range attempts {
		if !a.attended {
			return true
		}
	}
	return false
}

// appFolderNameFor is only used in the "no attempts configured" message, where
// the installer path is the more useful thing to name.
func appFolderNameFor(installerPath string) string {
	return filepath.Base(filepath.Dir(installerPath))
}

// silentFlagSets returns the argument sets to try, in order. Explicitly
// configured args are used alone when present: guessing past a known-good
// command line only risks running a partial install twice.
func silentFlagSets(app models.AppDefinition) [][]string {
	if explicit := SplitArgs(app.SilentArgs); len(explicit) > 0 {
		return [][]string{explicit}
	}
	return fallbackSilentFlags
}

// launchInstaller runs an installer, routing .msi files through msiexec and
// setting the working directory to the installer's own folder so relative
// paths in installer scripts resolve correctly.
func launchInstaller(ctx context.Context, installerPath string, args []string) error {
	var cmd *exec.Cmd
	if strings.EqualFold(filepath.Ext(installerPath), ".msi") {
		// Verbose logging is the only way to tell "the package is wrong" apart
		// from "a prerequisite is missing" — msiexec reports both as a bare
		// exit code 1. The log is written next to the package on the
		// provisioning drive so it survives the run and can be collected.
		logPath := strings.TrimSuffix(installerPath, filepath.Ext(installerPath)) + ".msi.log"
		msiArgs := append([]string{"/i", installerPath, "/l*v", logPath}, args...)
		cmd = exec.CommandContext(ctx, "msiexec.exe", msiArgs...)
		fmt.Printf("[installer] MSI verbose log: %s\n", logPath)
	} else {
		cmd = exec.CommandContext(ctx, installerPath, args...)
	}
	cmd.Dir = filepath.Dir(installerPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	runErr := cmd.Run()
	if runErr == nil {
		return nil
	}

	// The exit code is only meaningful when the process actually ran to
	// completion; a killed process reports -1 and must not be mistaken for a
	// successful install.
	code := -1
	if cmd.ProcessState != nil {
		code = cmd.ProcessState.ExitCode()
	}
	return &installerExitError{code: code, err: runErr}
}

// installerRunner adapts launchInstaller to the runFunc signature, binding the
// installer path.
func installerRunner(installerPath string) runFunc {
	return func(ctx context.Context, a attempt) error {
		return launchInstaller(ctx, installerPath, a.args)
	}
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
			exitCode := -1
			if cmd.ProcessState != nil {
				exitCode = cmd.ProcessState.ExitCode()
			}
			if successExitCodes[exitCode] {
				result.Status = models.TaskStatusSuccess
				result.Message = fmt.Sprintf("Installed from %s (exit=%d)", filepath.Base(exePath), exitCode)
			} else {
				result.Status = models.TaskStatusFailed
				result.Message = fmt.Sprintf("Installer failed (exit=%d)", exitCode)
				result.Err = fmt.Errorf("run discovered installer %s: %w", exePath, runErr)
			}
		} else {
			result.Status = models.TaskStatusSuccess
			result.Message = "Installed from " + filepath.Base(exePath)
		}
		result.Duration = duration
		results = append(results, result)
	}
	return results
}
