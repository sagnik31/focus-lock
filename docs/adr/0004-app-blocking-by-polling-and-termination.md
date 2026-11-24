# 0004. Block apps by polling and terminating processes

- **Status:** Accepted
- **Date:** 2025-12-26

## Context

Users pick apps to block (for example `discord.exe`, or an app from the installed-apps
picker). Blocking has to start and stop with a session and stay out of the way the rest of
the time. It should also catch renamed or helper executables of the same product.

## Decision

- The enforcer **polls the process list and terminates matches**:
  - every **500 ms**, an exact, case-insensitive executable-name match;
  - every **5 s**, a "deep" match of each blocked name (without `.exe`, at least three
    characters) against the process's version metadata (product name and file
    description).
- Blocked names are validated by `storage.ValidAppName` when added or imported, so entries
  like `""` or `"."` can't match everything. Processes that Windows or Focus Lock needs
  (`storage.ProtectedApp`) can't be added, and the enforcer never terminates them. The deep
  match also skips everything in the Windows folder.
- Every kill is recorded (per app and per hour) for the Insights panel.

## Alternatives considered

- **Image File Execution Options (`Debugger` key).** Stops a process before it starts, but
  it's a well-known malware technique that antivirus tools flag. It's also global until
  removed, and is left behind if cleanup fails.
- **AppLocker or Software Restriction Policies.** Not available on Windows Home, and they
  apply through Group Policy at a pace that doesn't suit per-session toggling.
- **WMI or ETW process-start events.** React faster than polling, but are more complex and
  fragile. Polling at 500 ms is fast enough for the purpose.
- **Suspending instead of killing.** Leaves the window visible and the app's resources
  held. It also makes it unclear whether the app is blocked.

## Consequences

- A blocked app can run for up to about half a second before it is closed. Any unsaved
  work in it is lost, which the README warns about.
- The deep match can catch a different app whose metadata contains a blocked name. The
  three-character minimum and the skipped Windows folder keep that rare.
- Polling costs a little CPU all the time during a session. Process enumeration is cheap,
  and metadata reads happen only on the slow tick.
- Terminating elevated processes needs the enforcer's elevation.
