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
  that enforce blocking automatically with no manual start required. A window can run
  overnight (for example, 22:00–06:00 starts on each selected day and ends the next
  morning). While a session or window is active, enabled schedules can't be deleted,
  disabled, or have their days or times changed; you can still add new ones.
- **Application blocking.** Block apps by executable name (e.g. `discord.exe`). Blocked
  apps are closed if they are launched during a session.
- **System-wide website blocking.** Domains are blocked through the Windows `hosts` file,
  so the block applies across every browser, including private/incognito windows.
- **Chrome policy blocking.** During a session the enforcer also adds the blocked sites to
  Chrome's `URLBlocklist` policy. Chrome's Secure DNS can bypass the `hosts` file, and
  offline-capable sites can load from cache; the policy blocks them on every visit or
  refresh regardless. Chrome shows "Managed by your organization" only while a session is
  active; the policy entries are removed when it ends, and any `URLBlocklist` entries you
  had before are left alone. A tab that was already open on a site keeps its page until
  you navigate or refresh it.
- **Category presets and your own groups.** One-click lists for common categories such as
  social media, entertainment, gaming, and adult content, plus named groups you create
  yourself (shown first). Sites you have blocked before appear under "Recently used" so
  they can be re-added in one click.
- **Flexible durations.** Pick a preset, type an exact duration, or focus until a chosen
  date and time.
- **VPN awareness.** Optionally blocks common VPN applications and domains so a session
  cannot be routed around.
- **Clock-tamper resistance.** Session timing is checked against network time (NTP) so the
  countdown cannot be shortened by changing the system clock.
- **Resilient enforcement.** A lightweight background process keeps enforcing the active
  session even when the main window is closed, and persists across restarts.
- **Limited emergency unlocks.** Two one-minute pauses per session, for genuine
  emergencies, without defeating the point of the lock. Each schedule window counts as
  one session; starting a timer during a window doesn't add more.

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
  profile. In strong mode the enforcer also keeps a signed copy in `%ProgramData%\FocusLock`,
  which only administrators can change, so deleting or replacing the files in your profile
  doesn't change an active session.

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

### Download

Get the latest installer from
**[Releases → latest](https://github.com/sagnik31/focus-lock/releases/latest)**: the
`FocusLock-Setup.exe` asset, also at
`https://github.com/sagnik31/focus-lock/releases/latest/download/FocusLock-Setup.exe`. Each
release also publishes `SHA256SUMS.txt`. The installer isn't code-signed, so Windows
SmartScreen warns the first time you run a copy downloaded in a browser (**More info → Run
anyway**). In-app updates don't trigger that warning.

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

This produces `installer\Output\FocusLock-Setup-<version>.exe`, plus `FocusLock-Setup.exe` and
`SHA256SUMS.txt`. A local build is installer version `0.0.0.0` and the app reports `dev`; pass
`-Version YYYY.M.D.N` to stamp a release version. Run it, approve the single
UAC prompt, and the one-time enforcement setup runs automatically and starts the enforcer
(no reboot needed). If that setup step fails, Setup says so and the app runs in reduced mode;
the details are in `%TEMP%\FocusLock-setup.log`.

Uninstalling (from **Settings → Apps**) stops the enforcer and the app, removes the scheduled
task, any active `hosts`-file block, the `HKLM` and `HKCU` state, both Defender exclusions,
`%APPDATA%\FocusLock` and `%ProgramData%\FocusLock` (log: `%TEMP%\FocusLock-uninstall.log`).

A manual verification checklist for the installer is in
[`installer/TESTING.md`](installer/TESTING.md).

### Updating

Install once; after that the app updates itself.

- **Weekly releases.** Every Sunday a GitHub Actions workflow takes the latest `main`, runs
  the tests and typecheck, and publishes a release. Weeks with no changes are skipped.
  Versions are calendar versions `YYYY.M.D.N` (tag `vYYYY.M.D.N`).
- **In the app.** Click **Updates** in the header. It reads **Update available** when there is
  one, and the app also checks quietly on start, at most once a day. **Download** fetches the
  new installer and verifies it against the release's SHA-256 checksum. **Install and
  restart** runs it with one UAC prompt, and Focus Lock reopens on the new version with your
  settings, history, schedules and tamper-protection secret intact.
- **Not during a session.** You can check and download at any time, but installing is refused
  while a timer lock or a schedule window is active. Both the app and the installer enforce
  this. Installing becomes available when the session ends.
- **By hand.** Running a newer `FocusLock-Setup-<version>.exe` over the existing install does
  the same thing: it stops the enforcer, replaces the exe, re-registers the task and starts it
  again. Running an older one rolls back.

Local builds (`wails build`, `wails dev`, `scripts\deploy.ps1`) report version `dev` and never
offer updates.

> Strong (tamper-resistant) mode assumes the signed-in user is a local administrator. On a
> standard-user account the app still installs and runs, but stays in reduced mode (a
> per-user `HKCU` secret).

### Alternative: manual one-time setup

Without the installer, run the consolidated setup script once from the `scripts` folder — it
self-elevates (one UAC prompt) and performs the same steps for whichever exe you point it at
(the task runs that exe with `--enforce`). For a dev build, run `scripts\deploy.ps1` by hand
first so the exe exists in `%APPDATA%\FocusLock` (don't mix this with an installed copy), then:

```powershell
powershell -ExecutionPolicy Bypass -File scripts\install_setup.ps1 -ExePath "$env:APPDATA\FocusLock\focus-lock.exe"
```

After setup, website blocking no longer prompts for UAC, enforcement continues when the UI
is closed, and future sessions start automatically. The enforcer's scheduled task is kept
battery-safe and has no run-time limit, so Windows does not silently interrupt it.

## Usage

1. **Block applications** — open the **Apps** tab and add executables (for example,
   `discord.exe`) or pick from the detected list. Installed apps whose program file can't be
   found aren't listed in the picker; add those by their `.exe` name instead.
2. **Block websites** — open the **Websites** tab and add domains (for example,
   `facebook.com`), or toggle a category preset.
3. **Start a manual session** — set a duration (or a "focus until" date and time), click **Start focus**, and confirm.
4. **Set up schedules** — switch the timer panel from **Manual** to **Scheduled**, create
   a schedule with days and a start and end time (**New**), and enable it. Blocking turns
   on automatically during that window. An end time earlier than the start time makes an
   overnight window. Imported schedules
   are checked too: an import with an invalid schedule (for example `25:00` or an unknown
   day) is refused and names the schedule to fix.

## Project layout

```
backend/        Go: enforcement, scheduling, storage, system info, Wails bridge
frontend/       React + TypeScript UI (Vite + Tailwind)
scripts/        PowerShell helpers for setup, builds, releases, and git hooks
.github/        Weekly release workflow
installer/      Inno Setup script for the bundled installer, plus its test checklist
main.go         Application entry point
wails.json      Wails project configuration
```

## Disclaimer

Focus Lock terminates other processes and modifies system files on the machine it runs on.
It is provided as-is, without warranty. The author is not responsible for data loss, lost
work, or being unable to reach a blocked application or site during an active session. Set
your sessions deliberately.
