package sysinfo

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

type AppInfo struct {
	Name string `json:"name"`
	Icon string `json:"icon"` // Base64 data URI
	Exe  string `json:"exe"`
}

// GetInstalledApps executes a PowerShell script to retrieve installed applications
func GetInstalledApps() ([]AppInfo, error) {
	// Locate the script - assuming it's in the current working directory or relative to executable
	// In development (run from root), it's scripts/get_apps.ps1
	// In production, we might need to look relative to the binary

	scriptPath := "scripts/get_apps.ps1"
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		// Try looking relative to executable if not found in CWD
		exePath, _ := os.Executable()
		scriptPath = filepath.Join(filepath.Dir(exePath), "scripts", "get_apps.ps1")
	}

	cmd := exec.Command("powershell", "-ExecutionPolicy", "Bypass", "-File", scriptPath)

	// Hide the window
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to execute script: %w", err)
	}

	var apps []AppInfo
	if len(output) == 0 {
		return []AppInfo{}, nil
	}

	if err := json.Unmarshal(output, &apps); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	return apps, nil
}
