# CLAUDE.md

Guidance for coding agents and contributors working in this repository.

## What this is

Focus Lock is a Windows-only app and website blocker: a Go backend and a React/TypeScript
UI, packaged with Wails v2. During a session it terminates blocked processes, writes
blocked sites to the `hosts` file and Chrome's `URLBlocklist` policy, and resists casual
bypass. It is meant to deter a tempted user, not stop a determined admin.

Read before changing anything substantial:

- [docs/architecture.md](docs/architecture.md): processes, enforcement loop, config
  integrity, operation policy, trusted time, code map.
- [docs/adr/](docs/adr/README.md): the major design decisions and their trade-offs. Don't
  reverse one without adding a new ADR that supersedes it.

If a gitignored `HANDOVER.md` exists in the repo root, it holds the current status and
backlog. Read it too.

## Commands

```bash
wails build                      # build/bin/focus-lock.exe
wails dev                        # hot-reload dev mode

# Verify every change (all must pass):
go build ./...
go vet ./...
staticcheck ./...
go test ./...
cd frontend && npx tsc --noEmit
cd frontend && npm run lint      # ESLint (typescript-eslint + react-hooks)
cd frontend && npm test          # Vitest
```

- `main.go` embeds `frontend/dist`. If it's missing, run `npm run build` in `frontend/`
  before `go vet` or `go test`.
- Installer: `powershell -ExecutionPolicy Bypass -File scripts\build_installer.ps1`
  (needs Inno Setup 6).
- CI (`.github/workflows/ci.yml`) runs these checks on every push and pull request to
  `main`, pinning staticcheck `2025.1.1`.
- Optional pre-push hook with the same checks as the release gate:
  `git config core.hooksPath scripts/hooks`.
- Frontend checks are `tsc`, ESLint (`eslint.config.js`) and Vitest (`src/**/*.test.ts`,
  node environment, no DOM). Put testable UI logic in pure modules such as
  `components/duration.ts` and mock the Wails bindings in tests that import them.
  A disabled lint rule needs a comment saying why.

## Rules you can't infer from the code

### Config and policy

- **Every config change goes through `storage.ApplyOperation`.** Add an `OpType` and a
  policy rule. Never mutate and save the config directly from the bridge.
- **Keep `ApplyOperation` pure.** Callers pass `now` (from `clock.Default.Now()`) and do
  side effects only after it succeeds.
- **During a session the policy may only allow strengthening.** Anything that ends,
  shortens or narrows a block must be refused while a lock or schedule window is active.
- **Validate in the policy, not the UI.** App names go through `storage.ValidAppName`
  and `storage.ProtectedApp`, sites through `hosts.ValidSite`, schedules through
  `storage.NormalizeSchedule`. In strong mode anyone can write a request
  file.
- **Strong mode: the UI never calls `Store.Save`.** It can't sign (`CanSign() == false`).
  Route changes through `applyOp` in `bridge/app.go`, which uses the request channel.
  Anything that records to the config (stats, warnings) must handle `CanSign() == false`.
- **Read-modify-write only through `Store.UpdateAtomic`, `ApplyRequest` or a method that takes
  `lockConfig`.** Two processes sign in reduced mode, so load, change and `Save` can lose a lock.
- **Decode configs with `decodeConfig`**, never over existing data.
- **To ask whether to block now, use `storage.Enforcing`** (it knows about pauses). To ask whether
  a change would weaken a session, use `storage.EnforcementActive`.
- **Read config from the bridge with `Store.Snapshot()`** or the locked getters, not
  `Store.Data`. The enforcer goroutine reloads it concurrently.
- **Use `clock.Default.Now()`**, not `time.Now()`, in enforcement and policy code.
- **Schedule-window logic lives in `storage/schedule.go`** (`SchedulesActiveAt`, which
  handles overnight windows). Reuse it rather than writing another check. The UI gets the
  lock in force and its end time from the bound `GetLockState()` (`storage.LockStateAt`);
  don't reimplement windows in TypeScript.

### Enforcer and Windows

- **Never reintroduce `RtlSetProcessIsCritical`** or terminate-denying DACLs on the
  enforcer. They caused blue screens. Recovery from a kill is the scheduled task's
  one-minute repetition.
- **The enforcer task XML is defined in three places that must stay identical:**
  `backend/scheduler/service.go`, `scripts/install_setup.ps1` and
  `scripts/setup_persistence.ps1`. The `FocusLock Updater` task (ADR 0009) is defined only
  in `scripts/install_setup.ps1`; the app never registers it. Setup must never end it,
  because a silent update runs Setup under it.
