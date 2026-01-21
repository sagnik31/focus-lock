//go:build !windows

package bridge

// isGhostProcessRunning returns false on non-Windows platforms
// as the Ghost mechanism is Windows-specific.
func isGhostProcessRunning() bool {
	return false
}
