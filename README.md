# winprovision

A self-contained Windows 11 provisioning tool. Drop it on a USB drive with your installers, plug it into a fresh machine, run `Setup.exe` as Administrator — done.

Copies software silently, configures Windows settings, installs applications, and creates desktop shortcuts — all in one shot with zero network dependency.

## What it does

1. **Copies software** from the USB to the largest non-system fixed volume (or a folder the operator types)
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
│   ├── CopyOnly/             ← copied as-is, never installed (printers/portable)
│   │   └── ...               ← any folder to just copy
│   └── ...                   ← drop any installer folder here
├── sources/
│   └── sxs/                  ← .NET 3.5 payload (copy NetFx3.cab here to be self-contained)
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
| `attendedFallback` | bool | No | If silent install fails, relaunch the installer without silent flags so the wizard UI appears for manual completion |
| `attendedOnly` | bool | No | Skip the silent attempt entirely and go straight to the wizard (for installers that reject every silent flag, e.g. Power Geez) |
| `copyOnly` | bool | No | Copy the app folder to the destination but never run its installers (e.g. printer drivers or portable tools) |
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
4. Detect the destination volume (see [Destination resolution](#destination-resolution)) or prompt for a folder
5. Verify the destination has enough free space for the payload — exits with code 2 if not
6. Build task plan from configuration
7. Display banner, destination, and action summary
8. Prompt for user confirmation
9. Execute tasks in order:
   - Copy `software/` to `<Destination>\Software` (idempotent sync)
   - Apply Windows configuration (firewall, RDP, admin, explorer settings)
   - Enable .NET Framework 3.5 (uses its own `sources\sxs` first, then a bootable Windows drive, then prompts)
   - Install each configured application
   - Create desktop shortcuts
   - Auto-discover and run unlisted installers
10. Print final provisioning summary
11. Write structured logs to `logs/setup.log`

### Destination resolution

`Setup.exe` **never creates, resizes, formats, or deletes partitions.** The disk
layout is laid down once during OS installation by `autounattend.xml`, which
creates a fixed-size `C:` plus a data volume taking the remainder of the disk.
By the time provisioning starts, the layout already exists.

The destination is then chosen at runtime, in this order:

1. **The largest fixed volume that is not the system drive.** Selected by size
   rather than by a hardcoded letter, because the letter the data volume
   receives during install depends on what else is attached — it may be `D:`,
   `E:`, or `F:`.
2. **A folder typed by the operator**, when no suitable volume is found and
   `destination.promptIfNoSecondaryDrive` is `true`.

Volumes are filtered before ranking:

| Filter | Why |
|--------|-----|
| `DriveType -eq 'Fixed'` | Excludes the removable USB that `Setup.exe` is running from — the payload must never be copied back onto its own source media |
| Drive letter present | The payload is addressed by drive letter |
| `FileSystem` non-empty | Excludes unformatted OEM/recovery volumes that would otherwise be candidates |

Both paths return a **root** (`D:\`), never the finished software folder.
`destination.folderName` is appended exactly once, by
`ResolveSoftwareDestination`, so the result is always `<root>\<folderName>` —
for example `D:\Softwares`, never `D:\Softwares\Softwares`.

### Destination space requirement

Before anything is copied, the tool measures `software/` and compares it with
the free space on the destination volume.

- **Required** = payload size + 512 MiB headroom, so the volume is not left
  completely full after the copy
- If free space is short, provisioning stops immediately with a report showing
  the destination, the required size, the available size, and the shortfall.
  Sizes use binary (IEC) units — `MiB`/`GiB`, not `MB`/`GB` — and the exact byte
  counts are printed alongside, because a rounded `4.5 GiB` vs `4.5 GiB` can
  otherwise hide a one-byte shortfall
- If free space **cannot be measured** (network or virtual volume) or the
  payload cannot be measured (missing `software/`), the tool warns and
  continues — a check that cannot be performed must not block a machine that
  would otherwise provision fine

The check runs before the confirmation prompt, so an undersized destination is
reported before the operator commits to anything, and never leaves a
half-provisioned machine behind.

### .NET Framework 3.5 source

When `windows.installDotNet35` is `true`, the tool locates the .NET 3.5 payload
(`NetFx3.cab`, i.e. `microsoft-windows-netfx3-ondemand-package*.cab`) in this order:

1. **The tool's own `sources\sxs` folder** — checked first, so a prepared USB
   works on any machine with no second flash attached
2. A bootable Windows drive plugged into the machine (auto-detected)
3. A path typed by the operator when prompted

A folder only counts as a valid source if it exists and holds at least one
`.cab` file; an empty `sources\sxs` is skipped and the search falls through to
the next option, so leaving the directory in place but forgetting the payload
degrades to auto-detection rather than failing.

To make the USB self-sufficient, copy the cab from a Windows installation media into
`<USB>\sources\sxs` once:

```
sources/sxs/microsoft-windows-netfx3-ondemand-package~31bf3856ad364e35~amd64~~.cab
```

The cab is ~68 MB and is gitignored, so it must be copied to each USB by hand.

## Exit codes

| Code | Meaning |
|------|---------|
| `0` | Success (skipped tasks are allowed) |
| `1` | One or more tasks failed |
| `2` | Fatal startup error (not admin, missing config, insufficient destination space, etc.) |

## Safety guarantees

- **Panic recovery**: Any task that panics is caught, logged, and recorded as failed — execution continues to the next task
- **Idempotent**: Re-running skips tasks that are already complete (detected via registry, file existence, etc.)
- **No disk writes**: `Setup.exe` cannot create, resize, format, or delete partitions — the layout is created by `autounattend.xml` during OS installation
- **Preflight space check**: provisioning stops before starting if the destination cannot hold the payload
- **No hardcoded paths**: All paths are resolved relative to the executable
- **No hardcoded passwords**: Use `ADMIN_PASSWORD` environment variable
- **Password security**: Administrator password is set via Win32 API, not command-line arguments
- **Path traversal protection**: `installerPath` in `apps.json` rejects `..` sequences and absolute paths

## Development

```bash
# Build for Windows
make build

# Vet
make vet

# Lint (requires staticcheck)
make lint

# Remove build output
make clean
```

This project ships without a Go test suite. `make test` is retained for
compatibility but currently has no test files to run. Verification is done by
running the built `Setup.exe` against the manual [testing checklist](#testing-checklist).

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
- [ ] Test the folder prompt with a bare drive (`D:\`) and with a folder (`D:\Work\`) — confirm the destination is always `<root>\<folderName>` and never duplicated
- [ ] Confirm `Setup.exe` leaves the partition table untouched — capture `Get-Partition` before and after a full run and diff
- [ ] Point the destination at a volume too small for the payload — confirm the shortfall report appears, the tool exits 2, and nothing was copied
- [ ] Confirm the destination is the **largest** non-system fixed volume, not merely the first one the OS happens to report
- [ ] Verify `.NET Framework 3.5` installs from `sources\sxs` when not already enabled
- [ ] Verify `logs/setup.log` contains structured entries with timestamp, module, action, duration, status
- [ ] Test panic recovery by providing a broken installer path — confirm tool continues

## Notes

- This tool targets **Windows 11** (English locale for netsh output fallback)
- Installers and `sources\sxs` payloads are not bundled in the repository
- Some antivirus products may block silent installers — review `logs/setup.log` for installer exit codes
- The tool is designed for personal/organizational provisioning use