- **`--apply-update` runs elevated, so it must not trust anything the user can write.**
  It takes no arguments, fetches and verifies the release itself, and runs Setup only from
  the admin-only `%ProgramData%\FocusLock\update`.
- **The same goes for setup:** never pass the elevated side a file from `%TEMP%` or `%APPDATA%`
  without holding it open and checking it. Setup must provision strong mode before it registers
  the enforcer task, which can fire at once.
- **The enforcer touches `%APPDATA%` only through `userfs.Do`** (in `storage`, through
  `readUser`/`writeUser`/`removeUser`). It runs elevated in a folder the user controls, so a
  plain `os.*` call there can be redirected by a planted link. Files in `secureDir` and
  `%ProgramData%` are accessed directly. Don't hand a user-folder path to anything elevated
  outside `userfs.Do`.
- **The hosts cleanup exists twice** and must match: `stripBlock` in `blocking/hosts/manager.go`
  and `Remove-FocusLockLines` in `scripts/install_setup.ps1`.
- **Releases must be signed.** The workflow signs `SHA256SUMS.txt` with the
  `RELEASE_SIGNING_KEY` secret, and the app refuses unsigned releases. If you rotate the key,
  change `releasePublicKeyB64` in `backend/updater/signature.go` in the same release
  (`go run ./cmd/signsums -genkey -out <file outside the repo>`).
- **Keep the `*_other.go` `!windows` stubs** so the packages without Windows imports build on
  other platforms. `main`, `bridge`, `storage`, `sysinfo` and `watchdog` build only on Windows.
- **Spawned console tools** (`schtasks`, PowerShell, `ipconfig`) must set `CREATE_NO_WINDOW`
  (and `HideWindow`) in `SysProcAttr` so no console flashes. See `bridge/spawn_windows.go`,
  or use the `scheduler.command` helper (`scheduler/console_windows.go`) for `schtasks`.

### Tests

- Put logic in pure functions in `storage` (or a small pure helper, as in
  `watchdog/match.go`) and unit-test it there. No GUI or Windows state is needed.
- **Never call `storage.NewStore()` in tests.** It touches the real registry. Build a
  `Store` literal with `filePath` in `t.TempDir()`, `activeSecret`, `canSign: true` and a
  nil `regStore` (see `testSigningStore` in `backend/storage/concurrency_test.go`). Tests in
  other packages use `storage.NewLocalStore(path, secret)`, which builds the same thing.

### Frontend and bindings

- `frontend/wailsjs/` is generated by `wails build`. When you add a bound method you can
  hand-edit `wailsjs/go/bridge/App.{d.ts,js}` to keep `tsc` passing, and the next build
  reconciles them.
- Bound methods that return a JSON string (for example `GetInsights`, the update API)
  avoid adding generated models.
- Don't commit unrelated regeneration noise in `frontend/wailsjs/go/models.ts`. Stage it
  only when your change actually alters a bound struct.

### Formatting and shell

- Run `gofmt -w` on touched Go files. With `core.autocrlf=true` the working tree is CRLF
  and `gofmt -l` lists every file. Check with `tr -d '\r' < file.go | gofmt -l` instead.
- Windows PowerShell 5.1: under `$ErrorActionPreference = 'Stop'`,
  `native.exe ... 2>$null` turns any stderr output into a terminating error. Use cmdlets
  (`Get-ScheduledTask` and so on), or set `Continue` around the call and check
  `$LASTEXITCODE`.

## What needs a real machine

Unit tests and `tsc` can't cover these. Flag them for manual testing on Windows with admin
rights, using [installer/TESTING.md](installer/TESTING.md):

- enforcer startup and lifecycle, the scheduled task, strong-mode setup;
- `hosts` and Chrome policy writes, and process killing;
- anything in `installer/` or the elevated `scripts/*.ps1`;
- the update flow and any UI change.

## Repository hygiene

- Tracked: `build/windows/` and `build/appicon.png` (CI needs them).
- Ignored: `build/bin/`, `*.exe`, `installer/Output/`, `frontend/dist`,
  `frontend/package.json.md5`, `node_modules`, `HANDOVER.md`, `.claude/gap-audits/`,
  `.claude/skills/`, `.claude/settings.local.json`.
- Releases are cut automatically every week from `main` by `.github/workflows/release.yml`.
  Don't bump versions by hand.
- When you make a new major design decision, add an ADR in `docs/adr/` and list it in
  `docs/adr/README.md`. When behaviour changes, update `docs/architecture.md` or
  `docs/install.md`.
