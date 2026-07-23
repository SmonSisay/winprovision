//go:build windows

// Package windows applies idempotent Windows configuration tasks.
package windows

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
	"unsafe"

	"github.com/SmonSisay/winprovision/internal/models"
	winreg "github.com/SmonSisay/winprovision/internal/registry"
	"golang.org/x/sys/windows"
)

const (
	firewallModule = "windows.firewall"
	rdpModule      = "windows.rdp"
	adminModule    = "windows.administrator"
	explorerModule = "windows.explorer"
)

// DisableFirewall disables Windows Firewall for all profiles when not already disabled.
func DisableFirewall(ctx context.Context) models.TaskResult {
	start := time.Now()
	result := models.TaskResult{
		Name:   "Firewall",
		Module: firewallModule,
	}

	if isFirewallDisabled() {
		result.Status = models.TaskStatusSkipped
		result.Message = "Firewall already disabled"
		result.Duration = time.Since(start)
		return result
	}

	cmd := exec.CommandContext(ctx, "netsh", "advfirewall", "set", "allprofiles", "state", "off")
	output, err := cmd.CombinedOutput()
	if err != nil {
		result.Status = models.TaskStatusFailed
		result.Message = strings.TrimSpace(string(output))
		result.Err = fmt.Errorf("disable firewall: %w", err)
		result.Duration = time.Since(start)
		return result
	}

	result.Status = models.TaskStatusSuccess
	result.Message = "Firewall disabled"
	result.Duration = time.Since(start)
	return result
}

// EnableRemoteDesktop enables Remote Desktop when not already enabled.
// It also disables NLA (Network Level Authentication) and enables Remote Assistance.
func EnableRemoteDesktop(ctx context.Context) models.TaskResult {
	start := time.Now()
	result := models.TaskResult{
		Name:   "Remote Desktop",
		Module: rdpModule,
	}

	const rdpKey = `HKLM\SYSTEM\CurrentControlSet\Control\Terminal Server`
	value, err := winreg.GetDWORD(rdpKey, "fDenyTSConnections")
	if err == nil && value == 0 {
		result.Status = models.TaskStatusSkipped
		result.Message = "Remote Desktop already enabled"
		result.Duration = time.Since(start)
		return result
	}

	// Allow remote connections (fDenyTSConnections = 0)
	if err := winreg.SetDWORD(rdpKey, "fDenyTSConnections", 0); err != nil {
		result.Status = models.TaskStatusFailed
		result.Message = "Failed to update RDP registry setting"
		result.Err = err
		result.Duration = time.Since(start)
		return result
	}

	// Disable NLA — uncheck "Allow connections only from computers running
	// Remote Desktop with Network Level Authentication"
	const rdpTcpKey = `HKLM\SYSTEM\CurrentControlSet\Control\Terminal Server\WinStations\RDP-Tcp`
	if err := winreg.SetDWORD(rdpTcpKey, "UserAuthentication", 0); err != nil {
		result.Status = models.TaskStatusFailed
		result.Message = "Failed to disable Network Level Authentication"
		result.Err = err
		result.Duration = time.Since(start)
		return result
	}

	// Enable Remote Assistance — check "Allow Remote Assistance connections"
	const raKey = `HKLM\SYSTEM\CurrentControlSet\Control\Remote Assistance`
	if err := winreg.SetDWORD(raKey, "fAllowToGetHelp", 1); err != nil {
		result.Status = models.TaskStatusFailed
		result.Message = "Failed to enable Remote Assistance"
		result.Err = err
		result.Duration = time.Since(start)
		return result
	}

	// Configure TermService to start automatically.
	if out, err := exec.CommandContext(ctx, "sc", "config", "TermService", "start=", "auto").CombinedOutput(); err != nil {
		result.Status = models.TaskStatusFailed
		result.Message = fmt.Sprintf("RDP registry set but TermService config failed: %s", strings.TrimSpace(string(out)))
		result.Err = fmt.Errorf("configure TermService: %w", err)
		result.Duration = time.Since(start)
		return result
	}

	// Start TermService — ignore "already running" (exit 2), fail on anything else.
	if out, err := exec.CommandContext(ctx, "net", "start", "TermService").CombinedOutput(); err != nil {
		outStr := strings.ToLower(string(out))
		if !strings.Contains(outStr, "already been started") {
			result.Status = models.TaskStatusFailed
			result.Message = fmt.Sprintf("RDP enabled but TermService failed to start: %s", strings.TrimSpace(string(out)))
			result.Err = fmt.Errorf("start TermService: %w", err)
			result.Duration = time.Since(start)
			return result
		}
	}

	result.Status = models.TaskStatusSuccess
	result.Message = "Remote Desktop enabled"
	result.Duration = time.Since(start)
	return result
}

