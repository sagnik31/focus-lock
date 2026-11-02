# Focus Lock

Focus Lock is a Windows application and website blocker built for people who want to
commit to a distraction-free session and then not be able to talk themselves out of it.
You choose what to block and for how long; once a session starts, the restrictions stay
in force until the timer runs out, even if you close the app or try to end the process.

It is a desktop app written in Go and React, packaged with the
[Wails](https://wails.io/) framework.

> **Note:** Focus Lock is intentionally difficult to bypass while a session is active. It
> terminates blocked applications and edits the system `hosts` file during sessions. Use
> it on your own machine, and understand what it does before starting a long session.

## Why it exists

Most blockers are easy to disable in the moment of temptation — that is exactly when they
are supposed to work. Focus Lock is designed around a single idea: the decision to focus
is made once, up front, and cannot be casually reversed while the session is running.
Closing the window, killing the background process, or changing the system clock will not
lift an active block.

## Features

- **Timed focus sessions.** Set a duration, start the session, and blocking stays active
  until it expires.
- **Scheduled blocking.** Define recurring windows (for example, weekdays 09:00–17:00)
  that enforce blocking automatically with no manual start required.
- **Application blocking.** Block apps by executable name (e.g. `discord.exe`). Blocked
  apps are closed if they are launched during a session.
- **System-wide website blocking.** Domains are blocked through the Windows `hosts` file,
  so the block applies across every browser, including private/incognito windows.
- **Category presets.** One-click lists for common categories such as social media,
  entertainment, gaming, and adult content.
- **VPN awareness.** Optionally blocks common VPN applications and domains so a session
  cannot be routed around.
- **Clock-tamper resistance.** Session timing is checked against network time (NTP) so the
  countdown cannot be shortened by changing the system clock.
- **Resilient enforcement.** A lightweight background process keeps enforcing the active
  session even when the main window is closed, and persists across restarts.
- **Limited emergency unlocks.** A small, capped number of short pauses per session, for
  genuine emergencies, without defeating the point of the lock.

## Architecture

| Layer | Technology |
| --- | --- |
| Frontend | React, TypeScript, Tailwind CSS (Vite) |
| Backend | Go, exposed to the UI through Wails v2 |
| Enforcement | A background Go process polls for blocked apps and reapplies website blocks |

At a high level:

- The **UI process** is what you interact with. It manages your block lists and schedules
  and starts and stops sessions.
- A **background enforcement process** does the actual work during a session: it watches
  for blocked applications and keeps the `hosts`-based website blocks in place. It runs
  independently of the UI so that closing the window does not end a session.
- Configuration and session state are stored locally as JSON under your Windows user
  profile.

Focus Lock is Windows-only. The repository contains stub implementations for other
platforms so the project compiles elsewhere, but the enforcement features require Windows.

## Getting started

### Prerequisites

- [Go](https://go.dev/dl/) 1.24 or newer
- [Node.js](https://nodejs.org/) 18 or newer
- The Wails CLI:

  ```bash
  go install github.com/wailsapp/wails/v2/cmd/wails@latest
  ```

Run `wails doctor` once after installing to confirm your environment has everything Wails
needs (including the WebView2 runtime, which ships with current versions of Windows).

### Build

```bash
git clone https://github.com/sagnik31/focus-lock.git
cd focus-lock
wails build
```

The compiled application is written to `build/bin/focus-lock.exe`.

For local development with hot-reloading of the frontend, run:

```bash
wails dev
```

### One-time elevated setup (recommended)

Editing the `hosts` file and keeping enforcement running across reboots require
administrator privileges. To avoid a UAC prompt every time, register a scheduled task once
from an **elevated PowerShell** so the enforcement process can start with the privileges it
needs:

```powershell
$action    = New-ScheduledTaskAction -Execute "$env:APPDATA\FocusLock\Bin\FocusLockGhost.exe" -Argument "--enforce"
$trigger   = New-ScheduledTaskTrigger -AtLogOn
$principal = New-ScheduledTaskPrincipal -UserId $env:USERNAME -LogonType Interactive -RunLevel Highest
$settings  = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -ExecutionTimeLimit 0
Register-ScheduledTask -TaskName "FocusLockGhost" -Action $action -Trigger $trigger -Principal $principal -Settings $settings -Force
```

The power-related settings matter: by default Windows Task Scheduler stops a task when the
machine switches to battery and after a fixed run time, which would silently interrupt
enforcement. The settings above keep it running.

After this step:

- Website blocking works without a UAC prompt each time.
- Enforcement continues when the UI window is closed.
- Future sessions start automatically without additional setup.

### Launch without a UAC prompt (optional)

The repository includes a helper that creates a desktop shortcut which launches the UI
through the scheduled task, avoiding the UAC prompt on each start:

1. Open the `scripts` folder.
2. Right-click `setup_uac_bypass.ps1` and choose **Run with PowerShell**.
3. Launch the app from the new **Focus Lock** desktop shortcut.

### Antivirus note

Because the enforcement process runs in the background and resists being terminated, some
antivirus software may flag it. If that happens on your own machine, you can add an
exclusion:

```powershell
Add-MpPreference -ExclusionPath "$env:APPDATA\FocusLock"
```

## Usage

1. **Block applications** — open the **Apps** tab and add executables (for example,
   `discord.exe`) or pick from the detected list.
2. **Block websites** — open the **Websites** tab and add domains (for example,
   `facebook.com`), or toggle a category preset.
3. **Start a manual session** — set a duration, click **Start Focus**, and confirm.
4. **Set up schedules** — open the **Schedules** tab, create a schedule with days and a
   start and end time, and enable it. Blocking turns on automatically during that window.

## Project layout

```
backend/        Go: enforcement, scheduling, storage, system info, Wails bridge
frontend/       React + TypeScript UI (Vite + Tailwind)
scripts/        PowerShell helpers for setup, deployment, and git hooks
main.go         Application entry point
wails.json      Wails project configuration
```

## Disclaimer

Focus Lock terminates other processes and modifies system files on the machine it runs on.
It is provided as-is, without warranty. The author is not responsible for data loss, lost
work, or being unable to reach a blocked application or site during an active session. Set
your sessions deliberately.
