# WinProvision — IT Officer & System Administrator User Guide

This comprehensive manual explains how to configure, deploy, and troubleshoot the **WinProvision** self-contained Windows 11 provisioning tool. It is designed for IT Officers, Systems Administrators, and Field Technicians responsible for mass workstation setup.

---

## 1. Executive Summary & Purpose

**WinProvision** is an automated, offline Windows 11 setup utility (`Setup.exe`). It eliminates manual software installation and system configuration by running directly from removable USB media.

### Key Capabilities

- **Offline Execution:** Operates with zero network dependency. All installers and system components (.NET Framework 3.5) are hosted directly on the USB drive.
- **Idempotent Operations:** Built-in application detection (Registry keys, executable paths, install directories, product versions) prevents duplicate installations. Re-running the tool safely skips completed tasks.
- **System Hardening & Configuration:** Automates Windows Firewall state, Remote Desktop Protocol (RDP), local Administrator account enablement, and File Explorer preferences.
- **Automated & Attended Fallbacks:** Supports fully silent background installations as well as automatic attended fallbacks for legacy installers requiring GUI wizard interaction.
- **Structured Logging & Diagnostics:** Generates machine-readable, time-stamped log reports for operational auditing.

---

## 2. Provisioning Time Estimations & Benchmarks

Using WinProvision dramatically reduces the time required to provision new workstations.

### Task-by-Task Execution Breakdown

| Provisioning Task                                                | Estimated Time (USB 3.0 / SSD) | Factors & Performance Notes                                |
| :--------------------------------------------------------------- | :----------------------------: | :--------------------------------------------------------- |
| **Startup & Elevation Check**                                    |          ~2–5 seconds          | Validates UAC elevation and loads JSON config files.       |
| **Software Directory Sync (`software/` $\rightarrow$ Local PC)** |         ~30–90 seconds         | Depends on total size of installers and USB read speed.    |
| **Windows OS Configuration (Firewall, RDP, Explorer)**           |         ~5–10 seconds          | Executes netsh and registry modifications instantly.       |
| **.NET Framework 3.5 Installation (DISM)**                       |         ~45–90 seconds         | Installs `.cab` package from local `sources/sxs/` offline. |
| **Standard App Installations (Chrome, Firefox, 7-Zip)**          |     ~15–30 seconds per app     | Silent background execution.                               |
| **Large App Packages (Microsoft Office, Kaspersky)**             |      ~2–4 minutes per app      | Large binary decompression and registration.               |
| **Desktop Shortcut Creation**                                    |          ~2–5 seconds          | COM shortcut creation for configured apps.                 |

### Performance Overview

Overall provisioning performance depends on the number and size of the software packages being copied and executed (e.g. large Office suites or endpoint security packages take longer than lightweight utilities). However, even with multiple heavy applications, WinProvision executes super fast compared to manual workstation setup, dramatically cutting deployment time. Additionally, re-running the tool on an already provisioned machine completes in seconds thanks to intelligent detection skipping.

> [!TIP]
> **Performance Tip:** Using a **USB 3.0 or USB 3.2 Flash Drive** plugged into a USB 3.0 port (blue port) on the PC significantly accelerates software file copying and DISM installation times compared to older USB 2.0 drives.

---

## 3. USB Directory Architecture & Layout

To ensure seamless execution, the USB drive payload must adhere to the following standard directory structure:

```
USB_ROOT (e.g., E:\ or F:\)
├── Setup.exe                  ← Primary provisioning executable (Run as Administrator)
│
├── config/                    ← Central configuration control
│   ├── settings.json          ← Windows OS configuration, logging preferences, target drive rules
│   └── apps.json              ← Master application catalog, detection rules, silent flags
│
├── software/                  ← Application installers & software packages (One subfolder per app)
│   ├── Chrome/                ← Google Chrome installer package
│   │   └── ChromeSetup.exe
│   ├── Firefox/               ← Mozilla Firefox installer package
│   │   └── FirefoxSetup.exe
│   ├── MicrosoftOffice/       ← Microsoft Office deployment folder
│   │   └── setup.exe
│   ├── AdobeReader/           ← Adobe Acrobat Reader installer
│   ├── Skype4Business/        ← Skype for Business installer
│   ├── VisualGeez/            ← Legacy Ethiopic font installer package
│   ├── PowerGeez/             ← Power Geez font installer package
│   ├── Kaspersky/             ← Kaspersky Endpoint Security installer package
│   ├── FortiNAC/              ← FortiNAC persistent agent installer
│   └── CopyOnly/              ← Static directory for non-executable payloads (e.g., printer drivers)
│
├── sources/
│   └── sxs/                   ← Local .NET Framework 3.5 payload (NetFx3.cab)
│
├── assets/                    ← Optional deployment resources (wallpapers, scripts, icons)
│
└── logs/                      ← Created automatically at runtime
    └── setup.log              ← Operational execution log report
```

