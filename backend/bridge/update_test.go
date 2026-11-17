package bridge

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"focus-lock/backend/storage"
)

func TestInstallAllowed(t *testing.T) {
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.Local) // a Tuesday
	idle := &storage.Config{}
	if err := installAllowed(idle, now); err != nil {
		t.Fatalf("idle: %v", err)
	}
	locked := &storage.Config{LockEndTime: now.Add(time.Minute)}
	if err := installAllowed(locked, now); err != errSessionActive {
		t.Fatalf("manual lock: %v", err)
	}
	expired := &storage.Config{LockEndTime: now.Add(-time.Minute)}
	if err := installAllowed(expired, now); err != nil {
		t.Fatalf("expired lock: %v", err)
	}
	sched := &storage.Config{Schedules: []storage.Schedule{{
		ID: "s", Enabled: true, Days: []string{"Tue"}, StartTime: "11:00", EndTime: "13:00",
	}}}
	if err := installAllowed(sched, now); err != errSessionActive {
		t.Fatalf("schedule window: %v", err)
	}
	if err := installAllowed(sched, now.Add(2*time.Hour)); err != nil {
		t.Fatalf("after schedule window: %v", err)
	}
}

func decodeState(t *testing.T, s string) updateState {
	t.Helper()
	var st updateState
	if err := json.Unmarshal([]byte(s), &st); err != nil {
		t.Fatalf("bad state JSON %q: %v", s, err)
	}
	return st
}

// newTestApp returns an App whose zero-value Store has no file behind it, so
// Load is a no-op and Data is whatever the test sets.
func newTestApp(t *testing.T) *App {
	t.Helper()
	prev := syncTrustedClock
	syncTrustedClock = func() {}
	t.Cleanup(func() { syncTrustedClock = prev })
	return &App{Store: &storage.Store{}}
}

func TestInstallUpdateRefusedDuringSession(t *testing.T) {
	a := newTestApp(t)
	setup := filepath.Join(t.TempDir(), "FocusLock-Setup-2026.11.17.0.exe")
	if err := os.WriteFile(setup, []byte("setup"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := a.updates()
	m.state.Dev = false
	m.state.Available = true
	m.state.Downloaded = true
	m.state.Phase = phaseReady
	m.setupPath = setup
	m.setupSHA = "0000" // never reached: the session gate comes first

	a.Store.Data.LockEndTime = time.Now().Add(time.Hour)
	st := decodeState(t, a.InstallUpdate())
	if st.Phase != phaseReady || !strings.Contains(st.Error, "focus session") || !st.SessionActive {
		t.Fatalf("manual lock: %+v", st)
	}

	a.Store.Data.LockEndTime = time.Time{}
	now := time.Now()
	a.Store.Data.Schedules = []storage.Schedule{{
		ID: "s", Enabled: true, Days: []string{now.Weekday().String()[:3]},
		StartTime: "00:00", EndTime: "23:59",
	}}
	if now.Hour() == 23 && now.Minute() == 59 {
		t.Skip("schedule window boundary")
	}
	st = decodeState(t, a.InstallUpdate())
	if st.Phase != phaseReady || !strings.Contains(st.Error, "focus session") {
		t.Fatalf("schedule window: %+v", st)
	}
	if _, err := os.Stat(setup); err != nil {
		t.Fatal("refused install touched the downloaded setup")
	}
}

func TestInstallUpdateRejectsTamperedSetup(t *testing.T) {
	a := newTestApp(t)
	setup := filepath.Join(t.TempDir(), "FocusLock-Setup-2026.11.17.0.exe")
	if err := os.WriteFile(setup, []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := a.updates()
	m.state.Dev = false
	m.state.Available = true
	m.state.Downloaded = true
	m.setupPath = setup
	m.setupSHA = strings.Repeat("ab", 32)

	st := decodeState(t, a.InstallUpdate())
	if st.Phase != phaseError || st.Downloaded || !strings.Contains(st.Error, "changed or damaged") {
		t.Fatalf("tampered: %+v", st)
	}
	if _, err := os.Stat(setup); !os.IsNotExist(err) {
		t.Fatal("tampered setup was not deleted")
	}
}

func TestUpdateActionsNeedPriorSteps(t *testing.T) {
	a := newTestApp(t)
	m := a.updates()
	m.state.Dev = false

	if st := decodeState(t, a.DownloadUpdate()); st.Phase != phaseError || st.Error == "" {
		t.Fatalf("download with nothing available: %+v", st)
	}
	if st := decodeState(t, a.InstallUpdate()); st.Phase != phaseError || st.Error == "" {
		t.Fatalf("install with nothing downloaded: %+v", st)
	}

	m.state.Dev = true
	if st := decodeState(t, a.CheckForUpdate()); st.Phase != phaseError || !strings.Contains(st.Error, "development") {
		t.Fatalf("dev check: %+v", st)
	}
	if st := decodeState(t, a.GetUpdateState()); !st.Dev || st.Current == "" {
		t.Fatalf("state: %+v", st)
	}
}
