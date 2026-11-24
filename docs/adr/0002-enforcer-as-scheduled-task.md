# 0002. Run enforcement in a separate process started by a repeating scheduled task

- **Status:** Accepted
- **Date:** 2026-11-08 (current form; a separate enforcer has existed since 2025-12)

## Context

Blocking has to keep working when the user closes the window, kills the app, or restarts
the machine. Editing `hosts` and HKLM policy also needs admin rights, which the UI shouldn't
hold. Earlier versions made the enforcer hard to kill. They used a separate obfuscated copy
of the exe (`Bin\FocusLockGhost.exe`), a DACL that denied terminate access, and marked the
process critical with `RtlSetProcessIsCritical`. The critical-process flag crashed the
machine (bug check 0xEF) whenever Task Scheduler stopped the task, for example on losing AC
power or signing out mid-session. The obfuscation also looked like malware to antivirus
tools.

The goal is to make casual bypass not worth the effort, not to make the app impossible to
remove.

## Decision

- The **same installed exe**, run with `--enforce`, is the enforcer. It is headless and
  single-instance, with no separate copy and no obfuscation.
- The installer registers the **`FocusLock Enforcer` scheduled task** from XML. It runs at
  logon with highest privileges and **repeats every minute** from a time trigger (a logon
  trigger's repetition would start only at the next sign-in), with
  `MultipleInstancesPolicy=IgnoreNew`, no run-time limit, and permission to run on battery.
- The enforcer can be killed freely. Recovery is the one-minute repetition, and the UI also
  starts it on launch (in reduced mode only when a lock is active or a schedule is enabled).
  Because of the repetition it stays resident when idle instead of exiting.
- Session state is persisted, so a relaunched enforcer picks up the same session.

## Alternatives considered

- **Windows service.** Survives logoff and restarts automatically, but runs in session 0
  with no access to the user's processes or profile without extra plumbing. It also needs
  a service installer and is a heavier footprint for a personal tool.
- **Anti-kill hardening (DACL, critical process).** Tried and removed. It caused blue
  screens, and a determined admin can still get around it.
- **UI-only enforcement.** Closing the window would end the session, which defeats the
  product.
- **Registry `Run` key.** Starts at logon but can't run elevated and doesn't recover from a
  kill.

## Consequences

- Killing the enforcer gives at most about a minute of unenforced time.
- An admin who deletes the task or boots into Safe Mode can bypass it. This is accepted and
  outside the threat model.
- The task XML must stay identical in `backend/scheduler/service.go`,
  `scripts/install_setup.ps1` and `scripts/setup_persistence.ps1`.
- Never reintroduce `RtlSetProcessIsCritical` or terminate-denying DACLs.
