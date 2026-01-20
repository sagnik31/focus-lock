package watchdog

import (
	"fmt"
	"focus-lock/backend/ntp"
	"focus-lock/backend/storage"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"focus-lock/backend/blocking/hosts"
	"focus-lock/backend/protection"
)

// Windows API constants and types
const (
	TH32CS_SNAPPROCESS = 0x00000002
)

// ProcessEntry32 structure
type ProcessEntry32 struct {
	Size            uint32
	CntUsage        uint32
	ProcessID       uint32
	DefaultHeapID   uintptr
	ModuleID        uint32
	CntThreads      uint32
	ParentProcessID uint32
	PriClassBase    int32
	Flags           uint32
	ExeFile         [windows.MAX_PATH]uint16
}

func debugLog(msg string) {
	configDir, _ := os.UserConfigDir()
	logPath := filepath.Join(configDir, "FocusLock", "debug.log")
	f, _ := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	defer f.Close()
	f.WriteString(time.Now().Format(time.RFC3339) + " " + msg + "\n")
}

// StartEnforcer runs deeply in the background. It monitors the lock time.
func StartEnforcer(store *storage.Store) {
	debugLog("Enforcer Watchdog Started")

	// Initial Load to calculate duration
	store.Load()
	if store.Data.LockEndTime.IsZero() {
		return
	}

	// SECURITY: Check Network Time to detect system time manipulation (e.g. user rebooted and changed BIOS time)
	// SECURITY: Check Network Time to detect system time manipulation
	offset, err := ntp.GetOffset()
	now := time.Now()
	var remaining time.Duration

	// 1. OFFLINE / NTP FAILURE FALLBACK
	if err != nil {
		debugLog(fmt.Sprintf("NTP Check failed: %s. Using Usage-Based Countdown.", err.Error()))

		// Fallback: If we trust the local timer, the user could have skipped ahead.
		// Instead, we trust RemainingDuration.
		// We RESET LockEndTime to Now + RemainingDuration.
		// This effectively PAUSES the timer while the machine was off/offline.
		// The user must spend 'RemainingDuration' amount of time ONLINE or RUNNING.

		if store.Data.RemainingDuration > 0 {
			remaining = store.Data.RemainingDuration
			// Reset end time to prevent immediate unlocking if system time jumped
			store.Data.LockEndTime = now.Add(remaining)
			store.Save()
			debugLog(fmt.Sprintf("Offline Fallback: Resuming with %v remaining", remaining))
		} else {
			// Weird state: LockEndTime set but RemainingDuration 0?
			// Maybe old version. Fallback to system time check.
			remaining = store.Data.LockEndTime.Sub(now)
		}
	} else {
		// 2. ONLINE / NTP SUCCESS
		debugLog(fmt.Sprintf("NTP Success. Offset: %v", offset))
		now = now.Add(offset)
		remaining = store.Data.LockEndTime.Sub(now)

		// Sync RemainingDuration valid
		if remaining > 0 {
			store.Data.RemainingDuration = remaining
			store.Save()
		}
	}

	if remaining <= 0 {
		debugLog("Lock time already expired (Network Validated). Exiting.")
		return
	}

	// SECURITY: Use Monotonic Time for the deadline!
	// time.Now() returns a time with a monotonic clock reading.
	// Adding duration to it preserves the monotonic reading.
	// Comparisons (After, Before) use the monotonic reading if present.
	// This means changing the System Wall Clock will NOT affect this deadline.
	monotonicDeadline := time.Now().Add(remaining)
	monotonicStartTime := time.Now()
	initialDuration := remaining

	debugLog(fmt.Sprintf("Locking for %v (Until monotonic: %v)", remaining, monotonicDeadline))

	// Main Polling Ticker (Aggressive for instant kill)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	// Update Loop: Save usage & Anti-Cheat (Deep Check)
	slowTicker := time.NewTicker(4 * time.Second)
	defer slowTicker.Stop()

	// 3. INITIAL HOSTS BLOCK
	blockSites(store)
	defer hosts.Unblock()

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
			lookup[strings.ToLower(app)] = true
		}
		return apps, lookup, nil
	}

	cachedBlockedApps, cachedLookup, _ := refreshCache()

	for {
		select {
		case <-ticker.C:
			// Check expiry against MONOTONIC deadline
			if time.Now().After(monotonicDeadline) {
				debugLog("Lock time expired (Monotonic match). Exiting Enforcer.")
				store.Data.RemainingDuration = 0
				store.Save()
				return
			}

			// CHECK PAUSE (Fast)
			// Accessed sequentially in single goroutine, so safe after refreshCache update
			if !store.Data.PausedUntil.IsZero() && time.Now().Before(store.Data.PausedUntil) {
				continue
			}

			// FAST CHECK (Name only, using Map) every 200ms
			enforceFast(cachedLookup, store)

		case <-slowTicker.C:
			// 2. Decrement Duration (Atomic Update)
			// We use UpdateAtomic to ensure we don't overwrite "PausedUntil" if the UI updated it
			// while we were sleeping.
			elapsed := time.Since(monotonicStartTime)
			newRemaining := initialDuration - elapsed
			if newRemaining < 0 {
				newRemaining = 0
			}

			err := store.UpdateAtomic(func(cfg *storage.Config) {
				cfg.RemainingDuration = newRemaining
			})
			if err != nil {
				debugLog("Failed to update remaining duration: " + err.Error())
				// Continue anyway, we'll try again next tick
			}

			// 1. Reload Config for local usage (Apps/Sites)
			// refreshCache calls Load(), which gets the latest state including any PausedUntil
			newApps, newLookup, err := refreshCache()
			if err != nil {
				debugLog("Config reload failed: " + err.Error())
				// We still enforce with old cache if possible
			} else {
				cachedBlockedApps, cachedLookup = newApps, newLookup
			}

			// CHECK PAUSE (Slow/Deep)
			if !store.Data.PausedUntil.IsZero() && time.Now().Before(store.Data.PausedUntil) {
				debugLog("Emergency Unlocked (Paused). Unblocking hosts temporarily.")
				hosts.Unblock()
				continue
			}

			// 3. DEEP CHECK (Metadata for renamed apps) & Re-apply Hosts
			enforceDeep(cachedBlockedApps, store)
			blockSites(store)
		}
	}
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
			if !store.Data.PausedUntil.IsZero() && time.Now().Before(store.Data.PausedUntil) {
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

// Wrapper for Process32First/Next since they are not in x/sys/windows directly or slightly different signatures
// Actually they SHOULD be in x/sys/windows, but sometimes under different names or need manual load.
// Let's check if they exist. Usually CreateToolhelp32Snapshot is there.
// Process32First might accept *ProcessEntry32.

// To be safe, I will implement the syscall wrapper manually for Process32First/Next to avoid dependency hell if the version differs.
var (
	kernel32                       = windows.NewLazySystemDLL("kernel32.dll")
	version                        = windows.NewLazySystemDLL("version.dll")
	procProcess32First             = kernel32.NewProc("Process32FirstW")
	procProcess32Next              = kernel32.NewProc("Process32NextW")
	procQueryFullProcessImageNameW = kernel32.NewProc("QueryFullProcessImageNameW")
	procGetFileVersionInfoSizeW    = version.NewProc("GetFileVersionInfoSizeW")
	procGetFileVersionInfoW        = version.NewProc("GetFileVersionInfoW")
	procVerQueryValueW             = version.NewProc("VerQueryValueW")
)

// getFileMetadata returns Product Name or File Description for a given executable path
func getFileMetadata(path string) (string, string) {
	// Get size of version info
	ptrPath, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", ""
	}

	var handle uint32 // This handle is not used by GetFileVersionInfoSizeW, it's an output parameter for GetFileVersionInfo.
	size, _, _ := procGetFileVersionInfoSizeW.Call(uintptr(unsafe.Pointer(ptrPath)), uintptr(unsafe.Pointer(&handle)))
	if size == 0 {
		return "", ""
	}

	// Allocate buffer
	data := make([]byte, size)
	ret, _, _ := procGetFileVersionInfoW.Call(
		uintptr(unsafe.Pointer(ptrPath)),
		0,
		size,
		uintptr(unsafe.Pointer(&data[0])),
	)
	if ret == 0 {
		return "", ""
	}

	// Helper to query string value
	query := func(key string) string {
		var transBlock *struct {
			LangID  uint16
			CharSet uint16
		}
		var transLen uint32
		subBlockTr, _ := windows.UTF16PtrFromString("\\VarFileInfo\\Translation")
		// Query language
		ret, _, _ := procVerQueryValueW.Call(
			uintptr(unsafe.Pointer(&data[0])),
			uintptr(unsafe.Pointer(subBlockTr)),
			uintptr(unsafe.Pointer(&transBlock)),
			uintptr(unsafe.Pointer(&transLen)),
		)

		langCodes := []string{"040904b0"} // Default US English
		if ret != 0 && transLen >= 4 {
			// Add found language, prioritizing it
			langCodes = append([]string{fmt.Sprintf("%04x%04x", transBlock.LangID, transBlock.CharSet)}, langCodes...)
		}

		for _, code := range langCodes {
			subBlock, _ := windows.UTF16PtrFromString(fmt.Sprintf("\\StringFileInfo\\%s\\%s", code, key))
			var valPtr uintptr
			var valLen uint32
			ret, _, _ = procVerQueryValueW.Call(
				uintptr(unsafe.Pointer(&data[0])),
				uintptr(unsafe.Pointer(subBlock)),
				uintptr(unsafe.Pointer(&valPtr)),
				uintptr(unsafe.Pointer(&valLen)),
			)
			if ret != 0 && valLen > 0 {
				return windows.UTF16PtrToString((*uint16)(unsafe.Pointer(valPtr)))
			}
		}
		return ""
	}

	return query("ProductName"), query("FileDescription")
}

func getProcessPath(pid uint32) string {
	hProcess, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(hProcess)

	buf := make([]uint16, windows.MAX_PATH)
	size := uint32(len(buf))
	// QueryFullProcessImageNameW(hProcess, 0, &buf, &size)
	ret, _, _ := procQueryFullProcessImageNameW.Call(
		uintptr(hProcess),
		0, // dwFlags: 0 for default (Win32 path format)
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
	)
	if ret == 0 {
		return ""
	}
	return windows.UTF16ToString(buf[:size])
}

func Process32First(snapshot windows.Handle, pe *ProcessEntry32) error {
	r1, _, err := procProcess32First.Call(uintptr(snapshot), uintptr(unsafe.Pointer(pe)))
	if r1 == 1 { // TRUE
		return nil
	}
	return err
}

func Process32Next(snapshot windows.Handle, pe *ProcessEntry32) error {
	r1, _, err := procProcess32Next.Call(uintptr(snapshot), uintptr(unsafe.Pointer(pe)))
	if r1 == 1 { // TRUE
		return nil
	}
	return err
}
