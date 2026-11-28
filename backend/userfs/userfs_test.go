//go:build windows

package userfs

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"golang.org/x/sys/windows"
)

func enable(t *testing.T) {
	t.Helper()
	if err := Enable(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(Disable)
}

func threadToken() (windows.Token, error) {
	var tok windows.Token
	err := windows.OpenThreadToken(windows.CurrentThread(), windows.TOKEN_QUERY, true, &tok)
	return tok, err
}

// G25: inside Do the thread acts as a non-admin, medium-integrity user, and
// the impersonation ends with Do.
func TestDoImpersonatesRestrictedUser(t *testing.T) {
	enable(t)
	runtime.LockOSThread() // check the same thread after Do
	defer runtime.UnlockOSThread()

	err := Do(func() error {
		tok, err := threadToken()
		if err != nil {
			t.Fatalf("no thread token inside Do: %v", err)
		}
		defer tok.Close()
		rid, err := integrityRID(tok)
		if err != nil {
			t.Fatal(err)
		}
		if rid > mediumRID {
			t.Errorf("integrity RID %#x, want medium or lower", rid)
		}
		admins, _ := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
		member, err := tok.IsMember(admins)
		if err != nil {
			t.Fatal(err)
		}
		if member {
			t.Error("the Administrators group is still enabled")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if tok, err := threadToken(); !errors.Is(err, windows.ERROR_NO_TOKEN) {
		if err == nil {
			tok.Close()
		}
		t.Errorf("thread still impersonating after Do (err %v)", err)
	}
}

// Ordinary work in the user's own folders still succeeds.
func TestDoWritesUserFiles(t *testing.T) {
	enable(t)
	path := filepath.Join(t.TempDir(), "a.json")
	err := Do(func() error {
		if err := os.WriteFile(path+".tmp", []byte("x"), 0o644); err != nil {
			return err
		}
		return os.Rename(path+".tmp", path)
	})
	if err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(path); err != nil || string(b) != "x" {
		t.Fatalf("read back %q, %v", b, err)
	}
}

// A folder only administrators may write to is refused, even when the test
// runs elevated (as on CI).
func TestDoRefusesAdminOnlyWrites(t *testing.T) {
	enable(t)
	win, err := windows.GetWindowsDirectory()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(win, "focuslock-userfs-test.tmp")
	err = Do(func() error { return os.WriteFile(path, []byte("x"), 0o644) })
	if err == nil {
		_ = os.Remove(path)
		t.Fatal("wrote to the Windows folder with the user's rights")
	}
	if !errors.Is(err, os.ErrPermission) {
		t.Logf("refused with %v", err)
	}
}

// Without a token Do runs nothing (fail closed).
func TestDoFailsClosed(t *testing.T) {
	mu.Lock()
	enabled, token, initErr = true, 0, errors.New("boom")
	mu.Unlock()
	t.Cleanup(Disable)
	ran := false
	if err := Do(func() error { ran = true; return nil }); !errors.Is(err, ErrDisabled) {
		t.Fatalf("got %v, want ErrDisabled", err)
	}
	if ran {
		t.Error("ran the function without the restricted token")
	}
}

func TestDoWithoutEnableRunsDirectly(t *testing.T) {
	Disable()
	ran := false
	if err := Do(func() error { ran = true; return nil }); err != nil || !ran {
		t.Fatalf("ran=%v err=%v", ran, err)
	}
}
