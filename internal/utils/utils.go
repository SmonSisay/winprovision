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

// PromptDestinationFolder asks the user to enter a destination folder path.
// It displays a formatted guide with examples to help the user.
func PromptDestinationFolder(folderName string) (string, error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println()
	fmt.Println("  ─── Destination Folder ───")
	fmt.Println()
	fmt.Println("  Where should the software files be copied to?")
	fmt.Println()
	fmt.Println("  Examples:")
	fmt.Printf("    D:\\%s\n", folderName)
	fmt.Printf("    E:\\%s\n", folderName)
	fmt.Printf("    D:\\Work\\%s\n", folderName)
	fmt.Println()
	fmt.Print("  Enter full path: ")

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
		return "", fmt.Errorf("please enter a full path (e.g. D:\\%s)", folderName)
	}

	if !DirExists(filepath.Dir(path)) {
		return "", fmt.Errorf("parent directory does not exist: %s", filepath.Dir(path))
	}

	return path, nil
}

// PromptBootableDrive asks the user to enter the bootable flash drive letter or full path.
func PromptBootableDrive() (string, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Bootable flash not detected. Enter drive letter (e.g. D:) or full path to sources\\sxs: ")
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
