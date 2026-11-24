package storage

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// hostsBlockErrPrefix tags the LastEnforcementError messages that report a
// site-blocking (hosts-file write) failure, so SetHostsBlockError can clear its
// own message on recovery without clobbering an unrelated enforcement warning
// (spawn/persistence/recovery) that shares the same field.
const hostsBlockErrPrefix = "Website blocking failed"

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
	SessionStart         time.Time     `json:"session_start"`          // Start of the current manual session (zero if none); used to record history on end
	ScheduleUnlockWindow string        `json:"schedule_unlock_window"` // Key of the schedule window the emergency-unlock quota is tied to; a set sharing no window with it refills the quota (see applyScheduleUnlockReset)
	ScheduleSessionStart time.Time     `json:"schedule_session_start"` // Start of the in-progress schedule-driven session (zero if none); recorded to history on its falling edge (see UpdateScheduleSession)
	ScheduleSessionEnd   time.Time     `json:"schedule_session_end"`   // When that session's windows end; caps its recorded end if the PC was off then
	RecentSites          []string      `json:"recent_sites"`           // Sites the user has blocked, most recent first (capped); powers the "Recently used" list
	SiteGroups           []SiteGroup   `json:"site_groups"`            // User-defined website sections, shown above the built-in suggestions
}

// SiteGroup is a user-defined, named set of websites that can be blocked or
// unblocked together (like the built-in suggestion categories). Groups are
// presets only: editing or deleting one never changes what is blocked.
type SiteGroup struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Sites []string `json:"sites"`
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

	// Reflection data (surfaced in the Insights UI).
	Sessions          []SessionRecord `json:"sessions"`            // Completed focus sessions, oldest first (capped)
	FocusDays         []string        `json:"focus_days"`          // Distinct YYYY-MM-DD days with focus activity (capped), for streaks
	KillsByHour       []int           `json:"kills_by_hour"`       // 24 buckets: kills by local hour-of-day ("you tried X at 2pm")
	TotalFocusSeconds int64           `json:"total_focus_seconds"` // Lifetime focused time across completed sessions
}

// SessionRecord is one completed focus session, kept for the history view.
type SessionRecord struct {
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"`
	DurationSec int64     `json:"duration_sec"`
	Kind        string    `json:"kind"` // "manual" | "scheduled"
	AppCount    int       `json:"app_count"`
	SiteCount   int       `json:"site_count"`
}

// Methods for Stats

type Store struct {
	mu           sync.Mutex
	reqMu        sync.Mutex // serialises SubmitRequest (one request slot; see request.go)
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

	// secureDir holds an admin-only copy of the signed config
	// (%ProgramData%\FocusLock, SYSTEM + Administrators only). It is set only
	// for the strong-mode signer (the elevated Ghost). The files in %APPDATA%
	// are the user's to delete or replace with an older signed copy; this one
	// is not, so loadInternal trusts it first (see loadSecure).
	secureDir string

	// legacySecret is the reduced-mode (HKCU) secret, set only for the first
	// strong-mode signer after setup (migratePending): the existing config is
	// still signed with it, so loadInternal adopts and re-signs that config
	// (see adoptLegacy). Both are cleared by the first successful save.
	legacySecret   []byte
	migratePending bool

	// ownWrite is config.json's modification time right after this Store's
	// last successful save. The enforcer compares it with the file's current
	// mtime to tell its own routine writes (remaining time, stats) from
	// changes made by someone else, so it does not reload after every save.
	ownWrite time.Time
}

// OwnWriteTime returns config.json's modification time as of this Store's
// most recent save (zero if it has not saved).
func (s *Store) OwnWriteTime() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ownWrite
}

// CanSign reports whether this process may write the config directly. When
// false (strong-mode UI), callers must use SubmitRequest instead of Save.
func (s *Store) CanSign() bool { return s.canSign }

// StrongMode reports whether the HKLM secret has been provisioned.
func (s *Store) StrongMode() bool { return s.strong }

func defaultConfig() Config {
	return Config{
		BlockedApps: []string{},
		Stats: Stats{
			KillCounts: make(map[string]int),
		},
		BlockedSites:   []string{},
		BlockCommonVPN: true,
	}
}

