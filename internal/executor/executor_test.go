package executor_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/SmonSisay/winprovision/internal/executor"
	"github.com/SmonSisay/winprovision/internal/models"
)

func TestRunRequiresAdministrator(t *testing.T) {
	if os.Getenv("CI") == "true" {
		t.Skip("admin integration test is environment-specific")
	}

	exitCode := executor.Run(context.Background(), executor.Options{
		Version: "test",
		Confirm: func() (bool, error) { return false, nil },
	})

	isAdmin := exitCode == models.ExitSuccess || exitCode == models.ExitTaskFailures
	if isAdmin {
		t.Logf("process is elevated; Run() exit code = %d", exitCode)
		return
	}

	if exitCode != models.ExitFatal {
		t.Fatalf("expected exit code %d without admin, got %d", models.ExitFatal, exitCode)
	}
}

func TestConfigFilesPresentInRepo(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	root = filepath.Clean(filepath.Join(root, "..", ".."))

	for _, file := range []string{
		filepath.Join(root, "config", "settings.json"),
		filepath.Join(root, "config", "apps.json"),
	} {
		if _, err := os.Stat(file); err != nil {
			t.Fatalf("expected config file %s: %v", file, err)
		}
	}
}
