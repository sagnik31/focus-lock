package storage

import (
	"fmt"
	"sort"
	"strconv"
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
// key changes whenever a window instance starts or ends. The per-session
// emergency-unlock quota is tied to it (see applyScheduleUnlockReset).
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
	windows := activeWindows(schedules, now)
	if len(windows) == 0 {
		return "", false
	}
	keys := make([]string, len(windows))
	for i, w := range windows {
		keys[i] = w.key
	}
	sort.Strings(keys)
	return strings.Join(keys, ","), true
}

// ActiveScheduleEnd reports whether any enabled schedule covers now and, if
// so, when the active window that ends last ends. An overnight window that
// started today ends tomorrow; its post-midnight half ends today.
func ActiveScheduleEnd(schedules []Schedule, now time.Time) (time.Time, bool) {
	var last time.Time
	for _, w := range activeWindows(schedules, now) {
		if w.end.After(last) {
			last = w.end
		}
	}
	return last, !last.IsZero()
}

// window is one active schedule window instance.
type window struct {
	key string    // "<schedule-ID>|<start-date>" (see ActiveScheduleWindow)
	end time.Time // when this instance ends
}

// activeWindows is the single schedule-window check behind
// ActiveScheduleWindow, ActiveScheduleEnd and SchedulesActiveAt.
func activeWindows(schedules []Schedule, now time.Time) []window {
	today := now.Format("Mon")         // "Mon", "Tue", ...
	todayDate := now.Format(dayLayout) // "YYYY-MM-DD"
	yest := now.AddDate(0, 0, -1)      // the previous calendar day
	yestDay := yest.Format("Mon")
	yestDate := yest.Format(dayLayout)
	currentTime := now.Format("15:04") // "HH:MM"

	var out []window
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
					out = append(out, window{s.ID + "|" + todayDate, clockOn(now, 1, s.EndTime)})
				}
			} else {
				// Same-day window: start .. end.
				if currentTime >= s.StartTime && currentTime < s.EndTime {
					out = append(out, window{s.ID + "|" + todayDate, clockOn(now, 0, s.EndTime)})
				}
			}
		}

		// Post-midnight half of a window that started yesterday: 00:00 .. end.
		if overnight && dayMatches(s.Days, yestDay) && currentTime < s.EndTime {
			out = append(out, window{s.ID + "|" + yestDate, clockOn(now, 0, s.EndTime)})
		}
	}
	return out
}

// clockOn returns the "HH:MM" time hhmm on the calendar day dayOffset days
// after now's, in now's location. An unparseable time gives that day's
// midnight; NormalizeSchedule keeps stored schedules well formed.
func clockOn(now time.Time, dayOffset int, hhmm string) time.Time {
	var h, m int
	if t, err := time.Parse("15:04", hhmm); err == nil {
		h, m = t.Hour(), t.Minute()
	}
	y, mo, d := now.Date()
	return time.Date(y, mo, d+dayOffset, h, m, 0, 0, now.Location())
}

func dayMatches(days []string, day string) bool {
	for _, d := range days {
		if d == day {
			return true
		}
	}
	return false
}

// weekDays are the canonical day names the window logic matches (now.Format("Mon")).
var weekDays = []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}

// NormalizeSchedule validates a schedule and returns it in the canonical form
// the window logic relies on: zero-padded 24h "HH:MM" times (the checks
// compare them as strings, so "9:00" would sort after "17:00") and day names
// "Mon".."Sun" (full names and any case are accepted), de-duplicated in week
// order. It rejects a missing ID, unparseable times, equal start and end
// (a window that never runs), and an empty or unknown day list.
func NormalizeSchedule(s Schedule) (Schedule, error) {
	label := strings.TrimSpace(s.Name)
	if label == "" {
		label = "unnamed"
	}
	if strings.TrimSpace(s.ID) == "" {
		return s, fmt.Errorf("schedule %q is missing an id", label)
	}
	start, err := normalizeClock(s.StartTime)
	if err != nil {
		return s, fmt.Errorf("schedule %q: start time %w", label, err)
	}
	end, err := normalizeClock(s.EndTime)
	if err != nil {
		return s, fmt.Errorf("schedule %q: end time %w", label, err)
	}
	if start == end {
		return s, fmt.Errorf("schedule %q: start and end time can't be the same", label)
	}
	want := map[string]bool{}
	for _, d := range s.Days {
		d = strings.TrimSpace(d)
		if len(d) < 3 {
			return s, fmt.Errorf("schedule %q: unknown day %q", label, d)
		}
		canon := strings.ToUpper(d[:1]) + strings.ToLower(d[1:3])
		full := strings.ToLower(d)
		ok := false
		for _, wd := range weekDays {
			if canon == wd && (len(d) == 3 || strings.HasPrefix(longDayName(wd), full)) {
				want[wd] = true
				ok = true
				break
			}
		}
		if !ok {
			return s, fmt.Errorf("schedule %q: unknown day %q", label, d)
		}
	}
	if len(want) == 0 {
		return s, fmt.Errorf("schedule %q needs at least one day", label)
	}
	days := make([]string, 0, len(want))
	for _, wd := range weekDays {
		if want[wd] {
			days = append(days, wd)
		}
	}
	s.StartTime, s.EndTime, s.Days = start, end, days
	return s, nil
}

func longDayName(short string) string {
	for d := time.Sunday; d <= time.Saturday; d++ {
		if d.String()[:3] == short {
			return strings.ToLower(d.String())
		}
	}
	return ""
}

// normalizeClock parses "H:MM" or "HH:MM" (24h) and returns "HH:MM".
func normalizeClock(v string) (string, error) {
	v = strings.TrimSpace(v)
	h, m, ok := strings.Cut(v, ":")
	if !ok || len(h) < 1 || len(h) > 2 || len(m) != 2 || strings.Trim(h+m, "0123456789") != "" {
		return "", fmt.Errorf("%q is not a 24-hour HH:MM time", v)
	}
	hh, err1 := strconv.Atoi(h)
	mm, err2 := strconv.Atoi(m)
	if err1 != nil || err2 != nil || hh < 0 || hh > 23 || mm < 0 || mm > 59 {
		return "", fmt.Errorf("%q is not a 24-hour HH:MM time", v)
	}
	return fmt.Sprintf("%02d:%02d", hh, mm), nil
}

// Lock kinds reported by LockStateAt.
const (
	LockNone     = "none"
	LockManual   = "manual"
	LockSchedule = "schedule"
)

// LockState is what the lock screen needs: whether anything is enforced and
// when enforcement ends.
type LockState struct {
	Kind    string    // LockNone, LockManual or LockSchedule
	EndTime time.Time // zero when Kind is LockNone
}

// LockStateAt reports the lock in force at now. Enforcement lasts until both
// the manual lock and every active schedule window are over, so EndTime is
// whichever ends later and Kind names that one.
func LockStateAt(cfg *Config, now time.Time) LockState {
	var st LockState
	if ManualLockActive(cfg, now) {
		st = LockState{Kind: LockManual, EndTime: cfg.LockEndTime}
	}
	if end, ok := ActiveScheduleEnd(cfg.Schedules, now); ok && end.After(st.EndTime) {
		st = LockState{Kind: LockSchedule, EndTime: end}
	}
	if st.Kind == "" {
		st.Kind = LockNone
	}
	return st
}
