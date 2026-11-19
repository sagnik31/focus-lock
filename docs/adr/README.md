# Architecture decision records

Each record covers one major decision: the problem, what was chosen, what else was
considered, and what it costs. Each one stands on its own. How the pieces fit together is in
[architecture.md](../architecture.md).

Records 0001–0008 were written retroactively in November 2026 from the commit history and
design notes. Their dates are when the decision landed.

| # | Decision | Status |
| --- | --- | --- |
| [0001](0001-wails-go-react-windows-only.md) | Build a Windows-only desktop app with Wails (Go + React) | Accepted |
| [0002](0002-enforcer-as-scheduled-task.md) | Run enforcement in a separate process started by a repeating scheduled task | Accepted |
| [0003](0003-website-blocking-hosts-and-chrome-policy.md) | Block websites with the `hosts` file plus Chrome's `URLBlocklist` policy | Accepted |
| [0004](0004-app-blocking-by-polling-and-termination.md) | Block apps by polling and terminating processes | Accepted |
| [0005](0005-signed-config-enforcer-as-sole-signer.md) | Sign the config and make the elevated enforcer the only signer | Accepted |
| [0006](0006-strengthen-only-operation-policy.md) | Route every change through one pure policy that only allows strengthening during a session | Accepted |
| [0007](0007-trusted-time-ntp-monotonic.md) | Measure sessions with NTP-anchored monotonic time | Accepted |
| [0008](0008-weekly-releases-installer-as-updater.md) | Ship weekly calendar-versioned releases and update through the installer | Accepted; prompt-per-update superseded by 0009 |
| [0009](0009-silent-updates-via-updater-task.md) | Install updates without a UAC prompt through an updater task, and sign releases | Accepted |

## Adding a record

Copy the shape of an existing record (Status, Date, Context, Decision, Alternatives
considered, Consequences). Number it next in sequence, and keep it readable without the
others. When a decision changes, add a new record and mark the old one
"Superseded by NNNN" rather than rewriting it.
