package watchdog

import (
	"path/filepath"
	"strings"

	"focus-lock/backend/storage"
)

// minDeepMatchLen is the shortest blocked name (without ".exe") that the
// metadata substring match applies to.
const minDeepMatchLen = 3

// metadataMatchable reports whether the loose metadata match may be applied to
// the executable at path. Protected processes and everything in the Windows
// folder are excluded: their product name is "Microsoft Windows Operating
// System", so one broad entry ("windows") would end the whole desktop.
// Programs there can still be blocked by their exact name.
func metadataMatchable(path, windowsDir string) bool {
	if storage.ProtectedApp(filepath.Base(path)) {
		return false
	}
	if windowsDir == "" {
		return true
	}
	prefix := strings.ToLower(filepath.Clean(windowsDir)) + string(filepath.Separator)
	return !strings.HasPrefix(strings.ToLower(filepath.Clean(path)), prefix)
}

// deepMatch reports whether a process whose version metadata has the given
// product name and file description should be blocked, given the configured
// blocked-app names. Matching is a case-insensitive substring test of either
// metadata field against each blocked name with its ".exe" suffix stripped.
//
// It is the pure core of enforceDeep's metadata match, split out so it can be
// unit-tested without enumerating real processes. Names that are invalid
// (storage.ValidAppName: "", ".", ".exe", ...) or shorter than
// minDeepMatchLen are skipped: a substring test with such a name would match
// almost every running process (strings.Contains(x, "") is always true, and
// "." appears in ".NET", "Node.js", ...). Short names are still blocked by the
// exact-name fast loop.
func deepMatch(prodName, fileDesc string, blockedApps []string) bool {
	prodName = strings.ToLower(prodName)
	fileDesc = strings.ToLower(fileDesc)

	for _, blocked := range blockedApps {
		if !storage.ValidAppName(blocked) {
			continue
		}
		blockedClean := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(blocked)), ".exe")
		if len([]rune(blockedClean)) < minDeepMatchLen {
			continue
		}
		if (prodName != "" && strings.Contains(prodName, blockedClean)) ||
			(fileDesc != "" && strings.Contains(fileDesc, blockedClean)) {
			return true
		}
	}
	return false
}
