package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/SmonSisay/winprovision/internal/config"
)

func TestLoadSettings(t *testing.T) {
	t.Setenv("ADMIN_PASSWORD", "test-password")
	root := filepath.Join("..", "..")
	settings, err := config.LoadSettings(root)
	if err != nil {
		t.Fatalf("LoadSettings() error = %v", err)
	}
	if settings.Destination.FolderName == "" {
		t.Fatal("expected destination folder name")
	}
	if !settings.Windows.DisableFirewall {
		t.Fatal("expected disableFirewall to be enabled in template")
	}
	if settings.Windows.AdministratorPassword != "test-password" {
		t.Fatalf("expected admin password from env var, got %q", settings.Windows.AdministratorPassword)
	}
}

func TestLoadSettingsRequiresAdminPassword(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	settingsJSON := `{
		"destination": { "promptIfNoSecondaryDrive": false, "folderName": "Software" },
		"windows": { "enableAdministrator": true, "administratorPassword": "" },
		"logging": { "file": "logs/setup.log", "level": "info" }
	}`
	if err := os.WriteFile(filepath.Join(configDir, "settings.json"), []byte(settingsJSON), 0o644); err != nil {
		t.Fatalf("write settings.json: %v", err)
	}

	_, err := config.LoadSettings(dir)
	if err == nil {
		t.Fatal("expected error when admin password is missing and no env var set")
	}
}

func TestLoadApps(t *testing.T) {
	root := filepath.Join("..", "..")
	apps, err := config.LoadApps(root)
	if err != nil {
		t.Fatalf("LoadApps() error = %v", err)
	}
	if len(apps) == 0 {
		t.Fatal("expected applications in template apps.json")
	}
}

func TestValidateAppRejectsAbsoluteInstallerPath(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	appsJSON := `{
		"applications": [
			{
				"name": "Bad App",
				"installerPath": "C:\\bad\\setup.exe",
				"silentArgs": "/S",
				"version": "1.0.0",
				"desktopShortcut": { "enabled": false },
				"detection": { "installDir": "C:\\Bad" }
			}
		]
	}`
	if err := os.WriteFile(filepath.Join(configDir, "apps.json"), []byte(appsJSON), 0o644); err != nil {
		t.Fatalf("write apps.json: %v", err)
	}

	_, err := config.LoadApps(dir)
	if err == nil {
		t.Fatal("expected validation error for absolute installer path")
	}
}
