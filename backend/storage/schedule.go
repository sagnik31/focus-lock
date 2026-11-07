package storage

import "time"

// SchedulesActiveAt reports whether any enabled schedule covers the given time.
// It is the single implementation of the schedule-window check, shared by the
// enforcer (watchdog.IsScheduleActive) and the operation policy here, so the UI
// and the Ghost can never disagree about whether a schedule is active.
//
// A schedule's Days list the day the window STARTS. A window whose EndTime is
// not after its StartTime (e.g. 22:00–06:00) crosses midnight: it runs from
// StartTime on the start day until EndTime the next calendar day. So the check
// has to consider both today's schedules (for the pre-midnight half) and
// yesterday's overnight schedules (for the post-midnight half).
func SchedulesActiveAt(schedules []Schedule, now time.Time) bool {
	today := now.Format("Mon")                       // "Mon", "Tue", ...
	yesterday := now.AddDate(0, 0, -1).Format("Mon") // the previous calendar day
	currentTime := now.Format("15:04")               // "HH:MM"

	for _, s := range schedules {
		if !s.Enabled {
			continue
		}
		// A zero-length / inverted-to-equal window never activates.
		if s.StartTime == s.EndTime {
			continue
		}

		overnight := s.EndTime < s.StartTime // crosses midnight

		if dayMatches(s.Days, today) {
			if overnight {
				// Pre-midnight half, on the start day itself: start .. 24:00.
				if currentTime >= s.StartTime {
					return true
				}
			} else {
				// Same-day window: start .. end.
				if currentTime >= s.StartTime && currentTime < s.EndTime {
					return true
				}
			}
		}

		// Post-midnight half of a window that started yesterday: 00:00 .. end.
		if overnight && dayMatches(s.Days, yesterday) && currentTime < s.EndTime {
			return true
		}
	}
	return false
}

func dayMatches(days []string, day string) bool {
	for _, d := range days {
		if d == day {
			return true
		}
	}
	return false
}
