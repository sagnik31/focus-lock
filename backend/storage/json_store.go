package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Config struct {
	BlockedApps   []string          `json:"blocked_apps"`
	Schedule      map[string]string `json:"schedule,omitempty"` // "Mon": "09:00-17:00"
	Stats         Stats             `json:"stats"`
	LockEndTime   time.Time         `json:"lock_end_time"`   // Zero if not locked
	GhostTaskName string            `json:"ghost_task_name"` // Obfuscated task name
	GhostExePath  string            `json:"ghost_exe_path"`  // Path to obfuscated executable
}

type Stats struct {
	KillCounts map[string]int `json:"kill_counts"`
}

type Store struct {
	mu       sync.Mutex
	filePath string
	Data     Config
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

	return &Store{
		filePath: filepath.Join(dir, "config.json"),
		Data: Config{
			BlockedApps: []string{},
			Stats: Stats{
				KillCounts: make(map[string]int),
			},
		},
	}, nil
}

func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filePath)
	if os.IsNotExist(err) {
		return nil // Use defaults
	}
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &s.Data)
}

func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(s.Data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, data, 0644)
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
