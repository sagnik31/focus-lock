package bridge

import (
	"path/filepath"
	"testing"
	"time"

	"focus-lock/backend/clock"
	"focus-lock/backend/storage"
)

// stubUnblock replaces the hosts unblock with a counter.
func stubUnblock(t *testing.T) *int {
	t.Helper()
	calls := 0
	prev := unblockHosts
	unblockHosts = func() error { calls++; return nil }
	t.Cleanup(func() { unblockHosts = prev })
	return &calls
}

func signingApp(t *testing.T, lockEnd time.Time) *App {
	t.Helper()
	s := storage.NewLocalStore(filepath.Join(t.TempDir(), "config.json"), []byte("test-secret"))
	s.Data.LockEndTime = lockEnd
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	return &App{Store: s}
}

// G14: a stop the policy refuses must not lift the website block.
func TestStopFocusRefusedKeepsBlock(t *testing.T) {
	calls := stubUnblock(t)
	a := signingApp(t, clock.Default.Now().Add(time.Hour))

	if err := a.StopFocus(); err == nil {
		t.Fatal("StopFocus succeeded during an active lock")
	}
	if *calls != 0 {
		t.Fatalf("hosts unblocked %d time(s) on a refused stop", *calls)
	}
	if a.Store.Snapshot().LockEndTime.IsZero() {
		t.Fatal("refused stop cleared the lock")
	}
}

func TestStopFocusAfterExpiryUnblocks(t *testing.T) {
	calls := stubUnblock(t)
	a := signingApp(t, clock.Default.Now().Add(-time.Minute))

	if err := a.StopFocus(); err != nil {
		t.Fatalf("StopFocus after expiry: %v", err)
	}
	if *calls != 1 {
		t.Fatalf("hosts unblocked %d time(s), want 1", *calls)
	}
}
