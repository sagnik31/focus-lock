package watchdog

import (
	"fmt"
	"os"
	"testing"
)

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
