package obfuscation

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// GenerateTaskName returns a fixed name for the ghost task to allow persistent Admin setup.
func GenerateTaskName() string {
	return "FocusLockGhost"
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
