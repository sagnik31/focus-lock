package bridge

import (
	"encoding/json"
	"fmt"
	"focus-lock/backend/storage"
	"focus-lock/backend/sysinfo"
	"sort"
	"strings"

	"github.com/google/uuid"
)

// ImportData represents the JSON structure for importing settings
type ImportData struct {
	Blocked   BlockedItems     `json:"blocked"`
	Schedules []ImportSchedule `json:"schedules"`
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

	// Merge blocked apps (avoid duplicates) with fuzzy matching
	existingApps := make(map[string]bool)
	for _, app := range a.Store.Data.BlockedApps {
		existingApps[strings.ToLower(app)] = true
	}
	for _, app := range importData.Blocked.Apps {
		resolvedApp := resolveAppName(app, installedApps)
		if !existingApps[strings.ToLower(resolvedApp)] {
			a.Store.Data.BlockedApps = append(a.Store.Data.BlockedApps, resolvedApp)
			existingApps[strings.ToLower(resolvedApp)] = true
		}
	}
	sort.Strings(a.Store.Data.BlockedApps)

	// Merge blocked sites (avoid duplicates)
	existingSites := make(map[string]bool)
	for _, site := range a.Store.Data.BlockedSites {
		existingSites[site] = true
	}
	for _, site := range importData.Blocked.Sites {
		if !existingSites[site] {
			a.Store.Data.BlockedSites = append(a.Store.Data.BlockedSites, site)
			existingSites[site] = true
		}
	}
	sort.Strings(a.Store.Data.BlockedSites)

	// Convert and append schedules
	for _, importSched := range importData.Schedules {
		schedule := storage.Schedule{
			ID:        uuid.New().String(),
			Name:      importSched.Name,
			Days:      importSched.ActiveDays, // Map activeDays -> days
			StartTime: importSched.StartTime,
			EndTime:   importSched.EndTime,
			Enabled:   true, // Enable by default
		}
		a.Store.Data.Schedules = append(a.Store.Data.Schedules, schedule)
	}

	return a.Store.Save()
}

// ExportSettings exports current settings to JSON format for sharing
func (a *App) ExportSettings() (string, error) {
	a.Store.Load()

	exportData := ImportData{
		Blocked: BlockedItems{
			Apps:  a.Store.Data.BlockedApps,
			Sites: a.Store.Data.BlockedSites,
		},
		Schedules: make([]ImportSchedule, 0, len(a.Store.Data.Schedules)),
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
