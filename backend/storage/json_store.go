package storage

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Config struct {
	BlockedApps          []string      `json:"blocked_apps"`
	BlockedSites         []string      `json:"blocked_sites"`
	BlockCommonVPN       bool          `json:"block_common_vpn"`
	Schedules            []Schedule    `json:"schedules"` // New schedule structure
	Stats                Stats         `json:"stats"`
	LockEndTime          time.Time     `json:"lock_end_time"`      // Zero if not locked
	RemainingDuration    time.Duration `json:"remaining_duration"` // For offline usage tracking
	GhostTaskName        string        `json:"ghost_task_name"`    // Obfuscated task name
	GhostExePath         string        `json:"ghost_exe_path"`     // Path to obfuscated executable
	PausedUntil          time.Time     `json:"paused_until"`       // Emergency unlock expiry
	EmergencyUnlocksUsed int           `json:"emergency_unlocks_used"`
	LastEnforcementError string        `json:"last_enforcement_error"` // Non-empty when enforcement setup failed; surfaced as a UI warning
}

// Schedule represents a weekly time window for automatic locking
type Schedule struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Days      []string `json:"days"`       // ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"]
	StartTime string   `json:"start_time"` // "HH:MM" 24h format
	EndTime   string   `json:"end_time"`   // "HH:MM" 24h format
	Enabled   bool     `json:"enabled"`
}

type Stats struct {
	KillCounts       map[string]int   `json:"kill_counts"`
	BlockedFrequency map[string]int   `json:"blocked_frequency"`
	BlockedDuration  map[string]int64 `json:"blocked_duration"` // Total seconds blocked
}

// Methods for Stats

type Store struct {
	mu           sync.Mutex
	filePath     string
	Data         Config
	regStore     *RegistryStore
	activeSecret []byte

	// strong is true once the admin setup has provisioned the HKLM secret.
	// canSign is true when this process holds the secret and may therefore
	// write (sign) the config: always true in reduced mode, but true in strong
	// mode only for the elevated Ghost. A strong-mode UI has canSign=false and
	// must route every change through the request channel.
	strong  bool
	canSign bool
}

// CanSign reports whether this process may write the config directly. When
// false (strong-mode UI), callers must use SubmitRequest instead of Save.
func (s *Store) CanSign() bool { return s.canSign }

// StrongMode reports whether the HKLM secret has been provisioned.
func (s *Store) StrongMode() bool { return s.strong }

func NewStore() (*Store, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}

	dir := filepath.Join(configDir, "FocusLock")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	store := &Store{
		filePath: filepath.Join(dir, "config.json"),
		Data: Config{
			BlockedApps: []string{},
			Stats: Stats{
				KillCounts: make(map[string]int),
			},
			BlockedSites:   []string{},
			BlockCommonVPN: true,
		},
		regStore: NewRegistryStore(),
	}

	// Resolve the HMAC secret and operating mode. In strong mode the UI gets
	// no secret (canSign=false) and reads the config without verifying it; the
	// elevated Ghost gets the secret and is the sole signer.
	secret, strong, canSign := store.regStore.ResolveSecret()
	store.activeSecret = secret
	store.strong = strong
	store.canSign = canSign

	return store, nil
}

func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.loadInternal()
}

// loadInternal is the actual load logic, assuming lock is held
func (s *Store) loadInternal() error {
	// Retry logic for file contention
	var data []byte
	var err error
	maxRetries := 3

	for i := 0; i < maxRetries; i++ {
		data, err = os.ReadFile(s.filePath)
		if err == nil {
			break
		}
		// If permission denied or locked, wait and retry
		time.Sleep(50 * time.Millisecond)
	}

	fileMissing := os.IsNotExist(err)
	corrupt := false

	// Strong-mode UI: no secret to verify with, and it must never heal/sign
	// the file. Read it unverified for display only; the elevated Ghost is the
	// authority that verifies, heals and signs the config.
	if !s.canSign {
		if err == nil {
			_ = json.Unmarshal(data, &s.Data)
		}
		return nil
	}

	if err == nil {
		// 2. Verify Signature
		sigPath := s.filePath + ".sig"
		sigData, sigErr := os.ReadFile(sigPath)
		if sigErr == nil {
			computed := s.computeHMAC(data)
			stored := string(sigData)
			if computed != stored {
				corrupt = true
			}
		} else {
			corrupt = true // Missing signature counts as tamper
		}

		if !corrupt {
			if jsonErr := json.Unmarshal(data, &s.Data); jsonErr != nil {
				corrupt = true
			}
		}
	}

	// 3. Redundancy / Restore Logic
	// If the file is missing or corrupt, recover without losing the block lists.
	if fileMissing || corrupt {
		// 3a. Prefer the full signed backup. It restores the ENTIRE config,
		//     including blocked apps/sites, so an unclean shutdown can no longer
		//     leave an active lock that enforces nothing. Heal the primary file
		//     immediately so the next load reads it cleanly. saveInternal does not
		//     take the lock (we already hold it), so calling it here is safe.
		if s.restoreFromBackup() {
			_ = s.saveInternal()
			return nil
		}

		// 3b. Last resort: the registry keeps the lock timing (so the lock can't be
		//     escaped by deleting files) but not the block lists. Restore the timing
		//     and, if we have no block lists to enforce, surface a warning instead of
		//     silently presenting a fully-configured session that does nothing.
		lockEnd, remDur, pausedUntil, regErr := s.regStore.LoadBackup()
		if regErr == nil {
			now := time.Now()
			if lockEnd.After(now) || remDur > 0 || (!pausedUntil.IsZero() && pausedUntil.After(now)) {
				s.Data.LockEndTime = lockEnd
				s.Data.RemainingDuration = remDur
				s.Data.PausedUntil = pausedUntil
				if len(s.Data.BlockedApps) == 0 && len(s.Data.BlockedSites) == 0 {
					s.Data.LastEnforcementError = "Block lists could not be recovered after an unclean shutdown. Please re-add your blocked apps and sites."
				}
				return nil
			}
		}

		if corrupt {
			return fmt.Errorf("config corrupted and no backup found")
		}
		// If just missing and no backup, return defaults (fresh start)
		return nil
	}

	return nil
}

