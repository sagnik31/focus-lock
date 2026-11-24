package storage

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// lockFile takes an exclusive lock on the file at path, shared by every Focus
// Lock process, and returns the function that releases it. After wait it gives
// up and returns a no-op: a stuck process must not stop enforcement.
func lockFile(path string, wait time.Duration) (release func()) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return func() {}
	}
	h := windows.Handle(f.Fd())
	deadline := time.Now().Add(wait)
	for {
		err = windows.LockFileEx(h, windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &windows.Overlapped{})
		if err == nil {
			return func() {
				_ = windows.UnlockFileEx(h, 0, 1, 0, &windows.Overlapped{})
				_ = f.Close()
			}
		}
		if !errors.Is(err, windows.ERROR_LOCK_VIOLATION) || time.Now().After(deadline) {
			_ = f.Close()
			return func() {}
		}
		time.Sleep(5 * time.Millisecond)
	}
}
