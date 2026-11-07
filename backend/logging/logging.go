// Package logging provides a tiny size-capped append logger for the debug logs
// in %APPDATA%\FocusLock. The logs were previously opened with a plain
// O_APPEND and never trimmed, so debug.log / ghost_debug.log grew without bound
// (and the kill log records user behaviour, so an unbounded file is also a
// privacy wart). Write caps each file by keeping at most one rotated backup.
package logging

import (
	"os"
	"sync"
	"time"
)

// maxBytes is the size at which a log file is rotated. With one retained
// backup (.1) the worst case on disk is ~2x this per log.
const maxBytes = 1 << 20 // 1 MiB

var mu sync.Mutex

// Write appends an RFC3339-timestamped line to the log at path, rotating it
// first if it has reached maxBytes. All errors are swallowed: logging must
// never disrupt enforcement.
func Write(path, msg string) {
	mu.Lock()
	defer mu.Unlock()

	rotateIfNeeded(path)

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(time.Now().Format(time.RFC3339) + " " + msg + "\n")
}

// rotateIfNeeded renames path to path+".1" (replacing any previous backup) once
// it reaches the size cap, so the active log starts fresh and total growth is
// bounded. The next Write recreates path.
func rotateIfNeeded(path string) {
	info, err := os.Stat(path)
	if err != nil || info.Size() < maxBytes {
		return
	}
	backup := path + ".1"
	_ = os.Remove(backup)       // discard the older generation
	_ = os.Rename(path, backup) // current -> .1 (best-effort; may fail if locked)
}
