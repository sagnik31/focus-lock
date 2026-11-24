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
| Enforcer (`--enforce`) | The `FocusLock Enforcer` scheduled task | Headless. Runs the enforcement loop and, in strong mode, is the only process that can sign the config. Single instance (a lock file in `%ProgramData%\FocusLock` in strong mode, `Global\FocusLockGhostUser` otherwise). The code calls it the "Ghost". |
| `--session-active` | The installer | Exits 3 if a session is active, so Setup can refuse to update mid-session. |
| `--apply-update` | The `FocusLock Updater` scheduled task (started by the UI) | Elevated. Fetches, verifies and silently installs the newest signed release (see Updates). |

The scheduled task runs at logon with highest privileges, repeats every minute (from a time
trigger, so from the moment it is registered), and ignores new instances while one is
running. A killed enforcer therefore comes back within a minute and resumes the persisted
session. The UI also starts the enforcer on launch: in strong mode it always asks the task to
run, and in reduced mode only when a lock is active or a schedule is enabled and no enforcer
is running.

The enforcer runs the loop `watchdog.StartEnforcer`. In reduced mode the UI also runs it, in
a goroutine. In strong mode the UI only keeps its trusted clock synced.

## The enforcement loop

`backend/watchdog/service.go`:

- **Fast tick (500 ms).** Applies a pending UI request, picks up config changes and, while
  enforcing, terminates processes whose executable name is blocked.
- **Slow tick (5 s).** Reloads the config. While enforcing, it matches blocked names against
  process metadata (product name, file description), rewrites the website blocks, updates the
  remaining time and records stats. Otherwise it lifts the website blocks.

Blocking is applied while a lock or schedule window is active and no emergency pause is
running (`storage.Enforcing`). Startup and config reloads check this too.

Processes Windows or Focus Lock needs (`storage.ProtectedApp`) are never terminated, and the
metadata match skips the Windows folder. Focus Lock's own exe is never terminated either,
and a process is ended only if its PID still belongs to the matched name. An expired lock is
cleared only after NTP has answered.

Website blocking has two layers:

- **`hosts` file** (`blocking/hosts`). Blocked domains and their common subdomains are
  written between `#### FOCUS LOCK START ####` and `#### FOCUS LOCK END ####`, then DNS is
  flushed. A leading `www.` is dropped, so `www.example.org` covers the whole site. If the end
  marker is missing, only lines Focus Lock wrote are removed.
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
  through a request file (`storage/request.go`). The enforcer claims it by renaming it,
  validates it, commits it and writes back a result. A request expires when the UI stops
  waiting (three seconds), so a change reported as failed isn't applied later. If it was
  already claimed, the UI waits for the verdict a little longer. The enforcer also keeps a
  signed copy in `%ProgramData%\FocusLock` (admins only) and trusts that copy first on load,
  so deleting or replacing the `%APPDATA%` files doesn't change an active session. Because
  the UI can't write the config, its own enforcement warnings (for example a failed enforcer
  spawn) are kept in memory and merged into `GetConfig`'s `LastEnforcementError`.
- **Reduced mode.** No HKLM secret, for example on a standard-user account or when setup
  hasn't run. A per-user `HKCU` secret is used and the UI signs directly.

Signers take a cross-process lock around every read-modify-write (`Store.lockConfig`), since
in reduced mode the UI and the enforcer both write the config. A save fails only if the
copy that loads trust first wasn't written. A config that can't be verified is kept as
`config.json.rejected`. When setup first provisions
strong mode it sets `MigratePending` in HKLM, and the enforcer then adopts the config signed
with the `HKCU` secret once and re-signs it.

## The operation policy

`storage.ApplyOperation(cfg, op, now)` is a pure function and the only place that decides
whether a change is allowed. Both the UI (reduced mode) and the enforcer (strong mode) call
it. While a lock or schedule window is active:

- **Allowed (strengthening):** start or extend a lock, add apps or sites, turn on VPN
  blocking, add schedules, append-only import, and an emergency unlock within the quota
  (two one-minute pauses per session). Unlocking again during a pause changes nothing.
- **Refused (weakening):** end or shorten a lock, remove apps or sites, turn off VPN
  blocking, delete, disable or re-time an enabled schedule, or refill the unlock quota
  with a new lock during a schedule window.

An emergency unlock is refused when nothing is active, and a fresh lock clears any pause.

