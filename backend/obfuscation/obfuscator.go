package obfuscation

import (
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

// GenerateTaskName creates a plausible-sounding Windows system task name
func GenerateTaskName() string {
	subsystems := []string{"Windows", "Win32", "Shell", "UserSession", "AppX", "Runtime", "System", "Net", "Host"}
	components := []string{"Experience", "Telemetry", "Broker", "Cache", "Component", "Host", "Service", "Manager", "Provider"}
	actions := []string{"Update", "Sync", "Maintenance", "Refresh", "Coordinator", "Handler", "Monitor", "Helper"}

	return fmt.Sprintf("%s%s%s",
		subsystems[rand.Intn(len(subsystems))],
		components[rand.Intn(len(components))],
		actions[rand.Intn(len(actions))])
}

// SetupGhostExecutable duplicates the current executable to a hidden location with a new name
func SetupGhostExecutable(originalPath, taskName string) (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user config dir: %w", err)
	}

	// Use a subdirectory in AppData/Roaming to look legit but be writable
	// e.g. AppData/Roaming/Microsoft/Windows/Templates/Cache
	// Using our own hidden folder for now to avoid permission issues with system folders
	binDir := filepath.Join(configDir, "FocusLock", "Bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create bin dir: %w", err)
	}

	newExeName := taskName + ".exe"
	newPath := filepath.Join(binDir, newExeName)

	// Copy file
	if err := copyFile(originalPath, newPath); err != nil {
		return "", fmt.Errorf("failed to copy executable: %w", err)
	}

	return newPath, nil
}

// CleanupGhostExecutable removes the obfuscated executable
func CleanupGhostExecutable(path string) error {
	if path == "" {
		return nil
	}
	// Best effort cleanup
	return os.Remove(path)
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}
