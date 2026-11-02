//go:build windows

package hosts

import (
	"os/exec"
	"syscall"
)

// flushDNS clears the Windows DNS resolver cache so hosts-file changes take
// effect immediately.
//
// It runs ipconfig with a hidden window. ipconfig is a console program, and
// launching it from this GUI process (the UI or the headless enforcer) without
// hiding the window pops a console window on screen for a fraction of a second
// every call. The enforcer flushes on its polling loop, so that looked like a
// window flashing at intervals. HideWindow + CREATE_NO_WINDOW keeps it silent.
func flushDNS() {
	cmd := exec.Command("ipconfig", "/flushdns")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	_ = cmd.Run()
}
