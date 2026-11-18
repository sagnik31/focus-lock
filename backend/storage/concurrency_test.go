package storage

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// testSigningStore is a reduced-mode (signing) store on a temp dir with no
// registry backup, so tests never touch the real config or registry.
func testSigningStore(t *testing.T) *Store {
	t.Helper()
	return &Store{
		filePath:     filepath.Join(t.TempDir(), "config.json"),
		activeSecret: []byte("test-secret"),
		canSign:      true,
	}
}

// The UI's reduced-mode applyOp uses ApplyRequest. Run against the
// enforcer's per-tick UpdateAtomic writes on the same store, no operation
// may be lost (the old Load / mutate / Save sequence could drop them).
func TestApplyRequestConcurrentWithEnforcerWrites(t *testing.T) {
	s := testSigningStore(t)
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}

	const n = 40
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { // the enforcer's slow-tick write
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			_ = s.UpdateAtomic(func(c *Config) { c.RemainingDuration = time.Duration(i) })
		}
	}()

	var ops sync.WaitGroup
	for i := 0; i < n; i++ {
		ops.Add(1)
		go func(i int) {
			defer ops.Done()
			op := Operation{Type: OpAddApp, Name: fmt.Sprintf("app%02d.exe", i)}
			if err := s.ApplyRequest(op, time.Now()); err != nil {
				t.Errorf("ApplyRequest: %v", err)
			}
		}(i)
	}
	ops.Wait()
	close(stop)
	wg.Wait()

	if err := s.Load(); err != nil {
		t.Fatal(err)
	}
	if got := len(s.Snapshot().BlockedApps); got != n {
		t.Errorf("BlockedApps has %d entries, want %d (operations were lost)", got, n)
	}
}

func TestSnapshotIsIndependentCopy(t *testing.T) {
	s := &Store{Data: Config{BlockedApps: []string{"b.exe", "a.exe"}, BlockedSites: []string{}}}
	c := s.Snapshot()
	c.BlockedApps[0] = "changed.exe"
	if s.Data.BlockedApps[0] != "b.exe" {
		t.Error("mutating the snapshot changed the store")
	}
	if c.BlockedSites == nil {
		t.Error("an empty slice must stay empty (not nil) so the UI keeps getting []")
	}
	if c.Schedules != nil {
		t.Error("a nil slice must stay nil")
	}
}
