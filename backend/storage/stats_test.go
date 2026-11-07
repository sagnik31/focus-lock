package storage

import (
	"testing"
	"time"
)

func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 12, 0, 0, 0, time.Local)
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