// decodeConfig decodes a whole config over the defaults. It never decodes over
// a previous config, which would keep map entries the new one doesn't have.
func decodeConfig(data []byte) (Config, error) {
	cfg := defaultConfig()
	err := json.Unmarshal(data, &cfg)
	return cfg, err
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
		Data:     defaultConfig(),
		regStore: NewRegistryStore(),
	}

	// Resolve the HMAC secret and operating mode. In strong mode the UI gets
	// no secret (canSign=false) and reads the config without verifying it; the
	// elevated Ghost gets the secret and is the sole signer.
	secret, strong, canSign := store.regStore.ResolveSecret()
	store.activeSecret = secret
	store.strong = strong
	store.canSign = canSign
	if strong && canSign {
		store.secureDir = secureBackupDir()
		if store.regStore.MigratePending() {
			store.migratePending = true
			store.legacySecret = store.regStore.LegacySecret()
		}
	}

	return store, nil
}

// SecureDir returns the admin-only folder of the strong-mode signer, or "".
func (s *Store) SecureDir() string { return s.secureDir }

// lockConfig takes the store lock and, for a signer, the cross-process config
// lock, and returns the function that releases both. In reduced mode the UI
// and the enforcer are separate processes that both write the config; without
// this, one could load, the other commit a lock, and the first save over it
// (or a load could see a new config.json with the old signature and roll it
// back). Every read-modify-write, and every load that may heal, goes through it.
//
// The strong-mode lock file is in the admin-only folder, so a non-elevated
// process can't hold it to stall the enforcer. Waiting is bounded either way.
func (s *Store) lockConfig() (unlock func()) {
	s.mu.Lock()
	path := ""
	switch {
	case !s.canSign:
	case s.secureDir != "":
		path = filepath.Join(s.secureDir, "config.lock")
	case !s.strong:
		path = s.filePath + ".lock"
	}
	release := func() {}
	if path != "" {
		release = lockFile(path, 2*time.Second)
	}
	return func() {
		release()
		s.mu.Unlock()
	}
}

func (s *Store) Load() error {
	defer s.lockConfig()()

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
			if cfg, err := decodeConfig(data); err == nil {
				s.Data = cfg
			}
		}
		return nil
	}

	// 1. The admin-only copy wins when it verifies. Deleting the user-writable
	//    files, or putting back an older signed config.json (fewer sites, an
	//    expired lock), can then never change what is enforced: the primary is
	//    rewritten from the secure copy instead.
	if s.loadSecure(data, err == nil) {
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
			if cfg, jsonErr := decodeConfig(data); jsonErr != nil {
				corrupt = true
			} else {
				s.Data = cfg
			}
		}
	}

	// 2b. First strong-mode load after setup: the config is still signed with
	//     the reduced-mode secret. Adopt it instead of treating it as tampered.
	if (fileMissing || corrupt) && s.adoptLegacy(data, err == nil) {
		_ = s.saveInternal()
		return nil
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

		// Nothing verifies: the next save will replace this file, so keep a copy.
		if corrupt {
			s.keepRejected(data)
		}
		if s.regStore == nil { // unit tests only
			if corrupt {
				return errConfigCorrupt
			}
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
			return errConfigCorrupt
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
	defer s.lockConfig()()

	// 1. Load latest state from disk (ignore error to allow defaults/recovery)
	_ = s.loadInternal()

	// 2. Apply modifications
	updater(&s.Data)

	// 3. Save directly (we already hold lock)
	return s.saveInternal()
}

// Snapshot returns a copy of the in-memory config taken under the store lock,
// with its top-level slices copied, so a caller can read or sort it while the
// enforcer reloads Data concurrently.
func (s *Store) Snapshot() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.Data
	c.BlockedApps = cloneSlice(s.Data.BlockedApps)
	c.BlockedSites = cloneSlice(s.Data.BlockedSites)
	c.Schedules = cloneSlice(s.Data.Schedules)
	c.RecentSites = cloneSlice(s.Data.RecentSites)
	c.SiteGroups = cloneSlice(s.Data.SiteGroups)
	return c
}

// cloneSlice copies src, keeping nil as nil and empty as empty (so the JSON
// the UI reads stays "null" vs "[]" exactly as before).
func cloneSlice[T any](src []T) []T {
	if src == nil {
		return nil
	}
	out := make([]T, len(src))
	copy(out, src)
	return out
}

