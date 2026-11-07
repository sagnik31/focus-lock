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
)

// Operation is a single config change. It is JSON-serialisable so the
// non-elevated UI can hand it to the elevated Ghost through the request
// channel. Only fields relevant to Type are populated.
type Operation struct {
	Type          OpType     `json:"type"`
	Seconds       int        `json:"seconds,omitempty"`
	Name          string     `json:"name,omitempty"`
	URL           string     `json:"url,omitempty"`
	Apps          []string   `json:"apps,omitempty"`
	Sites         []string   `json:"sites,omitempty"`
	Schedules     []Schedule `json:"schedules,omitempty"`
	Enabled       bool       `json:"enabled,omitempty"`
	GhostTaskName string     `json:"ghost_task_name,omitempty"`
	GhostExePath  string     `json:"ghost_exe_path,omitempty"`
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
		if locked && !isSuperset(op.Apps, cfg.BlockedApps) {
			return errors.New("cannot remove apps during an active focus session")
		}
		apps := append([]string{}, op.Apps...)
		sort.Strings(apps)
		cfg.BlockedApps = apps
		return nil

	case OpAddSite:
		cfg.BlockedSites = addUnique(cfg.BlockedSites, op.URL)
		sort.Strings(cfg.BlockedSites)
		return nil

	case OpAddSites:
		for _, s := range op.Sites {
			cfg.BlockedSites = addUnique(cfg.BlockedSites, s)
		}
		sort.Strings(cfg.BlockedSites)
		return nil

	case OpRemoveSite:
		if locked {
			return errors.New("cannot remove sites during an active focus session")
		}
		cfg.BlockedSites = remove(cfg.BlockedSites, op.URL)
		return nil

	case OpRemoveSites:
		if locked {
			return errors.New("cannot remove sites during an active focus session")
		}
		for _, s := range op.Sites {
			cfg.BlockedSites = remove(cfg.BlockedSites, s)
		}
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
		for _, a := range op.Apps {
			cfg.BlockedApps = addUnique(cfg.BlockedApps, a)
		}
		sort.Strings(cfg.BlockedApps)
		for _, s := range op.Sites {
			cfg.BlockedSites = addUnique(cfg.BlockedSites, s)
		}
		sort.Strings(cfg.BlockedSites)
		cfg.Schedules = append(cfg.Schedules, op.Schedules...)
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

func applySaveSchedules(cfg *Config, op Operation, locked bool) error {
	if locked {
		// A previously-enabled schedule may not be deleted or disabled while
		// anything is being enforced; everything else (adding, renaming,
		// editing times of non-active ones) is fine.
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
		}
	}
	cfg.Schedules = op.Schedules
	return nil
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
