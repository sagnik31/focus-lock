//go:build !windows

package scheduler

import "os/exec"

// command is exec.Command on platforms without console windows.
func command(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}
