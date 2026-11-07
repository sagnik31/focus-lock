package watchdog

import (
	"fmt"
	"focus-lock/backend/storage"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"focus-lock/backend/blocking/hosts"
	"focus-lock/backend/clock"
	"focus-lock/backend/protection"
)

func debugLog(msg string) {
	configDir, _ := os.UserConfigDir()
	logPath := filepath.Join(configDir, "FocusLock", "debug.log")
	f, _ := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	defer f.Close()
	f.WriteString(time.Now().Format(time.RFC3339) + " " + msg + "\n")
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
		apps := store.Data.BlockedApps
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
	processRequests(store)

	// Initial check to block immediately if needed
	store.Load()
	if !store.Data.LockEndTime.IsZero() && clock.Default.Now().Before(store.Data.LockEndTime) {
		blockSites(store)
	} else if IsScheduleActive(store.Data.Schedules) {
		blockSites(store)
	}

	defer hosts.Unblock()

	for {
		select {
		case <-ticker.C:
			// fast loop

			// 0a. Apply any UI request (strong mode: the UI cannot sign the
			// config, so it hands operations to us to validate and commit).
			processRequests(store)

			// 0. Check for Config Changes (Immediate Reaction)
			if info, err := os.Stat(configPath); err == nil {
				// Use !Equal to catch any modification (sometimes time resolution can be tricky)
				if !info.ModTime().Equal(lastModTime) {
					lastModTime = info.ModTime()
					debugLog("Config file changed. Reloading cache...")
					newApps, newLookup, err := refreshCache()
					if err == nil {
						cachedBlockedApps, cachedLookup = newApps, newLookup
						// Force block sites immediately (Flush DNS)
						// Also re-checks lock status inside blockSites/enforce
						store.Load() // Ensure store.Data is latest for below checks
						if !store.Data.LockEndTime.IsZero() && clock.Default.Now().Before(store.Data.LockEndTime) {
							blockSites(store)
						} else if IsScheduleActive(store.Data.Schedules) {
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

			// 3. Check Pause
			if !store.Data.PausedUntil.IsZero() && clock.Default.Now().Before(store.Data.PausedUntil) {
				debugLog("Emergency Unlocked (Paused). Unblocking hosts.")
				hosts.Unblock()
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
			} else {
				// Not enforcing. Ensure Unblock.
				hosts.Unblock()

				// Cleanup expired manual lock
				if !store.Data.LockEndTime.IsZero() && clock.Default.Now().After(store.Data.LockEndTime) {
					store.Data.LockEndTime = time.Time{}
					store.Data.RemainingDuration = 0
					store.Save()
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
func processRequests(store *storage.Store) {
	if !store.CanSign() {
		return
	}
	dir := store.RequestDir()
	req, ok := storage.TakePendingRequest(dir)
	if !ok {
		return
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
}

func blockSites(store *storage.Store) {
	sites := store.Data.BlockedSites
	if store.Data.BlockCommonVPN {
		sites = append(sites, protection.GetVPNDomains()...)
	}
	if len(sites) > 0 {
		if err := hosts.Block(sites); err != nil {
			debugLog(fmt.Sprintf("Failed to block sites: %v", err))
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

			killProcess(procEntry.ProcessID, exeName, store)
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
		if fullPath != "" {
			prodName, fileDesc := getFileMetadata(fullPath)
			// Normalize
			prodName = strings.ToLower(prodName)
			fileDesc = strings.ToLower(fileDesc)

			for _, blocked := range blockedApps {
				blockedClean := strings.TrimSuffix(strings.ToLower(blocked), ".exe")

				if (prodName != "" && strings.Contains(prodName, blockedClean)) ||
					(fileDesc != "" && strings.Contains(fileDesc, blockedClean)) {

					killProcess(procEntry.ProcessID, exeName, store)
					break // Killed
				}
			}
		}

		if err := Process32Next(snapshot, &procEntry); err != nil {
			break
		}
	}
}

func killProcess(pid uint32, name string, store *storage.Store) {
	// Open process with Terminate rights
	handle, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		debugLog("OpenProcess failed for " + name + ": " + err.Error())
		return
	}
	defer windows.CloseHandle(handle)

	// Terminate
	if err := windows.TerminateProcess(handle, 1); err == nil {
		debugLog(fmt.Sprintf("Process terminated: %s [PID: %d]", name, pid))
		store.IncrementKillCount(name)
	} else {
		debugLog(fmt.Sprintf("TerminateProcess failed for %s: %s", name, err.Error()))
	}
}
