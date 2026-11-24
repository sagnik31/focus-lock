//go:build !windows

package storage

import "time"

// lockFile does nothing off Windows, where nothing is enforced.
func lockFile(string, time.Duration) (release func()) { return func() {} }