It also validates input: app names (`ValidAppName`, `ProtectedApp`), sites
(`hosts.ValidSite`), schedules (`NormalizeSchedule`) and the session length
(`MaxFocusSeconds`, 365 days). Imports skip invalid apps and sites, but an invalid schedule
or group refuses the whole import with nothing changed. The frontend is not a trust boundary.

## Trusted time

`clock.Default.Now()` anchors to NTP (`pool.ntp.org`, `time.google.com`, `time.windows.com`)
once, then advances on Go's monotonic clock and re-syncs every five minutes. Enforcement and
the policy use it instead of `time.Now()`. When offline it falls back to monotonic time from
process start. The strong-mode UI syncs its own clock. The UI reads it through `GetNow()`
and offsets its countdown.

## Schedules

A schedule is a set of days plus a start and end time. An end earlier than the start makes
an overnight window. `storage.SchedulesActiveAt` is the single check for whether a window
is active. The UI asks `GetLockState()` (built on `storage.LockStateAt`) for the lock in
force and when it ends, so the lock screen never computes windows itself. Each window
instance counts as one session for the emergency-unlock quota, except that overlapping
windows and manual locks share one quota. In the history, a session ends when its lock or
window did, even if the PC was off at the time.

## Updates

A GitHub Actions workflow (`.github/workflows/release.yml`) publishes a calendar-versioned
release (`YYYY.M.D.N`) every Sunday when `main` has changed. Runs on other branches are
skipped. It signs `SHA256SUMS.txt`
with the release key (`cmd/signsums`, secret `RELEASE_SIGNING_KEY`). The app checks the
GitHub Releases API, then downloads the installer and verifies it against the signed
checksums. The public key is in `backend/updater/signature.go`, and an unsigned release is
refused.

**Install and restart** starts the on-demand `FocusLock Updater` task (registered by the
installer), which runs `focus-lock.exe --apply-update` elevated without a UAC prompt. That
mode fetches the latest release again itself and refuses anything not newer than the
installed version. It verifies the installer in the admin-only
`%ProgramData%\FocusLock\update`, checks for a session before and after the download, and
starts Setup with `/UPDATE=2`. Setup closes the app, replaces the exe and relaunches the app
non-elevated through `explorer.exe`. The UI polls the task and shows its exit code
(`updater.ApplyExitMessage`) if it refuses. Without the task (reduced mode, older installs)
the app runs the installer with one UAC prompt. See [ADR
0009](adr/0009-silent-updates-via-updater-task.md).

The app stays open until Setup closes it. If that doesn't happen within two minutes, it says
that the update didn't go ahead.

Installing is refused during a session by the app, by `--apply-update` and by the
installer. When the session state can't be read, in-app updates are refused too, but Setup
run by hand goes on, so a broken install can be repaired. A download that receives no data
for 60 seconds is given up. `backend/version.Version` is stamped at build time. Local builds
report `dev` and never update.

## Checks

`.github/workflows/ci.yml` runs on every push and pull request to `main`: `go build`, `go
vet`, staticcheck, `go test`, and in `frontend/` `tsc`, ESLint and Vitest. The release
workflow and the optional pre-push hook (`scripts/hooks/pre-push`) run the same gate. Go
tests cover pure logic (the policy, schedules, signing, hosts expansion, updater
verification) without GUI or Windows state. Frontend logic that needs testing lives in pure
modules (`components/duration.ts`, `lockState.ts`, `updates.ts`, `serialQueue.ts`) next to
its tests. Enforcement, the installer and the UI itself are checked by hand with
[installer/TESTING.md](../installer/TESTING.md).

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
backend/scheduler/           Creates and queries the enforcer's scheduled task (schtasks, no console window)
backend/updater/, backend/version/  Release check, download, signature + checksum verify, updater task, launch
backend/sysinfo/             Installed apps and icons
backend/protection/          VPN block list
backend/logging/             Size-capped log files
frontend/src/                React UI; pure logic in components/*.ts with Vitest tests
frontend/wailsjs/            Generated Go bindings (regenerated by wails build)
installer/                   Inno Setup script and manual test checklist
scripts/                     Setup, build, release and git-hook scripts
.github/workflows/           CI on every push (ci.yml) and the weekly release (release.yml)
```

Files ending in `_other.go` are `!windows` stubs that let the packages without Windows
imports compile on other platforms. The app itself (`main`, `bridge`, `storage`, `sysinfo`,
`watchdog`) builds only on Windows.