func (s *Store) Save() error {
	defer s.lockConfig()()
	return s.saveInternal()
}

// ApplyRequest is the Ghost's authoritative entry point for a UI operation.
// It reloads the latest config, runs the shared policy (refusing anything that
// would weaken an active lock), and signs+saves if allowed. The returned error
// is the policy verdict, relayed to the UI through the result file.
func (s *Store) ApplyRequest(op Operation, now time.Time) error {
	defer s.lockConfig()()

	_ = s.loadInternal()
	prev := s.Data
	if err := ApplyOperation(&s.Data, op, now); err != nil {
		return err
	}
	if err := s.saveInternal(); err != nil {
		s.Data = prev // not committed
		return err
	}
	return nil
}

// saveInternal signs and writes the config. It returns an error only when the
// config was not committed: once the copy the next load trusts first is in
// place, a failure to write the others must not be reported as a failed save
// (the UI would say a lock didn't start when it did).
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

	// 0. The admin-only copy first (strong-mode signer only; see loadSecure).
	//    If it can't be written, remove it: a stale secure copy would be
	//    preferred over this newer save on the next load and undo it.
	secured := false
	if s.secureDir != "" {
		sp := s.securePath()
		secured = true
		if atomicWrite(sp, data) != nil || atomicWrite(sp+".sig", []byte(sig)) != nil {
			secured = false
			_ = os.Remove(sp)
			_ = os.Remove(sp + ".sig")
		}
	}

	// 1. Write config + signature atomically (temp file + rename). A crash or
	//    power loss mid-write can then never leave a truncated/half-written
	//    config.json. Without this, an unclean shutdown corrupts the file, the
	//    next load falls back to the timing-only registry backup, and the block
	//    lists are lost while the lock still shows active.
	//    With a secure copy in place a failure here heals on the next load.
	if err := atomicWrite(s.filePath, data); err != nil && !secured {
		return err
	}
	if err := atomicWrite(s.filePath+".sig", []byte(sig)); err != nil && !secured {
		return err
	}
	if info, err := os.Stat(s.filePath); err == nil {
		s.ownWrite = info.ModTime()
	}

	// The config is now signed with the current secret; from here on a
	// reduced-mode (user-readable) one must not be accepted.
	s.legacySecret = nil
	if s.migratePending && s.regStore != nil && s.regStore.ClearMigratePending() == nil {
		s.migratePending = false
	}

	// 2. Full redundant backup (config + its signature). Written after the
	//    primary so it always holds a complete, verifiable config. Recovery
	//    restores the WHOLE config from here — block lists included, not just the
	//    lock timing — so we never resume with an active lock that blocks nothing.
	_ = atomicWrite(s.filePath+".bak", data)
	_ = atomicWrite(s.filePath+".bak.sig", []byte(sig))

	// 3. Registry backup of lock timing (survives even if the files are deleted,
	//    so the lock itself can't be escaped by removing config.json).
	//    NewStore always sets regStore; it is nil only in unit tests, which
	//    must not write the real registry.
	if s.regStore == nil {
		return nil
	}
	_ = s.regStore.SaveBackup(s.Data.LockEndTime, s.Data.RemainingDuration, s.Data.PausedUntil)
	return nil
}

var errConfigCorrupt = errors.New("config corrupted and no backup found")

// adoptLegacy loads a config (or its .bak) signed with the reduced-mode
// secret. It only does anything for the first signer after strong-mode setup
// (see legacySecret); the caller re-signs what it loaded.
func (s *Store) adoptLegacy(primary []byte, havePrimary bool) bool {
	if len(s.legacySecret) == 0 {
		return false
	}
	try := func(data []byte, sigPath string) bool {
		sig, err := os.ReadFile(sigPath)
		if err != nil || !hmac.Equal([]byte(hmacHex(s.legacySecret, data)), sig) {
			return false
		}
		cfg, err := decodeConfig(data)
		if err != nil {
			return false
		}
		s.Data = cfg
		return true
	}
	if havePrimary && try(primary, s.filePath+".sig") {
		return true
	}
	bak, err := os.ReadFile(s.filePath + ".bak")
	return err == nil && try(bak, s.filePath+".bak.sig")
}

