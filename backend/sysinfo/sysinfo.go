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
	Name string `json:"name"`
	Icon string `json:"icon"` // Base64 data URI
	Exe  string `json:"exe"`
}

//go:embed get_apps.ps1
var getAppsScript []byte

// GetInstalledApps executes a PowerShell script to retrieve installed applications
func GetInstalledApps() ([]AppInfo, error) {
	// Write embedded script to a temp file
	tmpFile, err := os.CreateTemp("", "get_apps_*.ps1")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name()) // Clean up

	if _, err := tmpFile.Write(getAppsScript); err != nil {
		return nil, fmt.Errorf("failed to write script to temp file: %w", err)
	}
	tmpFile.Close()

	cmd := exec.Command("powershell", "-ExecutionPolicy", "Bypass", "-File", tmpFile.Name())

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
