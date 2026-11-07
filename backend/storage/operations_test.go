package storage

import (
	"testing"
	"time"
)

func lockedConfig(now time.Time) *Config {
	return &Config{
		BlockedApps:  []string{"chrome.exe", "steam.exe"},
		BlockedSites: []string{"reddit.com"},
		Schedules:    []Schedule{{ID: "s1", Enabled: true, Days: []string{now.Format("Mon")}, StartTime: "00:00", EndTime: "23:59"}},
		LockEndTime:  now.Add(1 * time.Hour),
	}
}

func idleConfig() *Config {
	return &Config{
		BlockedApps:  []string{"chrome.exe"},
		BlockedSites: []string{"reddit.com"},
	}
}

func TestPolicyRefusesWeakeningDuringLock(t *testing.T) {
	now := time.Now()
	weakening := []Operation{
		{Type: OpStopFocus},
		{Type: OpRemoveApp, Name: "chrome.exe"},
		{Type: OpSetApps, Apps: []string{"chrome.exe"}}, // drops steam.exe
		{Type: OpRemoveSite, URL: "reddit.com"},
		{Type: OpRemoveSites, Sites: []string{"reddit.com"}},
		{Type: OpSetVPN, Enabled: false},
		{Type: OpSaveSchedules, Schedules: []Schedule{}}, // deletes the active schedule
	}
	for _, op := range weakening {
		cfg := lockedConfig(now)
		cfg.BlockCommonVPN = true
		if err := ApplyOperation(cfg, op, now); err == nil {
			t.Errorf("op %q should be refused during an active lock, but was allowed", op.Type)
		}
	}
}

func TestPolicyAllowsStrengtheningDuringLock(t *testing.T) {
	now := time.Now()
	strengthening := []Operation{
		{Type: OpAddApp, Name: "discord.exe"},
		{Type: OpAddSite, URL: "twitter.com"},
		{Type: OpAddSites, Sites: []string{"youtube.com"}},
		{Type: OpSetVPN, Enabled: true},
		{Type: OpSetApps, Apps: []string{"chrome.exe", "steam.exe", "discord.exe"}}, // superset
		{Type: OpImport, Apps: []string{"x.exe"}, Sites: []string{"y.com"}},
		{Type: OpStartFocus, Seconds: 7200}, // extend
	}
	for _, op := range strengthening {
		cfg := lockedConfig(now)
		if err := ApplyOperation(cfg, op, now); err != nil {
			t.Errorf("op %q should be allowed during an active lock, got: %v", op.Type, err)
		}
	}
}

func TestStopAllowedAfterExpiry(t *testing.T) {
	now := time.Now()
	cfg := idleConfig()
	cfg.LockEndTime = now.Add(-1 * time.Minute) // expired
	if err := ApplyOperation(cfg, Operation{Type: OpStopFocus}, now); err != nil {
		t.Fatalf("stop after expiry should be allowed, got: %v", err)
	}
	if !cfg.LockEndTime.IsZero() {
		t.Error("stop should clear LockEndTime")
	}
}

func TestStartFocusDoesNotShortenActiveLock(t *testing.T) {
	now := time.Now()
	cfg := lockedConfig(now)
	cfg.LockEndTime = now.Add(2 * time.Hour)
	cfg.EmergencyUnlocksUsed = 2
	if err := ApplyOperation(cfg, Operation{Type: OpStartFocus, Seconds: 60}, now); err != nil {
		t.Fatal(err)
	}
	if cfg.LockEndTime.Before(now.Add(2 * time.Hour)) {
		t.Error("a shorter StartFocus must not shorten an active lock")
	}
	if cfg.EmergencyUnlocksUsed != 2 {
		t.Error("extending an active lock must not refill the emergency-unlock quota")
	}
}

func TestEmergencyUnlockLimit(t *testing.T) {
	now := time.Now()
	cfg := lockedConfig(now)
	cfg.EmergencyUnlocksUsed = 0
	for i := 1; i <= 2; i++ {
		if err := ApplyOperation(cfg, Operation{Type: OpEmergencyUnlock}, now); err != nil {
			t.Fatalf("unlock %d should succeed: %v", i, err)
		}
	}
	if err := ApplyOperation(cfg, Operation{Type: OpEmergencyUnlock}, now); err == nil {
		t.Error("third emergency unlock must be refused")
	}
}

func TestEmergencyUnlockRefillsPerScheduleWindow(t *testing.T) {
	cfg := &Config{
		BlockedApps: []string{"chrome.exe"},
		Schedules:   []Schedule{{ID: "s1", Enabled: true, Days: []string{"Mon"}, StartTime: "09:00", EndTime: "17:00"}},
	}

	// Exhaust the quota during the first window instance (Mon 2026-11-02).
	week1 := at(0, 12, 0)
	for i := 1; i <= 2; i++ {
		if err := ApplyOperation(cfg, Operation{Type: OpEmergencyUnlock}, week1); err != nil {
			t.Fatalf("unlock %d in the first window should succeed: %v", i, err)
		}
	}
	if err := ApplyOperation(cfg, Operation{Type: OpEmergencyUnlock}, week1); err == nil {
		t.Fatal("a third unlock within the same schedule window must be refused")
	}

	// The next weekly occurrence is a new session: the quota refills.
	week2 := at(7, 12, 0) // the following Monday, same schedule
	if err := ApplyOperation(cfg, Operation{Type: OpEmergencyUnlock}, week2); err != nil {
		t.Fatalf("first unlock in the next window instance should succeed (quota refilled): %v", err)
	}
	if cfg.EmergencyUnlocksUsed != 1 {
		t.Errorf("quota should have reset to 0 then incremented to 1, got %d", cfg.EmergencyUnlocksUsed)
	}

	// Still only two per window: a fresh occurrence does not grant unlimited unlocks.
	if err := ApplyOperation(cfg, Operation{Type: OpEmergencyUnlock}, week2); err != nil {
		t.Fatalf("second unlock in the new window should succeed: %v", err)
	}
	if err := ApplyOperation(cfg, Operation{Type: OpEmergencyUnlock}, week2); err == nil {
		t.Error("a third unlock in the new window must be refused")
	}
}

func TestIdleAllowsRemoval(t *testing.T) {
	now := time.Now()
	cfg := idleConfig()
	if err := ApplyOperation(cfg, Operation{Type: OpRemoveApp, Name: "chrome.exe"}, now); err != nil {
		t.Fatalf("removal when idle should be allowed: %v", err)
	}
	for _, a := range cfg.BlockedApps {
		if a == "chrome.exe" {
			t.Error("chrome.exe should have been removed")
		}
	}
}
