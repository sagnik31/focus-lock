package main

import (
	"time"

	"focus-lock/backend/clock"
	"focus-lock/backend/storage"
)

// Exit codes of the --session-active mode (read by installer\focus-lock.iss).
const (
	exitNoSession     = 0
	exitSessionActive = 3
	exitUnknown       = 4 // config could not be read; the installer proceeds
)

// sessionActiveNow loads the config and reports whether enforcement is active
// right now. It tries to anchor the trusted clock to NTP for a few seconds
// first (so a moved system clock can't sneak an install in), falling back to
// the system clock when offline. ok is false when the store can't be created.
func sessionActiveNow() (active, ok bool) {
	store, err := storage.NewStore()
	if err != nil {
		return false, false
	}
	_ = store.Load() // on error Data still holds what could be recovered

	_ = clock.Default.SyncWithin(3 * time.Second)

	return storage.EnforcementActive(&store.Data, clock.Default.Now()), true
}

// sessionActiveExitCode is the --session-active mode.
func sessionActiveExitCode() int {
	active, ok := sessionActiveNow()
	switch {
	case !ok:
		return exitUnknown
	case active:
		return exitSessionActive
	default:
		return exitNoSession
	}
}