// EnableAdministrator enables the built-in Administrator account.
// Always runs "net user /active:yes" — it's idempotent and avoids false
// positives from the registry-based check (SpecialAccounts\UserList may not
// exist on fresh installs, causing the old check to skip when the account
// was still disabled at the SAM level).
func EnableAdministrator(ctx context.Context) models.TaskResult {
	start := time.Now()
	result := models.TaskResult{
		Name:   "Administrator",
		Module: adminModule,
	}

	cmd := exec.CommandContext(ctx, "net", "user", "administrator", "/active:yes")
	output, err := cmd.CombinedOutput()
	if err != nil {
		result.Status = models.TaskStatusFailed
		result.Message = strings.TrimSpace(string(output))
		result.Err = fmt.Errorf("enable administrator: %w", err)
		result.Duration = time.Since(start)
		return result
	}

	// Prevent the user from changing the Administrator password.
	lockCmd := exec.CommandContext(ctx, "net", "user", "administrator", "/passwordchg:no")
	lockOut, lockErr := lockCmd.CombinedOutput()

	// Make the password never expire.
	expireCmd := exec.CommandContext(ctx, "net", "user", "administrator", "/expires:never")
	expireOut, expireErr := expireCmd.CombinedOutput()

	outStr := strings.ToLower(string(output))
	if strings.Contains(outStr, "already") {
		result.Status = models.TaskStatusSkipped
		result.Message = "Administrator account already enabled"
	} else {
		result.Status = models.TaskStatusSuccess
		result.Message = "Administrator account enabled"
	}

	if lockErr != nil {
		result.Message += " (warning: could not lock password change: " + strings.TrimSpace(string(lockOut)) + ")"
	} else {
		result.Message += ", password change locked"
	}

	if expireErr != nil {
		result.Message += " (warning: could not set password to never expire: " + strings.TrimSpace(string(expireOut)) + ")"
	} else {
		result.Message += ", password never expires"
	}

	result.Duration = time.Since(start)
	return result
}

// SetAdministratorPassword sets the built-in Administrator password using the
// Win32 NetUserSetInfo API (level 1003). This keeps the password entirely out
// of process argument lists, command-line monitoring tools, and PowerShell logs.
func SetAdministratorPassword(_ context.Context, password string) models.TaskResult {
	start := time.Now()
	result := models.TaskResult{
		Name:   "Administrator Password",
		Module: adminModule,
	}

	utf16Pass, err := windows.UTF16PtrFromString(password)
	if err != nil {
		result.Status = models.TaskStatusFailed
		result.Message = "Failed to encode password"
		result.Err = fmt.Errorf("encode password: %w", err)
		result.Duration = time.Since(start)
		return result
	}

	// USER_INFO_1003 — the only field is the password pointer.
	info := userInfo1003{password: utf16Pass}

	server, _ := windows.UTF16PtrFromString("")
	userName, _ := windows.UTF16PtrFromString("Administrator")

	ret, _, _ := procNetUserSetInfo.Call(
		uintptr(unsafe.Pointer(server)),
		uintptr(unsafe.Pointer(userName)),
		1003, // level 1003 = set password
		uintptr(unsafe.Pointer(&info)),
		0, // no param error index needed
	)
	if ret != 0 {
		result.Status = models.TaskStatusFailed
		result.Message = fmt.Sprintf("NetUserSetInfo failed (error code %d)", ret)
		result.Err = fmt.Errorf("NetUserSetInfo for Administrator: error code %d", ret)
		result.Duration = time.Since(start)
		return result
	}

	result.Status = models.TaskStatusSuccess
	result.Message = "Administrator password updated"
	result.Duration = time.Since(start)
	return result
}

// DisableWindowsUpdate sets the Windows Update service to manual start.
func DisableWindowsUpdate(ctx context.Context) models.TaskResult {
	start := time.Now()
	result := models.TaskResult{
		Name:   "Windows Update",
		Module: "windows.update",
	}

	// Check current startup type: 2=Automatic, 3=Manual, 4=Disabled
	const svcKey = `HKLM\SYSTEM\CurrentControlSet\Services\wuauserv`
	current, err := winreg.GetDWORD(svcKey, "Start")
	if err == nil && current == 3 {
		result.Status = models.TaskStatusSkipped
		result.Message = "Windows Update already set to manual"
		result.Duration = time.Since(start)
		return result
	}

	// Set startup type to Manual (3)
	if err := winreg.SetDWORD(svcKey, "Start", 3); err != nil {
		result.Status = models.TaskStatusFailed
		result.Message = "Failed to change Windows Update startup type"
		result.Err = err
		result.Duration = time.Since(start)
		return result
	}

	// Stop the service if running
	if out, err := exec.CommandContext(ctx, "net", "stop", "wuauserv").CombinedOutput(); err != nil {
		outStr := strings.ToLower(string(out))
		if !strings.Contains(outStr, "not started") {
			// Ignore — service may already be stopped
			_ = outStr
		}
	}

	result.Status = models.TaskStatusSuccess
	result.Message = "Windows Update set to manual start"
	result.Duration = time.Since(start)
	return result
}

