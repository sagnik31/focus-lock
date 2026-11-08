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

### Install (recommended): the bundled installer

Editing the `hosts` file and keeping enforcement running across reboots require a one-time
setup with administrator privileges (Defender exclusions, the `FocusLock Enforcer` scheduled
task that runs the installed exe with `--enforce`, and a tamper-protection secret in
`HKLM`). The installer does all of it in one pass, so there is no manual PowerShell step.

Build the installer (needs the [Inno Setup 6](https://jrsoftware.org/isdl.php) compiler on
`PATH` or in its default location, plus Go/Node/Wails):

```powershell
powershell -ExecutionPolicy Bypass -File scripts\build_installer.ps1
```

This produces `installer\Output\FocusLock-Setup-<version>.exe`. Run it, approve the single
UAC prompt, and the one-time enforcement setup runs automatically and starts the enforcer
(no reboot needed). If that setup step fails, Setup says so and the app runs in reduced mode;
the details are in `%TEMP%\FocusLock-setup.log`.

Uninstalling (from **Settings → Apps**) stops the enforcer and the app, removes the scheduled
task, any active `hosts`-file block, the `HKLM` and `HKCU` state, both Defender exclusions,
and `%APPDATA%\FocusLock` (log: `%TEMP%\FocusLock-uninstall.log`).

A manual verification checklist for the installer is in
[`installer/TESTING.md`](installer/TESTING.md).

### Updating

There is no automatic update yet (planned). To update, build or download a newer
`FocusLock-Setup-<version>.exe` and run it over the existing install: it stops the enforcer,
replaces the exe, re-registers the task and starts it again, keeping your settings, history
and tamper-protection secret.

> Strong (tamper-resistant) mode assumes the signed-in user is a local administrator. On a
> standard-user account the app still installs and runs, but stays in reduced mode (a
> per-user `HKCU` secret).

### Alternative: manual one-time setup

Without the installer, run the consolidated setup script once from the `scripts` folder — it
self-elevates (one UAC prompt) and performs the same steps for whichever exe you point it at
(the task runs that exe with `--enforce`). Run `scripts\deploy.ps1` first so the exe exists
in `%APPDATA%\FocusLock`, then:

```powershell
powershell -ExecutionPolicy Bypass -File scripts\install_setup.ps1 -ExePath "$env:APPDATA\FocusLock\focus-lock.exe"
```

After setup, website blocking no longer prompts for UAC, enforcement continues when the UI
is closed, and future sessions start automatically. The enforcer's scheduled task is kept
battery-safe and has no run-time limit, so Windows does not silently interrupt it.

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
installer/      Inno Setup script for the bundled installer, plus its test checklist
main.go         Application entry point
wails.json      Wails project configuration
```

## Disclaimer

Focus Lock terminates other processes and modifies system files on the machine it runs on.
It is provided as-is, without warranty. The author is not responsible for data loss, lost
work, or being unable to reach a blocked application or site during an active session. Set
your sessions deliberately.
