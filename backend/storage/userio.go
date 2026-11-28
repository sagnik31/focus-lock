package storage

import (
	"os"

	"focus-lock/backend/userfs"
)

// The config, its signature and backups, and the request/result files live in
// the user-writable %APPDATA%\FocusLock. Every access to them goes through
// these helpers, which run under userfs.Do: in the elevated enforcer that
// checks each open against the user's own rights, so a link planted in that
// folder can't turn an elevated write or read into one somewhere else (G25).
// The admin-only secure copy (secureDir) is accessed directly.

func readUser(path string) ([]byte, error) {
	var data []byte
	err := userfs.Do(func() error {
		var err error
		data, err = os.ReadFile(path)
		return err
	})
	return data, err
}

func writeUser(path string, data []byte) error {
	return userfs.Do(func() error { return atomicWrite(path, data) })
}

func removeUser(path string) {
	_ = userfs.Do(func() error { return os.Remove(path) })
}

func statUser(path string) (os.FileInfo, error) {
	var info os.FileInfo
	err := userfs.Do(func() error {
		var err error
		info, err = os.Stat(path)
		return err
	})
	return info, err
}
