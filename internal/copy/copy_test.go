package copy_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SmonSisay/winprovision/internal/copy"
	"github.com/SmonSisay/winprovision/internal/logging"
)

func TestSyncDirectorySkipsIdenticalFiles(t *testing.T) {
	srcRoot := t.TempDir()
	dstRoot := t.TempDir()

	srcFile := filepath.Join(srcRoot, "app", "setup.exe")
	if err := os.MkdirAll(filepath.Dir(srcFile), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(srcFile, []byte("installer"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	logger := logging.NopLogger{}
	stats, err := copy.SyncDirectory(srcRoot, dstRoot, logger)
	if err != nil {
		t.Fatalf("SyncDirectory() error = %v", err)
	}
	if stats.Copied != 1 {
		t.Fatalf("expected 1 copied file, got %d", stats.Copied)
	}

	stats, err = copy.SyncDirectory(srcRoot, dstRoot, logger)
	if err != nil {
		t.Fatalf("second SyncDirectory() error = %v", err)
	}
	if stats.Skipped != 1 {
		t.Fatalf("expected 1 skipped file, got %d", stats.Skipped)
	}
}

func TestSyncDirectoryCopiesUpdatedFiles(t *testing.T) {
	srcRoot := t.TempDir()
	dstRoot := t.TempDir()

	srcFile := filepath.Join(srcRoot, "readme.txt")
	dstFile := filepath.Join(dstRoot, "readme.txt")
	if err := os.WriteFile(srcFile, []byte("v1"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	logger := logging.NopLogger{}
	if _, err := copy.SyncDirectory(srcRoot, dstRoot, logger); err != nil {
		t.Fatalf("initial sync error = %v", err)
	}

	// Set a distinct mtime on the source file to guarantee the copy detects the
	// change without relying on wall-clock time differences.
	futureTime := time.Now().Add(1 * time.Hour).Truncate(time.Second)
	if err := os.Chtimes(srcFile, futureTime, futureTime); err != nil {
		t.Fatalf("chtimes source: %v", err)
	}
	if err := os.WriteFile(srcFile, []byte("v2"), 0o644); err != nil {
		t.Fatalf("update source: %v", err)
	}

	stats, err := copy.SyncDirectory(srcRoot, dstRoot, logger)
	if err != nil {
		t.Fatalf("second sync error = %v", err)
	}
	if stats.Copied != 1 {
		t.Fatalf("expected updated file to be copied, got copied=%d skipped=%d", stats.Copied, stats.Skipped)
	}

	data, err := os.ReadFile(dstFile)
	if err != nil {
		t.Fatalf("read destination: %v", err)
	}
	if string(data) != "v2" {
		t.Fatalf("destination content = %q, want %q", string(data), "v2")
	}
}
