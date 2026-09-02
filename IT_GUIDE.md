# WinProvision — IT Officer User Guide

This guide explains how to use the **WinProvision** USB provisioning tool to set up a
Windows 11 computer with all the standard software automatically. It is written for
IT officers and technicians — **no programming knowledge is required**.

---

## 1. What is WinProvision?

WinProvision is a small program (`Setup.exe`) that you put on a USB drive together with
the installers. When you plug the USB into a computer and run it, it automatically:

1. **Copies** all the software files from the USB onto the PC.
2. **Configures Windows** — turns off the firewall, enables Remote Desktop, enables the
   Administrator account, and shows file extensions (as configured).
3. **Installs** the applications one by one, silently in the background.
4. **Creates desktop shortcuts** for the installed programs.
5. **Checks what is already installed** so you can safely run it again — it skips
   anything already done (no double installation).
6. **Writes a detailed log** so you can see exactly what happened.

The whole thing works **offline** — it does not need an internet connection. All
installers are stored on the USB.

---

## 2. What's on the USB drive?

```
USB DRIVE (e.g. E:\)
├── Setup.exe              ← THE tool. You run this.
│
├── config/                ← Configuration (usually you don't need to touch this)
│   ├── settings.json      ← Windows settings + destination folder
│   └── apps.json          ← the list of applications to install
│
├── software/              ← the installers (folders, one per application)
│   ├── Chrome/            ← Chrome installer
│   ├── Firefox/           ← Firefox installer
│   ├── MicrosoftOffice/   ← Office installer
│   ├── AdobeReader/
│   ├── Skype4Business/
│   ├── VisualGeez/
│   ├── PowerGeez/
│   ├── Kaspersky Endpoint installer 12.3/
│   ├── FortiNAC/
│   ├── CopyOnly/          ← files that are copied but NOT installed (e.g. printer drivers)
│   └── ...
│
├── sources/
│   └── sxs/               ← .NET Framework 3.5 (already included — no extra USB needed)
│
└── logs/
    └── setup.log          ← created after running — the report of what happened
```

---

## 3. How to use it (step by step)

1. **Plug the USB into the PC** you want to set up.
2. Make sure the PC is on and you can log in as **Administrator** or an account with
   Administrator rights. The tool **will not run** without Administrator privileges.
