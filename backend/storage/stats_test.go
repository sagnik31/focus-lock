package storage

import (
	"testing"
	"time"
)

func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 12, 0, 0, 0, time.Local)
}

func TestRecordScheduleSessionEdge(t *testing.T) {
	cfg := &Config{BlockedApps: []string{"a.exe"}, BlockedSites: []string{"x.com", "y.com"}}
	t0 := day(2026, time.November, 7)

	// Rising edge: start tracking, nothing recorded yet.
	if !recordScheduleSessionEdge(cfg, t0, true) {
		t.Fatal("rising edge should change cfg")
	}
	if cfg.ScheduleSessionStart.IsZero() {
		t.Error("start stamp should be set on the rising edge")
	}
	if len(cfg.Stats.Sessions) != 0 {
		t.Error("no session should be recorded on the rising edge")
	}

	// Still active: no change.
	if recordScheduleSessionEdge(cfg, t0.Add(10*time.Minute), true) {
		t.Error("staying active should not change cfg")
	}

	// Falling edge 30 min after the start: one scheduled session recorded.
	end := t0.Add(30 * time.Minute)
	if !recordScheduleSessionEdge(cfg, end, false) {
		t.Fatal("falling edge should change cfg")
	}
	if !cfg.ScheduleSessionStart.IsZero() {
		t.Error("start stamp should be cleared on the falling edge")
	}
	if len(cfg.Stats.Sessions) != 1 {
		t.Fatalf("expected 1 recorded session, got %d", len(cfg.Stats.Sessions))
	}
	got := cfg.Stats.Sessions[0]
	if got.Kind != "scheduled" {
		t.Errorf("kind = %q, want %q", got.Kind, "scheduled")
	}
	if got.DurationSec != 1800 {
		t.Errorf("duration = %d, want 1800", got.DurationSec)
	}
	if got.AppCount != 1 || got.SiteCount != 2 {
		t.Errorf("app/site counts = %d/%d, want 1/2", got.AppCount, got.SiteCount)
	}
	if cfg.Stats.TotalFocusSeconds != 1800 {
		t.Errorf("total focus = %d, want 1800", cfg.Stats.TotalFocusSeconds)
	}

	// Inactive with nothing in progress: no change.
	if recordScheduleSessionEdge(cfg, end.Add(time.Hour), false) {
		t.Error("inactive with no session in progress should not change cfg")
	}
}

func TestRecordScheduleSessionEdgeSkipsZeroLength(t *testing.T) {
	cfg := &Config{}
	t0 := day(2026, time.November, 7)

	recordScheduleSessionEdge(cfg, t0, true)
	// Falls inactive at the same instant: clear the stamp, record nothing.
	if !recordScheduleSessionEdge(cfg, t0, false) {
		t.Error("clearing a zero-length stamp is still a change")
	}
	if len(cfg.Stats.Sessions) != 0 {
		t.Error("a zero-length schedule session must not be recorded")
	}
	if !cfg.ScheduleSessionStart.IsZero() {
		t.Error("the stamp should be cleared")
	}
}

func TestStreakFromDays(t *testing.T) {
	now := day(2026, time.November, 7) // "today"

	cases := []struct {
		name             string
		days             []string
		wantCur, wantLng int
	}{
		{"empty", nil, 0, 0},
		{"today only", []string{"2026-11-07"}, 1, 1},
		{"three ending today", []string{"2026-11-05", "2026-11-06", "2026-11-07"}, 3, 3},
		{"grace: ending yesterday", []string{"2026-11-05", "2026-11-06"}, 2, 2},
		{"broken: newest two days ago", []string{"2026-11-04", "2026-11-05"}, 0, 2},
		{"gap splits current from longest", []string{"2026-11-01", "2026-11-02", "2026-11-03", "2026-11-06", "2026-11-07"}, 2, 3},
		{"duplicates ignored", []string{"2026-11-07", "2026-11-07", "2026-11-06"}, 2, 2},
		{"unordered input", []string{"2026-11-07", "2026-11-05", "2026-11-06"}, 3, 3},
		{"bad entries skipped", []string{"not-a-date", "2026-11-07"}, 1, 1},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cur, lng := StreakFromDays(c.days, now)
			if cur != c.wantCur || lng != c.wantLng {
				t.Errorf("StreakFromDays(%v) = (cur %d, longest %d), want (cur %d, longest %d)",
					c.days, cur, lng, c.wantCur, c.wantLng)
			}
		})
	}
}
