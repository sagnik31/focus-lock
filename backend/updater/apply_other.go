//go:build !windows

package updater

import "errors"

var errWindowsOnly = errors.New("installing updates is only supported on Windows")

// TaskSetupArgs are the installer flags for an update started by the task.
var TaskSetupArgs = []string{"/VERYSILENT", "/SUPPRESSMSGBOXES", "/NORESTART", "/UPDATE=2"}

// SecureUpdateDir is not supported off Windows.
func SecureUpdateDir() (string, error) { return "", errWindowsOnly }

// StartSetup is not supported off Windows.
func StartSetup(setupPath string) error { return errWindowsOnly }

// RunUpdaterTask is not supported off Windows.
func RunUpdaterTask() error { return errWindowsOnly }

// UpdaterTaskStatus is not supported off Windows.
func UpdaterTaskStatus() (TaskStatus, error) { return TaskStatus{}, errWindowsOnly }
