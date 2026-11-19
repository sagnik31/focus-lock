//go:build windows

package updater

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

const (
	createNoWindow         = 0x08000000
	createBreakawayFromJob = 0x01000000
)

// TaskSetupArgs are the installer flags for a silent update started by the
// updater task. /UPDATE=2 tells Setup it runs elevated without an unelevated
// parent, so it relaunches the app through the shell (non-elevated) instead
// of with runasoriginaluser.
var TaskSetupArgs = []string{"/VERYSILENT", "/SUPPRESSMSGBOXES", "/NORESTART", "/UPDATE=2"}

// adminOnlySDDL matches storage.secureDirSDDL: SYSTEM + Administrators only,
// nothing inherited from ProgramData (where users may create files).
const adminOnlySDDL = "O:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)"

// SecureUpdateDir returns %ProgramData%\FocusLock\update, created if needed,
// with both folders locked to SYSTEM + Administrators. Must run elevated.
func SecureUpdateDir() (string, error) {
	base, err := windows.KnownFolderPath(windows.FOLDERID_ProgramData, 0)
	if err != nil || base == "" {
		return "", fmt.Errorf("locate ProgramData: %v", err)
	}
	parent := filepath.Join(base, "FocusLock")
	dir := filepath.Join(parent, "update")
	for _, d := range []string{parent, dir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return "", err
		}
		if err := notReparsePoint(d); err != nil {
			return "", err
		}
		if err := protect(d); err != nil {
			return "", fmt.Errorf("secure %s: %w", d, err)
		}
	}
	return dir, nil
}

// notReparsePoint refuses a junction or symlink planted in place of a folder.
func notReparsePoint(dir string) error {
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return err
	}
	attrs, err := windows.GetFileAttributes(p)
	if err != nil {
		return err
	}
	if attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return fmt.Errorf("%s is a reparse point", dir)
	}
	return nil
}

func protect(dir string) error {
	sd, err := windows.SecurityDescriptorFromString(adminOnlySDDL)
	if err != nil {
		return err
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		owner, nil, dacl, nil)
}

// StartSetup starts the verified installer (the caller is already elevated)
// and returns without waiting. It asks to break away from the task's job
// object so Setup outlives the updater task, and retries without that flag if
// the job doesn't allow it.
func StartSetup(setupPath string) error {
	start := func(flags uint32) error {
		cmd := exec.Command(setupPath, TaskSetupArgs...)
		cmd.Dir = filepath.Dir(setupPath)
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: flags}
		if err := cmd.Start(); err != nil {
			return err
		}
		return cmd.Process.Release()
	}
	if err := start(createBreakawayFromJob); err == nil {
		return nil
	}
	return start(0)
}

// RunUpdaterTask starts the updater task. An error means the task is missing
// (reduced-mode or pre-ADR-0009 install) or can't be run by this user; the
// caller then falls back to Launch (one UAC prompt).
func RunUpdaterTask() error {
	cmd := exec.Command("schtasks", "/run", "/tn", UpdaterTaskName)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// UpdaterTaskStatus queries the updater task (state, last result and last
// run time) through the ScheduledTasks PowerShell module, whose values are
// not localised, unlike schtasks /query output.
func UpdaterTaskStatus() (TaskStatus, error) {
	script := `$t = Get-ScheduledTask -TaskName '` + UpdaterTaskName + `' -ErrorAction Stop; ` +
		`$i = $t | Get-ScheduledTaskInfo; ` +
		`"$($t.State)|$($i.LastTaskResult)|$($i.LastRunTime.ToUniversalTime().ToString('o'))"`
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	out, err := cmd.Output()
	if err != nil {
		return TaskStatus{}, err
	}
	return ParseTaskStatus(strings.TrimSpace(string(out)))
}
