package scheduler

import (
	"fmt"
	"os/exec"
)

// EnablePersistence registers the enforcement task securely.
func EnablePersistence(exePath, taskName string) error {
	// Command: <exePath> --enforce
	// /SC ONLOGON : Run when user logs on
	// /RL HIGHEST : Run with highest privileges (Admin)
	// /F : Force create

	args := []string{
		"/create",
		"/tn", taskName,
		"/tr", fmt.Sprintf("\"%s\" --enforce", exePath),
		"/sc", "ONLOGON",
		"/rl", "HIGHEST",
		"/f",
	}

	return exec.Command("schtasks", args...).Run()
}

// DisablePersistence removes the task.
func DisablePersistence(taskName string) error {
	if taskName == "" {
		return nil
	}
	return exec.Command("schtasks", "/delete", "/tn", taskName, "/f").Run()
}

// IsTaskActive checks if the task exists.
func IsTaskActive(taskName string) bool {
	if taskName == "" {
		return false
	}
	err := exec.Command("schtasks", "/query", "/tn", taskName).Run()
	return err == nil
}
