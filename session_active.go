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

// sessionActiveExitCode loads the config and reports whether enforcement is
// active right now. It tries to anchor the trusted clock to NTP for a few
// seconds first (so a moved system clock can't sneak an install in), falling
// back to the system clock when offline.
func sessionActiveExitCode() int {
	store, err := storage.NewStore()
	if err != nil {
		return exitUnknown
	}
	_ = store.Load() // on error Data still holds what could be recovered

	_ = clock.Default.SyncWithin(3 * time.Second)

	if storage.EnforcementActive(&store.Data, clock.Default.Now()) {
		return exitSessionActive
	}
	return exitNoSession
}
