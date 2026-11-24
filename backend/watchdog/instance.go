package watchdog

import (
	"errors"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// Named mutexes that keep each mode to a single instance.
const (
	GhostMutex = `Global\FocusLockGhost`
	// UserGhostMutex is for enforcers without the strong-mode secret. It is a
	// separate name so that one started by the direct-spawn fallback can never
	// keep the elevated enforcer out.
	UserGhostMutex = `Global\FocusLockGhostUser`
	UIMutex        = `Global\FocusLockMutex`
)

// AcquireEnforcer claims the single enforcer slot and reports whether another
// enforcer already has it. secureDir is the strong-mode signer's admin-only
// folder ("" for any other enforcer). The signer uses a file there, opened
// without sharing: any process can create a global mutex, but not that file.
func AcquireEnforcer(secureDir string, strong, canSign bool) (alreadyRunning bool) {
	switch {
	case secureDir != "":
		return acquireFile(filepath.Join(secureDir, "enforcer.lock"))
	case strong && canSign:
		return AcquireInstance(GhostMutex)
	default:
		return AcquireInstance(UserGhostMutex)
	}
}

// acquireFile opens path without sharing for the life of the process.
func acquireFile(path string) (alreadyRunning bool) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return false
	}
	_, err = windows.CreateFile(p, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil,
		windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL, 0)
	return errors.Is(err, windows.ERROR_SHARING_VIOLATION)
}

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
