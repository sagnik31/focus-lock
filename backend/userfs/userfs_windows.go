//go:build windows

// Package userfs runs file operations on the user's own folders
// (%APPDATA%\FocusLock) with the rights of the non-elevated user.
//
// The enforcer runs elevated but keeps its config, request/result files and
// logs in a folder the user controls. Any program running as that user can put
// a junction, an object-manager link or a hard link there, and an elevated
// path-following write would then land wherever the link points (G25). Under
// Do, the calling thread impersonates a copy of the process token with the
// Administrators group set to deny-only, every privilege but
// SeChangeNotifyPrivilege removed and medium integrity, so Windows checks
// each open against what the user could already do. A redirected write fails
// instead of reaching a file only admins may change.
package userfs

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	advapi32                  = windows.NewLazySystemDLL("advapi32.dll")
	procCreateRestrictedToken = advapi32.NewProc("CreateRestrictedToken")
)

const disableMaxPrivilege = 0x1 // DISABLE_MAX_PRIVILEGE

// ErrDisabled is returned by Do, without running the function, when Enable
// could not build the restricted token.
var ErrDisabled = errors.New("user file access is disabled")

const mediumRID = 0x2000 // SECURITY_MANDATORY_MEDIUM_RID

var (
	mu      sync.RWMutex
	enabled bool
	token   windows.Token // impersonation token; 0 when Enable failed
	initErr error
)

// Enable makes Do impersonate the restricted user token from now on. The
// enforcer calls it once at startup. If the token can't be built, Do refuses
// every operation instead of running it elevated (fail closed).
func Enable() error {
	tok, err := restrictedToken()
	mu.Lock()
	defer mu.Unlock()
	if enabled && token != 0 {
		_ = token.Close()
	}
	enabled, token, initErr = true, tok, err
	return err
}

// Disable undoes Enable (for tests).
func Disable() {
	mu.Lock()
	defer mu.Unlock()
	if token != 0 {
		_ = token.Close()
	}
	enabled, token, initErr = false, 0, nil
}

// Enabled reports whether Enable has been called.
func Enabled() bool {
	mu.RLock()
	defer mu.RUnlock()
	return enabled
}

// Do runs fn. After Enable it runs fn on a locked OS thread that impersonates
// the restricted token, so every file the function opens is checked against
// the user's own rights. fn must do its work on the calling goroutine.
func Do(fn func() error) error {
	mu.RLock()
	on, tok, ierr := enabled, token, initErr
	mu.RUnlock()
	if !on {
		return fn()
	}
	if tok == 0 {
		return fmt.Errorf("%w: %v", ErrDisabled, ierr)
	}

	runtime.LockOSThread()
	if err := windows.SetThreadToken(nil, tok); err != nil {
		runtime.UnlockOSThread()
		return fmt.Errorf("impersonate the user: %w", err)
	}
	defer func() {
		// If reverting fails, keep the thread locked: the runtime then ends
		// the thread with this goroutine instead of running other work on it
		// with the restricted token still set.
		if windows.RevertToSelf() == nil {
			runtime.UnlockOSThread()
		}
	}()
	return fn()
}

// restrictedToken builds the impersonation token used by Do.
func restrictedToken() (windows.Token, error) {
	var self windows.Token
	err := windows.OpenProcessToken(windows.CurrentProcess(),
		windows.TOKEN_DUPLICATE|windows.TOKEN_QUERY|windows.TOKEN_ASSIGN_PRIMARY|windows.TOKEN_ADJUST_DEFAULT, &self)
	if err != nil {
		return 0, fmt.Errorf("open process token: %w", err)
	}
	defer self.Close()

	admins, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return 0, err
	}
	disable := []windows.SIDAndAttributes{{Sid: admins}}
	var restricted windows.Token
	r, _, callErr := procCreateRestrictedToken.Call(
		uintptr(self), disableMaxPrivilege,
		uintptr(len(disable)), uintptr(unsafe.Pointer(&disable[0])),
		0, 0, // no privileges to delete beyond DISABLE_MAX_PRIVILEGE
		0, 0, // no restricting SIDs
		uintptr(unsafe.Pointer(&restricted)))
	if r == 0 {
		return 0, fmt.Errorf("CreateRestrictedToken: %w", callErr)
	}
	defer restricted.Close()

	var imp windows.Token
	if err := windows.DuplicateTokenEx(restricted, windows.MAXIMUM_ALLOWED, nil,
		windows.SecurityImpersonation, windows.TokenImpersonation, &imp); err != nil {
		return 0, fmt.Errorf("duplicate token: %w", err)
	}
	ok := false
	defer func() {
		if !ok {
			imp.Close()
		}
	}()

	// An elevated token's default owner is usually BUILTIN\Administrators,
	// which is now deny-only and can't own new files: make it the user.
	user, err := imp.GetTokenUser()
	if err != nil {
		return 0, err
	}
	owner := struct{ Owner *windows.SID }{user.User.Sid}
	if err := windows.SetTokenInformation(imp, windows.TokenOwner,
		(*byte)(unsafe.Pointer(&owner)), uint32(unsafe.Sizeof(owner))); err != nil {
		return 0, fmt.Errorf("set token owner: %w", err)
	}

	if err := lowerToMedium(imp); err != nil {
		return 0, err
	}
	ok = true
	return imp, nil
}

// lowerToMedium sets the token's integrity level to medium when it is higher.
func lowerToMedium(t windows.Token) error {
	level, err := integrityRID(t)
	if err != nil {
		return err
	}
	if level <= mediumRID {
		return nil
	}
	medium, err := windows.CreateWellKnownSid(windows.WinMediumLabelSid)
	if err != nil {
		return err
	}
	label := windows.Tokenmandatorylabel{Label: windows.SIDAndAttributes{Sid: medium, Attributes: windows.SE_GROUP_INTEGRITY}}
	if err := windows.SetTokenInformation(t, windows.TokenIntegrityLevel,
		(*byte)(unsafe.Pointer(&label)), label.Size()); err != nil {
		return fmt.Errorf("lower integrity: %w", err)
	}
	return nil
}

// integrityRID returns the integrity level RID of t.
func integrityRID(t windows.Token) (uint32, error) {
	var n uint32
	err := windows.GetTokenInformation(t, windows.TokenIntegrityLevel, nil, 0, &n)
	if err != nil && !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) {
		return 0, err
	}
	buf := make([]byte, n)
	if err := windows.GetTokenInformation(t, windows.TokenIntegrityLevel, &buf[0], n, &n); err != nil {
		return 0, err
	}
	label := (*windows.Tokenmandatorylabel)(unsafe.Pointer(&buf[0]))
	sid := label.Label.Sid
	return sid.SubAuthority(uint32(sid.SubAuthorityCount()) - 1), nil
}
