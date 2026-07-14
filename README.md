# winprovision

A self-contained Windows 11 provisioning tool. Drop it on a USB drive with your installers, plug it into a fresh machine, run `Setup.exe` as Administrator — done.

Copies software silently, configures Windows settings, installs applications, and creates desktop shortcuts — all in one shot with zero network dependency.

## What it does

1. **Copies software** from the USB to a secondary drive (or a user-specified folder)
2. **Configures Windows** — disables firewall, enables RDP, activates built-in Administrator, shows file extensions, shows hidden files
3. **Installs .NET Framework 3.5** from local `sources\sxs` when needed
4. **Installs applications** using their silent installer arguments
5. **Creates desktop shortcuts** for each configured application
6. **Auto-discovers** any extra folders in `software/` not listed in `apps.json`
7. **Idempotent** — re-run skips tasks that are already complete

## Deployment layout

```
USB_ROOT/
├── Setup.exe                 ← the provisioning tool
├── config/
│   ├── settings.json         ← Windows configuration (firewall, RDP, admin, etc.)
│   └── apps.json             ← applications to install
├── software/
│   ├── Chrome/
│   │   └── setup.exe         ← Chrome offline installer
│   ├── Firefox/
│   │   └── FirefoxSetup.exe
│   ├── MicrosoftOffice/
│   │   └── setup.exe
│   └── ...                   ← drop any installer folder here
├── sources/
│   └── sxs/                  ← .NET 3.5 source files (optional)
├── assets/                   ← optional assets
└── logs/                     ← created at runtime
```

All paths are resolved relative to the executable location. No drive letters are hardcoded.

## Quick start

### Build

Requirements: Go 1.24+

```bash
# Linux/macOS cross-compile
make build

# Or directly
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w -X main.version=1.0.0" -o Setup.exe ./cmd/setup
```

### Configure

Edit `config/settings.json`:

```json
{
  "destination": {
    "promptIfNoSecondaryDrive": true,
    "folderName": "Software"
  },
  "windows": {
    "disableFirewall": true,
    "enableRemoteDesktop": true,
    "enableAdministrator": true,
    "administratorPassword": "",
    "installDotNet35": true,
    "showFileExtensions": false,
    "showHiddenFiles": false
  },
  "logging": {
    "file": "logs/setup.log",
    "level": "info"
  }
}
```

Edit `config/apps.json` to define your applications:

```json
{
  "applications": [
    {
      "name": "Google Chrome",
      "installerPath": "Chrome/setup.exe",
      "silentArgs": "/silent /install",
      "version": "latest",
      "desktopShortcut": {
        "enabled": true,
        "name": "Google Chrome",
        "targetPath": "C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe"
      },
      "detection": {
        "registry": {
          "key": "HKLM\\SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\Google Chrome",
          "valueName": "DisplayName"
        },
        "executablePath": "C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe"
      }
    }
  ]
}
```

### Run

```
Setup.exe
```

Must be run as Administrator. The tool will:

1. Show a banner with version, Windows version, and logged-in user
2. Detect the destination drive or prompt for a folder
3. Display a summary of planned actions
4. Ask for confirmation
5. Execute each task, showing progress and status
6. Print a final provisioning summary

## Configuration reference

### settings.json

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `destination.promptIfNoSecondaryDrive` | bool | `true` | Prompt user if no secondary drive is found |
| `destination.folderName` | string | `"Software"` | Subfolder name on the destination drive |
| `windows.disableFirewall` | bool | `false` | Disable Windows Firewall for all profiles |
| `windows.enableRemoteDesktop` | bool | `false` | Enable Remote Desktop Protocol |
| `windows.enableAdministrator` | bool | `false` | Enable built-in Administrator account |
| `windows.administratorPassword` | string | `""` | Password for Administrator account |
| `windows.installDotNet35` | bool | `false` | Install .NET Framework 3.5 from local sources |
| `windows.showFileExtensions` | bool | `false` | Show file extensions in Explorer |
| `windows.showHiddenFiles` | bool | `false` | Show hidden files in Explorer |
| `logging.file` | string | `"logs/setup.log"` | Log file path (relative to exe) |
| `logging.level` | string | `"info"` | Log level: `debug`, `info`, `warn`, `error` |

### Administrator password

The administrator password is resolved in this order:

1. `ADMIN_PASSWORD` environment variable (preferred for production)
2. `administratorPassword` field in `settings.json` (development only)

**Never commit real passwords to the repository.** Use the environment variable in production.

### apps.json

