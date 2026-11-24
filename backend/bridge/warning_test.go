package bridge

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"focus-lock/backend/storage"
)

// A zero-value Store can't sign, like a strong-mode UI, and Load is a no-op.
func strongModeApp() *App { return &App{Store: &storage.Store{}} }

func stubSpawn(t *testing.T, running bool, err error) *int {
	t.Helper()
	calls := 0
	prevSpawn, prevRunning := spawnEnforcer, ghostRunning
	spawnEnforcer = func(string, string) error { calls++; return err }
	ghostRunning = func() bool { return running }
	prevClock := startClock
	startClock = func() {} // no network in tests
	t.Cleanup(func() { spawnEnforcer, ghostRunning, startClock = prevSpawn, prevRunning, prevClock })
	return &calls
}

// G11: in strong mode the warning survives the next Load and reaches GetConfig.
func TestStrongModeWarningSurvivesLoad(t *testing.T) {
	a := strongModeApp()
	a.warnEnforcement("Enforcement process failed to start", errors.New("boom"))
	a.Store.Load()
	got := a.GetConfig().LastEnforcementError
	if got != "Enforcement process failed to start: boom" {
		t.Fatalf("GetConfig warning = %q", got)
	}
	a.clearLocalWarning()
	if got := a.GetConfig().LastEnforcementError; got != "" {
		t.Fatalf("after clear = %q", got)
	}
}

// G11: Startup reports a failed enforcer spawn in strong mode.
func TestStrongModeStartupReportsSpawnFailure(t *testing.T) {
	calls := stubSpawn(t, false, errors.New("access denied"))
	a := strongModeApp()
	a.Startup(t.Context())
	if *calls != 1 {
		t.Fatalf("spawn called %d times", *calls)
	}
	if got := a.GetConfig().LastEnforcementError; !strings.Contains(got, "access denied") {
		t.Fatalf("GetConfig warning = %q", got)
	}
}

// G26: a mutex that only looks like the enforcer's must not stop the UI from
// asking the task to run (a no-op while the enforcer is alive).
// G35: the UI keeps its own trusted clock synced.
func TestStrongModeStartupAlwaysAsksTask(t *testing.T) {
	calls := stubSpawn(t, true, nil)
	clocks := 0
	startClock = func() { clocks++ }
	a := strongModeApp()
	a.localWarning = "stale"
	a.Startup(t.Context())
	if *calls != 1 || clocks != 1 {
		t.Fatalf("spawn called %d times, clock started %d times; want 1 and 1", *calls, clocks)
	}
	if got := a.GetConfig().LastEnforcementError; got != "" {
		t.Fatalf("stale warning kept: %q", got)
	}
}

// In reduced mode the warning goes to the signed config, not memory.
func TestReducedModeWarningIsSaved(t *testing.T) {
	s := storage.NewLocalStore(filepath.Join(t.TempDir(), "config.json"), []byte("test-secret"))
	a := &App{Store: s}
	a.warnEnforcement("ctx", errors.New("err"))
	if a.localWarning != "" {
		t.Fatalf("reduced mode kept a local warning: %q", a.localWarning)
	}
	s.Data = storage.Config{}
	if err := s.Load(); err != nil {
		t.Fatal(err)
	}
	if got := a.GetConfig().LastEnforcementError; got != "ctx: err" {
		t.Fatalf("saved warning = %q", got)
	}
}

func TestMergeWarnings(t *testing.T) {
	cases := []struct{ stored, local, want string }{
		{"", "", ""},
		{"a", "", "a"},
		{"", "b", "b"},
		{"a", "b", "a; b"},
		{"a; b", "b", "a; b"},
	}
	for _, c := range cases {
		if got := mergeWarnings(c.stored, c.local); got != c.want {
			t.Errorf("mergeWarnings(%q, %q) = %q, want %q", c.stored, c.local, got, c.want)
		}
	}
}
