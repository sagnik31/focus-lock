//go:build !windows

package bridge

import (
	"fmt"
	"os/exec"
)

func spawnGhost(exePath, taskName string) error {
	// On non-Windows platforms, just spawn directly without special process attributes
	fmt.Println("Spawning Ghost process (non-Windows mode).")
	cmd := exec.Command(exePath, "--enforce")
	return cmd.Start()
}
