package storage

import (
	"errors"
	"sort"
	"strings"
	"time"
)

// OpType identifies a config-mutating operation requested by the UI.
type OpType string

const (
	OpStartFocus      OpType = "start_focus"
	OpStopFocus       OpType = "stop_focus"
	OpEmergencyUnlock OpType = "emergency_unlock"
	OpAddApp          OpType = "add_app"
	OpRemoveApp       OpType = "remove_app"
	OpSetApps         OpType = "set_apps"
	OpAddSite         OpType = "add_site"
	OpAddSites        OpType = "add_sites"
	OpRemoveSite      OpType = "remove_site"
	OpRemoveSites     OpType = "remove_sites"
	OpSetVPN          OpType = "set_vpn"
	OpSaveSchedules   OpType = "save_schedules"
	OpImport          OpType = "import"
	OpSetGhostInfo    OpType = "set_ghost_info"
	OpForgetRecent    OpType = "forget_recent_site"
	OpSaveSiteGroups  OpType = "save_site_groups"
)

// maxRecentSites caps the "Recently used" history kept in the config.
const maxRecentSites = 20

// Operation is a single config change. It is JSON-serialisable so the
// non-elevated UI can hand it to the elevated Ghost through the request
// channel. Only fields relevant to Type are populated.
type Operation struct {
	Type          OpType      `json:"type"`
	Seconds       int         `json:"seconds,omitempty"`
	Name          string      `json:"name,omitempty"`
	URL           string      `json:"url,omitempty"`
	Apps          []string    `json:"apps,omitempty"`
	Sites         []string    `json:"sites,omitempty"`
	Schedules     []Schedule  `json:"schedules,omitempty"`
	Enabled       bool        `json:"enabled,omitempty"`
	GhostTaskName string      `json:"ghost_task_name,omitempty"`
	GhostExePath  string      `json:"ghost_exe_path,omitempty"`
	SiteGroups    []SiteGroup `json:"site_groups,omitempty"`
	RecentSites   []string    `json:"recent_sites,omitempty"`
}

// ManualLockActive reports whether a manual (timer) lock is currently running.
func ManualLockActive(cfg *Config, now time.Time) bool {
	return !cfg.LockEndTime.IsZero() && now.Before(cfg.LockEndTime)
}

// EnforcementActive reports whether anything is being enforced right now
// (a manual lock or an active schedule window).
func EnforcementActive(cfg *Config, now time.Time) bool {
	return ManualLockActive(cfg, now) || SchedulesActiveAt(cfg.Schedules, now)
}

