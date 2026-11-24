package bridge

import (
	"encoding/json"
	"fmt"
	"focus-lock/backend/storage"
	"focus-lock/backend/sysinfo"
	"strings"

	"github.com/google/uuid"
)

// ImportData represents the JSON structure for importing settings
type ImportData struct {
	Blocked     BlockedItems     `json:"blocked"`
	Schedules   []ImportSchedule `json:"schedules"`
	Groups      []ImportGroup    `json:"groups,omitempty"`      // custom website sections
	RecentSites []string         `json:"recentSites,omitempty"` // "Recently used" websites
}

// ImportGroup is a custom website section in import format. A group whose
// name matches an existing one is merged into it.
type ImportGroup struct {
	Name  string   `json:"name"`
	Sites []string `json:"sites"`
}

// BlockedItems represents the blocked apps and sites in import format
type BlockedItems struct {
	Apps  []string `json:"apps"`
	Sites []string `json:"sites"`
}

// ImportSchedule represents a schedule in import format
type ImportSchedule struct {
	Name       string   `json:"name"`
	ActiveDays []string `json:"activeDays"`
	StartTime  string   `json:"startTime"`
	EndTime    string   `json:"endTime"`
}

// resolveAppName attempts to match an imported app name to an installed application
// Returns the executable name (e.g., "WhatsApp.exe") if found, otherwise returns the original input
func resolveAppName(inputName string, installedApps []sysinfo.AppInfo) string {
	inputLower := strings.ToLower(strings.TrimSpace(inputName))
	if inputLower == "" {
		return inputName
	}

	// Only installed apps with a real exe name can be match targets: an app
	// whose exe could not be resolved has Exe "" (older builds: "."), and the
	// substring rules below would otherwise map almost any input onto it.
	candidates := make([]sysinfo.AppInfo, 0, len(installedApps))
	for _, app := range installedApps {
		if storage.ValidAppName(app.Exe) {
			candidates = append(candidates, app)
		}
	}
	installedApps = candidates

	// Priority 1: Exact match on exe name (case-insensitive)
	for _, app := range installedApps {
		if strings.EqualFold(app.Exe, inputName) {
			return app.Exe
		}
	}

	// Priority 2: Exact match on display name (case-insensitive)
	for _, app := range installedApps {
		if strings.EqualFold(app.Name, inputName) {
			return app.Exe
		}
	}

	// An explicit file name means that program, installed or not.
	if strings.HasSuffix(inputLower, ".exe") {
		return strings.TrimSpace(inputName)
	}

	// The fuzzy rules need a few characters to be meaningful.
	if len([]rune(inputLower)) < 3 {
		return inputName
	}

	// Priority 3: Input is contained in display name or exe (case-insensitive)
	for _, app := range installedApps {
		nameLower := strings.ToLower(app.Name)
		exeLower := strings.ToLower(app.Exe)
		if strings.Contains(nameLower, inputLower) || strings.Contains(exeLower, inputLower) {
			return app.Exe
		}
	}

	// Priority 4: Display name or exe contains input (for partial matches like "Steam" -> "steam.exe")
	for _, app := range installedApps {
		exeWithoutExt := strings.TrimSuffix(strings.ToLower(app.Exe), ".exe")
		if len([]rune(exeWithoutExt)) < 3 {
			continue
		}
		if strings.Contains(inputLower, exeWithoutExt) || strings.Contains(exeWithoutExt, inputLower) {
			return app.Exe
		}
	}

	// No match found - return original (user may have entered an exe name directly)
	return inputName
}

// ImportSettings imports settings from a JSON string and merges with existing config
func (a *App) ImportSettings(jsonContent string) error {
	var importData ImportData
	if err := json.Unmarshal([]byte(jsonContent), &importData); err != nil {
		return fmt.Errorf("invalid JSON format: %w", err)
	}

	a.Store.Load()

	// Get installed apps for fuzzy matching
	installedApps, err := sysinfo.GetInstalledApps()
	if err != nil {
		installedApps = []sysinfo.AppInfo{} // Continue without matching if error
	}

	// Resolve + dedupe apps against the current list (case-insensitive).
	existingApps := make(map[string]bool)
	for _, app := range a.Store.Data.BlockedApps {
		existingApps[strings.ToLower(app)] = true
	}
	newApps := []string{}
	for _, app := range importData.Blocked.Apps {
		resolvedApp := resolveAppName(app, installedApps)
		key := strings.ToLower(resolvedApp)
		if !existingApps[key] {
			newApps = append(newApps, resolvedApp)
			existingApps[key] = true
		}
	}

	// Dedupe sites against the current list.
	existingSites := make(map[string]bool)
	for _, site := range a.Store.Data.BlockedSites {
		existingSites[site] = true
	}
	newSites := []string{}
	for _, site := range importData.Blocked.Sites {
		if !existingSites[site] {
			newSites = append(newSites, site)
			existingSites[site] = true
		}
	}

	// Convert schedules.
	newSchedules := make([]storage.Schedule, 0, len(importData.Schedules))
	for _, importSched := range importData.Schedules {
		newSchedules = append(newSchedules, storage.Schedule{
			ID:        uuid.New().String(),
			Name:      importSched.Name,
			Days:      importSched.ActiveDays, // Map activeDays -> days
			StartTime: importSched.StartTime,
			EndTime:   importSched.EndTime,
			Enabled:   true, // Enable by default
		})
	}

	// Convert groups. Unnamed groups are skipped; the policy merges groups
	// whose name already exists and validates the rest.
	newGroups := make([]storage.SiteGroup, 0, len(importData.Groups))
	for _, g := range importData.Groups {
		if strings.TrimSpace(g.Name) == "" {
			continue
		}
		newGroups = append(newGroups, storage.SiteGroup{
			ID:    uuid.New().String(),
			Name:  g.Name,
			Sites: g.Sites,
		})
	}

	// Import is append-only, so the policy always allows it, even mid-lock.
	return a.applyOp(storage.Operation{
		Type:        storage.OpImport,
		Apps:        newApps,
		Sites:       newSites,
		Schedules:   newSchedules,
		SiteGroups:  newGroups,
		RecentSites: importData.RecentSites,
	})
}

// ExportSettings exports current settings to JSON format for sharing
func (a *App) ExportSettings() (string, error) {
	a.Store.Load()

	exportData := ImportData{
		Blocked: BlockedItems{
			Apps:  a.Store.Data.BlockedApps,
			Sites: a.Store.Data.BlockedSites,
		},
		Schedules:   make([]ImportSchedule, 0, len(a.Store.Data.Schedules)),
		RecentSites: a.Store.Data.RecentSites,
	}
	for _, g := range a.Store.Data.SiteGroups {
		exportData.Groups = append(exportData.Groups, ImportGroup{Name: g.Name, Sites: g.Sites})
	}

	for _, sched := range a.Store.Data.Schedules {
		exportData.Schedules = append(exportData.Schedules, ImportSchedule{
			Name:       sched.Name,
			ActiveDays: sched.Days,
			StartTime:  sched.StartTime,
			EndTime:    sched.EndTime,
		})
	}

	jsonBytes, err := json.MarshalIndent(exportData, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to export settings: %w", err)
	}

	return string(jsonBytes), nil
}
