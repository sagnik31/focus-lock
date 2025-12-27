package sysinfo

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

type AppInfo struct {
	Name     string `json:"name"`
	Icon     string `json:"icon"` // Base64 data URI
	Exe      string `json:"exe"`
	FullPath string `json:"-"`      // Internal use for icon fetching
	Source   string `json:"source"` // "Registry", "AppPath", or "Store"
}

//go:embed get_icons.ps1
var getIconsScript []byte

// GetInstalledApps retrieves installed applications using Go registry scanning
// and powershell for icon extraction.
func GetInstalledApps() ([]AppInfo, error) {
	// 1. Get List of Apps from Registry (and running processes)
	// getAppsFromRegistry is defined in apps_windows.go
	apps, err := getAppsFromRegistry()
	if err != nil {
		return nil, fmt.Errorf("failed to get apps from registry: %w", err)
	}

	// 2. Prepare paths for icon extraction
	pathMap := make(map[string]*AppInfo)
	var paths []string

	for i := range apps {
		info := &apps[i]
		if info.FullPath != "" {
			paths = append(paths, info.FullPath)
			pathMap[info.FullPath] = info
		}
	}

	// 3. Batch fetch icons (if any paths found)
	if len(paths) > 0 {
		iconMap, err := fetchIconsBatch(paths)
		if err == nil {
			for path, iconRes := range iconMap {
				if target, ok := pathMap[path]; ok {
					target.Icon = iconRes
				}
			}
		} else {
			// Log error but continue? Or return error?
			// Ideally we just have no icons if this fails.
			fmt.Printf("Error fetching icons: %v\n", err)
		}
	} else {
		// If no paths found, returns the apps without icons (which is fine)
	}

	return apps, nil
}

// fetchIconsBatch calls the PowerShell script to get icons for a list of paths
func fetchIconsBatch(paths []string) (map[string]string, error) {
	// Create temp input file
	tmpInput, err := os.CreateTemp("", "icon_paths_*.json")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpInput.Name())

	encoder := json.NewEncoder(tmpInput)
	if err := encoder.Encode(paths); err != nil {
		return nil, err
	}
	tmpInput.Close()

	// Create temp script file
	tmpScript, err := os.CreateTemp("", "get_icons_*.ps1")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpScript.Name())

	if _, err := tmpScript.Write(getIconsScript); err != nil {
		return nil, err
	}
	tmpScript.Close()

	// Execute PowerShell
	cmd := exec.Command("powershell", "-ExecutionPolicy", "Bypass", "-File", tmpScript.Name(), "-InputPath", tmpInput.Name())

	// Hide the window
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to execute icon script: %w", err)
	}

	var result map[string]string
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("failed to parse icon JSON: %w", err)
	}

	return result, nil
}
