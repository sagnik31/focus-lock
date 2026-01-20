//go:build windows

package bridge

import (
	"fmt"
	"os/exec"
	"syscall"
)

func spawnGhost(exePath, taskName string) error {
	// 1. Try to launch via Scheduled Task (for Admin privileges without UAC)
	// This only works if the task was created previously (e.g. by installer or Admin setup).
	// We use "schtasks /run" which doesn't trigger UAC if the task is already set up.
	if err := exec.Command("schtasks", "/run", "/tn", taskName).Run(); err == nil {
		fmt.Println("Ghost spawned via Scheduled Task (Admin Mode).")
		return nil
	}

	// 2. Fallback: Direct spawn (User Mode)
	// This won't be able to block websites, but will handle other logic or fail gracefully.
	fmt.Println("Warning: Failed to run Scheduled Task (falling back to User Mode spawn). Blocking may fail.")
	cmd := exec.Command(exePath, "--enforce")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | 0x00000008 | 0x01000000,
	}
	return cmd.Start()
}
