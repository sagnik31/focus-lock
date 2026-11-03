package storage

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"golang.org/x/sys/windows/registry"
)

const (
	// baseSubkey holds the state backup and the strong-mode marker. Under HKLM
	// its default ACL is Users-read / Administrators-write, which is exactly
	// what we want: the UI can read the backup, only the elevated Ghost writes.
	baseSubkey = `Software\FocusLock`
	// securitySubkey holds the HMAC secret in strong mode. The admin setup
	// script creates it with an ACL that DENIES the interactive user, so only
	// SYSTEM/Administrators (i.e. the elevated Ghost) can read it.
	securitySubkey = `Software\FocusLock\Security`

	keySecret      = "SecretKey"
	keyProvisioned = "Provisioned"
	keyLockEnd     = "LockEndTime"
	keyRemDur      = "RemainingDuration"
	keyPausedUse   = "PausedUntil"
)

// RegistryStore handles the registry-backed secret and state backup.
// strong is set by ResolveSecret and selects the HKLM hive for the backup.
type RegistryStore struct {
	strong bool
}

func NewRegistryStore() *RegistryStore {
	return &RegistryStore{}
}

// ResolveSecret determines the operating mode and returns the HMAC secret.
//
//   - Strong mode (admin setup has run): the secret lives in an ACL-restricted
//     HKLM subkey. The elevated Ghost can read it (canSign=true); the
//     non-elevated UI cannot (canSign=false) and must use the request channel.
//   - Reduced mode (not provisioned, e.g. dev or pre-setup): falls back to the
//     per-user HKCU secret, and whoever runs can sign, as before.
func (r *RegistryStore) ResolveSecret() (secret []byte, strong bool, canSign bool) {
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, baseSubkey, registry.QUERY_VALUE); err == nil {
		prov, _, perr := k.GetIntegerValue(keyProvisioned)
		k.Close()
		if perr == nil && prov == 1 {
			r.strong = true
			// Try to read the secret from the restricted Security subkey.
			if sk, serr := registry.OpenKey(registry.LOCAL_MACHINE, securitySubkey, registry.QUERY_VALUE); serr == nil {
				val, _, verr := sk.GetStringValue(keySecret)
				sk.Close()
				if verr == nil && val != "" {
					if b, derr := hex.DecodeString(val); derr == nil {
						return b, true, true // Ghost: can read the secret, can sign.
					}
				}
			}
			// Provisioned but the secret is unreadable here: this is the UI.
			return nil, true, false
		}
	}

	// Reduced mode: legacy per-user secret.
	r.strong = false
	b, _ := r.getOrCreateHKCUSecret()
	return b, false, true
}

// getOrCreateHKCUSecret retrieves or creates the legacy per-user HMAC secret.
func (r *RegistryStore) getOrCreateHKCUSecret() ([]byte, error) {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, baseSubkey, registry.ALL_ACCESS)
	if err != nil {
		return nil, fmt.Errorf("registry create failed: %w", err)
	}
	defer k.Close()

	val, _, err := k.GetStringValue(keySecret)
	if err == nil && val != "" {
		return hex.DecodeString(val)
	}

	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return nil, err
	}
	hexStr := hex.EncodeToString(bytes)
	if err := k.SetStringValue(keySecret, hexStr); err != nil {
		return nil, err
	}
	return bytes, nil
}

// backupHive returns the registry root used for the state backup, which
// follows the mode: HKLM in strong mode, HKCU in reduced mode.
func (r *RegistryStore) backupHive() registry.Key {
	if r.strong {
		return registry.LOCAL_MACHINE
	}
	return registry.CURRENT_USER
}

// SaveBackup persists critical lock-timing state. In strong mode this writes
// to HKLM and therefore requires elevation (only the Ghost calls it).
func (r *RegistryStore) SaveBackup(lockEnd time.Time, remaining time.Duration, pausedUntil time.Time) error {
	k, _, err := registry.CreateKey(r.backupHive(), baseSubkey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()

	if err := k.SetQWordValue(keyLockEnd, uint64(lockEnd.Unix())); err != nil {
		return err
	}
	if err := k.SetQWordValue(keyRemDur, uint64(remaining)); err != nil {
		return err
	}
	if !pausedUntil.IsZero() {
		return k.SetQWordValue(keyPausedUse, uint64(pausedUntil.Unix()))
	}
	return k.SetQWordValue(keyPausedUse, 0)
}

// LoadBackup retrieves the state backup. Readable by the UI in both modes.
func (r *RegistryStore) LoadBackup() (time.Time, time.Duration, time.Time, error) {
	k, err := registry.OpenKey(r.backupHive(), baseSubkey, registry.QUERY_VALUE)
	if err != nil {
		return time.Time{}, 0, time.Time{}, err
	}
	defer k.Close()

	lockEndUnix, _, err := k.GetIntegerValue(keyLockEnd)
	if err != nil {
		return time.Time{}, 0, time.Time{}, err
	}
	remDur, _, err := k.GetIntegerValue(keyRemDur)
	if err != nil {
		return time.Time{}, 0, time.Time{}, err
	}

	var pausedUntil time.Time
	if pausedUnix, _, perr := k.GetIntegerValue(keyPausedUse); perr == nil && pausedUnix > 0 {
		pausedUntil = time.Unix(int64(pausedUnix), 0)
	}

	lockEnd := time.Time{}
	if lockEndUnix > 0 {
		lockEnd = time.Unix(int64(lockEndUnix), 0)
	}
	return lockEnd, time.Duration(remDur), pausedUntil, nil
}
