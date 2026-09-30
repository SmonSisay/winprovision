// Package utils provides cross-platform helpers with Windows-specific implementations.
package utils

import (
	"bufio"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

// GetExecutableDir returns the directory containing the running executable.
func GetExecutableDir() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve executable path: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(exePath)
	if err != nil {
		resolved = exePath
	}
	return filepath.Dir(resolved), nil
}

// GetLoggedInUser returns the name of the currently logged-in user.
func GetLoggedInUser() (string, error) {
	current, err := user.Current()
	if err == nil && current.Username != "" {
		return current.Username, nil
	}
	username := os.Getenv("USERNAME")
	if username == "" {
		return "", fmt.Errorf("determine logged-in user")
	}
	domain := os.Getenv("USERDOMAIN")
	if domain != "" {
		return domain + "\\" + username, nil
	}
	return username, nil
}

// PromptDestinationFolder asks the operator which drive or folder the software
// should be copied into. It returns a root — never the final software folder —
// so the caller can append settings.Destination.FolderName exactly once no
// matter which code path produced the value. The examples therefore show a bare
// drive or folder, not the finished destination.
func PromptDestinationFolder(folderName string) (string, error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println()
	fmt.Println("  ─── Destination Folder ───")
	fmt.Println()
	fmt.Println("  No fixed volume other than the system drive was found.")
	fmt.Println("  Enter the drive or folder that should receive the software.")
	fmt.Println()
	fmt.Println("  Examples:")
	fmt.Println(`    D:\`)
	fmt.Println(`    E:\`)
	fmt.Println(`    D:\Work\`)
	fmt.Println()
	fmt.Printf("  Setup.exe will create a '%s' folder inside it.\n", folderName)
	fmt.Println()
	fmt.Print("  Enter path: ")

	line, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("read destination folder: %w", err)
	}
	path := strings.TrimSpace(line)
	if path == "" {
		return "", fmt.Errorf("destination folder cannot be empty")
	}

	path = filepath.Clean(path)
	if !IsAbsoluteWindowsPath(path) {
		return "", fmt.Errorf("please enter a full path (e.g. D:\\)")
	}

	if !DirExists(path) {
		return "", fmt.Errorf("path does not exist or is not accessible: %s", path)
	}

	return path, nil
}

// PromptBootableDrive asks the user to enter the bootable flash drive letter or full path.
func PromptBootableDrive() (string, error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println()
	fmt.Println("  ─── Bootable Flash Drive ───")
	fmt.Println()
	fmt.Println("  Windows installation media not detected.")
	fmt.Println("  Enter the drive letter or path to the sources\\sxs folder:")
	fmt.Println()
	fmt.Println("  Examples:")
	fmt.Println("    D:")
	fmt.Println("    E:")
	fmt.Println("    D:\\sources\\sxs")
	fmt.Println()
	fmt.Print("  Enter path: ")

	line, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("read bootable drive: %w", err)
	}
	input := strings.TrimSpace(line)
	if input == "" {
		return "", fmt.Errorf("bootable drive path cannot be empty")
	}
	return input, nil
}

// IsSxSDirectory reports whether path is a Windows sources\sxs directory
// holding a usable .NET Framework 3.5 payload, i.e. at least one .cab file.
// An existing but empty directory is not a usable payload source.
//
// This is the single definition of "valid sources\sxs"; the DISM source
// lookup and the bootable-drive probe both use it so they cannot disagree.
func IsSxSDirectory(path string) bool {
	if !DirExists(path) {
		return false
	}
	matches, err := filepath.Glob(filepath.Join(path, "*.cab"))
	return err == nil && len(matches) > 0
}

// ResolveSoftwareDestination returns the full path to the software destination directory.
func ResolveSoftwareDestination(destinationRoot, folderName string) string {
	return filepath.Join(destinationRoot, folderName)
}

// FileExists reports whether a path exists and is not a directory.
func FileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// DirExists reports whether a directory exists.
func DirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// DirSize returns the combined size in bytes of every regular file under path.
// It is used to work out how much free space a destination volume needs before
// the software copy is attempted.
func DirSize(path string) (int64, error) {
	var total int64
	err := filepath.WalkDir(path, func(_ string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("measure directory %s: %w", path, err)
	}
	return total, nil
}

// IsAbsoluteWindowsPath reports whether path looks like an absolute Windows path.
func IsAbsoluteWindowsPath(path string) bool {
	if len(path) < 3 {
		return false
	}
	return path[1] == ':' && (path[2] == '\\' || path[2] == '/')
}

// EnsureDir creates a directory and all parent directories if missing.
func EnsureDir(path string) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("create directory %s: %w", path, err)
	}
	return nil
}

// ExpandEnv expands environment variables in a path string.
func ExpandEnv(path string) string {
	return os.ExpandEnv(path)
}

// SameFile compares size, modification time, and permissions of two files.
func SameFile(src, dst string) (bool, error) {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return false, err
	}
	dstInfo, err := os.Stat(dst)
	if err != nil {
		return false, err
	}
	if srcInfo.Size() != dstInfo.Size() {
		return false, nil
	}
	if srcInfo.Mode() != dstInfo.Mode() {
		return false, nil
	}
	return srcInfo.ModTime().Equal(dstInfo.ModTime()), nil
}
