package utils_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SmonSisay/winprovision/internal/utils"
)

func TestIsAbsoluteWindowsPath(t *testing.T) {
	if !utils.IsAbsoluteWindowsPath(`C:\Windows\System32`) {
		t.Fatal("expected absolute Windows path to be detected")
	}
	if utils.IsAbsoluteWindowsPath(`software\setup.exe`) {
		t.Fatal("expected relative path to be rejected as absolute")
	}
}

func TestSameFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.txt")
	dst := filepath.Join(dir, "dest.txt")

	content := []byte("provisioning")
	if err := os.WriteFile(src, content, 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := os.WriteFile(dst, content, 0o644); err != nil {
		t.Fatalf("write dest: %v", err)
	}

	modTime := time.Now().Add(-2 * time.Hour).Truncate(time.Second)
	if err := os.Chtimes(src, modTime, modTime); err != nil {
		t.Fatalf("chtimes source: %v", err)
	}
	if err := os.Chtimes(dst, modTime, modTime); err != nil {
		t.Fatalf("chtimes dest: %v", err)
	}

	same, err := utils.SameFile(src, dst)
	if err != nil {
		t.Fatalf("SameFile() error = %v", err)
	}
	if !same {
		t.Fatal("expected files to be considered identical")
	}
}

func TestResolveSoftwareDestination(t *testing.T) {
	got := utils.ResolveSoftwareDestination(`D:\`, "Software")
	want := filepath.Join(`D:\`, "Software")
	if got != want {
		t.Fatalf("ResolveSoftwareDestination() = %q, want %q", got, want)
	}
}

func TestEnsureDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "logs")
	if err := utils.EnsureDir(dir); err != nil {
		t.Fatalf("EnsureDir() error = %v", err)
	}
	if !utils.DirExists(dir) {
		t.Fatal("expected directory to exist")
	}
}
