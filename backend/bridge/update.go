package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"time"

	"focus-lock/backend/clock"
	"focus-lock/backend/storage"
	"focus-lock/backend/updater"
	"focus-lock/backend/version"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// UpdateStateEvent is the Wails event carrying the update state (as the same
// JSON string GetUpdateState returns) whenever it changes.
const UpdateStateEvent = "update:state"

// Update phases shown by the UI (HANDOVER.md §7.5).
const (
	phaseIdle        = "idle"
	phaseChecking    = "checking"
	phaseUpToDate    = "upToDate"
	phaseAvailable   = "available"
	phaseDownloading = "downloading"
	phaseReady       = "ready"
	phaseInstalling  = "installing"
	phaseError       = "error"
)

// errSessionActive is the authoritative refusal to install mid-session. The
// UI greys the button out too, but this check is the one that counts.
var errSessionActive = errors.New("updates can't be installed during a focus session; it will be available when your session ends")

// syncTrustedClock anchors the trusted clock to NTP before the install check,
// waiting at most a few seconds (offline falls back to the running clock).
// A variable so tests can skip the network.
var syncTrustedClock = func() { _ = clock.Default.SyncWithin(3 * time.Second) }

// installAllowed is the install gate: nil only when no manual lock and no
// schedule window is active at now.
func installAllowed(cfg *storage.Config, now time.Time) error {
	if storage.EnforcementActive(cfg, now) {
		return errSessionActive
	}
	return nil
}

// updateState is serialised to the UI. Available/Downloaded are kept across
// an error so the UI can still offer the right retry.
type updateState struct {
	Phase         string `json:"phase"`
	Current       string `json:"current"`
	Dev           bool   `json:"dev"`
	Available     bool   `json:"available"`
	Latest        string `json:"latest"`
	Notes         string `json:"notes"`
	PublishedAt   string `json:"publishedAt"`
	Percent       int    `json:"percent"`
	Downloaded    bool   `json:"downloaded"`
	Error         string `json:"error"`
	LastChecked   string `json:"lastChecked"` // RFC 3339, "" if never
	SessionActive bool   `json:"sessionActive"`
}

// updateManager holds the in-memory update state machine. The verified
// installer's path and hash are kept in memory only: InstallUpdate re-hashes
// the file against this value right before launching it.
type updateManager struct {
	mu        sync.Mutex
	client    *updater.Client
	state     updateState
	release   *updater.Release
	setupPath string
	setupSHA  string
	busy      bool
}

func newUpdateManager() *updateManager {
	m := &updateManager{state: updateState{
		Phase:   phaseIdle,
		Current: version.Version,
		Dev:     version.IsDev(version.Version),
	}}
	if dir, err := updater.DefaultDir(); err == nil {
		m.client = updater.New(dir)
	}
	return m
}

func (a *App) updates() *updateManager {
	a.updOnce.Do(func() { a.upd = newUpdateManager() })
	return a.upd
}

// snapshot returns the state as JSON. Callers hold m.mu.
func (a *App) updateJSON(m *updateManager) string {
	st := m.state
	st.SessionActive = storage.EnforcementActive(&a.Store.Data, clock.Default.Now())
	b, _ := json.Marshal(st)
	return string(b)
}

// publish emits the current state to the UI and returns it. Callers hold m.mu.
func (a *App) publishUpdate(m *updateManager) string {
	s := a.updateJSON(m)
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, UpdateStateEvent, s)
	}
	return s
}

// begin marks the manager busy in phase, or reports false if another update
// action is already running. Callers hold m.mu.
func (m *updateManager) begin(phase string) bool {
	if m.busy {
		return false
	}
	m.busy = true
	m.state.Phase = phase
	m.state.Error = ""
	return true
}

// fail ends a busy action with an error. Callers hold m.mu.
func (m *updateManager) fail(err error) {
	m.busy = false
	m.state.Phase = phaseError
	m.state.Error = err.Error()
}

// GetVersion returns the running build's version ("dev" for local builds).
func (a *App) GetVersion() string {
	return version.Version
}

// GetUpdateState returns the update state as a JSON string.
func (a *App) GetUpdateState() string {
	a.Store.Load()
	m := a.updates()
	m.mu.Lock()
	defer m.mu.Unlock()
	return a.updateJSON(m)
}

// CheckForUpdate asks GitHub for the latest release. It always returns the
// resulting state (JSON); failures are reported in its "error" field.
func (a *App) CheckForUpdate() string {
	return a.checkForUpdate(false)
}

// AutoCheckForUpdate is the quiet start-up check: it only goes to the network
// if the last successful check is at least 24h old, otherwise it restores the
// remembered result. Dev builds never check.
func (a *App) AutoCheckForUpdate() string {
	return a.checkForUpdate(true)
}

