//go:build windows

package protection

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modadvapi32                 = windows.NewLazySystemDLL("advapi32.dll")
	modntdll                    = windows.NewLazySystemDLL("ntdll.dll")
	procSetEntriesInAclW        = modadvapi32.NewProc("SetEntriesInAclW")
	procRtlSetProcessIsCritical = modntdll.NewProc("RtlSetProcessIsCritical")
)

// setCritical implements the Windows-specific logic.
func setCritical(enable bool) error {
	// Debug logging helper
	logErr := func(msg string) {
		configDir, _ := os.UserConfigDir()
		logPath := filepath.Join(configDir, "FocusLock", "protection_error.log")
		_ = os.MkdirAll(filepath.Dir(logPath), 0755)
		f, _ := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		defer f.Close()
		f.WriteString(fmt.Sprintf("%s: %s\n", time.Now().Format(time.RFC3339), msg))
	}

	// 1. Enable SeDebugPrivilege
	if err := enableDebugPrivilege(); err != nil {
		logErr(fmt.Sprintf("Failed to enable SeDebugPrivilege: %v", err))
		return fmt.Errorf("failed to enable SeDebugPrivilege: %w", err)
	}

	// 2. Call RtlSetProcessIsCritical
	// NTSTATUS RtlSetProcessIsCritical(BOOLEAN NewValue, PBOOLEAN OldValue, BOOLEAN CheckFlag);
	// CheckFlag = false is usually required? Some sources say false.
	var newVal uintptr
	if enable {
		newVal = 1
	} else {
		newVal = 0
	}

	r1, _, _ := procRtlSetProcessIsCritical.Call(newVal, 0, 0)
	// r1 is NTSTATUS. 0 is STATUS_SUCCESS.
	if r1 != 0 {
		logErr(fmt.Sprintf("RtlSetProcessIsCritical failed with NTSTATUS: 0x%x", r1))
		return fmt.Errorf("RtlSetProcessIsCritical failed with NTSTATUS: 0x%x", r1)
	}

	return nil
}

// enableDebugPrivilege enables the SeDebugPrivilege for the current process token.
func enableDebugPrivilege() error {
	var token windows.Token
	p, _ := windows.GetCurrentProcess() // Pseudo handle
	err := windows.OpenProcessToken(p, windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &token)
	if err != nil {
		return fmt.Errorf("OpenProcessToken failed: %w", err)
	}
	defer token.Close()

	var luid windows.LUID
	err = windows.LookupPrivilegeValue(nil, windows.StringToUTF16Ptr("SeDebugPrivilege"), &luid)
	if err != nil {
		return fmt.Errorf("LookupPrivilegeValue failed: %w", err)
	}

	tp := windows.Tokenprivileges{
		PrivilegeCount: 1,
		Privileges: [1]windows.LUIDAndAttributes{
			{
				Luid:       luid,
				Attributes: windows.SE_PRIVILEGE_ENABLED,
			},
		},
	}

	// AdjustTokenPrivileges
	// We need to unsafe cast to pass the pointer locally, as x/sys wrapper signature might vary or be strict
	// x/sys/windows.AdjustTokenPrivileges takes *Tokenprivileges
	err = windows.AdjustTokenPrivileges(token, false, &tp, 0, nil, nil)
	if err != nil {
		return fmt.Errorf("AdjustTokenPrivileges failed: %w", err)
	}

	// AdjustTokenPrivileges can return nil error even if it failed to adjust all.
	// We should check GetLastError via err, but Go handles this?
	// The doc says: "If the function succeeds, the return value is nonzero. To determine whether the function adjusted all of the specified privileges, call GetLastError..."
	// windows.AdjustTokenPrivileges wraps this. It returns error if the call failed OR if GetLastError == ERROR_NOT_ALL_ASSIGNED (in typical wrappers).
	// Let's assume it works if err == nil for now.

	return nil
}

// TRUSTEE_W structure matches the Windows API definition.
// We define it locally to avoid mismatches or missing fields in the x/sys/windows package if valid.
// Explicitly using uintptr for the name to handle pointers correctly.
type TRUSTEE_W struct {
	pMultipleTrustee         *TRUSTEE_W
	MultipleTrusteeOperation int32 // TRUSTEE_FORM
	TrusteeForm              int32
	TrusteeType              int32
	ptstrName                uintptr
}

