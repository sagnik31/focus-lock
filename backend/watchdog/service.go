package watchdog

import (
	"fmt"
	"focus-lock/backend/storage"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"focus-lock/backend/blocking/browserpolicy"
	"focus-lock/backend/blocking/hosts"
	"focus-lock/backend/clock"
	"focus-lock/backend/logging"
	"focus-lock/backend/protection"
	"focus-lock/backend/userfs"
)

func debugLog(msg string) {
	configDir, _ := os.UserConfigDir()
	logPath := filepath.Join(configDir, "FocusLock", "debug.log")
	// The log is in the user's folder: write it with the user's rights (G25).
	_ = userfs.Do(func() error {
		logging.Write(logPath, msg)
		return nil
	})
}

// IsScheduleActive checks if any enabled schedule matches the current time.
// The actual window logic lives in storage.SchedulesActiveAt so the enforcer
// and the operation policy share one implementation.
func IsScheduleActive(schedules []storage.Schedule) bool {
	return storage.SchedulesActiveAt(schedules, clock.Default.Now())
}

// StartEnforcer runs deeply in the background. It monitors the lock time and schedules.
func StartEnforcer(store *storage.Store, isGhost bool) {
	debugLog(fmt.Sprintf("Enforcer Watchdog Started (Ghost=%v)", isGhost))

	// Anchor the trusted clock to NTP and keep it synced. Enforcement decisions
	// below read clock.Default.Now() (not the raw system clock), so advancing the
	// Windows time-of-day can no longer end a lock early or skip a schedule.
	clock.Default.Start(5*time.Minute, func(err error) {
		if err != nil {
			debugLog("NTP sync failed; using best-effort local time: " + err.Error())
		} else {
			debugLog("NTP sync ok; trusted time anchored")
		}
	})

	// Main Polling Ticker (Aggressive for coverage)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	// Update Loop: Save usage & Anti-Cheat (Deep Check)
	slowTicker := time.NewTicker(5 * time.Second)
	defer slowTicker.Stop()

	// Helper to refresh cache
	refreshCache := func() ([]string, map[string]bool, error) {
		if err := store.Load(); err != nil {
			return nil, nil, err
		}
		// Junk entries (e.g. "." saved by older builds for an app whose exe
		// could not be resolved) are ignored so they can never match broadly.
		apps := storage.ValidApps(store.Data.BlockedApps)
		if store.Data.BlockCommonVPN {
			apps = append(apps, protection.GetVPNExecutables()...)
		}

		// Build map for O(1) lookup
		lookup := make(map[string]bool)
		for _, app := range apps {
			name := strings.ToLower(app)
			lookup[name] = true
			if !strings.HasSuffix(name, ".exe") {
				lookup[name+".exe"] = true
			}
		}
		return apps, lookup, nil
	}

	cachedBlockedApps, cachedLookup, _ := refreshCache()

	// Initialize File Watcher
	configPath := store.GetFilePath()
	var lastModTime time.Time
	if info, err := os.Stat(configPath); err == nil {
		lastModTime = info.ModTime()
	}

	// Process any request the UI queued before we started (e.g. the very first
	// StartFocus, which the non-elevated UI cannot write itself).
	_ = processRequests(store)

	// Initial check to block immediately if needed
	store.Load()
	if storage.Enforcing(&store.Data, clock.Default.Now()) {
		blockSites(store)
	}

	defer unblockSites()

	for {
		select {
		case <-ticker.C:
			// fast loop

			// 0a. Apply any UI request (strong mode: the UI cannot sign the
			// config, so it hands operations to us to validate and commit).
			applied := processRequests(store)

			// 0. Check for Config Changes (Immediate Reaction)
			if info, err := os.Stat(configPath); err == nil {
				// Use !Equal to catch any modification (sometimes time resolution can be tricky)
				changed := !info.ModTime().Equal(lastModTime)
				if changed {
					lastModTime = info.ModTime()
				}
				// Skip our own routine writes (remaining time, stats, quota):
				// the slow loop rewrites the config every tick, and reacting
				// to that reloaded everything every 5s and flooded the log.
				// A UI request we just applied is also our write, but it must
				// take effect now, so it always reloads.
				ownWrite := info.ModTime().Equal(store.OwnWriteTime())
				if changed && (applied || !ownWrite) {
					debugLog("Config file changed. Reloading cache...")
					newApps, newLookup, err := refreshCache()
					if err == nil {
						cachedBlockedApps, cachedLookup = newApps, newLookup
						// Force block sites immediately (Flush DNS)
						// Also re-checks lock status inside blockSites/enforce
						store.Load() // Ensure store.Data is latest for below checks
						if storage.Enforcing(&store.Data, clock.Default.Now()) {
							blockSites(store)
						}
					}
				}
			}

			// RELOAD Config on fast loop? No, too expensive.
			// But we need to know if we should be enforcing.

			// For Manual Lock, we trust the in-memory state or lightweight check?
			// We MUST check time. So we use the loaded store data.

			// 1. Check Locked State
			manualActive := !store.Data.LockEndTime.IsZero() && clock.Default.Now().Before(store.Data.LockEndTime)

			// 2. Check Schedule State
			// Schedule needs current time, which changes.
			// However schedule definitions (Schedules list) change rarely.
			// We use cached store data for schedule definitions until slow tick reloads.
			scheduleActive := IsScheduleActive(store.Data.Schedules)

			shouldEnforce := manualActive || scheduleActive

			// 3. Check Pause
			if !store.Data.PausedUntil.IsZero() && clock.Default.Now().Before(store.Data.PausedUntil) {
				shouldEnforce = false
			}

			if shouldEnforce {
				enforceFast(cachedLookup, store)
			} else {
				// If we just exited a lock state, we should unblock (Hosts).
				// But doing it here every 500ms is spammy.
				// We rely on SlowLoop to handle state transitions or just leave it until SlowLoop cleans up.
			}

		case <-slowTicker.C:
			// SLOW LOOP - Reload Config & Deep Check

			// 1. Reload Config
			newApps, newLookup, err := refreshCache()
			if err != nil {
				debugLog("Config reload failed: " + err.Error())
			} else {
				cachedBlockedApps, cachedLookup = newApps, newLookup
			}

			// 2. Recalculate State with fresh data
			manualActive := !store.Data.LockEndTime.IsZero() && clock.Default.Now().Before(store.Data.LockEndTime)

			// NTP Check logic could go here, but for now we trust local time for simplicity in V1 schedule
			// For Manual Lock, we still respect the monotonic expiration if we were tracking it,
			// but we simplify here to just check valid LockEndTime.

			scheduleActive := IsScheduleActive(store.Data.Schedules)
			shouldEnforce := manualActive || scheduleActive

			// 2b. Refill the emergency-unlock quota at the start of a new schedule
			// window so the UI shows the right count even before an unlock is
			// attempted (the policy also refills authoritatively at unlock time).
			if scheduleActive {
				store.RefreshScheduleQuota(clock.Default.Now())
			}

			// 2c. Record schedule-driven focus as sessions (history/Insights):
			// a schedule window counts as a completed session on its falling
			// edge, not only toward streaks. Called every tick (including when
			// idle) so the falling edge is always observed.
			store.UpdateScheduleSession(clock.Default.Now(), scheduleActive)

			// 3. Check Pause
			if !store.Data.PausedUntil.IsZero() && clock.Default.Now().Before(store.Data.PausedUntil) {
				debugLog("Emergency Unlocked (Paused). Unblocking hosts.")
				unblockSites()
				continue
			}

			if shouldEnforce {
				// 4. Update Remaining Duration (Only for Manual Lock)
				if manualActive {
					updatedRemaining := store.Data.LockEndTime.Sub(clock.Default.Now())
					if updatedRemaining < 0 {
						updatedRemaining = 0
					}

					// Atomic update to avoid race
					store.UpdateAtomic(func(cfg *storage.Config) {
						cfg.RemainingDuration = updatedRemaining
					})
				}

				// 5. Deep Enforce
				enforceDeep(cachedBlockedApps, store)
				blockSites(store)

				// 6. Mark today as a focus day (for streaks). Cheap: writes at
				// most once per day (no-op when today is already recorded).
				store.RecordFocusDay(clock.Default.Now())
			} else {
				// Not enforcing. Ensure Unblock.
				unblockSites()

				// Clean up an expired manual lock and record the session. Not
				// before NTP has answered: until then the time is the system
				// clock's, and a wrong one would erase a lock that is still
				// running. Left alone, the lock resumes once the clock is right.
				if !store.Data.LockEndTime.IsZero() && clock.Default.Anchored() {
					store.FinishExpiredLock(clock.Default.Now())
				}

				// The Ghost stays resident even when idle. It does NOT exit here.
				//
				// Exiting when idle used to be fine, but the scheduled task now has a
				// 1-minute repetition trigger (so a killed Ghost is relaunched within a
				// minute, which is what makes killing it pointless rather than a bypass).
				// If the Ghost exited whenever idle, that repetition would relaunch it and
				// it would exit again every single minute forever. Staying resident makes
				// the repeat a no-op (the single-instance mutex is held) and lets the Ghost
				// pick up the next manual lock or schedule window immediately. An idle
				// Ghost is cheap: the loop just re-checks state and enforces nothing.

			}
		}
	}
}