3. Open the USB drive (e.g. `E:\`) in File Explorer.
4. **Double-click `Setup.exe`**.
   - If a security warning appears, choose **Run / Yes**.
5. The tool shows a screen with:
   - The version of the tool
   - The Windows version and logged-in user
   - Where it will copy the software
   - A summary of all the actions it is about to do
6. It asks you to **confirm** before starting. Press **Y** then Enter (or click confirm).
7. Watch the progress. Each task shows its status:
   - **SUCCESS** — done ✔
   - **SKIPPED** — already done, not repeated
   - **FAILED** — something went wrong
   - **WARNING** — partly done, check the details
8. When it finishes, the final summary is shown and a copy is saved to `logs/setup.log`.

> **Note for two special programs (Visual Geez and Power Geez):**
> These old Geez (Ethiopic font) programs cannot install silently on their own.
> If the silent install fails, the **installer window will pop up automatically**.
> When that happens, **click through the wizard yourself** (Next → Next → Finish) to
> complete the installation manually. This is expected — it is designed that way.

---

## 4. After it finishes — checking the result

The most important file is the log:

```
logs/setup.log   (on the USB drive)
```

Open it with Notepad. Every line is one step, like this:

```
timestamp=... module=installer action=Google Chrome duration=12s status=SUCCESS message="Installed successfully"
```

How to read it:

| word | meaning |
|------|---------|
| `module` | which part of the tool did this (e.g. `copy`, `windows`, `installer`, `dism`) |
| `action` | the specific item (e.g. the app name) |
| `status` | `SUCCESS` / `SKIPPED` / `FAILED` / `WARNING` |
| `message` | short explanation of the outcome |
| `duration` | how long that step took |

**Good sign:** the last line says `status=SUCCESS message="Provisioning completed..."`.
**Look at** any line where `status=FAILED` — that tells you which app / setting needs attention.

---

## 5. The configuration — explained simply

Everything is controlled by two small text files in the `config/` folder. They use JSON
format (simple text with `{`, `}`, `"` and `,`). You can edit them with Notepad.

> **Tip:** before editing, make a backup copy of the original file.

### 5.1 `config/settings.json` — the Windows settings

| Block | Setting | What it does |
|-------|---------|--------------|
| `destination.folderName` | `"Softwares"` | Where on the PC the software is copied (a folder named `Softwares`) |
| `windows.disableFirewall` | `true`/`false` | Turn off the Windows Firewall |
| `windows.enableRemoteDesktop` | `true`/`false` | Allow Remote Desktop connections to the PC |
| `windows.enableAdministrator` | `true`/`false` | Activate the built-in Administrator account |
| `windows.administratorPassword` | `"..."` | Password for the Administrator account |
| `windows.installDotNet35` | `true`/`false` | Try to install .NET Framework 3.5 from the USB |
| `windows.disableWindowsUpdate` | `true`/`false` | Turn Windows Update to manual |
| `logging.file` | `"logs/setup.log"` | Where the log file is saved |

Most of the time you do **not** need to change anything here.

### 5.2 `config/apps.json` — the list of applications

This file lists every application that will be installed. **To add a new application**,
you create one "block" like the examples below.

#### A normal app (installed silently)

```json
{
  "name": "Google Chrome",
  "installerPath": "Chrome/ChromeStandaloneSetup64.exe",
  "silentArgs": "/silent /install",
  "desktopShortcut": {
    "enabled": false
  },
  "detection": {
    "executablePath": "C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe"
  }
}
```

#### An app that needs a person to click the wizard (like Geez)

```json
{
  "name": "Power Geez",
  "installerPath": "PowerGeez/iNSTALL Ge'ez 10.exe",
  "silentArgs": "/s",
  "attendedFallback": true,
  "desktopShortcut": {
    "enabled": false
  },
  "detection": {
    "installDir": "C:\\Program Files\\Power Geez"
  }
}
```

The `"attendedFallback": true` line means: *try silent first; if that fails, pop up the
installer window so someone can finish it by hand.*

#### A "copy only" folder (files that are copied but never installed)

```json
{
  "name": "Copy-only Files",
  "installerPath": "CopyOnly/",
  "copyOnly": true,
  "desktopShortcut": {
    "enabled": false
  }
}
```

This is used for things like **printer drivers** — you just want the files delivered to
the PC, not automatically installed.

---

## 6. Field-by-field reference (for `apps.json`)

| Field | Required | What it means |
|-------|----------|---------------|
| `name` | Yes | The name shown in the progress and log |
| `installerPath` | Yes | Location of the installer, *relative to the `software/` folder* |
| `silentArgs` | Yes | The command-line switches that make the app install quietly |
| `version` | Yes | Just a label for display |
| `attendedFallback` | No | `true` = if silent fails, show the wizard for manual install |
| `copyOnly` | No | `true` = copy the folder only, never run the installer |
| `desktopShortcut.enabled` | Yes | `true` = create a desktop shortcut |
| `desktopShortcut.targetPath` | When enabled | Full path to the app's `.exe` for the shortcut |
| `detection.*` | Yes (at least one) | How the tool knows the app is already installed |

### Understanding `installerPath`
It is always relative to the `software/` folder. So if the installer sits in
`software/Chrome/ChromeStandaloneSetup64.exe`, you write
`"installerPath": "Chrome/ChromeStandaloneSetup64.exe"`.

### Understanding `detection` (very useful)
This is how the tool decides "is this already installed?" before it runs the installer.
If the app is detected, the tool **skips** it. To add a NEW app:

- If you know where the program installs to, use:
  ```json
  "detection": { "installDir": "C:\\Program Files\\MyApp" }
  ```
- If you know the main program file, use:
  ```json
  "detection": { "executablePath": "C:\\Program Files\\MyApp\\myapp.exe" }
  ```

> **Important:** In JSON, backslashes in paths must be written as **double** backslashes,
> e.g. `C:\\Program Files\\MyApp`. Forgetting this breaks the file.

---

## 7. Common tasks for IT officers

### I want to add a new program
1. Create a folder inside `software/` and drop the installer there.
2. Add a block to `apps.json` (see section 5.2).
3. Set a `detection` rule so the tool can skip it if already installed.
4. Save, then run `Setup.exe` again.

### I want to REMOVE a program from provisioning
Just delete its block from `apps.json`. (You can leave the folder in `software/` — the
tool only installs what is listed.) It will then be auto-discovered and installed
though, so if you want a folder on the USB that is *never* installed, move it into
`software/CopyOnly/`.

### The tool says a program FAILED to install
1. Open `logs/setup.log`.
2. Find the line for that program.
3. Read the `message`. Common causes:
   - The installer file name in `apps.json` doesn't match the actual file in `software/`.
   - Antivirus blocked the silent install (some programs block automation).
   - For Geez apps: that's normal — the wizard should have popped up instead.

### Only SOME apps installed — is that OK?
Yes. `SKIPPED` usually means the app was already there. Look only for `FAILED`.

---

## 8. Safety and rules

- **Always run as Administrator.** Otherwise the tool stops with exit code `2`.
- **The administrator password** in `settings.json` is best set through the
  `ADMIN_PASSWORD` environment variable on the PC, not stored in the file.
- **Never run this on a PC you don't want changed** — it turns off the firewall and
  enables Remote Desktop by default.
- Editing config files: **keep valid JSON.** A missing comma or `{`/`}` breaks the file
  and the tool will not start.

---

## 9. Quick troubleshooting

| Problem | What to do |
|---------|-----------|
| `Setup.exe` won't start | Make sure you are Administrator. Right-click → Run as administrator. |
| Tool exits with code 2 at once | Usually not admin, or a config file has a mistake. Check `logs/setup.log`. |
| "Copy-only Files" never "installs" | Expected — that folder is only copied, never run. |
| Visual/Power Geez show in log but no window | Confirm the wizard finished and the app opens. |
| An app shows FAILED every time | Verify the installer path in `apps.json` matches the real file in `software/`. |

---

For further technical details (development, build steps, full configuration reference),
see the separate **README.md** file in the same folder as this guide.
