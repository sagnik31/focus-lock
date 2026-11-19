# Architecture

This page explains how Focus Lock works. The reasons behind the main choices are in the
[architecture decision records](adr/README.md).

## Threat model

Focus Lock is meant to make casual bypass not worth the effort. It should hold up against
a tempted user who closes the window, kills the process, deletes config files or changes
the system clock. It is not meant to stop someone determined, with admin rights, who
deletes the scheduled task, boots into Safe Mode or spoofs NTP on their network.

## Processes

The same executable, `focus-lock.exe`, runs in four modes chosen in `main.go`:

| Mode | Started by | Job |
| --- | --- | --- |
| UI (no flag) | The user | The Wails window. Edits block lists and schedules, starts sessions, shows insights and updates. Single instance (`Global\FocusLockMutex`). |
| Enforcer (`--enforce`) | The `FocusLock Enforcer` scheduled task | Headless. Runs the enforcement loop and, in strong mode, is the only process that can sign the config. Single instance (`Global\FocusLockGhost`). The code calls it the "Ghost". |
| `--session-active` | The installer | Exits 3 if a session is active, so Setup can refuse to update mid-session. |
| `--apply-update` | The `FocusLock Updater` scheduled task (started by the UI) | Elevated. Fetches, verifies and silently installs the newest signed release (see Updates). |

The scheduled task runs at logon with highest privileges, repeats every minute, and ignores
new instances while one is running. A killed enforcer therefore comes back within a minute
and resumes the persisted session. The UI also starts the enforcer on launch whenever a
lock or schedule is active.

Both the UI and the enforcer run the same loop, `watchdog.StartEnforcer`. The UI runs it in
a goroutine, and the enforcer is dedicated to it.

## The enforcement loop

`backend/watchdog/service.go`:

- **Fast tick (500 ms).** Lists processes and terminates any whose executable name is
  blocked.
- **Slow tick (5 s).** Reloads the config, applies pending UI requests, matches blocked
  names against process metadata (product name, file description), rewrites the website
  blocks, updates the remaining time and records stats.

Website blocking has two layers:

- **`hosts` file** (`blocking/hosts`). Blocked domains and their common subdomains are
  written between `#### FOCUS LOCK ####` markers, then DNS is flushed.
- **Chrome policy** (`blocking/browserpolicy`). The same sites go into
  `HKLM\SOFTWARE\Policies\Google\Chrome\URLBlocklist` for the length of the session.
  Focus Lock records which entries it added and removes only those.

Optional VPN blocking adds known VPN executables and domains to the same lists
(`protection/vpn_blacklist.go`).

## Configuration and integrity

`storage.Config` is stored as JSON in `%APPDATA%\FocusLock\config.json`. It is
HMAC-SHA256 signed, written atomically (temp file and rename), and has a signed `.bak` and
a registry backup of the lock timing.

Where the signing secret lives sets the mode:

- **Strong mode.** Setup stores the secret in `HKLM\SOFTWARE\FocusLock\Security`, readable
  only by SYSTEM and Administrators. Only the elevated enforcer can read it, so it is the
  only signer. The UI (`Store.CanSign() == false`) sends each change as an `Operation`
  through a request file (`storage/request.go`). The enforcer validates it, commits it and
  writes back a result. The enforcer also keeps a signed copy in `%ProgramData%\FocusLock`
  (admins only) and trusts that copy first on load, so deleting or replacing the
  `%APPDATA%` files doesn't change an active session.
  Because the UI can't write the config, its own enforcement warnings (for example a
  failed enforcer spawn) are kept in memory and merged into `GetConfig`'s
  `LastEnforcementError`.
- **Reduced mode.** No HKLM secret, for example on a standard-user account or when setup
  hasn't run. A per-user `HKCU` secret is used and the UI signs directly.

## The operation policy

`storage.ApplyOperation(cfg, op, now)` is a pure function and the only place that decides
whether a change is allowed. Both the UI (reduced mode) and the enforcer (strong mode) call
it. While a lock or schedule window is active:

- **Allowed (strengthening):** start or extend a lock, add apps or sites, turn on VPN
  blocking, add schedules, append-only import, and an emergency unlock within the quota
  (two one-minute pauses per session).
- **Refused (weakening):** end or shorten a lock, remove apps or sites, turn off VPN
  blocking, delete, disable or re-time an enabled schedule, or refill the unlock quota
  with a new lock during a schedule window.

It also validates input: app names (`ValidAppName`) and schedules (`NormalizeSchedule`).
The frontend is not a trust boundary.

## Trusted time

`clock.Default.Now()` anchors to NTP (`pool.ntp.org`, `time.google.com`,
`time.windows.com`) once, then advances on Go's monotonic clock and re-syncs every five
minutes. Enforcement and the policy use it instead of `time.Now()`. When offline it falls
back to monotonic time from process start. The UI reads it through `GetNow()` and offsets
its countdown.

## Schedules

A schedule is a set of days plus a start and end time. An end earlier than the start makes
an overnight window. `storage.SchedulesActiveAt` is the single check for whether a window
is active. Each window instance counts as one session for the emergency-unlock quota.

## Updates

A GitHub Actions workflow (`.github/workflows/release.yml`) publishes a calendar-versioned
release (`YYYY.M.D.N`) every Sunday when `main` has changed. It signs `SHA256SUMS.txt`
with the release key (`cmd/signsums`, secret `RELEASE_SIGNING_KEY`). The app checks the
GitHub Releases API, then downloads the installer and verifies it against the signed
checksums. The public key is in `backend/updater/signature.go`, and an unsigned release is
refused.

**Install and restart** starts the on-demand `FocusLock Updater` task (registered by the
installer), which runs `focus-lock.exe --apply-update` elevated without a UAC prompt. That
mode fetches the latest release again itself and refuses anything not newer than the
installed version. It verifies the installer in the admin-only
`%ProgramData%\FocusLock\update`, checks for a session, and starts Setup with `/UPDATE=2`.
Setup then relaunches the app non-elevated through `explorer.exe`. The UI polls the task
and shows its exit code (`updater.ApplyExitMessage`) if it refuses. Without the task
(reduced mode, older installs) the app runs the installer with one UAC prompt. See
[ADR 0009](adr/0009-silent-updates-via-updater-task.md).

Installing is refused during a session by the app, by `--apply-update` and by the
installer. `backend/version.Version` is stamped at build time. Local builds report `dev`
and never update.

## Code map

```
main.go, session_active.go,  Entry point and the four runtime modes
apply_update.go
cmd/signsums/                Signs SHA256SUMS.txt in the release workflow; generates keys
backend/bridge/              Wails-bound API used by the UI (App struct)
backend/watchdog/            Enforcement loop and Win32 process plumbing
backend/storage/             Config, signing, the operation policy, schedules, stats, request channel
backend/blocking/hosts/      hosts-file blocking
backend/blocking/browserpolicy/  Chrome URLBlocklist policy
backend/clock/, backend/ntp/ Trusted time
backend/scheduler/           Creates and queries the enforcer's scheduled task
backend/updater/, backend/version/  Release check, download, signature + checksum verify, updater task, launch
backend/sysinfo/             Installed apps and icons
backend/protection/          VPN block list
backend/logging/             Size-capped log files
frontend/src/                React UI; frontend/wailsjs/ holds generated bindings
installer/                   Inno Setup script and manual test checklist
scripts/                     Setup, build, release and git-hook scripts
```

Files ending in `_other.go` are `!windows` stubs that let the project compile on other
platforms. Enforcement only works on Windows.
