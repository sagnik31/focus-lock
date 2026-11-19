package bridge

import (
	"focus-lock/backend/storage"
	"strings"
)

// Enforcement warnings.
//
// In reduced mode a warning is written to the signed config
// (LastEnforcementError), where the enforcer also records its own. In strong
// mode the UI can't sign, so a Save would fail and the next Load would drop
// the message. The UI keeps it in memory instead (localWarning) and GetConfig
// merges it into the config it returns.

// warnEnforcement records a user-visible warning when an enforcement setup
// step fails, so the UI can tell the user that blocking may not be active.
// A nil error is a no-op (it does not clear an existing warning; callers
// reset the warning before a fresh attempt).
func (a *App) warnEnforcement(context string, err error) {
	if err == nil {
		return
	}
	msg := context + ": " + err.Error()
	if !a.Store.CanSign() {
		a.warnMu.Lock()
		a.localWarning = msg
		a.warnMu.Unlock()
		return
	}
	a.Store.Data.LastEnforcementError = msg
	_ = a.Store.Save()
}

// clearLocalWarning drops the UI-local warning before a fresh attempt.
func (a *App) clearLocalWarning() {
	a.warnMu.Lock()
	a.localWarning = ""
	a.warnMu.Unlock()
}

// withLocalWarning merges the UI-local warning into cfg. It only applies when
// this process can't sign; in reduced mode warnings live in the config.
func (a *App) withLocalWarning(cfg storage.Config) storage.Config {
	if a.Store.CanSign() {
		return cfg
	}
	a.warnMu.Lock()
	local := a.localWarning
	a.warnMu.Unlock()
	cfg.LastEnforcementError = mergeWarnings(cfg.LastEnforcementError, local)
	return cfg
}

// mergeWarnings joins two warnings, skipping empty and repeated ones.
func mergeWarnings(stored, local string) string {
	switch {
	case local == "" || strings.Contains(stored, local):
		return stored
	case stored == "":
		return local
	default:
		return stored + "; " + local
	}
}
