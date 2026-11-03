package storage

import "time"

// SchedulesActiveAt reports whether any enabled schedule covers the given time.
// It is the single implementation of the schedule-window check, shared by the
// enforcer (watchdog.IsScheduleActive) and the operation policy here, so the UI
// and the Ghost can never disagree about whether a schedule is active.
func SchedulesActiveAt(schedules []Schedule, now time.Time) bool {
	currentDay := now.Format("Mon")    // "Mon", "Tue", ...
	currentTime := now.Format("15:04") // "HH:MM"

	for _, s := range schedules {
		if !s.Enabled {
			continue
		}

		dayMatch := false
		for _, d := range s.Days {
			if d == currentDay {
				dayMatch = true
				break
			}
		}
		if !dayMatch {
			continue
		}

		// Simple string comparison works for 24h "HH:MM" format.
		if currentTime >= s.StartTime && currentTime < s.EndTime {
			return true
		}
	}
	return false
}
