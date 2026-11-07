package storage

import (
	"sort"
	"time"
)

const (
	maxSessions  = 200 // cap on retained session history
	maxFocusDays = 400 // cap on retained day markers (streaks)
	dayLayout    = "2006-01-02"
)

// RecordKill records a successful process termination: the per-app kill count
// and the local hour-of-day bucket that powers the "you tried X at 2pm"
// insight. Only a signer persists (strong-mode UI is a no-op).
func (s *Store) RecordKill(appName string, now time.Time) {
	if !s.canSign {
		return
	}
	s.mu.Lock()
	_ = s.loadInternal()
	if s.Data.Stats.KillCounts == nil {
		s.Data.Stats.KillCounts = make(map[string]int)
	}
	s.Data.Stats.KillCounts[appName]++
	if len(s.Data.Stats.KillsByHour) != 24 {
		s.Data.Stats.KillsByHour = make([]int, 24)
	}
	s.Data.Stats.KillsByHour[now.Hour()]++
	err := s.saveInternal()
	s.mu.Unlock()
	_ = err
}

// RecordFocusDay marks today as a day with focus activity (for streaks). It is
// cheap to call every enforcement tick: it writes only when today is not
// already the latest recorded day, so at most once per day.
func (s *Store) RecordFocusDay(now time.Time) {
	if !s.canSign {
		return
	}
	day := now.Format(dayLayout)
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.loadInternal()
	days := s.Data.Stats.FocusDays
	if n := len(days); n > 0 && days[n-1] == day {
		return // already recorded today; no write
	}
	days = append(days, day)
	if len(days) > maxFocusDays {
		days = days[len(days)-maxFocusDays:]
	}
	s.Data.Stats.FocusDays = days
	_ = s.saveInternal()
}