func (a *App) checkForUpdate(quiet bool) string {
	m := a.updates()
	m.mu.Lock()
	if m.state.Dev || m.client == nil {
		if !quiet {
			m.state.Phase = phaseError
			m.state.Error = "Updates are turned off in development builds."
		}
		defer m.mu.Unlock()
		return a.publishUpdate(m)
	}
	if quiet {
		if lc, ok := m.client.LoadLastCheck(); !updater.CheckDue(lc, ok, time.Now()) {
			// Re-derive availability: the app may have been updated since.
			if m.release == nil && !m.busy && m.state.Phase == phaseIdle {
				m.applyCheck(updater.CheckResult{
					Available:   lc.Result.Available && version.Compare(lc.Result.Latest, version.Version) > 0,
					Latest:      lc.Result.Latest,
					Current:     version.Version,
					Notes:       lc.Result.Notes,
					PublishedAt: lc.Result.PublishedAt,
				}, lc.CheckedAt)
			}
			defer m.mu.Unlock()
			return a.publishUpdate(m)
		}
	}
	if !m.begin(phaseChecking) {
		defer m.mu.Unlock()
		return a.updateJSON(m)
	}
	a.publishUpdate(m)
	client := m.client
	m.mu.Unlock()

	rel, err := client.Latest(context.Background())

	m.mu.Lock()
	defer m.mu.Unlock()
	if err != nil {
		if quiet {
			// A failed background check is not worth an error banner.
			m.busy = false
			m.state.Phase = phaseIdle
		} else {
			m.fail(err)
		}
		return a.publishUpdate(m)
	}
	res := updater.Compare(version.Version, rel)
	now := time.Now()
	_ = client.SaveLastCheck(updater.LastCheck{CheckedAt: now, Result: res})
	if m.release == nil || m.release.Version() != rel.Version() {
		// A different release than the one downloaded (if any).
		m.setupPath, m.setupSHA = "", ""
		m.state.Downloaded = false
	}
	m.release = rel
	m.busy = false
	m.applyCheck(res, now)
	return a.publishUpdate(m)
}

// applyCheck stores a check result in the state. Callers hold m.mu.
func (m *updateManager) applyCheck(res updater.CheckResult, at time.Time) {
	m.state.Available = res.Available
	m.state.Latest = res.Latest
	m.state.Notes = res.Notes
	m.state.PublishedAt = res.PublishedAt
	m.state.LastChecked = at.UTC().Format(time.RFC3339)
	switch {
	case res.Available && m.state.Downloaded:
		m.state.Phase = phaseReady
	case res.Available:
		m.state.Phase = phaseAvailable
	default:
		m.state.Phase = phaseUpToDate
	}
}

