package main

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"focus-lock/backend/logging"
	"focus-lock/backend/updater"
	"focus-lock/backend/version"
)

// sessionGate returns the exit code that refuses the update, or ApplyStarted.
// A silent update is also refused when the session state can't be read: the
// user can still repair the install by running Setup by hand.
func sessionGate() int {
	active, ok := sessionActiveNow()
	switch {
	case active:
		return updater.ApplySessionActive
	case !ok:
		return updater.ApplySessionUnknown
	default:
		return updater.ApplyStarted
	}
}

// applyUpdateExitCode is the --apply-update mode, run elevated by the
// "FocusLock Updater" task when the user clicks "Install and restart"
// (ADR 0009). It trusts nothing the non-elevated app could have changed: it
// fetches the latest release from GitHub itself, refuses one that isn't newer
// than this (installed) exe, checks the signed checksums, keeps the installer
// in the admin-only %ProgramData%\FocusLock\update, re-checks that no focus
// session is active and then starts Setup silently. The exit code
// (updater.Apply*) is read back by the app as the task's last result.
func applyUpdateExitCode() int {
	dir, err := updater.SecureUpdateDir()
	if err != nil {
		return updater.ApplyFailed
	}
	logPath := filepath.Join(dir, "apply.log")
	logf := func(format string, args ...any) {
		logging.Write(logPath, fmt.Sprintf(format, args...))
	}
	logf("apply-update started (installed %s)", version.Version)

	if code := sessionGate(); code != updater.ApplyStarted {
		logf("refused: session check (code %d)", code)
		return code
	}

	userDir, _ := updater.DefaultDir() // the app's own download, reused if it verifies
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	path, err := updater.New(dir).PrepareApply(ctx, version.Version, userDir)
	if err != nil {
		code := updater.ApplyCode(err)
		logf("refused (code %d): %v", code, err)
		return code
	}

	// The download can take a while: check the session again right before
	// Setup stops the enforcer. (Setup checks once more itself.)
	if code := sessionGate(); code != updater.ApplyStarted {
		logf("refused: session check after the download (code %d)", code)
		return code
	}
	if err := updater.StartSetup(path); err != nil {
		logf("could not start %s: %v", path, err)
		return updater.ApplyLaunchFailed
	}
	logf("started %s", path)
	return updater.ApplyStarted
}
