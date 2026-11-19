package updater

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"focus-lock/backend/version"
)

// UpdaterTaskName is the on-demand scheduled task that installs an update
// without a UAC prompt (ADR 0009). It runs the installed exe with
// --apply-update at the highest available run level and has no trigger: the
// app starts it with `schtasks /run`. It is registered only by the installer
// (scripts/install_setup.ps1), never by the app: dev builds never update.
const UpdaterTaskName = "FocusLock Updater"

// Exit codes of `focus-lock.exe --apply-update`. The app reads them back as
// the updater task's last result (ApplyExitMessage).
const (
	ApplyStarted       = 0  // Setup is running; it closes and relaunches the app
	ApplySessionActive = 3  // a focus session is active (same code as --session-active)
	ApplyNotNewer      = 20 // the latest release is not newer than the installed version
	ApplyNetwork       = 21 // GitHub unreachable or the download failed
	ApplyVerifyFailed  = 22 // bad signature or checksum
	ApplyLaunchFailed  = 23 // Setup could not be started
	ApplyFailed        = 24 // anything else (e.g. the secure folder could not be prepared)
	ApplyDevBuild      = 25 // the installed exe is a dev build
)

// ApplyExitMessage is the message the app shows for an --apply-update exit
// code other than ApplyStarted.
func ApplyExitMessage(code int) string {
	switch code {
	case ApplyStarted:
		return ""
	case ApplySessionActive:
		return "Updates can't be installed during a focus session; it will be available when your session ends."
	case ApplyNotNewer:
		return "The installed version is already the latest release."
	case ApplyNetwork:
		return "The updater could not download the release from GitHub. Check your connection and try again."
	case ApplyVerifyFailed:
		return "The release failed its signature or checksum check, so it was not installed."
	case ApplyLaunchFailed:
		return "The updater could not start the installer."
	case ApplyDevBuild:
		return "Updates are turned off in development builds."
	default:
		return fmt.Sprintf("The updater failed (code %d).", code)
	}
}

// ApplyError carries the exit code for a failed PrepareApply.
type ApplyError struct {
	Code int
	Err  error
}

func (e *ApplyError) Error() string { return e.Err.Error() }
func (e *ApplyError) Unwrap() error { return e.Err }

// PrepareApply is the elevated half of a silent update. Everything it trusts
// is fetched or checked here, never taken from the caller: it asks GitHub for
// the latest release itself, refuses one that is not newer than current (no
// downgrades), checks the signed checksums, and leaves the verified installer
// in c.Dir, which must be an admin-only folder so the file can't be swapped
// between the check and the launch.
//
// userDir is the app's own (user-writable) download folder. A setup already
// downloaded there is copied into c.Dir and only used if the copy matches the
// signed checksum; otherwise it is downloaded again.
func (c *Client) PrepareApply(ctx context.Context, current, userDir string) (string, error) {
	if version.IsDev(current) {
		return "", &ApplyError{ApplyDevBuild, errors.New("dev build")}
	}
	rel, err := c.Latest(ctx)
	if err != nil {
		return "", &ApplyError{ApplyNetwork, err}
	}
	if !Compare(current, rel).Available {
		return "", &ApplyError{ApplyNotNewer, fmt.Errorf("latest %s is not newer than %s", rel.Version(), current)}
	}
	setup, err := rel.SetupAsset()
	if err != nil {
		return "", &ApplyError{ApplyNetwork, err}
	}
	if err := os.MkdirAll(c.Dir, 0o700); err != nil {
		return "", &ApplyError{ApplyFailed, err}
	}
	if userDir != "" {
		// Best effort: a missing or bad copy just means downloading again.
		_ = copyBounded(filepath.Join(userDir, setup.Name), filepath.Join(c.Dir, setup.Name), maxSetupBytes)
	}
	path, _, err := c.Download(ctx, rel, nil)
	if err != nil {
		code := ApplyNetwork
		if errors.Is(err, ErrBadSignature) || errors.Is(err, ErrChecksumMismatch) {
			code = ApplyVerifyFailed
		}
		return "", &ApplyError{code, err}
	}
	return path, nil
}

// ApplyCode returns the exit code for an error from PrepareApply.
func ApplyCode(err error) int {
	if err == nil {
		return ApplyStarted
	}
	var ae *ApplyError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return ApplyFailed
}

// copyBounded copies at most limit bytes of src to dst, failing (and removing
// dst) if src is larger.
func copyBounded(src, dst string, limit int64) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	n, err := io.Copy(out, io.LimitReader(in, limit+1))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > limit {
		err = errors.New("file too large")
	}
	if err != nil {
		_ = os.Remove(dst)
	}
	return err
}

// TaskStatus is the updater task's state as reported by Task Scheduler.
type TaskStatus struct {
	Running    bool
	LastResult int64
	LastRun    time.Time
}

// ParseTaskStatus parses "State|LastTaskResult|LastRunTime(RFC 3339)".
func ParseTaskStatus(s string) (TaskStatus, error) {
	parts := strings.Split(s, "|")
	if len(parts) != 3 {
		return TaskStatus{}, errors.New("unexpected task status: " + s)
	}
	code, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
	if err != nil {
		return TaskStatus{}, fmt.Errorf("unexpected task result %q", parts[1])
	}
	at, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(parts[2]))
	if err != nil {
		return TaskStatus{}, fmt.Errorf("unexpected task run time %q", parts[2])
	}
	return TaskStatus{
		Running:    strings.EqualFold(strings.TrimSpace(parts[0]), "Running"),
		LastResult: code,
		LastRun:    at,
	}, nil
}
