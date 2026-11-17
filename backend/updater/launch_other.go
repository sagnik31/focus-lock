//go:build !windows

package updater

import "errors"

// SetupArgs are the installer flags for an in-app update.
const SetupArgs = "/VERYSILENT /SUPPRESSMSGBOXES /NORESTART /UPDATE=1"

// ErrUACDeclined means the user declined elevation.
var ErrUACDeclined = errors.New("the administrator prompt was declined, so the update was not installed")

// Launch is not supported off Windows.
func Launch(setupPath string) error {
	return errors.New("installing updates is only supported on Windows")
}
