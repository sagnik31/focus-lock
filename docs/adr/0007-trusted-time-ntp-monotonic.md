# 0007. Measure sessions with NTP-anchored monotonic time

- **Status:** Accepted
- **Date:** 2026-11-07

## Context

Sessions end at a wall-clock time, and schedules depend on the day and time. If enforcement
reads the system clock, moving the Windows clock forward ends a lock early or skips a
schedule window. Changing the clock is easy and is one of the first things a tempted user
tries.

## Decision

- `backend/clock` provides `clock.Default.Now()`. It **anchors to NTP** once
  (`pool.ntp.org`, `time.google.com`, `time.windows.com`), then **advances on Go's
  monotonic clock**, which the Windows time setting can't move, and re-syncs every five
  minutes. It re-anchors on every process start.
- Enforcement and the operation policy use it instead of `time.Now()`. The UI reads it
  through `GetNow()` and offsets its countdown to match.
- **When offline**, it falls back to monotonic time from process start, measured from the
  system clock at that moment. Enforcement keeps going rather than stopping.
- The installer gate does a short, bounded sync (`SyncWithin`) before deciding whether a
  session is active.

## Alternatives considered

- **System clock only.** Defeated by changing the clock.
- **Detect clock jumps and refuse them.** Hard to tell real changes (time-zone travel, NTP
  correction, resuming from sleep) from cheating, and nothing to fall back on.
- **Store elapsed time instead of an end time.** Moves the problem into persistence and
  restarts. Sleep and shutdown make "elapsed" unclear.
- **Authenticated time (Roughtime, NTS).** Stronger against spoofing, but the threat model
  doesn't include an attacker who controls the network.

## Consequences

- Moving the clock while the enforcer runs has no effect.
- **Remaining gap:** kill the enforcer, go offline, and move the clock forward before the
  task relaunches it. That takes deliberate effort and is accepted.
- NTP responses are trusted. Spoofing needs control of the network.
- The app makes outbound NTP requests (UDP 123). It works without them, with weaker
  protection.
