package storage

import "testing"

// A schedule window is active Mon 09:00-17:00 (at(0, 12, 0) is Monday noon).
func scheduleLockedConfig() *Config {
	return &Config{
		BlockedApps: []string{"chrome.exe"},
		Schedules: []Schedule{
			{ID: "work", Name: "Work", Enabled: true, Days: []string{"Mon", "Tue"}, StartTime: "09:00", EndTime: "17:00"},
			{ID: "night", Name: "Night", Enabled: true, Days: []string{"Fri"}, StartTime: "22:00", EndTime: "06:00"},
		},
	}
}

func withSchedule(cfg *Config, id string, edit func(*Schedule)) []Schedule {
	out := make([]Schedule, len(cfg.Schedules))
	for i, s := range cfg.Schedules {
		s.Days = append([]string{}, s.Days...)
		if s.ID == id {
			edit(&s)
		}
		out[i] = s
	}
	return out
}

func TestPolicyRefusesRetimingEnabledSchedulesWhileLocked(t *testing.T) {
	now := at(0, 12, 0)
	edits := map[string]func(*Schedule){
		"end moved to the past":   func(s *Schedule) { s.EndTime = "11:00" },
		"start moved after now":   func(s *Schedule) { s.StartTime = "13:00" },
		"today dropped from days": func(s *Schedule) { s.Days = []string{"Tue"} },
		"days emptied":            func(s *Schedule) { s.Days = nil },
	}
	for name, edit := range edits {
		cfg := scheduleLockedConfig()
		op := Operation{Type: OpSaveSchedules, Schedules: withSchedule(cfg, "work", edit)}
		if err := ApplyOperation(cfg, op, now); err == nil {
			t.Errorf("%s: should be refused during an active window", name)
		}
	}

	// Re-timing a different enabled schedule is refused too (it is part of
	// the committed setup while anything is locked).
	cfg := scheduleLockedConfig()
	op := Operation{Type: OpSaveSchedules, Schedules: withSchedule(cfg, "night", func(s *Schedule) { s.EndTime = "05:00" })}
	if err := ApplyOperation(cfg, op, now); err == nil {
		t.Error("re-timing another enabled schedule should be refused while locked")
	}
}

func TestPolicyAllowsHarmlessScheduleEditsWhileLocked(t *testing.T) {
	now := at(0, 12, 0)

	// Rename, and the same days in a different order.
	cfg := scheduleLockedConfig()
	op := Operation{Type: OpSaveSchedules, Schedules: withSchedule(cfg, "work", func(s *Schedule) {
		s.Name = "Deep work"
		s.Days = []string{"Tue", "Mon"}
	})}
	if err := ApplyOperation(cfg, op, now); err != nil {
		t.Errorf("renaming / reordering days should be allowed: %v", err)
	}

	// Adding a new schedule.
	cfg = scheduleLockedConfig()
	added := append(withSchedule(cfg, "", nil), Schedule{ID: "new", Enabled: true, Days: []string{"Sat"}, StartTime: "10:00", EndTime: "12:00"})
	if err := ApplyOperation(cfg, Operation{Type: OpSaveSchedules, Schedules: added}, now); err != nil {
		t.Errorf("adding a schedule should be allowed: %v", err)
	}

	// When nothing is enforced, re-timing is fine.
	idle := at(0, 18, 0) // Monday 18:00, after the window
	cfg = scheduleLockedConfig()
	op = Operation{Type: OpSaveSchedules, Schedules: withSchedule(cfg, "work", func(s *Schedule) { s.EndTime = "16:00" })}
	if err := ApplyOperation(cfg, op, idle); err != nil {
		t.Errorf("re-timing when idle should be allowed: %v", err)
	}
}

func TestStartFocusDuringScheduleDoesNotRefillUnlocks(t *testing.T) {
	now := at(0, 12, 0) // inside the "work" window
	cfg := scheduleLockedConfig()
	for i := 0; i < 2; i++ {
		if err := ApplyOperation(cfg, Operation{Type: OpEmergencyUnlock}, now); err != nil {
			t.Fatalf("unlock %d: %v", i+1, err)
		}
	}
	if err := ApplyOperation(cfg, Operation{Type: OpStartFocus, Seconds: 60}, now); err != nil {
		t.Fatalf("starting a manual lock during a window should be allowed: %v", err)
	}
	if cfg.EmergencyUnlocksUsed != 2 {
		t.Errorf("EmergencyUnlocksUsed = %d, want 2 (no refill mid-window)", cfg.EmergencyUnlocksUsed)
	}
	if err := ApplyOperation(cfg, Operation{Type: OpEmergencyUnlock}, now); err == nil {
		t.Error("a third unlock in the same window must still be refused")
	}

	// Outside any window, a fresh manual lock still refills.
	idle := at(0, 18, 0)
	cfg = scheduleLockedConfig()
	cfg.EmergencyUnlocksUsed = 2
	if err := ApplyOperation(cfg, Operation{Type: OpStartFocus, Seconds: 60}, idle); err != nil {
		t.Fatal(err)
	}
	if cfg.EmergencyUnlocksUsed != 0 {
		t.Errorf("a fresh lock outside a window should refill, got %d", cfg.EmergencyUnlocksUsed)
	}
}