// processRequests drains one queued UI operation (if any), validates it with
// the shared policy, and writes the verdict back for the UI. Only a signer
// (the elevated Ghost in strong mode) acts; a non-signer never reaches here.
// It reports whether an operation was applied (and the config changed).
func processRequests(store *storage.Store) bool {
	if !store.CanSign() {
		return false
	}
	dir := store.RequestDir()
	req, ok := storage.TakePendingRequest(dir)
	if !ok {
		return false
	}
	err := store.ApplyRequest(req.Op, clock.Default.Now())
	res := storage.Result{ID: req.ID, OK: err == nil}
	if err != nil {
		res.Error = err.Error()
		debugLog("Request refused (" + string(req.Op.Type) + "): " + err.Error())
	} else {
		debugLog("Request applied: " + string(req.Op.Type))
	}
	storage.WriteResult(dir, res)
	return err == nil
}

func blockSites(store *storage.Store) {
	sites := slices.Clone(store.Data.BlockedSites)
	if store.Data.BlockCommonVPN {
		sites = append(sites, protection.GetVPNDomains()...)
	}
	// Chrome policy layer first: it is what actually holds against Chrome's
	// Secure DNS and cached/offline sites. Independent of the hosts result.
	logPolicyErr("apply", browserpolicy.ApplyChrome(sites))
	if len(sites) == 0 {
		store.SetHostsBlockError(nil) // nothing to block; clear any stale warning
		return
	}
	if err := hosts.Block(sites); err != nil {
		debugLog(fmt.Sprintf("Failed to block sites: %v", err))
		store.SetHostsBlockError(err) // surface it in the UI, not just the log
		return
	}
	store.SetHostsBlockError(nil) // blocked successfully; clear any prior failure
}