Each application entry supports:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Display name for the application |
| `installerPath` | string | Yes | Path to installer (relative to `software/` folder) |
| `silentArgs` | string | Yes | Silent install arguments |
| `version` | string | Yes | Version string (used for display only) |
| `desktopShortcut.enabled` | bool | Yes | Whether to create a desktop shortcut |
| `desktopShortcut.name` | string | When enabled | Shortcut name (without `.lnk` extension) |
| `desktopShortcut.targetPath` | string | When enabled | Full path to the shortcut target |
| `detection.registry.key` | string | No | Registry key to check for existing installation |
| `detection.registry.valueName` | string | No | Registry value name to read |
| `detection.executablePath` | string | No | Path to the installed executable |
| `detection.installDir` | string | No | Path to the installation directory |
| `detection.productVersion` | string | No | Expected product version for version-based detection |

At least one detection rule is required per application.

### Detection methods

The tool checks if an application is already installed before running the installer. Multiple detection methods can be combined — if any method succeeds, the application is considered installed and skipped.

- **Registry**: Checks if a specific registry key/value exists
- **Executable**: Checks if a known executable file exists on disk
- **Install directory**: Checks if a known directory exists
- **Product version**: Compares a registry value against an expected version string

## Runtime behavior

1. Verify Administrator privileges (exit code 2 if not elevated)
2. Load `config/settings.json` and `config/apps.json`
3. Initialize structured file logger
4. Detect destination drive (`D:`, `E:`, etc.) or prompt for a folder
5. Build task plan from configuration
6. Display banner, destination, and action summary
7. Prompt for user confirmation
8. Execute tasks in order:
   - Copy `software/` to `<Destination>\Software` (idempotent sync)
   - Apply Windows configuration (firewall, RDP, admin, explorer settings)
   - Enable .NET Framework 3.5 from `sources\sxs`
   - Install each configured application
   - Create desktop shortcuts
   - Auto-discover and run unlisted installers
9. Print final provisioning summary
10. Write structured logs to `logs/setup.log`

## Exit codes

| Code | Meaning |
|------|---------|
| `0` | Success (skipped tasks are allowed) |
| `1` | One or more tasks failed |
| `2` | Fatal startup error (not admin, missing config, etc.) |

## Safety guarantees

- **Panic recovery**: Any task that panics is caught, logged, and recorded as failed — execution continues to the next task
- **Idempotent**: Re-running skips tasks that are already complete (detected via registry, file existence, etc.)
- **No hardcoded paths**: All paths are resolved relative to the executable
- **No hardcoded passwords**: Use `ADMIN_PASSWORD` environment variable
- **Password security**: Administrator password is set via Win32 API, not command-line arguments
- **Path traversal protection**: `installerPath` in `apps.json` rejects `..` sequences and absolute paths

## Development

```bash
# Run tests
go test ./...

# Run tests + vet
make check

# Build for Windows
make build

# Lint (requires staticcheck)
make lint
```

### Project structure

```
cmd/setup/              Entry point
internal/config/        JSON loading and validation
internal/copy/          Idempotent directory synchronization
internal/dism/          .NET Framework 3.5 via DISM
internal/executor/      Workflow orchestration
internal/installer/     App detection, installation, and auto-discovery
internal/logging/       Structured file logging with level filtering
internal/models/        Shared domain types (TaskResult, Settings, AppDefinition)
internal/progress/      Console UI, progress display, and final report
internal/registry/      Windows registry helpers
internal/shortcut/      Desktop shortcut creation via COM
internal/utils/         Path, admin, drive, and OS version helpers
internal/windows/       Firewall, RDP, Administrator, Explorer configuration
```

## Testing checklist

- [ ] Run without Administrator privileges — confirm exit code 2
- [ ] Run on a fresh VM with a secondary drive — confirm software copy + installs
- [ ] Run a second time — confirm most tasks show `SKIPPED`
- [ ] Remove one installer from `software/` — confirm provisioning continues with error in summary
- [ ] Test on a machine with no secondary drive — confirm folder prompt works
- [ ] Verify `.NET Framework 3.5` installs from `sources\sxs` when not already enabled
- [ ] Verify `logs/setup.log` contains structured entries with timestamp, module, action, duration, status
- [ ] Test panic recovery by providing a broken installer path — confirm tool continues

## Notes

- This tool targets **Windows 11** (English locale for netsh output fallback)
- Installers and `sources\sxs` payloads are not bundled in the repository
- Some antivirus products may block silent installers — review `logs/setup.log` for installer exit codes
- The tool is designed for personal/organizational provisioning use