// RecordManualSessionEnd appends a completed manual session to history if one
// is in progress (SessionStart set), updates lifetime focus time, marks the
// focus day, and clears SessionStart. It reloads first and no-ops if another
// process already finalised the session, so it is safe to call from both the
// expiry-cleanup path and StopFocus.
func (s *Store) RecordManualSessionEnd(now time.Time) {
	if !s.canSign {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.loadInternal()
	start := s.Data.SessionStart
	if start.IsZero() {
		return // no session in progress (or already recorded by the other process)
	}
	dur := int64(now.Sub(start).Seconds())
	if dur < 0 {
		dur = 0
	}
	rec := SessionRecord{
		Start:       start,
		End:         now,
		DurationSec: dur,
		Kind:        "manual",
		AppCount:    len(s.Data.BlockedApps),
		SiteCount:   len(s.Data.BlockedSites),
	}
	s.Data.Stats.Sessions = append(s.Data.Stats.Sessions, rec)
	if len(s.Data.Stats.Sessions) > maxSessions {
		s.Data.Stats.Sessions = s.Data.Stats.Sessions[len(s.Data.Stats.Sessions)-maxSessions:]
	}
	s.Data.Stats.TotalFocusSeconds += dur

	// Mark the focus day inline (we already hold the lock).
	day := now.Format(dayLayout)
	fd := s.Data.Stats.FocusDays
	if n := len(fd); n == 0 || fd[n-1] != day {
		fd = append(fd, day)
		if len(fd) > maxFocusDays {
			fd = fd[len(fd)-maxFocusDays:]
		}
		s.Data.Stats.FocusDays = fd
	}

	s.Data.SessionStart = time.Time{}
	_ = s.saveInternal()
}

// RefreshScheduleQuota refills the emergency-unlock quota at the start of each
// new schedule window so the count the UI shows stays current even before the
// user tries to unlock. It writes only when the active window actually changed,
// so it is cheap to call on every enforcement tick. Only a signer persists
// (strong-mode UI is a no-op; the elevated Ghost is authoritative).
func (s *Store) RefreshScheduleQuota(now time.Time) {
	if !s.canSign {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.loadInternal()
	if applyScheduleUnlockReset(&s.Data, now) {
		_ = s.saveInternal()
	}
}

// UpdateScheduleSession records schedule-driven focus as completed sessions, so
// the history/Insights view counts schedule windows and not only manual timers
// (previously a schedule window contributed to streaks via focus_days but was
// never added to the session list). "active" is whether any schedule window
// currently covers now:
//
//   - Rising edge (active, none in progress): stamp ScheduleSessionStart.
//   - Falling edge (inactive, one in progress): append a completed "scheduled"
//     SessionRecord and add its duration to the lifetime total.
//
// It reloads and writes only on an edge, so it is cheap to call every tick and
// idempotent across the UI+Ghost enforcers (whichever observes the edge first
// records it; the other reloads and sees nothing to do). Only a signer persists.
func (s *Store) UpdateScheduleSession(now time.Time, active bool) {
	if !s.canSign {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.loadInternal()
	if recordScheduleSessionEdge(&s.Data, now, active) {
		_ = s.saveInternal()
	}
}

// recordScheduleSessionEdge advances the schedule-session state machine on cfg
// and reports whether cfg changed (so callers persist only on an edge). It is
// pure (no I/O), so the edge logic is unit-tested directly without touching the
// signed store or the registry backup. A zero-length window (rising and falling
// on the same instant) clears the stamp without recording a session.
func recordScheduleSessionEdge(cfg *Config, now time.Time, active bool) bool {
	inProgress := !cfg.ScheduleSessionStart.IsZero()
	switch {
	case active && !inProgress:
		cfg.ScheduleSessionStart = now
		return true

	case !active && inProgress:
		start := cfg.ScheduleSessionStart
		cfg.ScheduleSessionStart = time.Time{}
		dur := int64(now.Sub(start).Seconds())
		if dur <= 0 {
			return true // nothing worth recording; just clear the stamp
		}
		cfg.Stats.Sessions = append(cfg.Stats.Sessions, SessionRecord{
			Start:       start,
			End:         now,
			DurationSec: dur,
			Kind:        "scheduled",
			AppCount:    len(cfg.BlockedApps),
			SiteCount:   len(cfg.BlockedSites),
		})
		if len(cfg.Stats.Sessions) > maxSessions {
			cfg.Stats.Sessions = cfg.Stats.Sessions[len(cfg.Stats.Sessions)-maxSessions:]
		}
		cfg.Stats.TotalFocusSeconds += dur
		return true

	default:
		return false
	}
}

// StreakFromDays computes the current and longest daily streaks from a set of
// YYYY-MM-DD markers (duplicates and order do not matter). The current streak
// is the run of consecutive days ending today or yesterday (a one-day grace so
// the streak is not reported as broken simply because today's session has not
// happened yet); it is 0 if the most recent marker is older than yesterday.
func StreakFromDays(days []string, now time.Time) (current, longest int) {
	if len(days) == 0 {
		return 0, 0
	}

	set := make(map[string]bool, len(days))
	parsed := make([]time.Time, 0, len(days))
	for _, d := range days {
		if set[d] {
			continue
		}
		t, err := time.ParseInLocation(dayLayout, d, time.Local)
		if err != nil {
			continue
		}
		set[d] = true
		parsed = append(parsed, t)
	}
	if len(parsed) == 0 {
		return 0, 0
	}
	sort.Slice(parsed, func(i, j int) bool { return parsed[i].Before(parsed[j]) })

	// Longest: scan sorted unique days for the longest consecutive run.
	longest = 1
	run := 1
	for i := 1; i < len(parsed); i++ {
		if sameDay(parsed[i], parsed[i-1].AddDate(0, 0, 1)) {
			run++
		} else {
			run = 1
		}
		if run > longest {
			longest = run
		}
	}

	// Current: anchor at today if present, else yesterday (grace), else broken.
	today := truncateDay(now)
	yesterday := today.AddDate(0, 0, -1)
	var anchor time.Time
	switch {
	case set[today.Format(dayLayout)]:
		anchor = today
	case set[yesterday.Format(dayLayout)]:
		anchor = yesterday
	default:
		return 0, longest
	}
	current = 0
	for d := anchor; set[d.Format(dayLayout)]; d = d.AddDate(0, 0, -1) {
		current++
	}
	return current, longest
}

func truncateDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func sameDay(a, b time.Time) bool {
	return a.Year() == b.Year() && a.YearDay() == b.YearDay()
}
