# 0006. Route every change through one pure policy that only allows strengthening during a session

- **Status:** Accepted
- **Date:** 2026-11-03

## Context

Some changes are harmless during a session, such as adding a site. Others undo the lock,
such as removing a site, re-timing the active schedule or ending the timer. The check has
to be correct wherever the change is committed: in the UI process (reduced mode) and in the
enforcer (strong mode, where changes arrive as request files that anyone can write). Rules
spread across UI handlers and the enforcer drifted apart and could be skipped by writing a
request directly.

## Decision

- Every config change is an **`Operation`** (JSON-serialisable) applied by one function,
  **`storage.ApplyOperation(cfg, op, now)`**. It either changes `cfg` or returns an error
  that refuses the change.
- It is **pure**. The caller passes `now`, from trusted time, and handles side effects
  (starting the enforcer, unblocking) only after it succeeds.
- While a lock or schedule window is active, it **allows strengthening**: start or extend a
  lock, add apps or sites, turn on VPN blocking, add schedules, append-only import, and an
  emergency unlock within the quota (two one-minute pauses per session, where each schedule
  window is one session, and overlapping windows and locks share a quota).
- It **refuses weakening**: end or shorten a lock, remove apps or sites, turn off VPN
  blocking, delete, disable or re-time an enabled schedule, or refill the unlock quota with
  a new lock during a schedule window.
- It also **validates input**: app names, sites, schedules and the session length. The
  frontend is not a trust boundary.

## Alternatives considered

- **Checks in the UI only.** Skipped by anyone who writes a request file or edits the
  config in reduced mode.
- **Separate checks in the UI and the enforcer.** Two copies that drift apart.
- **Freeze the whole config during a session.** Simpler, but it blocks harmless and useful
  changes, such as adding a site that just distracted you.

## Consequences

- One function to read, test and change. It's covered by unit tests with no Windows or GUI
  dependencies.
- New mutations need a new `OpType` and a policy rule. They must not write the config
  directly.
- Strictness is a product choice. For example, any enabled schedule can't be re-timed
  during a lock, not only the active one, and an invalid schedule or group rejects the whole import.
- The UI doesn't decide what is locked. Since 2026-11-20 it gets the lock in force and its
  end time from the bound `GetLockState()` (`storage.LockStateAt`). Before that it had its
  own TypeScript copy of the schedule-window logic, which could drift from the Go version.
