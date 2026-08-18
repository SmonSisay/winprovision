// Package models defines shared domain types for the provisioning tool.
package models

import "time"

// TaskStatus represents the outcome of a provisioning task.
type TaskStatus string

const (
	TaskStatusSuccess TaskStatus = "SUCCESS"
	TaskStatusSkipped TaskStatus = "SKIPPED"
	TaskStatusFailed  TaskStatus = "FAILED"
)

// TaskResult captures the outcome of a single provisioning task.
type TaskResult struct {
	Name     string
	Module   string
	Status   TaskStatus
	Message  string
	Duration time.Duration
	Err      error
}

// Settings holds application-wide configuration loaded from settings.json.
type Settings struct {
	Destination DestinationSettings `json:"destination"`
	Windows     WindowsSettings     `json:"windows"`
	Logging     LoggingSettings     `json:"logging"`
}

// DestinationSettings controls where software is copied on the target machine.
type DestinationSettings struct {
	PromptIfNoSecondaryDrive bool   `json:"promptIfNoSecondaryDrive"`
	FolderName               string `json:"folderName"`
}

// WindowsSettings controls Windows configuration tasks.
type WindowsSettings struct {
	DisableFirewall       bool   `json:"disableFirewall"`
	EnableRemoteDesktop   bool   `json:"enableRemoteDesktop"`
	EnableAdministrator   bool   `json:"enableAdministrator"`
	AdministratorPassword string `json:"administratorPassword"`
	InstallDotNet35       bool   `json:"installDotNet35"`
	DisableWindowsUpdate  bool   `json:"disableWindowsUpdate"`
	ShowFileExtensions    bool   `json:"showFileExtensions"`
	ShowHiddenFiles       bool   `json:"showHiddenFiles"`
}

// LoggingSettings controls structured log output.
type LoggingSettings struct {
	File  string `json:"file"`
	Level string `json:"level"`
}

// AppsConfig is the root structure for apps.json.
type AppsConfig struct {
	Applications []AppDefinition `json:"applications"`
}

// AppDefinition describes a single application to install.
// When CopyOnly is true the app folder is only copied to the destination
// (e.g. printer drivers or portable tools) and its installer is never run.
// When Deploy is set, the installer is attempted first and, if the
// application is not detected afterwards, the pre-extracted payload under
// Deploy.SourceDir is deployed directly (fonts, COM registration, etc.).
type AppDefinition struct {
	Name            string           `json:"name"`
	InstallerPath   string           `json:"installerPath"`
	SilentArgs      string           `json:"silentArgs"`
	Version         string           `json:"version"`
	AlwaysInstall   bool             `json:"alwaysInstall"`
	CopyOnly        bool             `json:"copyOnly"`
	DesktopShortcut ShortcutConfig   `json:"desktopShortcut"`
	Detection       DetectionRule    `json:"detection"`
	Deploy          *DeployConfig    `json:"deploy,omitempty"`
}

// DeployConfig describes a deterministic, installer-free installation of a
// pre-extracted application payload (used for installers that cannot run
// silently). The payload lives under <app folder>/<SourceDir>.
type DeployConfig struct {
	// SourceDir is the folder inside the app folder holding the payload
	// to copy into InstallDir.
	SourceDir string `json:"sourceDir"`
	// InstallDir is the destination directory on the target machine
	// (supports %VAR% expansion), e.g. C:\Program Files\Power Geez.
	InstallDir string `json:"installDir"`
	// FontsDir, relative to SourceDir, holds *.ttf fonts to register with
	// Windows (copied to C:\Windows\Fonts and registered in the registry).
	FontsDir string `json:"fontsDir,omitempty"`
	// SystemFiles, relative to SourceDir, are copied to both System32 and
	// SysWOW64 and self-registered (COM/OCX/DLL). Omitted when not needed.
	SystemFiles []string `json:"systemFiles,omitempty"`
	// RegisterFiles, relative to InstallDir, are self-registered in place
	// with regsvr32 after the payload is copied.
	RegisterFiles []string `json:"registerFiles,omitempty"`
	// Executable, relative to InstallDir, is verified to exist after deploy.
	Executable string `json:"executable,omitempty"`
}

// ShortcutConfig controls desktop shortcut creation for an application.
type ShortcutConfig struct {
	Enabled    bool   `json:"enabled"`
	Name       string `json:"name"`
	TargetPath string `json:"targetPath"`
}

// DetectionRule defines how to detect whether an application is installed.
type DetectionRule struct {
	Registry       *RegistryDetection `json:"registry,omitempty"`
	ExecutablePath string             `json:"executablePath,omitempty"`
	InstallDir     string             `json:"installDir,omitempty"`
	ProductVersion string             `json:"productVersion,omitempty"`
}

// RegistryDetection defines a registry-based installation check.
type RegistryDetection struct {
	Key       string `json:"key"`
	ValueName string `json:"valueName"`
}

// CopyStats summarizes a directory synchronization operation.
type CopyStats struct {
	Copied  int
	Skipped int
	Failed  int
}

// ExitCode constants for the provisioning tool.
const (
	ExitSuccess        = 0
	ExitTaskFailures   = 1
	ExitFatal          = 2
)
