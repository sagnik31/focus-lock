# Installer click-test checklist

Run on the real machine, signed in as your **local admin** account (approve the
UAC prompt yourself — strong mode needs the elevated account to be the
signed-in user). Use an admin PowerShell for the checks.

## 0. Before you start (dev machine only)

- The existing `FocusLock Enforcer` task currently runs the **dev build** in
  `%APPDATA%\FocusLock\focus-lock.exe` (from `deploy.ps1` / `setup_persistence.ps1`).
  The installer re-points it at Program Files and stops the dev enforcer.
- Expect two "Focus Lock" desktop shortcuts afterwards if you ever ran
  `deploy.ps1`: your dev one (user desktop) and the installer's (public desktop).
  The pre-push hook no longer redeploys the dev exe; delete the dev shortcut and
  `%APPDATA%\FocusLock\focus-lock.exe` if you only want the installed copy.
- Optional, to test the hosts cleanup later: start a short manual lock before uninstalling (step 3).

## 1. Install

- [ ] Run `installer\Output\FocusLock-Setup-<version>.exe` (or a release's `FocusLock-Setup.exe`), accept UAC, finish with "Launch Focus Lock" ticked.
- [ ] No "enforcement setup failed" error dialog appeared. (If it did: read `%TEMP%\FocusLock-setup.log`.)
- [ ] App launches non-elevated (Task Manager → Details → "Elevated" column = No for the UI `focus-lock.exe`).

## 2. Verify strong mode

```powershell
Get-ItemProperty HKLM:\SOFTWARE\FocusLock | Select Provisioned          # 1
Test-Path HKLM:\SOFTWARE\FocusLock\Security                              # True
Get-ScheduledTask 'FocusLock Enforcer' | Select State, @{n='Cmd';e={$_.Actions.Execute}}, @{n='Args';e={$_.Actions.Arguments}}
#   State Ready/Running, Cmd "C:\Program Files\Focus Lock\focus-lock.exe", Args --enforce
Get-ScheduledTask FocusLockGhost, FocusLockLauncher -ErrorAction SilentlyContinue   # nothing
(Get-MpPreference).ExclusionPath                                         # both paths below
#   C:\Program Files\Focus Lock
#   C:\Users\<you>\AppData\Roaming\FocusLock
Test-Path $env:APPDATA\FocusLock\Bin                                     # False
Get-CimInstance Win32_Process -Filter "Name='focus-lock.exe'" | Select ProcessId, ExecutablePath, CommandLine
#   one with --enforce from C:\Program Files\Focus Lock (running already, no reboot needed)
```

- [ ] Start a short manual lock; a blocked site fails to load and a blocked app is closed.
- [ ] Kill the `--enforce` process in Task Manager; it is back within ~1 minute and the lock is still on.
- [ ] **Reboot**, sign in: the `--enforce` process is running again, elevated, from Program Files.
- [ ] Re-run the same Setup.exe over the top (upgrade path): no "file in use" dialog, no error dialog, task still points at Program Files, enforcer running afterwards.

## 3. Uninstall

- [ ] (Optional) start a manual lock first so a hosts block is present.
- [ ] Settings → Apps → Focus Lock → Uninstall (accept UAC). Completes without a file-in-use prompt.

## 4. Verify full teardown

```powershell
Get-ScheduledTask 'FocusLock Enforcer', 'FocusLock Updater', FocusLockGhost, FocusLockLauncher -ErrorAction SilentlyContinue  # nothing
Test-Path HKLM:\SOFTWARE\FocusLock                                       # False
Test-Path HKCU:\Software\FocusLock                                       # False
(Get-MpPreference).ExclusionPath                                         # neither Focus Lock path
Test-Path $env:APPDATA\FocusLock                                         # False (see note)
Test-Path 'C:\Program Files\Focus Lock'                                  # False
Test-Path $env:ProgramData\FocusLock                                     # False
Get-Process focus-lock -ErrorAction SilentlyContinue                     # nothing
Select-String 'FOCUS LOCK' C:\Windows\System32\drivers\etc\hosts         # nothing; blocked sites load again
```

Note: if you still use the dev `deploy.ps1` flow, uninstall deletes the dev exe in
`%APPDATA%\FocusLock` too; run `deploy.ps1` by hand to put it back.

If anything is off, `%TEMP%\FocusLock-setup.log` / `%TEMP%\FocusLock-uninstall.log` have the transcript.

## 5. Releases and in-app updates

Needs the release workflow on `main` on GitHub. Steps 5.2 onward start from an
installed **release** build (a local `build_installer.ps1` build without
`-Version` reports `dev` and never offers updates). For 5.3 you need two releases:
run the workflow once, install that, then run it again (a manual run always
releases; a second run on the same UTC day gets `.1`).

### 5.1 Release pipeline

- [ ] GitHub → Actions → **release** → **Run workflow** on `main` (or
      `scripts\release.ps1 -Watch`). The run is green.
- [ ] The new release `vYYYY.M.D.N` has 4 assets: `FocusLock-Setup-<ver>.exe`,
      `FocusLock-Setup.exe`, `SHA256SUMS.txt` and `SHA256SUMS.txt.sig` (see §5.8).
- [ ] `https://github.com/sagnik31/focus-lock/releases/latest/download/FocusLock-Setup.exe`
      downloads, and its hash matches `SHA256SUMS.txt`:
      `(Get-FileHash .\FocusLock-Setup.exe).Hash.ToLower()`.

### 5.2 Fresh install from a release

- [ ] Install from the downloaded `FocusLock-Setup.exe` (SmartScreen warns once: More info → Run anyway).
- [ ] The header shows `v<ver>` next to "Focus Lock"; **Updates** says "You're on the latest version" after **Check for updates**.
- [ ] Settings → Apps lists Focus Lock with version `<ver>`.
- [ ] `& 'C:\Program Files\Focus Lock\focus-lock.exe' --session-active; $LASTEXITCODE` → `0` with no session, and no window opens.

### 5.3 Update to a newer release

- [ ] Publish a newer release (run the workflow again). Restart the app or click **Check for updates**:
      the header button turns into **Update available** with a dot, and the panel shows the new version and notes.
- [ ] **Download** shows progress, then "Downloaded and verified". The file is in `%APPDATA%\FocusLock\update\`.
- [ ] **Install and restart** → exactly one UAC prompt → the app closes, then reopens by itself on the new version.
      (This is the fallback path, for installs without the `FocusLock Updater` task. With the task there is no
      prompt; see §5.8.)
- [ ] Afterwards: blocked apps/sites, schedules, groups, Insights history intact; strong mode still on (§2 checks);
      `FocusLock Enforcer` task running from Program Files.
- [ ] Older downloaded setups are gone from `%APPDATA%\FocusLock\update\`.

### 5.4 No install during a session

With an update available (downloaded or not):

- [ ] Start a manual lock. Open **Settings** from the lock screen → **Updates**: **Check for updates** and
      **Download** still work; **Install and restart** is disabled with "Available when your session ends".
- [ ] The lock screen shows "Update <ver> downloaded — it installs after this session".
- [ ] Backend refusal, bypassing the greyed-out button (also unit-tested in `backend/bridge/update_test.go`):
      quit the installed UI, build an old-versioned copy with devtools,
      `wails build -devtools -ldflags "-X focus-lock/backend/version.Version=2026.1.1.0"`, and run
      `build\bin\focus-lock.exe` directly (same config). Check → Download, start a lock, then right-click →
      Inspect → Console: `await window.go.bridge.App.InstallUpdate()` → the result's `error` says updates
      can't be installed during a focus session; no UAC prompt, nothing runs.
- [ ] Run the downloaded `FocusLock-Setup-<ver>.exe` (or any newer Setup) by hand → Setup refuses with
      "A Focus Lock session is active…" before anything is stopped; the enforcer keeps running.
- [ ] `focus-lock.exe --session-active; $LASTEXITCODE` → `3`.
- [ ] After the lock ends, **Install and restart** is enabled and works.

### 5.5 Schedule window

- [ ] Create a schedule window covering now, no manual lock. Repeat 5.4: same results (install refused in the
      app and in Setup, `--session-active` → `3`). Disable/finish the window → install works.

### 5.6 Failure cases (each shows a clear message and runs nothing)

- [ ] **Offline** (disable the network adapter) → Check for updates: "could not reach GitHub (are you offline?)…".
- [ ] **Declined UAC** → click **No** at the prompt: the panel goes back to "Downloaded and verified" with
      "the administrator prompt was declined…"; the app keeps running; Install can be retried.
- [ ] **Tampered download** → after Download, overwrite the file in `%APPDATA%\FocusLock\update\` (e.g.
      `Set-Content` some text into it), then **Install and restart**: "The downloaded installer was changed or
      damaged…", the file is deleted, no UAC prompt; **Download** is offered again.

### 5.7 Empty week

- [ ] A scheduled (Sunday) run with no commits since the last release tag succeeds and logs
      "No commits since … skipping", and no new release appears. (Or wait for a quiet week and check the Actions log.)
- [ ] If the repo had no activity for 60 days, GitHub disables the schedule: re-enable it in Actions.

### 5.8 Updates without a UAC prompt (ADR 0009)

Needs the `RELEASE_SIGNING_KEY` repository secret. Start from an install of a release that has this
feature (the update *onto* that release still prompts once, through §5.3).

- [ ] The release has a 4th asset, `SHA256SUMS.txt.sig`. A workflow run without the secret fails at
      **Sign checksums** and publishes nothing.
- [ ] After installing, the updater task exists and runs the installed exe (in an admin PowerShell):
      `Get-ScheduledTask 'FocusLock Updater' | % { $_.Actions.Execute, $_.Actions.Arguments, $_.Principal.RunLevel }`
      → `"C:\Program Files\Focus Lock\focus-lock.exe"`, `--apply-update`, `Highest`. `%TEMP%\FocusLock-setup.log`
      shows "Registered 'FocusLock Updater'" and no warning about it.
- [ ] From a normal (non-elevated) PowerShell: `schtasks /change /tn "FocusLock Updater" /disable` → access denied
      (only admins can change it). Don't run it from an admin prompt.
- [ ] Publish a newer release. **Check for updates** → **Download** → **Install and restart** → **no UAC prompt**;
      the app closes and reopens by itself on the new version within about a minute.
- [ ] The reopened app is **not** elevated: Task Manager → Details → right-click a column → Select columns →
      **Elevated** → the `focus-lock.exe` UI row says **No** (the enforcer row says Yes).
- [ ] Afterwards: settings, schedules and history intact; both tasks present; `%ProgramData%\FocusLock\update\`
      holds the new setup and `apply.log` ("started …"), readable only from an admin prompt.
- [ ] **During a session**: start a lock, then run the task by hand: `schtasks /run /tn "FocusLock Updater"`, and after
      a few seconds `(Get-ScheduledTask 'FocusLock Updater' | Get-ScheduledTaskInfo).LastTaskResult` → `3`; nothing
      installs. (The app's button is greyed out anyway.)
- [ ] **Already up to date**: on the latest version, `schtasks /run /tn "FocusLock Updater"` → last result `20`; nothing
      installs.
- [ ] **Fallback**: in an admin PowerShell, `Disable-ScheduledTask 'FocusLock Updater'`, then **Install and restart**
      with an update available → one UAC prompt, as in §5.3. Re-enable it afterwards with `Enable-ScheduledTask`.

Known gaps (accepted): uninstalling mid-session isn't blocked; the first update from a pre-calendar
`1.0.0` install isn't gated by the installer (the app-side check still applies); installers older than this
feature don't refuse mid-session either.

## 6. Gap-audit fixes (2026-11-18)

These fixes are unit-tested where the logic allows, but the UI parts have never been seen
running. Any installed build from `main` after `1f46f89` (or `wails build`) will do.

- [ ] **App picker (G1).** Apps → open the app picker: no entries with a "?" icon and no name
      that select together; the footer says how many apps without a program file are hidden.
      Start a short lock with a normal app blocked: only that app closes, nothing else
      (e.g. a `.NET` or Node.js tool you have open keeps running).
- [ ] **Overnight schedule (G4).** Timer panel → **Scheduled** → New, 22:00–06:00 on today: the editor shows the
      "Overnight" hint and saves; the list shows `22:00 - 06:00 (+1 day)`. It enforces before and
      after midnight (or set it to cover "now" across midnight to check quickly).
- [ ] **No PowerShell churn (G5).** Leave the main window open and idle for a minute with Task
      Manager → Details sorted by name: no `powershell.exe` keeps appearing.
- [ ] **Lock-screen end time (G15).** With a schedule window active, start a manual lock that
      ends after the window: the countdown shows the manual lock's end and reads "Focus Mode
      Active"; no Quit button appears when the window ends.
- [ ] **Schedule list (G16).** With the **Scheduled** view open, import a settings file that adds a
      schedule, then toggle another schedule: both are still listed. During a session, a refused
      change shows a red banner (dismissible) and the list shows what is actually saved.
- [ ] **Schedules can't be re-timed mid-session (G2/G3).** During a schedule window there is no
      Edit/Delete/toggle on schedules. Use both emergency unlocks, then start a 1-minute focus from
      Settings: the unlock button stays at the limit.
- [ ] **Deleting the config doesn't end a session (G10).** After installing a build with
      `0c69fff`, `%ProgramData%\FocusLock` exists and a non-admin Explorer window can't open it.
      Start a 10-minute session with an app and a site blocked. In an admin PowerShell run
      `schtasks /End /TN "FocusLock Enforcer"`, stop `focus-lock.exe --enforce` in Task Manager,
      and delete `config.json`, `config.json.sig`, `config.json.bak` and `config.json.bak.sig`
      from `%APPDATA%\FocusLock`. Within a minute the enforcer is back, the four files are back,
      the app and site are still blocked, schedules are intact and no "could not be recovered"
      banner shows. Uninstall removes `%ProgramData%\FocusLock`.

## 7. Gap-audit fixes (2026-11-20)

From G11–G20 (`0c402bd`..`ed2be63`). The logic is unit-tested; these check what tests can't.
Any installed build from `main` after `ed2be63` will do.

- [ ] **Lock-screen countdown from Go (G17).** Start a 5-minute manual lock: the countdown
      starts at about 5:00 and ends on time. Repeat with a schedule window covering now, and
      with an overnight window (for example 22:00–06:00 while it's active): the lock screen
      shows the window's end, and nothing unlocks early.
- [ ] **No console flash (G13).** Click **Start focus**: no console window flashes up.
- [ ] **Refused stop keeps the block (G14).** The UI offers no way to end an active lock, so
      use a devtools build as in §5.4: during a lock, run
      `await window.go.bridge.App.StopFocus()` in the console. It is refused and blocked sites
      still fail to load.
- [ ] **Error banners (G18).** Trigger a refused change during a session: a red banner
      shows with a dismiss button, and a later successful action clears it. The lock screen
      shows App errors (such as a refused quit) with a dismiss button too, and a failed
      emergency unlock shows its error under the button.
- [ ] **Strong-mode warnings stay visible (G11).** Hard to force by hand; if the enforcer fails
      to start from the UI in strong mode, the amber banner should show it instead of nothing.