### Folder Breakdown & Usage Rules

| Directory            | Purpose                                                     | Administrative Rules                                                             |
| :------------------- | :---------------------------------------------------------- | :------------------------------------------------------------------------------- |
| `config/`            | Stores JSON configuration files.                            | Must contain valid JSON syntax. Back up files before editing.                    |
| `software/`          | Contains installer subdirectories.                          | Each app gets its own subfolder. Unlisted folders are subject to auto-discovery. |
| `software/CopyOnly/` | Stores static files/drivers to be copied without execution. | Used for printer drivers, portable tools, and static documentation.              |
| `sources/sxs/`       | Holds the `.cab` package for .NET 3.5 offline installation. | Ensure `microsoft-windows-netfx3-ondemand-package*.cab` is present.              |
| `logs/`              | Runtime log directory.                                      | Automatically generated; inspect `setup.log` for troubleshooting.                |

---

## 4. Operational Workflow (Step-by-Step Deployment)

### Step 1: USB Insertion & Elevation Verification

1. Insert the prepared WinProvision USB drive into the target Windows 11 machine.
2. Log into an account with **Administrator privileges**.
3. Open File Explorer, navigate to the USB root directory (e.g., `E:\`), right-click `Setup.exe`, and select **Run as administrator**.

> [!IMPORTANT]
> WinProvision strictly requires UAC elevation. If launched without Administrator privileges, execution immediately halts with **Exit Code 2**.

### Step 2: System Banner & Target Drive Selection

Upon startup, the console interface displays:

- Utility version and build metadata.
- Detected Windows OS version and active user identity.
- Target installation path (Secondary drive e.g., `D:\Softwares`, or user-prompted custom folder if no secondary drive exists).
- Execution plan summary detailing planned OS tweaks and app installations.

### Step 3: Execution Confirmation

Review the displayed task plan. Prompt response:

- Press **`Y`** and Enter to initiate execution.
- Press **`N`** to safely abort execution without making changes.

### Step 4: Monitoring Progress & Task Statuses

As tasks execute, real-time status indicators report progress:

| Status Code   | Meaning                                              | Technical Action Taken                                           |
| :------------ | :--------------------------------------------------- | :--------------------------------------------------------------- |
| **`SUCCESS`** | Task completed successfully.                         | Setting applied or installer finished with return code `0`.      |
| **`SKIPPED`** | Pre-check passed; item already configured/installed. | Detection rule matched; installer execution avoided.             |
| **`WARNING`** | Task completed with non-fatal issue.                 | Non-critical step encountered a minor fallback condition.        |
| **`FAILED`**  | Task encountered an error and could not complete.    | Caught by panic/error recovery; execution proceeds to next task. |

---

## 5. Configuration Deep-Dive (`config/settings.json`)

`config/settings.json` controls system-level settings, destination paths, and logging levels.

### Full Schema Reference & Types

```json
{
  "destination": {
    "promptIfNoSecondaryDrive": true,
    "folderName": "Softwares"
  },
  "windows": {
    "disableFirewall": true,
    "enableRemoteDesktop": true,
    "enableAdministrator": true,
    "administratorPassword": "",
    "installDotNet35": true,
    "showFileExtensions": true,
    "showHiddenFiles": true
  },
  "logging": {
    "file": "logs/setup.log",
    "level": "info"
  }
}
```

### What IT Officers Can Configure

1. **Software Destination (`destination`):**
   - `folderName`: Sets the subfolder name on the target drive where installer source files are backed up (default: `"Softwares"`).
   - `promptIfNoSecondaryDrive`: When set to `true`, if the target PC has only a single drive (`C:`), WinProvision prompts the technician to choose or confirm a destination folder.

2. **Windows System & Security Policies (`windows`):**
   - `disableFirewall`: Set to `true` to disable Windows Firewall across Domain, Private, and Public profiles for uninhibited network setup.
   - `enableRemoteDesktop`: Set to `true` to enable Remote Desktop Protocol (RDP) and open port 3389 in Windows Firewall.
   - `enableAdministrator`: Set to `true` to activate the built-in local Administrator account.
   - `installDotNet35`: Set to `true` to install .NET Framework 3.5 offline via DISM using `sources/sxs/`.
   - `showFileExtensions`: Set to `true` to configure File Explorer to show file extensions (e.g., `.exe`, `.json`).
   - `showHiddenFiles`: Set to `true` to reveal hidden system files and directories in File Explorer.

---

## 6. Administrator Password Management & Security

IT Officers can configure the password for the built-in local Administrator account using one of two methods:

### Method 1: Environment Variable (Recommended for Enterprise / Production)

To prevent storing sensitive plain-text passwords on a shared USB drive, set the `ADMIN_PASSWORD` environment variable in PowerShell or Command Prompt before running `Setup.exe`:

**PowerShell (Run as Administrator):**

```powershell
$env:ADMIN_PASSWORD="YourSecurePassword123!"
.\Setup.exe
```

**Command Prompt (Run as Administrator):**

```cmd
set ADMIN_PASSWORD=YourSecurePassword123!
Setup.exe
```

### Method 2: Configuration File (`settings.json`) (Development / Lab Use Only)

Set the password in `config/settings.json`:

```json
"windows": {
  "enableAdministrator": true,
  "administratorPassword": "YourSecurePassword123!"
}
```

> [!SECURITY]
> **Resolution Priority:** WinProvision checks `ADMIN_PASSWORD` environment variable first. If set, it overrides any password in `settings.json`.
> **Security Protection:** WinProvision sets the Administrator password via native Win32 APIs (`NetUserSetInfo`), preventing password strings from appearing in system process lists or command-line logs.

---

## 7. Application Catalog Management (`config/apps.json`)

`config/apps.json` contains the master array of application deployment definitions.

### Master Application Entry Schema

```json
{
  "applications": [
    {
      "name": "Google Chrome",
      "installerPath": "Chrome/ChromeStandaloneSetup64.exe",
      "silentArgs": "/silent /install",
      "version": "latest",
      "attendedFallback": false,
      "copyOnly": false,
      "desktopShortcut": {
        "enabled": false
      },
      "detection": {
        "executablePath": "C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe"
      }
    }
  ]
}
```

### Parameter Reference & Rules

| Field                        | Type      | Required    | Description                                                                        |
| :--------------------------- | :-------- | :---------- | :--------------------------------------------------------------------------------- |
| `name`                       | `string`  | **Yes**     | Display name shown in execution logs and console progress.                         |
| `installerPath`              | `string`  | **Yes**     | Path to installer executable **relative to `software/` folder**.                   |
| `silentArgs`                 | `string`  | **Yes**     | Silent command-line flags (e.g., `/S`, `/silent`, `/qn`, `/VERYSILENT`).           |
| `version`                    | `string`  | **Yes**     | Display version string for audit logs.                                             |
| `attendedFallback`           | `boolean` | No          | If `true`, pops up installer GUI if silent install fails or returns non-zero code. |
| `copyOnly`                   | `boolean` | No          | If `true`, copies files to target drive without executing installer.               |
| `desktopShortcut.enabled`    | `boolean` | **Yes**     | Controls whether WinProvision generates a desktop shortcut.                        |
| `desktopShortcut.name`       | `string`  | Conditional | Name of shortcut icon (omitting `.lnk`). Required if `enabled` is `true`.          |
| `desktopShortcut.targetPath` | `string`  | Conditional | Full absolute path to target executable on local system drive (`C:\...`).          |
| `detection`                  | `object`  | **Yes**     | Object containing at least one valid detection check method.                       |

---

## 8. Understanding Desktop Shortcuts & The Chrome Case Study

### Why is Chrome's `desktopShortcut.enabled` set to `false`?

> [!NOTE]
> **Key Insight:** Standard offline installers like Google Chrome (`ChromeStandaloneSetup64.exe`), Mozilla Firefox, and Adobe Acrobat Reader **natively create their own Desktop shortcut** during their silent installation process.
>
> If WinProvision's `desktopShortcut.enabled` were set to `true` for Google Chrome, WinProvision would attempt to create a second shortcut after installation finishes. This results in **duplicate shortcut icons** on the user's desktop (e.g., `Google Chrome.lnk` and `Google Chrome (1).lnk`).

### Rule of Thumb for IT Officers:

- **Set `"enabled": false`** when the software installer automatically creates a desktop icon during installation (e.g., Chrome, Firefox, VLC, Adobe Reader).
- **Set `"enabled": true`** when deploying custom tools, portable applications, or silent installers that do **not** automatically create a desktop shortcut.

---

## 9. Detection Strategies (Preventing Re-Installations)

WinProvision checks detection rules before running any installer. If **any** defined rule returns `true`, the installer is skipped.

### Supported Detection Types

#### 1. Executable Path Detection (Most Common)

Checks if the target binary exists on the system drive:

```json
"detection": {
  "executablePath": "C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe"
}
```

#### 2. Installation Directory Detection

Checks if the application root folder exists:

```json
"detection": {
  "installDir": "C:\\Program Files\\Power Geez"
}
```

#### 3. Windows Registry Key Detection

Checks for existence of registry key and value (HKLM or HKCU):

```json
"detection": {
  "registry": {
    "key": "HKLM\\SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\{CE2CDD12-0124-4B7E-8D4B-EDD67B276E0E}",
    "valueName": "DisplayName"
  }
}
```

#### 4. Product Version Check

Validates registry version string against minimum required version:

```json
"detection": {
  "productVersion": "12.3.0.495"
}
```

> [!CAUTION]
> In JSON, all Windows backslashes must be escaped with a double backslash (`\\`). For example: `C:\\Program Files\\App\\app.exe`. Single backslashes break JSON parsing.

---

## 10. Common IT Officer Playbooks & Recipes

### Scenario A: Adding a New Application (e.g., 7-Zip)

1. Create a folder `software/7Zip/` and place `7z2301-x64.exe` inside it.
2. Open `config/apps.json` and append the following object to `"applications"`:

```json
{
  "name": "7-Zip",
  "installerPath": "7Zip/7z2301-x64.exe",
  "silentArgs": "/S",
  "version": "23.01",
  "desktopShortcut": {
    "enabled": false
  },
  "detection": {
    "executablePath": "C:\\Program Files\\7-Zip\\7zFM.exe"
  }
}
```

3. Save the file and run `Setup.exe`.

### Scenario B: Configuring Attended Fallback for Legacy Packages (e.g., Ethiopic Geez Fonts)

Certain legacy packages (e.g., Visual Geez, Power Geez) do not support headless silent installation.

1. Set `"attendedFallback": true` in `apps.json`.
2. When WinProvision executes, it attempts silent execution (`/s`). If unattended setup fails, WinProvision automatically relaunches the installer without silent switches, bringing up the GUI setup wizard.
3. The field technician clicks through the GUI wizard (**Next $\rightarrow$ Next $\rightarrow$ Finish**). Once closed, WinProvision resumes automatic provisioning.

### Scenario C: Deploying Non-Executable Payloads (Printer Drivers, Portable Tools)

1. Place driver files inside `USB_ROOT/software/CopyOnly/` (or create a dedicated folder like `software/PrinterDrivers/`).
2. Add a `copyOnly` entry to `apps.json`:

```json
{
  "name": "Printer Drivers Package",
  "installerPath": "CopyOnly/",
  "copyOnly": true,
  "desktopShortcut": {
    "enabled": false
  }
}
```

3. WinProvision syncs the directory to `<TargetDrive>\Softwares\CopyOnly\` without running any setup executables.

---

## 11. Log Analysis & Diagnostics (`logs/setup.log`)

WinProvision generates structured line-formatted log files. Every operational step is logged with time, module, action, duration, status, and detail message.

### Sample Log Entry Structure

```
timestamp=2026-09-03T00:00:00+03:00 level=info module=installer action="Google Chrome" duration=14s status=SUCCESS message="Application installed successfully"
timestamp=2026-09-03T00:00:14+03:00 level=info module=installer action="Power Geez" duration=32s status=WARNING message="Silent install failed; attended fallback completed by operator"
timestamp=2026-09-03T00:00:46+03:00 level=error module=dism action="DotNet35" duration=5s status=FAILED message="Payload NetFx3.cab not found in sources/sxs"
```

### Log Key Reference

- **`module`**: Component originating the log (`config`, `copy`, `windows`, `dism`, `installer`, `shortcut`).
- **`action`**: Target item or app name.
- **`status`**: Outcome indicator (`SUCCESS`, `SKIPPED`, `WARNING`, `FAILED`).
- **`message`**: Technical explanation or error description.

---

## 12. Troubleshooting & Error Matrix

| Error Symptom                                    | Cause                                                                                      | Resolution                                                                                                                   |
| :----------------------------------------------- | :----------------------------------------------------------------------------------------- | :--------------------------------------------------------------------------------------------------------------------------- |
| **Exit Code 2 on startup**                       | Tool launched without Administrator rights OR JSON syntax error in config files.           | Right-click `Setup.exe` $\rightarrow$ **Run as administrator**. Validate `settings.json` and `apps.json` with a JSON linter. |
| **App shows `FAILED` status**                    | Incorrect `installerPath`, missing installer binary, or invalid silent arguments.          | Verify installer file name in `software/` matches `installerPath` in `apps.json`. Check `logs/setup.log`.                    |
| **App installs but is re-installed on next run** | Missing or incorrect `detection` rule in `apps.json`.                                      | Verify executable path or registry uninstall key path in `detection` block.                                                  |
| **Duplicate shortcuts on desktop**               | `desktopShortcut.enabled` set to `true` for an app that creates its own shortcut natively. | Change `"desktopShortcut.enabled": false` in `apps.json` for that application.                                               |
| **.NET 3.5 installation fails**                  | `NetFx3.cab` is missing from `sources/sxs/`.                                               | Copy `microsoft-windows-netfx3-ondemand-package*.cab` from Windows 11 installation media into `USB_ROOT/sources/sxs/`.       |

---

## 13. Exit Codes Summary

| Exit Code | Classification  | Operational Meaning                                                                   |
| :-------: | :-------------- | :------------------------------------------------------------------------------------ |
|  **`0`**  | **Success**     | Provisioning completed successfully (including skipped tasks).                        |
|  **`1`**  | **Task Error**  | Provisioning finished, but one or more tasks failed. Review `logs/setup.log`.         |
|  **`2`**  | **Fatal Error** | Fatal initialization error (Elevation missing, corrupted JSON, missing system paths). |

---

## 14. Contributing & Open Source (GitHub)

WinProvision is an open-source project hosted on GitHub! I welcome contributions from IT Officers, Systems Engineers, and Developers around the world.

### How You Can Contribute

1. **Submit New Application Templates:** Share tested `apps.json` definitions for popular enterprise software (including silent switches and detection rules).
2. **Report Bugs & Feature Requests:** Open an Issue on GitHub describing any bugs encountered or feature requests (e.g., new Windows OS policy toggles).
3. **Code Contributions:** Enhance the Go codebase, add new modules, or improve installer detection logic.

### Developer Build Instructions

Requirements: **Go 1.24+**

```bash
# Clone the public repository
git clone https://github.com/SmonSisay/winprovision.git
cd winprovision

# Run tests and static analysis
go test ./...
make check

# Cross-compile Windows Setup.exe on Linux / macOS
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w -X main.version=1.0.0" -o Setup.exe ./cmd/setup
```

### GitHub Pull Request (PR) Checklist

- [ ] Verify Go code compiles cleanly without warnings (`go vet ./...`).
- [ ] Ensure all existing tests pass (`go test ./...`).
- [ ] Validate any updated JSON config files using a linter.
- [ ] Test `Setup.exe`
