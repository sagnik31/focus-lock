//go:build windows

package scheduler

import (
	"os/exec"
	"syscall"
)

// createNoWindow is CREATE_NO_WINDOW. schtasks is a console program, so
// without it a console window can flash when the GUI runs it.
const createNoWindow = 0x08000000

// command builds an exec.Cmd that runs without a console window, using the
// same SysProcAttr as bridge/spawn_windows.go.
func command(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	return cmd
}
