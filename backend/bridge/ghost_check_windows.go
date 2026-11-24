//go:build windows

package bridge

import "focus-lock/backend/watchdog"

// isGhostProcessRunning checks if the reduced-mode Ghost is running, by whether
// its single-instance mutex exists. (In strong mode the UI doesn't ask.)
func isGhostProcessRunning() bool {
	return watchdog.InstanceRunning(watchdog.UserGhostMutex)
}
