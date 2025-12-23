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
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		store.Load()

		now := time.Now()
		if now.After(store.Data.LockEndTime) {
			if !store.Data.LockEndTime.IsZero() {
				continue
			}
		}

		if store.Data.LockEndTime.IsZero() {
			continue // No active lock
		}

		enforce(store.Data.BlockedApps, store)
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
			if strings.EqualFold(exeName, blocked) {
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
	kernel32           = windows.NewLazySystemDLL("kernel32.dll")
	procProcess32First = kernel32.NewProc("Process32FirstW")
	procProcess32Next  = kernel32.NewProc("Process32NextW")
)

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
