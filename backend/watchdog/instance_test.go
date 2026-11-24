package watchdog

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

// G26: the signer's slot is a file, so a squatted mutex can't take it.
func TestAcquireEnforcerFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "enforcer.lock")
	p, _ := windows.UTF16PtrFromString(path)
	h, err := windows.CreateFile(p, windows.GENERIC_READ, 0, nil, windows.OPEN_ALWAYS, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !acquireFile(path) {
		t.Error("a held slot was reported free")
	}
	windows.CloseHandle(h)
	dir, err := os.MkdirTemp("", "fl") // the slot is held until the test process exits
	if err != nil {
		t.Fatal(err)
	}
	if AcquireEnforcer(dir, true, true) {
		t.Error("a free slot was reported held")
	}
	if !AcquireEnforcer(dir, true, true) {
		t.Error("a second enforcer was let in")
	}
}

func TestInstanceMutex(t *testing.T) {
	name := fmt.Sprintf(`Local\FocusLockTest-%d`, os.Getpid())
	if InstanceRunning(name) {
		t.Fatal("nothing holds the mutex yet")
	}
	if AcquireInstance(name) {
		t.Fatal("the first acquire should succeed; InstanceRunning must not create the mutex")
	}
	if !InstanceRunning(name) {
		t.Error("the held mutex should be reported as running")
	}
	if !AcquireInstance(name) {
		t.Error("a second acquire should report an instance already running")
	}
}
