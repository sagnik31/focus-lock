package storage

import (
	"testing"
	"time"
)

// at builds a time on a known week: 2026-11-02 is a Monday, so dayOffset 0 = Mon.
func at(dayOffset, hour, min int) time.Time {
	base := time.Date(2026, 11, 2, 0, 0, 0, 0, time.Local) // Monday 00:00
	return base.AddDate(0, 0, dayOffset).Add(time.Duration(hour)*time.Hour + time.Duration(min)*time.Minute)
}

func sched(days []string, start, end string) Schedule {
	return Schedule{ID: "s", Enabled: true, Days: days, StartTime: start, EndTime: end}
}

func TestSchedulesActiveAt(t *testing.T) {
	mon := []string{"Mon"}

	cases := []struct {
		name  string
		sched Schedule
		now   time.Time
		want  bool
	}{
		// --- same-day window 09:00-17:00 on Mon ---
		{"sameday before", sched(mon, "09:00", "17:00"), at(0, 8, 59), false},
		{"sameday start", sched(mon, "09:00", "17:00"), at(0, 9, 0), true},
		{"sameday mid", sched(mon, "09:00", "17:00"), at(0, 12, 0), true},
		{"sameday at end (exclusive)", sched(mon, "09:00", "17:00"), at(0, 17, 0), false},
		{"sameday wrong day", sched(mon, "09:00", "17:00"), at(1, 12, 0), false},

		// --- overnight window 22:00-06:00 on Mon => Mon 22:00 .. Tue 06:00 ---
		{"overnight before start Mon", sched(mon, "22:00", "06:00"), at(0, 21, 59), false},
		{"overnight at start Mon", sched(mon, "22:00", "06:00"), at(0, 22, 0), true},
		{"overnight late Mon", sched(mon, "22:00", "06:00"), at(0, 23, 30), true},
		{"overnight after midnight Tue", sched(mon, "22:00", "06:00"), at(1, 2, 0), true},
		{"overnight at end Tue (exclusive)", sched(mon, "22:00", "06:00"), at(1, 6, 0), false},
		{"overnight after end Tue", sched(mon, "22:00", "06:00"), at(1, 6, 1), false},
		// The early-morning half must NOT fire on the start day itself.
		{"overnight early Mon morning", sched(mon, "22:00", "06:00"), at(0, 2, 0), false},
		// Nor should the pre-midnight half fire on the following day.
		{"overnight late Tue night", sched(mon, "22:00", "06:00"), at(1, 23, 0), false},

		// --- zero-length window never activates ---
		{"zero length", sched(mon, "09:00", "09:00"), at(0, 9, 0), false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := SchedulesActiveAt([]Schedule{c.sched}, c.now); got != c.want {
				t.Errorf("SchedulesActiveAt(%s @ %s) = %v, want %v",
					c.sched.StartTime+"-"+c.sched.EndTime, c.now.Format("Mon 15:04"), got, c.want)
			}
		})
	}
}

func TestActiveScheduleWindow(t *testing.T) {
	mon := []string{"Mon"}

	// Inactive before the window: no key.
	if k, ok := ActiveScheduleWindow([]Schedule{sched(mon, "09:00", "17:00")}, at(0, 8, 0)); ok || k != "" {
		t.Errorf("before window: got (%q, %v), want (\"\", false)", k, ok)
	}

	// Same-day window active: key uses the start day's date (Mon 2026-11-02).
	if k, ok := ActiveScheduleWindow([]Schedule{sched(mon, "09:00", "17:00")}, at(0, 12, 0)); !ok || k != "s|2026-11-02" {
		t.Errorf("same-day active: got (%q, %v), want (\"s|2026-11-02\", true)", k, ok)
	}

	// Overnight window, post-midnight half (Tue 02:00): keys off the start day (Mon).
	if k, ok := ActiveScheduleWindow([]Schedule{sched(mon, "22:00", "06:00")}, at(1, 2, 0)); !ok || k != "s|2026-11-02" {
		t.Errorf("overnight post-midnight: got (%q, %v), want (\"s|2026-11-02\", true)", k, ok)
	}
}

func TestActiveScheduleWindowOrderIndependent(t *testing.T) {
	mon := []string{"Mon"}
	a := Schedule{ID: "a", Enabled: true, Days: mon, StartTime: "08:00", EndTime: "18:00"}
	b := Schedule{ID: "b", Enabled: true, Days: mon, StartTime: "09:00", EndTime: "17:00"}

	k1, _ := ActiveScheduleWindow([]Schedule{a, b}, at(0, 12, 0))
	k2, _ := ActiveScheduleWindow([]Schedule{b, a}, at(0, 12, 0))
	if k1 != k2 {
		t.Errorf("key must be independent of slice order: %q vs %q", k1, k2)
	}
	if k1 != "a|2026-11-02,b|2026-11-02" {
		t.Errorf("unexpected combined key: %q", k1)
	}
}

func TestSchedulesActiveAtDisabledAndEmpty(t *testing.T) {
	if SchedulesActiveAt(nil, at(0, 12, 0)) {
		t.Error("no schedules should be inactive")
	}
	disabled := Schedule{ID: "s", Enabled: false, Days: []string{"Mon"}, StartTime: "00:00", EndTime: "23:59"}
	if SchedulesActiveAt([]Schedule{disabled}, at(0, 12, 0)) {
		t.Error("disabled schedule must never be active")
	}
}
