//go:build windows

package bridge

import "focus-lock/backend/watchdog"

// isGhostProcessRunning checks if the Ghost process is currently running, by
// whether its single-instance mutex exists.
func isGhostProcessRunning() bool {
	return watchdog.InstanceRunning(watchdog.GhostMutex)
}
