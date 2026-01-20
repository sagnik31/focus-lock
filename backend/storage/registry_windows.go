package storage

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"golang.org/x/sys/windows/registry"
)

const (
	registryPath = `Software\FocusLock`
	keySecret    = "SecretKey"
	keyLockEnd   = "LockEndTime"
	keyRemDur    = "RemainingDuration"
	keyPausedUse = "PausedUntil"
)

// RegistryStore handles backup storage in Windows Registry
type RegistryStore struct{}

func NewRegistryStore() *RegistryStore {
	return &RegistryStore{}
}

func (r *RegistryStore) openKey(access uint32) (registry.Key, error) {
	return registry.OpenKey(registry.CURRENT_USER, registryPath, access)
}

func (r *RegistryStore) createKey() (registry.Key, bool, error) {
	return registry.CreateKey(registry.CURRENT_USER, registryPath, registry.ALL_ACCESS)
}

// GetOrCreateSecret retrieves the HMAC secret key or creates a new one if missing.
func (r *RegistryStore) GetOrCreateSecret() ([]byte, error) {
	k, _, err := r.createKey()
	if err != nil {
		return nil, fmt.Errorf("registry create failed: %w", err)
	}
	defer k.Close()

	val, _, err := k.GetStringValue(keySecret)
	if err == nil && val != "" {
		return hex.DecodeString(val)
	}

	// Generate new secret
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

// SaveBackup persists critical state to Registry
func (r *RegistryStore) SaveBackup(lockEnd time.Time, remaining time.Duration, pausedUntil time.Time) error {
	k, _, err := r.createKey()
	if err != nil {
		return err
	}
	defer k.Close()

	// Store times as Unix timestamps / Duration nanoseconds
	if err := k.SetQWordValue(keyLockEnd, uint64(lockEnd.Unix())); err != nil {
		return err
	}
	if err := k.SetQWordValue(keyRemDur, uint64(remaining)); err != nil {
		return err
	}
	// Store PausedUntil
	if !pausedUntil.IsZero() {
		return k.SetQWordValue(keyPausedUse, uint64(pausedUntil.Unix()))
	} else {
		// If zero, delete the value or set to 0. Deleting is cleaner but Set 0 is easier.
		return k.SetQWordValue(keyPausedUse, 0)
	}
}

// LoadBackup retrieves state from Registry
func (r *RegistryStore) LoadBackup() (time.Time, time.Duration, time.Time, error) {
	k, err := r.openKey(registry.QUERY_VALUE)
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
		// Tolerable, might default to 0
		remDur = 0
	}
	pausedUnix, _, err := k.GetIntegerValue(keyPausedUse)
	if err != nil {
		pausedUnix = 0
	}

	// 0 means no lock usually, but Unix(0) is 1970.
	// If lockEndUnix is 0 or very old, assume unlocked?
	// But we strictly just return what's there.

	// Handles the zero case: time.Unix(0,0) is 1970.
	// Typically IsZero() checks for 0001-01-01.
	// We'll let the logic handle it, but note that 0 in registry is 1970.

	var t time.Time
	if lockEndUnix > 0 {
		t = time.Unix(int64(lockEndUnix), 0)
	}

	var pausedT time.Time
	if pausedUnix > 0 {
		pausedT = time.Unix(int64(pausedUnix), 0)
	}

	return t, time.Duration(remDur), pausedT, nil
}
