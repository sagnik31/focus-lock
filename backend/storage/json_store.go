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
}

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

	// Initialize Secret for HMAC
	secret, err := store.regStore.GetOrCreateSecret()
	if err != nil {
		// Fallback to memory-only secret if registry fails (unlikely)
		secret = make([]byte, 32)
		// We log or ignore, but better to proceed than crash
	}
	store.activeSecret = secret

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

	if err == nil {
		// 2. Verify Signature
		sigPath := s.filePath + ".sig"
		sigData, sigErr := os.ReadFile(sigPath)
		if sigErr == nil {
			computed := s.computeHMAC(data)
			stored := string(sigData)
			if computed != stored {
				corrupt = true
				// fmt.Println("Config TAMPERED: Signature mismatch") // Silence to prevent console window
			}
		} else {
			corrupt = true // Missing signature counts as tamper
			// fmt.Println("Config TAMPERED: Missing signature") // Silence
		}

		if !corrupt {
			if jsonErr := json.Unmarshal(data, &s.Data); jsonErr != nil {
				corrupt = true
			}
		}
	}

	// 3. Redundancy / Restore Logic
	// If file is missing OR corrupt, check Registry
	if fileMissing || corrupt {
		lockEnd, remDur, pausedUntil, regErr := s.regStore.LoadBackup()
		if regErr == nil {
			now := time.Now()
			// If Registry has an active lock
			if lockEnd.After(now) || remDur > 0 || (!pausedUntil.IsZero() && pausedUntil.After(now)) {
				// fmt.Println("Restoring Config from Registry Backup...") // Silence
				s.Data.LockEndTime = lockEnd
				s.Data.RemainingDuration = remDur
				s.Data.PausedUntil = pausedUntil
				// Force Save to restore the file
				// We need to unlock first because Save locks - BUT we are in internal load?
				// Actually Save() locks, so we cannot call it from here if we hold lock.
				// We should save AFTER returning from load if needed, or implement saveInternal.
				// For now, let's just populate Data and let the caller handle save if appropriate,
				// OR better, we trust the registry data and subsequent saves will write it.
				// However, the original code did: s.mu.Unlock(); s.Save(); s.mu.Lock();
				// This is dangerous if loadInternal is called from UpdateAtomic.
				// CORRECT FIX: Do NOT save here. Just load into memory.
				// The next Save() call will persist it to disk.
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

func (s *Store) saveInternal() error {
	data, err := json.MarshalIndent(s.Data, "", "  ")
	if err != nil {
		return err
	}

	// 1. Save Config File
	if err := os.WriteFile(s.filePath, data, 0644); err != nil {
		return err
	}

	// 2. Save HMAC Signature
	sig := s.computeHMAC(data)
	if err := os.WriteFile(s.filePath+".sig", []byte(sig), 0644); err != nil {
		return err
	}

	// 3. Save to Registry (Redundancy)
	return s.regStore.SaveBackup(s.Data.LockEndTime, s.Data.RemainingDuration, s.Data.PausedUntil)
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
