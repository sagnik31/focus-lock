# Installer click-test checklist

Run on the real machine, signed in as your **local admin** account (approve the
UAC prompt yourself — strong mode needs the elevated account to be the
signed-in user). Use an admin PowerShell for the checks.

## 0. Before you start (dev machine only)

- The existing `FocusLock Enforcer` task currently runs the **dev build** in
  `%APPDATA%\FocusLock\focus-lock.exe` (from `deploy.ps1` / `setup_persistence.ps1`).
  The installer re-points it at Program Files and stops the dev enforcer.
- Expect two "Focus Lock" desktop shortcuts afterwards: your dev one (user
  desktop) and the installer's (public desktop). The pre-push hook keeps
  redeploying the dev exe; that's fine, the task runs the installed one.
- Optional, to test the hosts cleanup later: start a short manual lock before uninstalling (step 3).

## 1. Install

- [ ] Run `installer\Output\FocusLock-Setup-1.0.0.exe`, accept UAC, finish with "Launch Focus Lock" ticked.
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
Get-ScheduledTask 'FocusLock Enforcer', FocusLockGhost, FocusLockLauncher -ErrorAction SilentlyContinue  # nothing
Test-Path HKLM:\SOFTWARE\FocusLock                                       # False
Test-Path HKCU:\Software\FocusLock                                       # False
(Get-MpPreference).ExclusionPath                                         # neither Focus Lock path
Test-Path $env:APPDATA\FocusLock                                         # False (see note)
Test-Path 'C:\Program Files\Focus Lock'                                  # False
Get-Process focus-lock -ErrorAction SilentlyContinue                     # nothing
Select-String 'FOCUS LOCK' C:\Windows\System32\drivers\etc\hosts         # nothing; blocked sites load again
```

Note: if you still use the dev `deploy.ps1` flow, uninstall deletes the dev exe in
`%APPDATA%\FocusLock` too; the next push re-deploys it.

If anything is off, `%TEMP%\FocusLock-setup.log` / `%TEMP%\FocusLock-uninstall.log` have the transcript.
