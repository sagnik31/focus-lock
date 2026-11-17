//go:build windows

package updater

import (
	"errors"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// SetupArgs are the installer flags for an in-app update: silent, no reboot,
// and /UPDATE=1 so Setup relaunches the app when it is done.
const SetupArgs = "/VERYSILENT /SUPPRESSMSGBOXES /NORESTART /UPDATE=1"

// ErrUACDeclined means the user said "No" at the UAC prompt.
var ErrUACDeclined = errors.New("the administrator prompt was declined, so the update was not installed")

// Launch starts the installer elevated (ShellExecute verb "runas", which shows
// one UAC prompt) and returns once it has started.
func Launch(setupPath string) error {
	verb, _ := windows.UTF16PtrFromString("runas")
	file, err := windows.UTF16PtrFromString(setupPath)
	if err != nil {
		return err
	}
	args, _ := windows.UTF16PtrFromString(SetupArgs)
	dir, _ := windows.UTF16PtrFromString(filepath.Dir(setupPath))
	err = windows.ShellExecute(0, verb, file, args, dir, windows.SW_SHOWNORMAL)
	if errors.Is(err, windows.ERROR_CANCELLED) {
		return ErrUACDeclined
	}
	return err
}
