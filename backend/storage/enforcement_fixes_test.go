package storage

import (
	"sync"
	"testing"
	"time"
)

// G29: an idle pause must not carry over into a new lock.
func TestPauseNeedsSessionAndNeverCarriesOver(t *testing.T) {
	now := time.Now()
	cfg := &Config{}
	if err := ApplyOperation(cfg, Operation{Type: OpEmergencyUnlock}, now); err == nil {
		t.Error("emergency unlock allowed while idle")
	}
	cfg.PausedUntil = now.Add(time.Minute)
	if err := ApplyOperation(cfg, Operation{Type: OpStartFocus, Seconds: 60}, now); err != nil {
		t.Fatal(err)
	}
	if !cfg.PausedUntil.IsZero() {
		t.Error("a pause carried over into a fresh lock")
	}
}

// G27: processes Windows or Focus Lock needs can't be blocked.
func TestProtectedApps(t *testing.T) {
	for _, n := range []string{"explorer.exe", "SVCHOST", " lsass.exe ", "Focus-Lock.exe", "msedgewebview2.exe"} {
		if err := ApplyOperation(&Config{}, Operation{Type: OpAddApp, Name: n}, time.Now()); err == nil {
			t.Errorf("%q can be blocked", n)
		}
	}
	got := ValidApps([]string{"explorer.exe", "notepad.exe", "msedge.exe"})
	if len(got) != 2 {
		t.Errorf("ValidApps = %v", got)
	}
}

// G28: separate stores on one file stand in for the UI and enforcer processes.
func TestConcurrentStoresDoNotLoseWrites(t *testing.T) {
	a := testSigningStore(t)
	if err := a.Save(); err != nil {
		t.Fatal(err)
	}
	b := NewLocalStore(a.filePath, a.activeSecret)
	const n = 20
	var wg sync.WaitGroup
	for _, s := range []*Store{a, b} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range n {
				if err := s.UpdateAtomic(func(c *Config) { c.Stats.TotalFocusSeconds++ }); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
	if err := a.Load(); err != nil {
		t.Fatal(err)
	}
	if got := a.Data.Stats.TotalFocusSeconds; got != 2*n {
		t.Errorf("%d of %d writes survived", got, 2*n)
	}
}

// G28: the expiry cleanup must not clear a lock committed since its last load.
func TestFinishExpiredLockKeepsNewLock(t *testing.T) {
	now := time.Now()
	ghost := testSigningStore(t)
	ghost.Data.LockEndTime = now.Add(-time.Minute)
	ghost.Data.SessionStart = now.Add(-time.Hour)
	if err := ghost.Save(); err != nil {
		t.Fatal(err)
	}
	ui := NewLocalStore(ghost.filePath, ghost.activeSecret)
	if err := ui.ApplyRequest(Operation{Type: OpStartFocus, Seconds: 3600}, now); err != nil {
		t.Fatal(err)
	}
	ghost.FinishExpiredLock(now)
	if !ManualLockActive(&ghost.Data, now) {
		t.Fatal("new lock cleared")
	}
	ss := ghost.Data.Stats.Sessions
	if len(ss) != 1 || ss[0].DurationSec != 59*60 {
		t.Errorf("expired session not recorded up to its end: %+v", ss)
	}
	ghost.FinishExpiredLock(now.Add(2 * time.Hour))
	if !ghost.Data.LockEndTime.IsZero() || len(ghost.Data.Stats.Sessions) != 2 {
		t.Error("expired lock not finished")
	}
}