// ApplyOperation is the single source of truth for every config mutation.
// It validates the operation against the lock-integrity policy and, if
// allowed, mutates cfg in place. A non-nil error means the operation was
// REFUSED and cfg is left untouched for that change.
//
// The policy, during any active enforcement window (manual lock or schedule):
//   - STRENGTHENING is always allowed: start/extend a lock, add apps/sites,
//     enable VPN blocking, add schedules, import (append-only), emergency
//     unlock within its remaining quota.
//   - WEAKENING is refused: ending/shortening a lock, removing apps/sites,
//     disabling VPN blocking, deleting/disabling an active schedule.
//
// This function is pure with respect to side effects: it only touches cfg.
// Callers perform process/task side effects (spawning, cleanup) separately.
func ApplyOperation(cfg *Config, op Operation, now time.Time) error {
	locked := EnforcementActive(cfg, now)

	switch op.Type {
	case OpStartFocus:
		return applyStartFocus(cfg, op, now)

	case OpStopFocus:
		// Clearing a manual lock is only allowed once it has expired. An
		// active lock cannot be ended early (emergency unlock is the only
		// sanctioned pause). A schedule window ends on its own clock, so
		// StopFocus never clears schedules.
		if ManualLockActive(cfg, now) {
			return errors.New("cannot stop an active focus session")
		}
		cfg.LockEndTime = time.Time{}
		cfg.RemainingDuration = 0
		return nil

	case OpEmergencyUnlock:
		// Refill the quota if a new schedule window has become active since it
		// was last tied to one. Manual sessions refill in applyStartFocus; this
		// covers recurring schedule windows, which otherwise never refilled.
		applyScheduleUnlockReset(cfg, now)
		if cfg.EmergencyUnlocksUsed >= 2 {
			return errors.New("emergency unlock limit reached (2/2)")
		}
		cfg.PausedUntil = now.Add(1 * time.Minute)
		cfg.EmergencyUnlocksUsed++
		return nil

	case OpAddApp:
		name := strings.TrimSpace(op.Name)
		if name == "" {
			return errors.New("app name cannot be empty")
		}
		if !ValidAppName(name) {
			return errors.New("\"" + name + "\" is not a valid app name; use the program's .exe name")
		}
		cfg.BlockedApps = addUnique(cfg.BlockedApps, name)
		sort.Strings(cfg.BlockedApps)
		return nil

	case OpRemoveApp:
		if locked {
			return errors.New("cannot remove apps during an active focus session")
		}
		cfg.BlockedApps = remove(cfg.BlockedApps, op.Name)
		return nil

	case OpSetApps:
		// Wholesale replace. During a lock this may only ADD: the new set
		// must contain every currently blocked app, or it is a removal.
		// Invalid names (e.g. "." from an app with no resolved exe) are
		// dropped: they never blocked anything useful, so dropping one is
		// not a weakening and is allowed even mid-lock.
		apps := ValidApps(op.Apps)
		if locked && !isSuperset(apps, ValidApps(cfg.BlockedApps)) {
			return errors.New("cannot remove apps during an active focus session")
		}
		sort.Strings(apps)
		cfg.BlockedApps = apps
		return nil

	case OpAddSite:
		cfg.BlockedSites = addUnique(cfg.BlockedSites, op.URL)
		sort.Strings(cfg.BlockedSites)
		touchRecent(cfg, op.URL)
		return nil

	case OpAddSites:
		for _, s := range op.Sites {
			cfg.BlockedSites = addUnique(cfg.BlockedSites, s)
		}
		sort.Strings(cfg.BlockedSites)
		touchRecent(cfg, op.Sites...)
		return nil

	case OpRemoveSite:
		if locked {
			return errors.New("cannot remove sites during an active focus session")
		}
		cfg.BlockedSites = remove(cfg.BlockedSites, op.URL)
		// Removing a site is exactly when it becomes useful in "Recently
		// used" (it was blocked, now it is not), and it backfills sites that
		// were blocked before recent history existed.
		touchRecent(cfg, op.URL)
		return nil

	case OpRemoveSites:
		if locked {
			return errors.New("cannot remove sites during an active focus session")
		}
		for _, s := range op.Sites {
			cfg.BlockedSites = remove(cfg.BlockedSites, s)
		}
		touchRecent(cfg, op.Sites...)
		return nil

	case OpSetVPN:
		// Enabling strengthens (always ok); disabling weakens (refused while locked).
		if locked && !op.Enabled && cfg.BlockCommonVPN {
			return errors.New("cannot disable VPN blocking during an active focus session")
		}
		cfg.BlockCommonVPN = op.Enabled
		return nil

	case OpSaveSchedules:
		return applySaveSchedules(cfg, op, locked)

	case OpImport:
		// Import is append-only, so it only ever strengthens.
		for _, a := range ValidApps(op.Apps) {
			cfg.BlockedApps = addUnique(cfg.BlockedApps, a)
		}
		sort.Strings(cfg.BlockedApps)
		for _, s := range op.Sites {
			cfg.BlockedSites = addUnique(cfg.BlockedSites, s)
		}
		sort.Strings(cfg.BlockedSites)
		cfg.Schedules = append(cfg.Schedules, op.Schedules...)
		if err := mergeSiteGroups(cfg, op.SiteGroups); err != nil {
			return err
		}
		mergeRecent(cfg, op.RecentSites)
		return nil

	case OpForgetRecent:
		// History only; never touches what is blocked, so always allowed.
		cfg.RecentSites = remove(cfg.RecentSites, op.URL)
		return nil

	case OpSaveSiteGroups:
		// Groups are presets: saving them never changes what is blocked, so
		// this is allowed even during a session (toggling a group off goes
		// through RemoveSites, which the lock policy guards).
		groups, err := cleanSiteGroups(op.SiteGroups)
		if err != nil {
			return err
		}
		cfg.SiteGroups = groups
		return nil

	case OpSetGhostInfo:
		// Bookkeeping only: record where the Ghost exe/task live. Never
		// weakens enforcement.
		if op.GhostTaskName != "" {
			cfg.GhostTaskName = op.GhostTaskName
		}
		if op.GhostExePath != "" {
			cfg.GhostExePath = op.GhostExePath
		}
		cfg.LastEnforcementError = ""
		return nil

	default:
		return errors.New("unknown operation: " + string(op.Type))
	}
}

