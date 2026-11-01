package watchdog

import (
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

// This file holds the low-level Windows API plumbing used by the enforcer:
// the process-snapshot types and the kernel32.dll / version.dll calls for
// reading process paths and executable metadata. The enforcement logic
// itself lives in service.go.

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

// Lazy-loaded procedures. Process32First/Next are wrapped manually to avoid
// depending on a specific x/sys/windows signature across versions.
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
			var valPtr unsafe.Pointer
			var valLen uint32
			ret, _, _ = procVerQueryValueW.Call(
				uintptr(unsafe.Pointer(&data[0])),
				uintptr(unsafe.Pointer(subBlock)),
				uintptr(unsafe.Pointer(&valPtr)),
				uintptr(unsafe.Pointer(&valLen)),
			)
			if ret != 0 && valLen > 0 {
				s := windows.UTF16PtrToString((*uint16)(valPtr))
				runtime.KeepAlive(data)
				return s
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
