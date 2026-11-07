package storage

import (
	"sort"
	"strings"
	"time"
)

// SchedulesActiveAt reports whether any enabled schedule covers the given time.
// It is the single implementation of the schedule-window check, shared by the
// enforcer (watchdog.IsScheduleActive) and the operation policy here, so the UI
// and the Ghost can never disagree about whether a schedule is active.
func SchedulesActiveAt(schedules []Schedule, now time.Time) bool {
	_, ok := ActiveScheduleWindow(schedules, now)
	return ok
}

// ActiveScheduleWindow reports whether any enabled schedule covers now and, if
// so, returns a stable key identifying the set of active window instances. The
// key changes whenever a window instance starts or ends, so callers can detect
// "a new schedule window just became active" (used to refill the per-session
// emergency-unlock quota — see applyScheduleUnlockReset).
//
// A schedule's Days list the day the window STARTS. A window whose EndTime is
// not after its StartTime (e.g. 22:00–06:00) crosses midnight: it runs from
// StartTime on the start day until EndTime the next calendar day. So the check
// has to consider both today's schedules (for the pre-midnight half) and
// yesterday's overnight schedules (for the post-midnight half).
//
// Each active window instance contributes "<schedule-ID>|<start-date>" to the
// key, where start-date is the calendar date on which that instance began. The
// date makes the same weekly schedule recur as a distinct instance each week
// (so the quota refills), and the keys are sorted + joined so the result is
// independent of the schedules' slice order.
func ActiveScheduleWindow(schedules []Schedule, now time.Time) (string, bool) {
	today := now.Format("Mon")         // "Mon", "Tue", ...
	todayDate := now.Format(dayLayout) // "YYYY-MM-DD"
	yest := now.AddDate(0, 0, -1)      // the previous calendar day
	yestDay := yest.Format("Mon")
	yestDate := yest.Format(dayLayout)
	currentTime := now.Format("15:04") // "HH:MM"

	var keys []string
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
					keys = append(keys, s.ID+"|"+todayDate)
				}
			} else {
				// Same-day window: start .. end.
				if currentTime >= s.StartTime && currentTime < s.EndTime {
					keys = append(keys, s.ID+"|"+todayDate)
				}
			}
		}

		// Post-midnight half of a window that started yesterday: 00:00 .. end.
		if overnight && dayMatches(s.Days, yestDay) && currentTime < s.EndTime {
			keys = append(keys, s.ID+"|"+yestDate)
		}
	}

	if len(keys) == 0 {
		return "", false
	}
	sort.Strings(keys)
	return strings.Join(keys, ","), true
}

func dayMatches(days []string, day string) bool {
	for _, d := range days {
		if d == day {
			return true
		}
	}
	return false
}
