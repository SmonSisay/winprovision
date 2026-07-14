// Package config loads and validates provisioning configuration files.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/SmonSisay/winprovision/internal/models"
	"github.com/SmonSisay/winprovision/internal/utils"
)

const (
	settingsFileName = "settings.json"
	appsFileName     = "apps.json"
)

// LoadSettings loads settings.json from the config directory under rootDir.
func LoadSettings(rootDir string) (*models.Settings, error) {
	path := filepath.Join(rootDir, "config", settingsFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read settings file: %w", err)
	}

	var settings models.Settings
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("parse settings file: %w", err)
	}
	if err := validateSettings(&settings); err != nil {
		return nil, err
	}
	return &settings, nil
}

// LoadApps loads apps.json from the config directory under rootDir.
func LoadApps(rootDir string) ([]models.AppDefinition, error) {
	path := filepath.Join(rootDir, "config", appsFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read apps file: %w", err)
	}

	var appsConfig models.AppsConfig
	if err := json.Unmarshal(data, &appsConfig); err != nil {
		return nil, fmt.Errorf("parse apps file: %w", err)
	}
	if len(appsConfig.Applications) == 0 {
		// An empty apps list is valid — the tool may be used for Windows
		// configuration tasks only without installing any applications.
		return []models.AppDefinition{}, nil
	}

	for i := range appsConfig.Applications {
		if err := validateApp(&appsConfig.Applications[i]); err != nil {
			return nil, fmt.Errorf("application %q: %w", appsConfig.Applications[i].Name, err)
		}
	}
	return appsConfig.Applications, nil
}

func validateSettings(settings *models.Settings) error {
	if settings.Destination.FolderName == "" {
		settings.Destination.FolderName = "Software"
	}
	if settings.Logging.File == "" {
		settings.Logging.File = "logs/setup.log"
	}
	if settings.Logging.Level == "" {
		settings.Logging.Level = "info"
	}
	// Administrator password resolution order:
	// 1. ADMIN_PASSWORD environment variable (preferred for production)
	// 2. administratorPassword field in settings.json (development only)
	if settings.Windows.EnableAdministrator {
		if pw := os.Getenv("ADMIN_PASSWORD"); pw != "" {
			settings.Windows.AdministratorPassword = pw
		}
		if settings.Windows.AdministratorPassword == "" {
			return fmt.Errorf(
				"administrator password is required when enableAdministrator is true; " +
					"set the ADMIN_PASSWORD environment variable or the administratorPassword field in settings.json",
			)
		}
	}
	return nil
}

func validateApp(app *models.AppDefinition) error {
	if strings.TrimSpace(app.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if strings.TrimSpace(app.InstallerPath) == "" {
		return fmt.Errorf("installerPath is required")
	}
	if utils.IsAbsoluteWindowsPath(app.InstallerPath) {
		return fmt.Errorf("installerPath must be relative to the software directory")
	}
	// Block path traversal: "../../../Windows/System32/cmd.exe" would pass
	// the absolute-path check but escape the software directory entirely.
	if strings.Contains(filepath.ToSlash(app.InstallerPath), "..") {
		return fmt.Errorf("installerPath must not contain path traversal sequences (..)")
	}
	if app.DesktopShortcut.Enabled {
		if strings.TrimSpace(app.DesktopShortcut.Name) == "" {
			return fmt.Errorf("desktop shortcut name is required when enabled")
		}
		if strings.TrimSpace(app.DesktopShortcut.TargetPath) == "" {
			return fmt.Errorf("desktop shortcut targetPath is required when enabled")
		}
	}
	if app.Detection.Registry == nil &&
		app.Detection.ExecutablePath == "" &&
		app.Detection.InstallDir == "" &&
		app.Detection.ProductVersion == "" {
		return fmt.Errorf("at least one detection rule is required")
	}
	return nil
}
