package storage

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// G45: in the repeated hour the window's end is in the future while it is active.
func TestScheduleEndInRepeatedHour(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip(err)
	}
	s := []Schedule{{ID: "s", Enabled: true, Days: []string{"Sun"}, StartTime: "01:00", EndTime: "01:30"}}
	first := time.Date(2026, 11, 1, 1, 15, 0, 0, ny)
	for _, now := range []time.Time{first, first.Add(time.Hour)} {
		end, ok := ActiveScheduleEnd(s, now)
		if !ok || end.Sub(now) != 15*time.Minute {
			t.Errorf("at %v: end %v (active %v), want 15m later", now, end, ok)
		}
	}
}

// G46: a schedule session ends when its window did, even if the PC was off.
func TestScheduleSessionCappedAtWindowEnd(t *testing.T) {
	start := at(0, 12, 0)
	newCfg := func() *Config {
		cfg := &Config{Schedules: []Schedule{{ID: "s", Enabled: true, Days: weekDays, StartTime: "09:00", EndTime: "13:00"}}}
		recordScheduleSessionEdge(cfg, start, true)
		return cfg
	}

	cfg := newCfg()
	recordScheduleSessionEdge(cfg, at(0, 20, 0), false)
	if n := len(cfg.Stats.Sessions); n != 1 || cfg.Stats.Sessions[0].DurationSec != 3600 {
		t.Fatalf("sessions %+v, want one of 3600s", cfg.Stats.Sessions)
	}

	// Back on during the next day's window: the old session is closed, a new one starts.
	cfg = newCfg()
	next := at(1, 10, 0)
	recordScheduleSessionEdge(cfg, next, true)
	if n := len(cfg.Stats.Sessions); n != 1 || cfg.Stats.Sessions[0].DurationSec != 3600 {
		t.Fatalf("sessions %+v, want one of 3600s", cfg.Stats.Sessions)
	}
	if !cfg.ScheduleSessionStart.Equal(next) {
		t.Fatalf("new session start %v, want %v", cfg.ScheduleSessionStart, next)
	}
}

// G47: a reload replaces the config; nothing of the old one is left.
func TestLoadReplacesStats(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	a, b := NewLocalStore(path, []byte("k")), NewLocalStore(path, []byte("k"))
	a.Data.Stats.KillCounts = map[string]int{"before.exe": 1}
	if err := a.Save(); err != nil {
		t.Fatal(err)
	}
	_ = b.Load()
	a.Data.Stats.KillCounts = map[string]int{"after.exe": 1}
	if err := a.Save(); err != nil {
		t.Fatal(err)
	}
	_ = b.Load()
	if want := map[string]int{"after.exe": 1}; !reflect.DeepEqual(b.Data.Stats.KillCounts, want) {
		t.Fatalf("KillCounts = %v, want %v", b.Data.Stats.KillCounts, want)
	}
}

// G48: a refused import changes nothing.
func TestRefusedImportLeavesConfig(t *testing.T) {
	cfg := &Config{BlockedApps: []string{"before.exe"}}
	err := ApplyOperation(cfg, Operation{Type: OpImport, Apps: []string{"after.exe"}, Sites: []string{"x.com"},
		SiteGroups: []SiteGroup{{Name: "no id"}}}, time.Now())
	if err == nil || !reflect.DeepEqual(cfg, &Config{BlockedApps: []string{"before.exe"}}) {
		t.Fatalf("err %v, cfg %+v", err, cfg)
	}
}

// G50: entries that no blocking layer could act on are refused.
func TestInvalidSitesRefused(t *testing.T) {
	cfg := &Config{}
	for _, op := range []Operation{
		{Type: OpAddSite, URL: "youtube"},
		{Type: OpAddSites, Sites: []string{"x.com", "http://"}},
	} {
		if err := ApplyOperation(cfg, op, time.Now()); err == nil || len(cfg.BlockedSites) != 0 {
			t.Errorf("%+v: err %v, sites %v", op, err, cfg.BlockedSites)
		}
	}
	_ = ApplyOperation(cfg, Operation{Type: OpImport, Sites: []string{"junk", " x.com "}}, time.Now())
	if !reflect.DeepEqual(cfg.BlockedSites, []string{"x.com"}) {
		t.Errorf("import kept %v", cfg.BlockedSites)
	}
}

// G51: a request whose submitter gave up is taken back, and one left over from
// an earlier run is dropped.
func TestExpiredRequestNotApplied(t *testing.T) {
	dir := t.TempDir()
	ui := &Store{filePath: filepath.Join(dir, "config.json")}
	if err := ui.SubmitRequest(Operation{Type: OpStopFocus}, 60*time.Millisecond); err == nil {
		t.Fatal("no error without a Ghost")
	}
	if _, err := os.Stat(requestPath(dir)); !os.IsNotExist(err) {
		t.Fatal("the timed-out request was left for the Ghost")
	}
	if err := writeJSONAtomic(requestPath(dir), Request{ID: "old", Expires: time.Now().Add(-time.Second).UnixMilli()}); err != nil {
		t.Fatal(err)
	}
	if _, ok := TakePendingRequest(dir); ok {
		t.Fatal("an expired request was taken")
	}
}

// G55: the longest accepted session, and one second more.
func TestMaxFocusSeconds(t *testing.T) {
	now := time.Now()
	cfg := &Config{}
	if err := ApplyOperation(cfg, Operation{Type: OpStartFocus, Seconds: MaxFocusSeconds + 1}, now); err == nil {
		t.Fatal("too long a session was accepted")
	}
	if err := ApplyOperation(cfg, Operation{Type: OpStartFocus, Seconds: MaxFocusSeconds}, now); err != nil || !cfg.LockEndTime.After(now) {
		t.Fatalf("err %v, end %v", err, cfg.LockEndTime)
	}
}

// G57: a second click during a pause costs nothing. G52: a pause suspends enforcing.
func TestUnlockDuringPauseIsFree(t *testing.T) {
	now := time.Now()
	cfg := &Config{LockEndTime: now.Add(time.Hour)}
	for i := 0; i < 2; i++ {
		if err := ApplyOperation(cfg, Operation{Type: OpEmergencyUnlock}, now.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	if cfg.EmergencyUnlocksUsed != 1 {
		t.Fatalf("used %d, want 1", cfg.EmergencyUnlocksUsed)
	}
	if Enforcing(cfg, now) || !Enforcing(cfg, now.Add(2*time.Minute)) {
		t.Fatal("Enforcing ignores the pause")
	}
}