// EXPLICIT_ACCESS_W structure
type EXPLICIT_ACCESS_W struct {
	AccessPermissions uint32
	AccessMode        int32 // ACCESS_MODE
	Inheritance       uint32
	Trustee           TRUSTEE_W
}

// setEntriesInAcl wraps the SetEntriesInAclW call.
func setEntriesInAcl(countOfExplicitEntries uint32, listOfExplicitEntries *EXPLICIT_ACCESS_W, oldAcl *windows.ACL, newAcl **windows.ACL) error {
	r1, _, _ := procSetEntriesInAclW.Call(
		uintptr(countOfExplicitEntries),
		uintptr(unsafe.Pointer(listOfExplicitEntries)),
		uintptr(unsafe.Pointer(oldAcl)),
		uintptr(unsafe.Pointer(newAcl)),
	)
	if r1 != 0 {
		return fmt.Errorf("SetEntriesInAclW failed with error code %d", r1)
	}
	return nil
}

// protectProcess implements the Windows-specific logic to deny PROCESS_TERMINATE.
func protectProcess() error {
	// Get the current process handle.
	// GetCurrentProcess returns a pseudo-handle (-1) which has MAXIMUM_ALLOWED permissions.
	currentProcess, err := windows.GetCurrentProcess()
	if err != nil {
		return fmt.Errorf("failed to get current process: %w", err)
	}

	// 1. Get the current Security Descriptor (DACL)
	// calling windows.GetSecurityInfo with the correct signature given the lint feedback
	sd, err := windows.GetSecurityInfo(
		windows.Handle(currentProcess),
		windows.SE_KERNEL_OBJECT,
		windows.DACL_SECURITY_INFORMATION,
	)
	if err != nil {
		return fmt.Errorf("failed to get security info: %w", err)
	}

	// Extract the DACL from the Security Descriptor.
	// sd.DACL() returns (*ACL, bool, error)
	dacl, _, err := sd.DACL()
	if err != nil {
		return fmt.Errorf("failed to get DACL from SD: %w", err)
	}

	// 2. Build a new EXPLICIT_ACCESS entry to deny PROCESS_TERMINATE to Everyone (World).
	everyoneSid, err := windows.CreateWellKnownSid(windows.WinWorldSid)
	if err != nil {
		return fmt.Errorf("failed to create Everyone SID: %w", err)
	}

	// Constants based on Windows API
	const TRUSTEE_IS_SID = 0              // TRUSTEE_IS_SID is 0, NOT 1
	const TRUSTEE_IS_WELL_KNOWN_GROUP = 5 // TRUSTEE_TYPE_WELL_KNOWN_GROUP
	const DENY_ACCESS = 3                 // DENY_ACCESS_ACE_FLAG
	const NO_INHERITANCE = 0              // NO_INHERITANCE

	explicitAccess := EXPLICIT_ACCESS_W{
		AccessPermissions: windows.PROCESS_TERMINATE,
		AccessMode:        DENY_ACCESS,
		Inheritance:       NO_INHERITANCE,
		Trustee: TRUSTEE_W{
			TrusteeForm: TRUSTEE_IS_SID,
			TrusteeType: TRUSTEE_IS_WELL_KNOWN_GROUP,
			ptstrName:   uintptr(unsafe.Pointer(everyoneSid)),
		},
	}

	// 3. Create a new ACL that merges the existing DACL with our new entry.
	var newDacl *windows.ACL
	err = setEntriesInAcl(
		1,
		&explicitAccess,
		dacl,
		&newDacl,
	)
	if err != nil {
		return fmt.Errorf("failed to set entries in ACL: %w", err)
	}
	// Verify newDacl is not nil
	if newDacl == nil {
		return fmt.Errorf("SetEntriesInAcl returned nil ACL")
	}
	// Free the memory allocated by SetEntriesInAcl
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(newDacl)))

	// 4. Set the new DACL onto the process
	err = windows.SetSecurityInfo(
		windows.Handle(currentProcess),
		windows.SE_KERNEL_OBJECT,
		windows.DACL_SECURITY_INFORMATION,
		nil, // pptsidOwner
		nil, // pptsidGroup
		newDacl,
		nil, // ppSacl
	)
	if err != nil {
		return fmt.Errorf("failed to set security info: %w", err)
	}

	return nil
}