// unblockSites lifts every website block layer: the hosts-file section and
// the Chrome URLBlocklist entries Focus Lock added. Both are no-ops when there
// is nothing to remove, so this is safe on every idle tick.
func unblockSites() {
	hosts.Unblock()
	logPolicyErr("clear", browserpolicy.ClearChrome())
}

// lastPolicyErr de-duplicates policy failures: blockSites runs every few
// seconds, and a persistent failure (e.g. a non-elevated enforcer that cannot
// write HKLM) would otherwise flood the debug log.
var lastPolicyErr string

func logPolicyErr(action string, err error) {
	msg := ""
	if err != nil {
		msg = "Chrome policy " + action + " failed: " + err.Error()
	}
	if msg != lastPolicyErr {
		lastPolicyErr = msg
		if msg != "" {
			debugLog(msg)
		}
	}
}

// enforceFast uses O(1) map lookup for filenames
func enforceFast(blockedMap map[string]bool, store *storage.Store) {
	if len(blockedMap) == 0 {
		return
	}

	snapshot, err := windows.CreateToolhelp32Snapshot(TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return // Silent fail for speed
	}
	defer windows.CloseHandle(snapshot)

	var procEntry ProcessEntry32
	procEntry.Size = uint32(unsafe.Sizeof(procEntry))

	if err := Process32First(snapshot, &procEntry); err != nil {
		return
	}

	for {
		exeName := windows.UTF16ToString(procEntry.ExeFile[:])

		// Check against map (O(1))
		if blockedMap[strings.ToLower(exeName)] {
			// CRITICAL FIX: Reload Config BEFORE killing to check for Emergency Unlock
			// This prevents race condition where we overwrite the pause command with old data.
			store.Load()
			if !store.Data.PausedUntil.IsZero() && clock.Default.Now().Before(store.Data.PausedUntil) {
				return // Stop enforcing if paused
			}

			if !ownProcess(procEntry.ProcessID, getProcessPath(procEntry.ProcessID)) {
				killProcess(procEntry.ProcessID, exeName, store)
			}
		}

		if err := Process32Next(snapshot, &procEntry); err != nil {
			break
		}
	}
}

