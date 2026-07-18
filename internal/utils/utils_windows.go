//go:build windows

package utils

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// IsAdmin reports whether the current process has administrator privileges.
func IsAdmin() (bool, error) {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token); err != nil {
		return false, fmt.Errorf("open process token: %w", err)
	}
	defer token.Close()

	var elevation uint32
	var returned uint32
	err := windows.GetTokenInformation(
		token,
		windows.TokenElevation,
		(*byte)(unsafe.Pointer(&elevation)),
		uint32(unsafe.Sizeof(elevation)),
		&returned,
	)
	if err == nil {
		return elevation != 0, nil
	}

	return isAdministratorsGroupMember(token)
}

func isAdministratorsGroupMember(token windows.Token) (bool, error) {
	var sid *windows.SID
	err := windows.AllocateAndInitializeSid(
		&windows.SECURITY_NT_AUTHORITY,
		2,
		windows.SECURITY_BUILTIN_DOMAIN_RID,
		windows.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0,
		&sid,
	)
	if err != nil {
		return false, fmt.Errorf("allocate admin SID: %w", err)
	}
	defer windows.FreeSid(sid)

	member, err := token.IsMember(sid)
	if err != nil {
		return false, fmt.Errorf("check admin membership: %w", err)
	}
	return member, nil
}

// GetWindowsVersion returns a human-readable Windows version string.
func GetWindowsVersion() (string, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return "", fmt.Errorf("open version registry key: %w", err)
	}
	defer k.Close()

	productName, _, err := k.GetStringValue("ProductName")
	if err != nil {
		productName = "Windows"
	}
	build, _, err := k.GetStringValue("CurrentBuild")
	if err != nil {
		build, _, _ = k.GetStringValue("CurrentBuildNumber")
	}
	display, _, _ := k.GetStringValue("DisplayVersion")

	// Windows 11 builds start at 22000; registry may still say "Windows 10"
	if buildNum, parseErr := strconv.Atoi(build); parseErr == nil && buildNum >= 22000 {
		productName = strings.Replace(productName, "Windows 10", "Windows 11", 1)
	}

	if display != "" {
		return fmt.Sprintf("%s %s (Build %s)", productName, display, build), nil
	}
	return fmt.Sprintf("%s (Build %s)", productName, build), nil
}

// DetectDestinationDrive returns the first available non-system fixed drive
// (internal hard drive, not USB removable).
func DetectDestinationDrive() (string, error) {
	systemDrive := strings.ToUpper(strings.TrimSuffix(os.Getenv("SystemDrive"), `\`))
	if systemDrive == "" {
		systemDrive = "C:"
	}

	drives, err := listLogicalDrives()
	if err != nil {
		return "", err
	}

	for _, drive := range drives {
		if strings.EqualFold(drive, systemDrive) {
			continue
		}
		root := drive + `\`
		driveType := windows.GetDriveType(syscall.StringToUTF16Ptr(root))
		if driveType == windows.DRIVE_FIXED {
			return drive, nil
		}
	}
	return "", fmt.Errorf("no secondary fixed drive found")
}

// DetectBootableDrive scans all drives for a Windows bootable disk
// containing the sources\sxs directory, excluding the system drive.
func DetectBootableDrive() (string, error) {
	systemDrive := strings.ToUpper(strings.TrimSuffix(os.Getenv("SystemDrive"), `\`))
	if systemDrive == "" {
		systemDrive = "C:"
	}

	drives, err := listLogicalDrives()
	if err != nil {
		return "", err
	}

	for _, drive := range drives {
		if strings.EqualFold(drive, systemDrive) {
			continue
		}
		sxsPath := drive + `\sources\sxs`
		if DirExists(sxsPath) {
			return drive, nil
		}
	}
	return "", fmt.Errorf("no bootable Windows drive found with sources\\sxs directory")
}

// GetOSBuildNumber returns the Windows build number.
func GetOSBuildNumber() (uint32, error) {
	var info osVersionInfoEx
	info.Size = uint32(unsafe.Sizeof(info))
	ret, _, err := procRtlGetVersion.Call(uintptr(unsafe.Pointer(&info)))
	if ret != 0 {
		return 0, fmt.Errorf("RtlGetVersion failed: %w", err)
	}
	return info.BuildNumber, nil
}

type osVersionInfoEx struct {
	Size             uint32
	MajorVersion     uint32
	MinorVersion     uint32
	BuildNumber      uint32
	PlatformId       uint32
	CSDVersion       [128]uint16
	ServicePackMajor uint16
	ServicePackMinor uint16
	SuiteMask        uint16
	ProductType      byte
	Reserved         byte
}

var (
	ntdll             = windows.NewLazySystemDLL("ntdll.dll")
	procRtlGetVersion = ntdll.NewProc("RtlGetVersion")
)

func listLogicalDrives() ([]string, error) {
	buffer := make([]uint16, 256)
	n, err := windows.GetLogicalDriveStrings(uint32(len(buffer)), &buffer[0])
	if err != nil {
		return nil, fmt.Errorf("enumerate logical drives: %w", err)
	}
	if n == 0 {
		return nil, fmt.Errorf("no logical drives found")
	}

	raw := syscall.UTF16ToString(buffer[:n])
	parts := strings.Split(raw, "\x00")
	var drives []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		drives = append(drives, strings.TrimSuffix(part, `\`))
	}
	return drives, nil
}
