package storage

import (
	"testing"
	"time"
)

func TestActiveScheduleEnd(t *testing.T) {
	mon := []string{"Mon"}
	cases := []struct {
		name    string
		scheds  []Schedule
		now     time.Time
		want    time.Time
		wantAct bool
	}{
		{"same day", []Schedule{sched(mon, "09:00", "17:00")}, at(0, 12, 0), at(0, 17, 0), true},
		{"same day inactive", []Schedule{sched(mon, "09:00", "17:00")}, at(0, 17, 0), time.Time{}, false},
		{"overnight pre-midnight ends tomorrow", []Schedule{sched(mon, "22:00", "06:00")}, at(0, 23, 0), at(1, 6, 0), true},
		{"overnight post-midnight ends today", []Schedule{sched(mon, "22:00", "06:00")}, at(1, 2, 0), at(1, 6, 0), true},
		{"overnight from Sunday into Monday", []Schedule{sched([]string{"Sun"}, "23:00", "01:30")}, at(0, 0, 30), at(0, 1, 30), true},
		{"overlapping windows take the later end", []Schedule{
			sched(mon, "09:00", "12:00"),
			{ID: "b", Enabled: true, Days: mon, StartTime: "10:00", EndTime: "15:00"},
		}, at(0, 11, 0), at(0, 15, 0), true},
		{"disabled", []Schedule{{ID: "s", Days: mon, StartTime: "09:00", EndTime: "17:00"}}, at(0, 12, 0), time.Time{}, false},
	}
	for _, c := range cases {
		got, ok := ActiveScheduleEnd(c.scheds, c.now)
		if ok != c.wantAct || !got.Equal(c.want) {
			t.Errorf("%s: got (%v, %v), want (%v, %v)", c.name, got, ok, c.want, c.wantAct)
		}
		if ok != SchedulesActiveAt(c.scheds, c.now) {
			t.Errorf("%s: disagrees with SchedulesActiveAt", c.name)
		}
	}
}

func TestLockStateAt(t *testing.T) {
	mon := []string{"Mon"}
	now := at(0, 12, 0)
	cases := []struct {
		name     string
		cfg      Config
		wantKind string
		wantEnd  time.Time
	}{
		{"nothing", Config{}, LockNone, time.Time{}},
		{"expired manual", Config{LockEndTime: at(0, 11, 0)}, LockNone, time.Time{}},
		{"manual", Config{LockEndTime: at(0, 13, 0)}, LockManual, at(0, 13, 0)},
		{"schedule", Config{Schedules: []Schedule{sched(mon, "09:00", "17:00")}}, LockSchedule, at(0, 17, 0)},
		{"manual outlasts schedule", Config{LockEndTime: at(0, 18, 0), Schedules: []Schedule{sched(mon, "09:00", "17:00")}}, LockManual, at(0, 18, 0)},
		{"schedule outlasts manual", Config{LockEndTime: at(0, 13, 0), Schedules: []Schedule{sched(mon, "09:00", "17:00")}}, LockSchedule, at(0, 17, 0)},
		{"overnight", Config{Schedules: []Schedule{sched(mon, "11:00", "06:00")}}, LockSchedule, at(1, 6, 0)},
	}
	for _, c := range cases {
		got := LockStateAt(&c.cfg, now)
		if got.Kind != c.wantKind || !got.EndTime.Equal(c.wantEnd) {
			t.Errorf("%s: got %+v, want %s %v", c.name, got, c.wantKind, c.wantEnd)
		}
	}
}
