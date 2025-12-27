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
	BlockedApps       []string          `json:"blocked_apps"`
	Schedule          map[string]string `json:"schedule,omitempty"` // "Mon": "09:00-17:00"
	Stats             Stats             `json:"stats"`
	LockEndTime       time.Time         `json:"lock_end_time"`      // Zero if not locked
	RemainingDuration time.Duration     `json:"remaining_duration"` // For offline usage tracking
	GhostTaskName     string            `json:"ghost_task_name"`    // Obfuscated task name
	GhostExePath      string            `json:"ghost_exe_path"`     // Path to obfuscated executable
}

type Stats struct {
	KillCounts map[string]int `json:"kill_counts"`
}

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

	// 1. Try to load File
	data, err := os.ReadFile(s.filePath)
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
				fmt.Println("Config TAMPERED: Signature mismatch")
			}
		} else {
			corrupt = true // Missing signature counts as tamper
			fmt.Println("Config TAMPERED: Missing signature")
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
		lockEnd, remDur, regErr := s.regStore.LoadBackup()
		if regErr == nil {
			now := time.Now()
			// If Registry has an active lock
			if lockEnd.After(now) || remDur > 0 {
				fmt.Println("Restoring Config from Registry Backup...")
				s.Data.LockEndTime = lockEnd
				s.Data.RemainingDuration = remDur
				// Force Save to restore the file
				// We need to unlock first because Save locks
				s.mu.Unlock()
				s.Save()
				s.mu.Lock()
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

func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

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
	return s.regStore.SaveBackup(s.Data.LockEndTime, s.Data.RemainingDuration)
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