// ShowFileExtensions configures Explorer to show file extensions.
func ShowFileExtensions() models.TaskResult {
	start := time.Now()
	result := models.TaskResult{
		Name:   "Show File Extensions",
		Module: explorerModule,
	}

	const key = `HKCU\Software\Microsoft\Windows\CurrentVersion\Explorer\Advanced`
	value, err := winreg.GetDWORD(key, "HideFileExt")
	if err == nil && value == 0 {
		result.Status = models.TaskStatusSkipped
		result.Message = "File extensions already shown"
		result.Duration = time.Since(start)
		return result
	}

	if err := winreg.SetDWORD(key, "HideFileExt", 0); err != nil {
		result.Status = models.TaskStatusFailed
		result.Message = "Failed to update HideFileExt registry value"
		result.Err = err
		result.Duration = time.Since(start)
		return result
	}

	result.Status = models.TaskStatusSuccess
	result.Message = "File extensions enabled"
	result.Duration = time.Since(start)
	return result
}

// ShowHiddenFiles configures Explorer to show hidden files.
func ShowHiddenFiles() models.TaskResult {
	start := time.Now()
	result := models.TaskResult{
		Name:   "Show Hidden Files",
		Module: explorerModule,
	}

	const key = `HKCU\Software\Microsoft\Windows\CurrentVersion\Explorer\Advanced`
	value, err := winreg.GetDWORD(key, "Hidden")
	if err == nil && value == 1 {
		result.Status = models.TaskStatusSkipped
		result.Message = "Hidden files already shown"
		result.Duration = time.Since(start)
		return result
	}

	if err := winreg.SetDWORD(key, "Hidden", 1); err != nil {
		result.Status = models.TaskStatusFailed
		result.Message = "Failed to update Hidden registry value"
		result.Err = err
		result.Duration = time.Since(start)
		return result
	}

	result.Status = models.TaskStatusSuccess
	result.Message = "Hidden files enabled"
	result.Duration = time.Since(start)
	return result
}

// isFirewallDisabled checks whether all firewall profiles are already off.
// Uses per-profile registry keys instead of parsing netsh text output, which
// is locale-dependent and fragile across Windows builds.
func isFirewallDisabled() bool {
	// Check each profile's EnableFirewall DWORD. A value of 0 means disabled.
	// If any profile is still enabled, the firewall is not fully disabled.
	for _, profileKey := range []string{
		`HKLM\SOFTWARE\Policies\Microsoft\WindowsFirewall\DomainProfile`,
		`HKLM\SOFTWARE\Policies\Microsoft\WindowsFirewall\StandardProfile`,
		`HKLM\SOFTWARE\Policies\Microsoft\WindowsFirewall\PrivateProfile`,
		`HKLM\SYSTEM\CurrentControlSet\Services\SharedAccess\Parameters\FirewallPolicy\DomainProfile`,
		`HKLM\SYSTEM\CurrentControlSet\Services\SharedAccess\Parameters\FirewallPolicy\StandardProfile`,
		`HKLM\SYSTEM\CurrentControlSet\Services\SharedAccess\Parameters\FirewallPolicy\PublicProfile`,
	} {
		val, err := winreg.GetDWORD(profileKey, "EnableFirewall")
		if err == nil && val != 0 {
			return false
		}
	}
	// Fallback: if we couldn't read any profile, check netsh for "state    off"
	// in all profiles (English-only, but better than nothing).
	cmd := exec.Command("netsh", "advfirewall", "show", "allprofiles", "state")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return false
	}
	output := strings.ToLower(stdout.String())
	// All three profiles must show "state    off".
	// Count occurrences — if any profile still has "state    on", not disabled.
	offCount := strings.Count(output, "state                                 on")
	return offCount == 0
}

// Win32 API types and procs for NetUserSetInfo.

type userInfo1003 struct {
	password *uint16
}

var (
	netapi32              = windows.NewLazySystemDLL("netapi32.dll")
	procNetUserSetInfo    = netapi32.NewProc("NetUserSetInfo")
)
