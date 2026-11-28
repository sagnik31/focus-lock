//go:build windows

package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

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

// LaunchVerified starts the downloaded installer with one UAC prompt and makes
// sure the file that runs is the one whose checksum matched (G24). The
// download folder is user-writable, so checking a path and then launching the
// same path could run a file swapped in between. Instead the file is opened
// without write or delete sharing, so while it is open nothing can change,
// rename or delete it (or rename the folders above it); it is hashed through
// that handle; and the path launched is the one the handle resolves to, with
// no links left in it.
//
// The file stays locked until release is called. Setup reads its own file
// after it starts, so keep the lock until Setup has taken over (it closes the
// app) or has evidently not gone ahead.
func LaunchVerified(setupPath, wantSHA256 string) (release func(), err error) {
	f, resolved, err := openVerified(setupPath, wantSHA256)
	if err != nil {
		return nil, err
	}
	if err := Launch(resolved); err != nil {
		f.Close()
		return nil, err
	}
	return func() { f.Close() }, nil
}

// openVerified opens path locked against changes, checks its SHA-256 through
// the handle and returns the open file with its resolved path.
func openVerified(path, wantSHA256 string) (*os.File, string, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, "", err
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil,
		windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrInstallerUnreadable, err)
	}
	f := os.NewFile(uintptr(h), path)
	sum := sha256.New()
	if _, err := io.Copy(sum, f); err != nil {
		f.Close()
		return nil, "", fmt.Errorf("%w: %v", ErrInstallerUnreadable, err)
	}
	if !strings.EqualFold(hex.EncodeToString(sum.Sum(nil)), wantSHA256) {
		f.Close()
		return nil, "", ErrChecksumMismatch
	}
	resolved, err := finalPath(h)
	if err != nil {
		f.Close()
		return nil, "", fmt.Errorf("%w: resolve its path: %v", ErrInstallerUnreadable, err)
	}
	return f, resolved, nil
}

// finalPath returns the fully resolved DOS path of an open file.
func finalPath(h windows.Handle) (string, error) {
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n, err := windows.GetFinalPathNameByHandle(h, &buf[0], uint32(len(buf)), 0) // FILE_NAME_NORMALIZED | VOLUME_NAME_DOS
	if err != nil {
		return "", err
	}
	if int(n) >= len(buf) {
		return "", errors.New("path too long")
	}
	return trimLongPathPrefix(windows.UTF16ToString(buf[:n])), nil
}

// trimLongPathPrefix turns \\?\C:\x into C:\x and \\?\UNC\srv\x into \\srv\x.
func trimLongPathPrefix(p string) string {
	if rest, ok := strings.CutPrefix(p, `\\?\UNC\`); ok {
		return `\\` + rest
	}
	return strings.TrimPrefix(p, `\\?\`)
}