// UpdateAtomic provides a thread-safe way to read-modify-write the config.
// It ensures that we are modifying the most recent version of the config
// and avoids race conditions where the UI updates the config while the
// watchdog is calculating time.
func (s *Store) UpdateAtomic(updater func(*Config)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. Load latest state from disk (ignore error to allow defaults/recovery)
	_ = s.loadInternal()

	// 2. Apply modifications
	updater(&s.Data)

	// 3. Save directly (we already hold lock)
	return s.saveInternal()
}

func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveInternal()
}

// ApplyRequest is the Ghost's authoritative entry point for a UI operation.
// It reloads the latest config, runs the shared policy (refusing anything that
// would weaken an active lock), and signs+saves if allowed. The returned error
// is the policy verdict, relayed to the UI through the result file.
func (s *Store) ApplyRequest(op Operation, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_ = s.loadInternal()
	if err := ApplyOperation(&s.Data, op, now); err != nil {
		return err
	}
	return s.saveInternal()
}

func (s *Store) saveInternal() error {
	// In strong mode only the elevated Ghost (which holds the secret) may
	// write the config. A UI save here would be unsigned garbage, so refuse it
	// — the UI must go through SubmitRequest instead. This is a safety net;
	// the bridge should never call Save in strong mode.
	if !s.canSign {
		return errorString("config is read-only in this process; use the request channel")
	}

	data, err := json.MarshalIndent(s.Data, "", "  ")
	if err != nil {
		return err
	}
	sig := s.computeHMAC(data)

	// 1. Write config + signature atomically (temp file + rename). A crash or
	//    power loss mid-write can then never leave a truncated/half-written
	//    config.json. Without this, an unclean shutdown corrupts the file, the
	//    next load falls back to the timing-only registry backup, and the block
	//    lists are lost while the lock still shows active.
	if err := atomicWrite(s.filePath, data); err != nil {
		return err
	}
	if err := atomicWrite(s.filePath+".sig", []byte(sig)); err != nil {
		return err
	}

	// 2. Full redundant backup (config + its signature). Written after the
	//    primary so it always holds a complete, verifiable config. Recovery
	//    restores the WHOLE config from here — block lists included, not just the
	//    lock timing — so we never resume with an active lock that blocks nothing.
	_ = atomicWrite(s.filePath+".bak", data)
	_ = atomicWrite(s.filePath+".bak.sig", []byte(sig))

	// 3. Registry backup of lock timing (survives even if the files are deleted,
	//    so the lock itself can't be escaped by removing config.json).
	return s.regStore.SaveBackup(s.Data.LockEndTime, s.Data.RemainingDuration, s.Data.PausedUntil)
}

// atomicWrite writes data to a temp file in the same directory, flushes it to
// disk, and renames it over path. os.Rename replaces the destination atomically
// on Windows (MoveFileEx with MOVEFILE_REPLACE_EXISTING), so a reader — or a
// crash — always sees either the old complete file or the new complete file,
// never a partial one.
func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

// restoreFromBackup loads the full config from the signed .bak file written on
// each successful save. It returns true only if a backup with a matching HMAC
// signature was found and parsed, so a stale pre-signature backup or a tampered
// one is ignored.
func (s *Store) restoreFromBackup() bool {
	bdata, err := os.ReadFile(s.filePath + ".bak")
	if err != nil {
		return false
	}
	bsig, err := os.ReadFile(s.filePath + ".bak.sig")
	if err != nil {
		return false
	}
	if s.computeHMAC(bdata) != string(bsig) {
		return false
	}
	var restored Config
	if err := json.Unmarshal(bdata, &restored); err != nil {
		return false
	}
	s.Data = restored
	return true
}

func (s *Store) computeHMAC(data []byte) string {
	if len(s.activeSecret) == 0 {
		return ""
	}
	h := hmac.New(sha256.New, s.activeSecret)
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}
func (s *Store) IncrementKillCount(appName string) {
	s.mu.Lock()
	if s.Data.Stats.KillCounts == nil {
		s.Data.Stats.KillCounts = make(map[string]int)
	}
	s.Data.Stats.KillCounts[appName]++
	s.mu.Unlock()
	s.Save() // Auto-save on stats update
}

func (s *Store) UpdateBlockedStats(apps []string, durationSec int) {
	s.mu.Lock()
	if s.Data.Stats.BlockedFrequency == nil {
		s.Data.Stats.BlockedFrequency = make(map[string]int)
	}
	if s.Data.Stats.BlockedDuration == nil {
		s.Data.Stats.BlockedDuration = make(map[string]int64)
	}

	for _, app := range apps {
		s.Data.Stats.BlockedFrequency[app]++
		s.Data.Stats.BlockedDuration[app] += int64(durationSec)
	}
	s.mu.Unlock()
	s.Save()
}

func (s *Store) GetBlockedDuration() map[string]int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Data.Stats.BlockedDuration == nil {
		return make(map[string]int64)
	}

	copyMap := make(map[string]int64, len(s.Data.Stats.BlockedDuration))
	for k, v := range s.Data.Stats.BlockedDuration {
		copyMap[k] = v
	}
	return copyMap
}

func (s *Store) GetFilePath() string {
	return s.filePath
}
