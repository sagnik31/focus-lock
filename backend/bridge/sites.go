package bridge

import (
	"fmt"
	"focus-lock/backend/blocking/hosts"
	"focus-lock/backend/storage"
	"sort"
)

// GetBlockedSites returns the list of blocked websites
func (a *App) GetBlockedSites() []string {
	a.Store.Load()
	sort.Strings(a.Store.Data.BlockedSites)
	return a.Store.Data.BlockedSites
}

// blockNow pushes the current block list into the hosts file, but only in
// reduced mode. In strong mode the non-elevated UI cannot write the hosts file
// (and must not); the elevated Ghost re-applies it on its next loop tick.
func (a *App) blockNow() {
	if !a.Store.CanSign() {
		return
	}
	a.Store.Load()
	if err := hosts.Block(a.Store.Data.BlockedSites); err != nil {
		fmt.Println("Warning: Failed to update hosts immediately (likely Permission Denied):", err)
	}
}

// AddBlockedSite adds a website to the blocked list
func (a *App) AddBlockedSite(url string) error {
	if err := a.applyOp(storage.Operation{Type: storage.OpAddSite, URL: url}); err != nil {
		return err
	}
	a.blockNow()
	return nil
}

// RemoveBlockedSite removes a website from the blocked list. Removal during an
// active session is refused by the policy (in strong mode, inside the Ghost).
func (a *App) RemoveBlockedSite(url string) error {
	if err := a.applyOp(storage.Operation{Type: storage.OpRemoveSite, URL: url}); err != nil {
		return err
	}
	a.blockNow()
	return nil
}

// AddBlockedSites adds multiple websites to the blocked list
func (a *App) AddBlockedSites(urls []string) error {
	if err := a.applyOp(storage.Operation{Type: storage.OpAddSites, Sites: urls}); err != nil {
		return err
	}
	a.blockNow()
	return nil
}

// RemoveBlockedSites removes multiple websites from the blocked list
func (a *App) RemoveBlockedSites(urls []string) error {
	if err := a.applyOp(storage.Operation{Type: storage.OpRemoveSites, Sites: urls}); err != nil {
		return err
	}
	a.blockNow()
	return nil
}

// SetBlockCommonVPN enables or disables blocking of common VPN sites. Disabling
// during an active session is refused by the policy.
func (a *App) SetBlockCommonVPN(enabled bool) error {
	return a.applyOp(storage.Operation{Type: storage.OpSetVPN, Enabled: enabled})
}

// GetBlockCommonVPN returns whether common VPN blocking is enabled
func (a *App) GetBlockCommonVPN() bool {
	a.Store.Load()
	return a.Store.Data.BlockCommonVPN
}