// keepRejected keeps a copy of a config that failed verification.
func (s *Store) keepRejected(data []byte) {
	path := s.filePath + ".rejected"
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, data) {
		return
	}
	_ = atomicWrite(path, data)
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

func (s *Store) securePath() string { return filepath.Join(s.secureDir, "config.json") }

// loadSecure loads the config from the admin-only copy when there is one and
// its signature verifies, and heals the user-visible primary (config.json and
// its .sig) when it differs, is missing or was replaced. primary is the
// current config.json content (havePrimary false when it couldn't be read).
// It returns false, leaving s.Data alone, when there is no usable secure copy;
// the caller then falls back to the primary and the .bak.
func (s *Store) loadSecure(primary []byte, havePrimary bool) bool {
	if s.secureDir == "" {
		return false
	}
	sp := s.securePath()
	sdata, err := os.ReadFile(sp)
	if err != nil {
		return false
	}
	ssig, err := os.ReadFile(sp + ".sig")
	if err != nil || s.computeHMAC(sdata) != string(ssig) {
		return false
	}
	cfg, err := decodeConfig(sdata)
	if err != nil {
		return false
	}
	s.Data = cfg

	psig, sigErr := os.ReadFile(s.filePath + ".sig")
	if !havePrimary || !bytes.Equal(primary, sdata) || sigErr != nil || string(psig) != string(ssig) {
		_ = s.saveInternal()
	}
	return true
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
	restored, err := decodeConfig(bdata)
	if err != nil {
		return false
	}
	s.Data = restored
	return true
}

func (s *Store) computeHMAC(data []byte) string {
	if len(s.activeSecret) == 0 {
		return ""
	}
	return hmacHex(s.activeSecret, data)
}

func hmacHex(secret, data []byte) string {
	h := hmac.New(sha256.New, secret)
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

func (s *Store) UpdateBlockedStats(apps []string, durationSec int) {
	_ = s.UpdateAtomic(func(cfg *Config) {
		if cfg.Stats.BlockedFrequency == nil {
			cfg.Stats.BlockedFrequency = make(map[string]int)
		}
		if cfg.Stats.BlockedDuration == nil {
			cfg.Stats.BlockedDuration = make(map[string]int64)
		}
		for _, app := range apps {
			cfg.Stats.BlockedFrequency[app]++
			cfg.Stats.BlockedDuration[app] += int64(durationSec)
		}
	})
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

// GetKillCounts returns a copy of the per-app kill counts (recorded by the
// enforcer in both modes).
func (s *Store) GetKillCounts() map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]int, len(s.Data.Stats.KillCounts))
	for k, v := range s.Data.Stats.KillCounts {
		out[k] = v
	}
	return out
}

func (s *Store) GetFilePath() string {
	return s.filePath
}

// SetHostsBlockError records (err != nil) or clears (err == nil) a site-blocking
// failure in LastEnforcementError so the UI surfaces it as the amber enforcement
// banner. It is the remaining enforcement hole: blockSites previously only wrote
// to the debug log, so site-blocking silently failing (e.g. a reduced/user-mode
// process that cannot write the hosts file) was invisible in the UI.
//
// Only a signer persists (strong-mode UI is a no-op; the elevated Ghost, which
// actually writes the hosts file, is the authoritative reporter). It writes only
// when the message changes, so it is cheap to call every enforcement tick. To
// avoid clobbering an unrelated warning in the shared field, it never overwrites
// a non-hosts message, and on recovery clears only its own message.
func (s *Store) SetHostsBlockError(err error) {
	if !s.canSign {
		return
	}
	defer s.lockConfig()()
	_ = s.loadInternal()

	cur := s.Data.LastEnforcementError
	if err == nil {
		// Recovery: clear only if the banner is currently our hosts message.
		if strings.HasPrefix(cur, hostsBlockErrPrefix) {
			s.Data.LastEnforcementError = ""
			_ = s.saveInternal()
		}
		return
	}

	// Don't overwrite a different, pre-existing enforcement warning; surface the
	// hosts failure only when the banner is free or already showing it.
	if cur != "" && !strings.HasPrefix(cur, hostsBlockErrPrefix) {
		return
	}
	msg := hostsBlockErrPrefix + " (could not update the hosts file): " + err.Error()
	if cur == msg {
		return // no change
	}
	s.Data.LastEnforcementError = msg
	_ = s.saveInternal()
}