func applyStartFocus(cfg *Config, op Operation, now time.Time) error {
	if op.Seconds <= 0 {
		return errors.New("duration must be positive")
	}
	newEnd := now.Add(time.Duration(op.Seconds) * time.Second)

	if ManualLockActive(cfg, now) {
		// Extending an already-running lock. Never shorten it, and do NOT
		// refill the emergency-unlock quota (that would be a weakening).
		if newEnd.After(cfg.LockEndTime) {
			cfg.LockEndTime = newEnd
			cfg.RemainingDuration = newEnd.Sub(now)
		}
	} else {
		// Fresh lock.
		cfg.LockEndTime = newEnd
		cfg.RemainingDuration = time.Duration(op.Seconds) * time.Second
		cfg.EmergencyUnlocksUsed = 0
		cfg.SessionStart = now // marks the start for history; recorded when the session ends
	}

	if op.GhostTaskName != "" {
		cfg.GhostTaskName = op.GhostTaskName
	}
	if op.GhostExePath != "" {
		cfg.GhostExePath = op.GhostExePath
	}
	cfg.LastEnforcementError = ""
	return nil
}

// applyScheduleUnlockReset refills the emergency-unlock quota when a new
// schedule window becomes active. For a recurring schedule, "a session" is one
// window instance (identified by ActiveScheduleWindow's key); when the active
// window set differs from the one the quota is currently tied to, the quota is
// reset and re-tied. It is a no-op when no schedule window is active (manual
// locks refill separately in applyStartFocus) or when the window is unchanged,
// so it is safe to call on every operation and every enforcement tick. Returns
// true if it changed cfg.
func applyScheduleUnlockReset(cfg *Config, now time.Time) bool {
	key, active := ActiveScheduleWindow(cfg.Schedules, now)
	if !active || cfg.ScheduleUnlockWindow == key {
		return false
	}
	cfg.ScheduleUnlockWindow = key
	cfg.EmergencyUnlocksUsed = 0
	return true
}

func applySaveSchedules(cfg *Config, op Operation, locked bool) error {
	if locked {
		// A previously-enabled schedule may not be deleted, disabled or
		// re-timed (days, start, end) while anything is being enforced:
		// moving the end of the active window to the past would end it.
		// Adding schedules, enabling one and renaming are fine.
		newByID := make(map[string]Schedule, len(op.Schedules))
		for _, s := range op.Schedules {
			newByID[s.ID] = s
		}
		for _, old := range cfg.Schedules {
			if !old.Enabled {
				continue
			}
			ns, exists := newByID[old.ID]
			if !exists {
				return errors.New("cannot delete enabled schedules during an active focus session")
			}
			if !ns.Enabled {
				return errors.New("cannot disable active schedules during an active focus session")
			}
			if ns.StartTime != old.StartTime || ns.EndTime != old.EndTime || !sameDays(ns.Days, old.Days) {
				return errors.New("cannot change the days or times of enabled schedules during an active focus session")
			}
		}
	}
	cfg.Schedules = op.Schedules
	return nil
}

