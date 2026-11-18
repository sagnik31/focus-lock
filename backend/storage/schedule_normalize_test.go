package storage

import (
	"reflect"
	"testing"
	"time"
)

func TestNormalizeSchedule(t *testing.T) {
	ok := []struct {
		in       Schedule
		start    string
		end      string
		wantDays []string
	}{
		{Schedule{ID: "a", StartTime: "9:00", EndTime: "17:30", Days: []string{"Mon"}}, "09:00", "17:30", []string{"Mon"}},
		{Schedule{ID: "b", StartTime: "22:00", EndTime: "06:00", Days: []string{"friday", "MON", "Fri"}}, "22:00", "06:00", []string{"Mon", "Fri"}},
		{Schedule{ID: "c", StartTime: " 07:05 ", EndTime: "8:15", Days: []string{"Tues", "thu"}}, "07:05", "08:15", []string{"Tue", "Thu"}},
	}
	for _, c := range ok {
		got, err := NormalizeSchedule(c.in)
		if err != nil {
			t.Errorf("NormalizeSchedule(%+v): unexpected error %v", c.in, err)
			continue
		}
		if got.StartTime != c.start || got.EndTime != c.end || !reflect.DeepEqual(got.Days, c.wantDays) {
			t.Errorf("NormalizeSchedule(%+v) = %s-%s %v, want %s-%s %v", c.in, got.StartTime, got.EndTime, got.Days, c.start, c.end, c.wantDays)
		}
	}

	bad := []Schedule{
		{ID: "", StartTime: "09:00", EndTime: "17:00", Days: []string{"Mon"}},     // no id
		{ID: "x", StartTime: "25:00", EndTime: "17:00", Days: []string{"Mon"}},    // hour out of range
		{ID: "x", StartTime: "09:60", EndTime: "17:00", Days: []string{"Mon"}},    // minute out of range
		{ID: "x", StartTime: "9am", EndTime: "17:00", Days: []string{"Mon"}},      // not HH:MM
		{ID: "x", StartTime: "+9:00", EndTime: "17:00", Days: []string{"Mon"}},    // sign
		{ID: "x", StartTime: "09:00", EndTime: "9:00", Days: []string{"Mon"}},     // zero-length
		{ID: "x", StartTime: "09:00", EndTime: "17:00", Days: nil},                // no days
		{ID: "x", StartTime: "09:00", EndTime: "17:00", Days: []string{"Funday"}}, // unknown day
		{ID: "x", StartTime: "09:00", EndTime: "17:00", Days: []string{"Mo"}},     // too short
	}
	for _, s := range bad {
		if _, err := NormalizeSchedule(s); err == nil {
			t.Errorf("NormalizeSchedule(%+v) should be refused", s)
		}
	}
}

func TestSaveSchedulesNormalisesAndRefusesInvalid(t *testing.T) {
	now := time.Now()
	cfg := idleConfig()
	op := Operation{Type: OpSaveSchedules, Schedules: []Schedule{{ID: "w", Name: "Work", Enabled: true, Days: []string{"monday"}, StartTime: "9:00", EndTime: "17:00"}}}
	if err := ApplyOperation(cfg, op, now); err != nil {
		t.Fatal(err)
	}
	if s := cfg.Schedules[0]; s.StartTime != "09:00" || s.Days[0] != "Mon" {
		t.Errorf("schedule stored un-normalised: %+v", s)
	}

	bad := Operation{Type: OpSaveSchedules, Schedules: []Schedule{{ID: "w", Enabled: true, Days: []string{"Mon"}, StartTime: "9", EndTime: "17:00"}}}
	if err := ApplyOperation(cfg, bad, now); err == nil {
		t.Error("an invalid schedule should be refused")
	}
}

func TestImportRefusesInvalidScheduleWithoutPartialChanges(t *testing.T) {
	cfg := idleConfig()
	op := Operation{
		Type:      OpImport,
		Apps:      []string{"slack.exe"},
		Schedules: []Schedule{{ID: "i", Name: "Broken", Enabled: true, Days: []string{"Mon"}, StartTime: "09:00", EndTime: "09:00"}},
	}
	if err := ApplyOperation(cfg, op, time.Now()); err == nil {
		t.Fatal("import with an invalid schedule should be refused")
	}
	if len(cfg.BlockedApps) != 1 || len(cfg.Schedules) != 0 {
		t.Errorf("a refused import must change nothing; got apps %v schedules %v", cfg.BlockedApps, cfg.Schedules)
	}
}

// A schedule stored as "9:00" by an older build must not count as re-timed
// when the UI saves it back (normalised) during a lock.
func TestLockedSaveToleratesLegacyUnpaddedTimes(t *testing.T) {
	now := at(0, 12, 0) // Monday noon; "09:00"-"17:00" is active
	cfg := &Config{Schedules: []Schedule{{ID: "w", Name: "Work", Enabled: true, Days: []string{"Mon"}, StartTime: "09:00", EndTime: "17:00"}}}
	cfg.Schedules[0].StartTime = "9:00" // legacy form (inactive by string compare, but stored)
	cfg.LockEndTime = now.Add(time.Hour)
	op := Operation{Type: OpSaveSchedules, Schedules: []Schedule{{ID: "w", Name: "Deep work", Enabled: true, Days: []string{"Mon"}, StartTime: "09:00", EndTime: "17:00"}}}
	if err := ApplyOperation(cfg, op, now); err != nil {
		t.Errorf("renaming a legacy-format schedule during a lock should be allowed: %v", err)
	}
}
