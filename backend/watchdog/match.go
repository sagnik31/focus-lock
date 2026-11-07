package watchdog

import "strings"

// deepMatch reports whether a process whose version metadata has the given
// product name and file description should be blocked, given the configured
// blocked-app names. Matching is a case-insensitive substring test of either
// metadata field against each blocked name with its ".exe" suffix stripped.
//
// It is the pure core of enforceDeep's metadata match, split out so it can be
// unit-tested without enumerating real processes. An empty blocked name is
// skipped: strings.Contains(x, "") is always true, so without the guard a stray
// "" (or ".exe") in the block list would match every running process.
func deepMatch(prodName, fileDesc string, blockedApps []string) bool {
	prodName = strings.ToLower(prodName)
	fileDesc = strings.ToLower(fileDesc)

	for _, blocked := range blockedApps {
		blockedClean := strings.TrimSuffix(strings.ToLower(blocked), ".exe")
		if blockedClean == "" {
			continue
		}
		if (prodName != "" && strings.Contains(prodName, blockedClean)) ||
			(fileDesc != "" && strings.Contains(fileDesc, blockedClean)) {
			return true
		}
	}
	return false
}
