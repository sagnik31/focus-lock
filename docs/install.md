# Installing, updating and building

## Download

Get `FocusLock-Setup.exe` from
**[Releases → latest](https://github.com/sagnik31/focus-lock/releases/latest)**. Each release
also publishes `SHA256SUMS.txt`. The installer isn't code-signed, so Windows SmartScreen
warns the first time you run a copy downloaded in a browser (**More info → Run anyway**).
In-app updates don't trigger that warning.

## Install

Run the installer and approve the single UAC prompt. It does the one-time elevated setup:

- adds Defender exclusions,
- registers the `FocusLock Enforcer` scheduled task, which runs the installed exe with
  `--enforce` at logon and relaunches it every minute,
- stores the tamper-protection secret in `HKLM` and creates the admin-only
  `%ProgramData%\FocusLock` folder.

It then starts the enforcer, with no reboot needed. If setup fails, Setup says so and the app
runs in reduced mode. The log is `%TEMP%\FocusLock-setup.log`.

> Strong (tamper-resistant) mode assumes the signed-in user is a local administrator. On a
> standard-user account the app still installs and runs, but stays in reduced mode with a
> per-user `HKCU` secret.

## Update

Install once. After that the app updates itself.

- **Weekly releases.** Every Sunday a GitHub Actions workflow takes the latest `main`, runs
  the tests and typecheck, and publishes a release. Weeks with no changes are skipped.
  Versions are calendar versions `YYYY.M.D.N` (tag `vYYYY.M.D.N`).
- **In the app.** Click **Updates** in the header. The app also checks quietly on start, at
  most once a day. **Download** fetches the new installer and verifies its SHA-256 checksum.
  **Install and restart** runs it with one UAC prompt, and Focus Lock reopens on the new
  version with your settings, history and schedules intact.
- **Not during a session.** You can check and download at any time, but installing is refused
  while a timer lock or a schedule window is active.
- **By hand.** Running a newer `FocusLock-Setup-<version>.exe` over the existing install does
  the same thing. Running an older one rolls back.

## Uninstall

Uninstall from **Settings → Apps**. It stops the enforcer and the app and removes the
scheduled task, any active `hosts` block, the `HKLM` and `HKCU` state, both Defender
exclusions, `%APPDATA%\FocusLock` and `%ProgramData%\FocusLock`. The log is
`%TEMP%\FocusLock-uninstall.log`.

## Build from source

Prerequisites: [Go](https://go.dev/dl/) 1.24+, [Node.js](https://nodejs.org/) 18+, and the
Wails CLI:

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
wails doctor      # checks WebView2 and the rest of the toolchain
wails build       # writes build/bin/focus-lock.exe
wails dev         # hot-reloading dev mode
```

### Build the installer

This needs the [Inno Setup 6](https://jrsoftware.org/isdl.php) compiler on `PATH` or in its
default location:

```powershell
powershell -ExecutionPolicy Bypass -File scripts\build_installer.ps1
```

This writes `installer\Output\FocusLock-Setup-<version>.exe`, `FocusLock-Setup.exe` and
`SHA256SUMS.txt`. A local build is installer version `0.0.0.0` and the app reports `dev`,
which never offers updates. Pass `-Version YYYY.M.D.N` to stamp a release version.

A manual test checklist for the installer and updates is in
[`installer/TESTING.md`](../installer/TESTING.md).

### Manual setup without the installer

For a dev build, run `scripts\deploy.ps1` first so the exe exists in `%APPDATA%\FocusLock`
(don't mix this with an installed copy). Then run the setup script, which elevates itself
with one UAC prompt:

```powershell
powershell -ExecutionPolicy Bypass -File scripts\install_setup.ps1 -ExePath "$env:APPDATA\FocusLock\focus-lock.exe"
```