// enforceDeep uses partial string matching on metadata (Slower)
func enforceDeep(blockedApps []string, store *storage.Store) {
	if len(blockedApps) == 0 {
		return
	}

	snapshot, err := windows.CreateToolhelp32Snapshot(TH32CS_SNAPPROCESS, 0)
	if err != nil {
		debugLog("Snapshot error: " + err.Error())
		return
	}
	defer windows.CloseHandle(snapshot)
	windowsDir, _ := windows.GetSystemWindowsDirectory()

	var procEntry ProcessEntry32
	procEntry.Size = uint32(unsafe.Sizeof(procEntry))

	if err := Process32First(snapshot, &procEntry); err != nil {
		return
	}

	for {
		exeName := windows.UTF16ToString(procEntry.ExeFile[:])

		// We only need to check DEEP if the name itself DOES NOT match.
		// If name matches, Fast Loop catches it (or we catch it here too, no harm).
		// But for efficiency, we assume Fast Loop does its job.

		// Do we check ALL processes? Yes.

		// Metadata check
		fullPath := getProcessPath(procEntry.ProcessID)
		if fullPath != "" && !ownProcess(procEntry.ProcessID, fullPath) && metadataMatchable(fullPath, windowsDir) {
			prodName, fileDesc := getFileMetadata(fullPath)
			if deepMatch(prodName, fileDesc, blockedApps) {
				killProcess(procEntry.ProcessID, exeName, store)
			}
		}

		if err := Process32Next(snapshot, &procEntry); err != nil {
			break
		}
	}
}

// ownProcess reports whether a process is this one or another instance of the
// same executable (the UI and the enforcer share it), whatever it is named.
func ownProcess(pid uint32, path string) bool {
	if pid == uint32(os.Getpid()) {
		return true
	}
	self, err := os.Executable()
	return err == nil && path != "" && strings.EqualFold(filepath.Clean(path), filepath.Clean(self))
}

func killProcess(pid uint32, name string, store *storage.Store) {
	// Open process with Terminate rights
	handle, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		debugLog("OpenProcess failed for " + name + ": " + err.Error())
		return
	}
	defer windows.CloseHandle(handle)

	// While the handle is open the PID can't be reused. If that happened before,
	// the PID now belongs to a process nobody asked to end.
	if now := processName(pid); !strings.EqualFold(now, name) {
		return
	}

	// Terminate
	if err := windows.TerminateProcess(handle, 1); err == nil {
		debugLog(fmt.Sprintf("Process terminated: %s [PID: %d]", name, pid))
		store.RecordKill(name, clock.Default.Now())
	} else {
		debugLog(fmt.Sprintf("TerminateProcess failed for %s: %s", name, err.Error()))
	}
}
