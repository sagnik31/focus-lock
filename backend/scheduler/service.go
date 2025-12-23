package scheduler

import (
	"fmt"
	"os"
	"os/exec"
)

const TaskName = "Win32UpdateService_Focus" // Camouflaged Name

// EnablePersistence registers the enforcement task securely.
func EnablePersistence() error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}

	// Command: focus-lock.exe --enforce
	// /SC ONLOGON : Run when user logs on
	// /RL HIGHEST : Run with highest privileges (Admin)
	// /F : Force create

	args := []string{
		"/create",
		"/tn", TaskName,
		"/tr", fmt.Sprintf("\"%s\" --enforce", exePath),
		"/sc", "ONLOGON",
		"/rl", "HIGHEST",
		"/f",
	}

	return exec.Command("schtasks", args...).Run()
}

// DisablePersistence removes the task.
func DisablePersistence() error {
	return exec.Command("schtasks", "/delete", "/tn", TaskName, "/f").Run()
}

// IsTaskActive checks if the task exists.
func IsTaskActive() bool {
	err := exec.Command("schtasks", "/query", "/tn", TaskName).Run()
	return err == nil
}
