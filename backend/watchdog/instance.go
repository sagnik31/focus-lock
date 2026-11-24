package watchdog

import (
	"errors"

	"golang.org/x/sys/windows"
)

// Named mutexes that keep each mode to a single instance.
const (
	GhostMutex = `Global\FocusLockGhost`
	UIMutex    = `Global\FocusLockMutex`
)

// AcquireInstance claims the named mutex for the life of the process and
// reports whether another process already has it. Any other failure returns
// false: running twice is better than not enforcing at all.
//
// windows.CreateMutex returns ERROR_ALREADY_EXISTS as its error (with a valid
// handle), so the error must be checked; a separate GetLastError call is not
// reliable in Go. ERROR_ACCESS_DENIED means the mutex exists but belongs to a
// process we may not open, such as an elevated one.
func AcquireInstance(name string) (alreadyRunning bool) {
	p, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return false
	}
	h, err := windows.CreateMutex(nil, true, p)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		// Keeping this handle would keep the mutex alive after its owner dies.
		windows.CloseHandle(h)
		return true
	}
	// On success the handle is deliberately left open until process exit.
	return errors.Is(err, windows.ERROR_ACCESS_DENIED)
}

// InstanceRunning reports whether some process holds the named mutex. It only
// opens the mutex: creating it, even briefly, could make an enforcer that is
// starting at that moment think another one is running and exit.
func InstanceRunning(name string) bool {
	p, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return false
	}
	h, err := windows.OpenMutex(windows.SYNCHRONIZE, false, p)
	if err == nil {
		windows.CloseHandle(h)
		return true
	}
	return errors.Is(err, windows.ERROR_ACCESS_DENIED)
}