// touchRecent moves the given sites to the front of the recent-sites history
// (most recent first), de-duplicated and capped at maxRecentSites.
func touchRecent(cfg *Config, sites ...string) {
	var front []string
	for i := len(sites) - 1; i >= 0; i-- {
		s := strings.TrimSpace(sites[i])
		if s == "" {
			continue
		}
		front = append([]string{s}, remove(front, s)...)
	}
	if len(front) == 0 {
		return
	}
	rest := cfg.RecentSites
	for _, s := range front {
		rest = remove(rest, s)
	}
	cfg.RecentSites = append(front, rest...)
	if len(cfg.RecentSites) > maxRecentSites {
		cfg.RecentSites = cfg.RecentSites[:maxRecentSites]
	}
}

// mergeSiteGroups folds imported groups into the existing ones. A group whose
// name matches an existing group (case-insensitive) adds its sites to that
// group; any other group is appended. Groups are presets, so this never
// changes what is blocked.
func mergeSiteGroups(cfg *Config, imported []SiteGroup) error {
	if len(imported) == 0 {
		return nil
	}
	merged := append([]SiteGroup{}, cfg.SiteGroups...)
	for _, g := range imported {
		name := strings.TrimSpace(g.Name)
		found := false
		for i := range merged {
			if strings.EqualFold(merged[i].Name, name) {
				sites := append([]string{}, merged[i].Sites...)
				for _, s := range g.Sites {
					sites = addUnique(sites, strings.TrimSpace(s))
				}
				merged[i].Sites = sites
				found = true
				break
			}
		}
		if !found {
			merged = append(merged, g)
		}
	}
	groups, err := cleanSiteGroups(merged)
	if err != nil {
		return err
	}
	cfg.SiteGroups = groups
	return nil
}

// mergeRecent appends imported recent sites after the existing history (the
// user's own recent activity stays first), de-duplicated and capped.
func mergeRecent(cfg *Config, imported []string) {
	for _, s := range imported {
		s = strings.TrimSpace(s)
		if s == "" || len(cfg.RecentSites) >= maxRecentSites {
			continue
		}
		cfg.RecentSites = addUnique(cfg.RecentSites, s)
	}
}

// cleanSiteGroups validates user-defined groups: every group needs an ID and
// a non-empty name, names are unique (case-insensitive), and each group's
// sites are trimmed and de-duplicated.
func cleanSiteGroups(in []SiteGroup) ([]SiteGroup, error) {
	out := make([]SiteGroup, 0, len(in))
	names := make(map[string]bool)
	for _, g := range in {
		name := strings.TrimSpace(g.Name)
		if g.ID == "" {
			return nil, errors.New("group is missing an id")
		}
		if name == "" {
			return nil, errors.New("group name cannot be empty")
		}
		key := strings.ToLower(name)
		if names[key] {
			return nil, errors.New("a group named \"" + name + "\" already exists")
		}
		names[key] = true
		sites := []string{}
		for _, s := range g.Sites {
			s = strings.TrimSpace(s)
			if s != "" {
				sites = addUnique(sites, s)
			}
		}
		out = append(out, SiteGroup{ID: g.ID, Name: name, Sites: sites})
	}
	return out, nil
}

// sameDays reports whether two day lists hold the same days, in any order.
func sameDays(a, b []string) bool {
	return isSuperset(a, b) && isSuperset(b, a)
}

// --- small slice helpers ---

func addUnique(list []string, v string) []string {
	for _, e := range list {
		if e == v {
			return list
		}
	}
	return append(list, v)
}

func remove(list []string, v string) []string {
	out := make([]string, 0, len(list))
	for _, e := range list {
		if e != v {
			out = append(out, e)
		}
	}
	return out
}

// isSuperset reports whether every element of sub is present in super.
func isSuperset(super, sub []string) bool {
	set := make(map[string]struct{}, len(super))
	for _, e := range super {
		set[e] = struct{}{}
	}
	for _, e := range sub {
		if _, ok := set[e]; !ok {
			return false
		}
	}
	return true
}
