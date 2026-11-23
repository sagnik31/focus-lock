# 0008. Ship weekly calendar-versioned releases and update through the installer

- **Status:** Accepted; "one UAC prompt per update" superseded by [0009](0009-silent-updates-via-updater-task.md)
- **Date:** 2026-11-17

## Context

Installing Focus Lock does elevated, one-time work: the scheduled task, the HKLM secret,
Defender exclusions and the `%ProgramData%` folder. Users should install once and then
receive fixes without reinstalling by hand. Releases shouldn't depend on someone
remembering to bump a version. An update must also never become a way out of a session,
since an install stops the enforcer.

## Decision

- **Weekly, automatic releases.** A GitHub Actions workflow runs every Sunday (and on
  demand) from `main`. It skips weeks with no new commits, runs the checks (originally
  `go vet`, `go test` and the frontend typecheck; now the same gate as CI), builds the
  installer, and publishes a GitHub Release. If the gate fails, there's no release.
- **Calendar versions** `YYYY.M.D.N` (tag `vYYYY.M.D.N`). They always increase, need no
  manual bump, and are valid as a Windows file version. The version is stamped into
  `backend/version.Version` at build time. Local builds are `dev` and never update.
- **The installer is the updater.** The app checks the GitHub Releases API (at most once a
  day, plus a manual button). It downloads `FocusLock-Setup-<ver>.exe` and verifies it
  against the release's `SHA256SUMS.txt`, re-checks the hash right before launch, then
  runs Setup silently with one UAC prompt. Setup already knows how to stop the enforcer,
  replace the exe, re-register the task and restart. There's no self-replacing exe.
- **No install during a session.** Checking and downloading are always allowed. Installing
  is refused by the app's backend and by the installer, which runs
  `focus-lock.exe --session-active` and aborts when it exits 3.
- **No code signing for now**, because of the cost. The checksum provides integrity.

## Alternatives considered

- **Manual releases with SemVer.** They depend on someone remembering, and SemVer's meaning
  adds little for an end-user app.
- **A self-updating exe** (swap the binary in place). It would need its own elevated
  replace-and-restart logic, duplicating the installer and its edge cases.
- **Squirrel, WinSparkle or an MSIX update feed.** Bring their own installers and
  conventions that don't fit the elevated, task-registering setup.
- **Allowing installs mid-session.** That would let anyone end a lock by running Setup.

## Consequences

- One UAC prompt per update.
- SmartScreen warns on the first install downloaded in a browser (unsigned). In-app
  updates don't trigger it.
- The checksum comes from the same release, so it catches corruption but not a compromised
  release. A detached signature with an embedded public key would close that gap.
- GitHub disables scheduled workflows after 60 days with no repository activity, so the
  workflow may need re-enabling.
- Old installers that predate `--session-active` can't apply the installer-side gate. The
  app-side check still applies.
