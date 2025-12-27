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

	// Main Polling Ticker
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	// Update Loop: Periodically save remaining usage
	usageTicker := time.NewTicker(20 * time.Second) // Save progress every 20s
	defer usageTicker.Stop()

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

			// Reload store only to check for new BLOCKED APPS
			// We DO NOT reload time here to avoid race conditions or external edits
			store.Load()
			enforce(store.Data.BlockedApps, store)

		case <-usageTicker.C:
			// Decrement RemainingDuration based on elapsed monotonic time
			elapsed := time.Since(monotonicStartTime)
			newRemaining := initialDuration - elapsed
			if newRemaining < 0 {
				newRemaining = 0
			}

			// Persist progress. If PC crashes, we lose at most 20s.
			store.Data.RemainingDuration = newRemaining
			store.Save()
		}
	}
}

func enforce(blockedApps []string, store *storage.Store) {
	if len(blockedApps) == 0 {
		return
	}

	// Create snapshot of running processes
	snapshot, err := windows.CreateToolhelp32Snapshot(TH32CS_SNAPPROCESS, 0)
	if err != nil {
		debugLog("Snapshot error: " + err.Error())
		return
	}
	defer windows.CloseHandle(snapshot)

	var procEntry ProcessEntry32
	procEntry.Size = uint32(unsafe.Sizeof(procEntry))

	// Get first process
	if err := Process32First(snapshot, &procEntry); err != nil {
		return
	}

	for {
		exeName := windows.UTF16ToString(procEntry.ExeFile[:])

		for _, blocked := range blockedApps {
			matched := false

			// 1. Check Filename (Fast)
			if strings.EqualFold(exeName, blocked) {
				matched = true
			}

			// 2. Check Metadata (Slow, but secure)
			// Only check if we haven't matched yet, OR checking everything?
			// Checking everything is expensive. But we must check if "notepad.exe" is actually "WhatsApp".
			// Wait, the 'blocked' strings are usually "WhatsApp.exe" or "WhatsApp".
			// If the user blocked "WhatsApp", we want to kill "renamed_whatsapp.exe".

			// We need the full path to check metadata.
			// ProcessEntry32 doesn't give full path easily. It takes more work (OpenProcess + GetModuleFileNameEx).

			// Optimization: Only do deep check if we suspect... actually we must periodically scan all if we want to catch renames.
			// But that is very heavy.

			// Let's try a hybrid:
			// If simple name match fails, should we check metadata?
			// The only way to find "renamed_whatsapp.exe" is to check metadata of ALL running processes.
			// That might spike CPU.

			// Let's assume the user selects "WhatsApp" from the UI. The UI sends "WhatsApp" or "WhatsApp.exe".
			// We want to match if ProductName == "WhatsApp" or FileDescription == "WhatsApp".

			if !matched {
				// We need full path.
				fullPath := getProcessPath(procEntry.ProcessID)
				if fullPath != "" {
					prodName, fileDesc := getFileMetadata(fullPath)
					// Check against blocked strings
					// If the blocked string is "WhatsApp", we match against "WhatsApp" in prodName/desc.
					// We should be lenient with substring or exact match?
					// Usually "WhatsApp" appears as "WhatsApp" EXACTLY in ProductName.
					// Let's try flexible matching.

					// Often blocked contains .exe, remove it for metadata check
					blockedClean := strings.TrimSuffix(strings.ToLower(blocked), ".exe")

					if strings.Contains(strings.ToLower(prodName), blockedClean) ||
						strings.Contains(strings.ToLower(fileDesc), blockedClean) {
						matched = true
					}
				}
			}

			if matched {
				killProcess(procEntry.ProcessID, exeName, store)
			}
		}

		// Next process
		if err := Process32Next(snapshot, &procEntry); err != nil {
			break // Done (ERROR_NO_MORE_FILES usually)
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
