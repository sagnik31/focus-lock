//go:build windows

package bridge

import (
	"golang.org/x/sys/windows"
)

// isGhostProcessRunning checks if the Ghost process is currently running
// by attempting to acquire its mutex. If the mutex already exists, a Ghost is running.
func isGhostProcessRunning() bool {
	mutexName, err := windows.UTF16PtrFromString("Global\\FocusLockGhost")
	if err != nil {
		return false
	}

	// Try to create/open the mutex
	handle, err := windows.CreateMutex(nil, false, mutexName)
	if err != nil {
		// If we get ACCESS_DENIED, it likely exists and is held by another process
		return true
	}

	// Check if it already existed (another process holds it)
	if windows.GetLastError() == windows.ERROR_ALREADY_EXISTS {
		// Close our handle since we don't need it
		windows.CloseHandle(handle)
		return true
	}

	// We successfully created/acquired it, meaning no Ghost is running
	// Release it immediately so the Ghost can acquire it when spawned
	windows.CloseHandle(handle)
	return false
}