// DownloadUpdate downloads and verifies the available release's installer,
// emitting progress. Allowed during a focus session. Returns the state JSON.
func (a *App) DownloadUpdate() string {
	m := a.updates()
	m.mu.Lock()
	if m.state.Dev || m.client == nil || !m.state.Available {
		defer m.mu.Unlock()
		if !m.busy {
			m.state.Phase = phaseError
			m.state.Error = "There is no update to download. Check for updates first."
		}
		return a.publishUpdate(m)
	}
	if !m.begin(phaseDownloading) {
		defer m.mu.Unlock()
		return a.updateJSON(m)
	}
	m.state.Percent = 0
	a.publishUpdate(m)
	client, rel, want := m.client, m.release, m.state.Latest
	m.mu.Unlock()

	ctx := context.Background()
	var err error
	if rel == nil {
		// Restored from the last-check record: fetch the release details.
		rel, err = client.Latest(ctx)
		if err == nil && rel.Version() != want {
			err = errors.New("a newer release was published; check for updates again")
		}
	}
	var path, sum string
	if err == nil {
		lastPct := -1
		path, sum, err = client.Download(ctx, rel, func(done, total int64) {
			if total <= 0 {
				return
			}
			pct := int(done * 100 / total)
			if pct == lastPct {
				return
			}
			lastPct = pct
			m.mu.Lock()
			m.state.Percent = pct
			a.publishUpdate(m)
			m.mu.Unlock()
		})
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if err != nil {
		m.fail(err)
		return a.publishUpdate(m)
	}
	m.release = rel
	m.setupPath, m.setupSHA = path, sum
	m.busy = false
	m.state.Downloaded = true
	m.state.Percent = 100
	m.state.Phase = phaseReady
	return a.publishUpdate(m)
}

// InstallUpdate installs the downloaded, verified update: through the
// "FocusLock Updater" task without a UAC prompt when it exists, otherwise by
// launching the installer with one prompt. The app then quits (Setup
// relaunches it when done). It is REFUSED while a manual
// lock or schedule window is active: the config is reloaded and checked
// against the trusted clock here, independent of what the UI shows. The
// installer itself refuses mid-session too. Returns the state JSON.
func (a *App) InstallUpdate() string {
	m := a.updates()
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.busy {
		return a.updateJSON(m)
	}
	if m.state.Dev || !m.state.Downloaded || m.setupPath == "" {
		m.state.Phase = phaseError
		m.state.Error = "Download the update before installing it."
		return a.publishUpdate(m)
	}

	// Authoritative session check: fresh config, trusted (NTP-anchored) time.
	a.Store.Load()
	syncTrustedClock()
	if err := installAllowed(&a.Store.Data, clock.Default.Now()); err != nil {
		m.state.Phase = phaseReady
		m.state.Error = err.Error()
		return a.publishUpdate(m)
	}

	// The update folder is user-writable: re-verify right before launch.
	if err := updater.VerifyFile(m.setupPath, m.setupSHA); err != nil {
		_ = os.Remove(m.setupPath)
		m.setupPath, m.setupSHA = "", ""
		m.state.Downloaded = false
		m.state.Phase = phaseError
		if errors.Is(err, updater.ErrChecksumMismatch) {
			m.state.Error = "The downloaded installer was changed or damaged, so it was deleted and not run. Download the update again."
		} else {
			m.state.Error = "The downloaded installer could not be read; download the update again."
		}
		return a.publishUpdate(m)
	}

	m.state.Phase = phaseInstalling
	m.state.Error = ""
	a.publishUpdate(m)

	// Preferred: the "FocusLock Updater" task registered at install time
	// installs the update elevated without a UAC prompt (ADR 0009). It
	// re-fetches and re-verifies the release itself, so the file checked
	// above is only a shortcut it may reuse.
	started := time.Now().Add(-2 * time.Second) // LastRunTime has 1s precision
	if err := runUpdaterTask(); err == nil {
		m.busy = true
		go a.waitForUpdaterTask(m, started)
		return a.updateJSON(m)
	}

	// Fallback (no task: reduced mode, or installed before ADR 0009): run
	// the verified setup with one UAC prompt.
	if err := launchSetup(m.setupPath); err != nil {
		m.state.Phase = phaseReady
		m.state.Error = err.Error()
		return a.publishUpdate(m)
	}

	// Setup is running (it kills any focus-lock.exe in PrepareToInstall
	// anyway); quit so it can replace the exe cleanly.
	m.busy = true
	a.quitSoon()
	return a.updateJSON(m)
}

// Seams for tests.
var (
	runUpdaterTask    = updater.RunUpdaterTask
	updaterTaskStatus = updater.UpdaterTaskStatus
	launchSetup       = updater.Launch
	taskPollInterval  = time.Second
	taskStartTimeout  = 30 * time.Second // the task never showed up as run
	taskRunTimeout    = 20 * time.Minute // matches --apply-update's own deadline
)

// quitSoon quits the app shortly, so Setup can replace the exe.
func (a *App) quitSoon() {
	if a.ctx != nil {
		ctx := a.ctx
		go func() {
			time.Sleep(500 * time.Millisecond)
			runtime.Quit(ctx)
		}()
	}
}

// waitForUpdaterTask follows the updater task run started at (or after)
// started. Exit code 0 means Setup is running: the app quits and Setup
// relaunches it. Any other code is shown as an error and the downloaded
// update stays ready to retry.
func (a *App) waitForUpdaterTask(m *updateManager, started time.Time) {
	seenRunning := false
	for {
		time.Sleep(taskPollInterval)
		st, err := updaterTaskStatus()
		elapsed := time.Since(started)
		switch {
		case err == nil && st.Running:
			seenRunning = true
		case err == nil && (seenRunning || !st.LastRun.Before(started)):
			a.finishUpdaterTask(m, int(st.LastResult))
			return
		case !seenRunning && elapsed > taskStartTimeout:
			a.failUpdaterTask(m, "The updater did not start. Try again, or reinstall Focus Lock from the Releases page.")
			return
		case elapsed > taskRunTimeout:
			a.failUpdaterTask(m, "The updater is taking too long. Try again later.")
			return
		}
	}
}

func (a *App) finishUpdaterTask(m *updateManager, code int) {
	if code == updater.ApplyStarted {
		a.quitSoon()
		return
	}
	a.failUpdaterTask(m, updater.ApplyExitMessage(code))
}

func (a *App) failUpdaterTask(m *updateManager, msg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.busy = false
	m.state.Phase = phaseReady
	m.state.Error = msg
	a.publishUpdate(m)
}
