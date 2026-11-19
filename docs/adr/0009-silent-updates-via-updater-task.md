# 0009. Install updates without a UAC prompt through an updater task, and sign releases

- **Status:** Accepted
- **Date:** 2026-11-20
- **Supersedes:** the "one UAC prompt per update" part of [0008](0008-weekly-releases-installer-as-updater.md)

## Context

ADR 0008 made the installer the updater and accepted one UAC prompt per update. With weekly
releases that is a prompt almost every week, for software the user already chose to trust
with admin rights at install time. Install is the one moment we have those rights, so it can
leave behind a way to apply later updates.

Without a human approving a prompt, the release itself has to be trustworthy. Under 0008 the
checksum comes from the same GitHub release as the installer, so it catches corruption but
not a compromised account or workflow.

## Decision

- **An on-demand updater task.** The installer (`scripts/install_setup.ps1`) registers
  `FocusLock Updater`: no trigger, highest available run level, running the installed
  `focus-lock.exe --apply-update`. Its security descriptor lets the signed-in user read and
  run it and only SYSTEM/Administrators change it. The app never registers it, and dev
  builds never update.
- **The elevated side trusts nothing from the user.** `--apply-update` fetches the latest
  release from GitHub itself, refuses anything not newer than the installed exe, checks the
  signed checksums, and keeps the installer in the admin-only
  `%ProgramData%\FocusLock\update`. It may reuse the app's own download only after copying
  it there and matching it against the signed checksum. It checks for a focus session
  before and after the download, then starts Setup silently with `/UPDATE=2` and exits.
  The exit code is the task's last result, which the app reads back.
- **Fallback.** When the task is missing or can't be run (reduced mode, a failed setup, or an
  install from before this change), the app keeps 0008's single UAC prompt.
- **Relaunch through the shell.** A Setup started by the task has no unelevated parent, so
  `runasoriginaluser` can't drop elevation. With `/UPDATE=2` Setup relaunches the app
  through `explorer.exe`, which hands it to the user's running, non-elevated shell.
- **Signed releases.** The release workflow signs `SHA256SUMS.txt` with an Ed25519 key
  (`cmd/signsums`). The private seed is the `RELEASE_SIGNING_KEY` repository secret, and the
  public key is built into the app (`backend/updater/signature.go`). Every update path, with
  or without a prompt, refuses a release without a valid `SHA256SUMS.txt.sig`. The workflow
  fails instead of publishing an unsigned release.

## Alternatives considered

- **A Windows service running as SYSTEM.** That's the classic updater design, but it adds a
  long-running privileged process and its own IPC to protect, for something needed once a
  week.
- **Making the enforcer install updates.** It already runs elevated, but Setup stops the
  enforcer and its task, which would kill Setup with it.
- **Passing the setup path to the elevated side.** That's simpler, but it would let any
  process running as the user get an arbitrary file run elevated.
- **Buying a code-signing certificate.** It costs money and wouldn't remove the prompt by
  itself. A detached signature gives the integrity the silent path needs for free.

## Consequences

- One UAC prompt at install. After that, updates install with a single click and no prompt.
  The first update onto a build with this change still prompts, because the task doesn't
  exist yet.
- Any process running as the user can start the task. All it can do is install the newest
  genuine, signed release outside a session.
- Losing the signing key means generating a new pair and shipping the new public key. One
  update (from the last build with the old key) then needs a manual install. A leaked key
  allows silent malicious updates until it is rotated, so keep it only in the repository
  secret and a password manager.
- The task XML lives only in `install_setup.ps1`. It isn't one of the three copies of the
  enforcer task that must stay identical.
- The shell relaunch and the task's permissions can only be verified on a real machine
  (`installer/TESTING.md` §5.8).
