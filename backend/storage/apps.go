package storage

import "strings"

// ValidAppName reports whether name can usefully be blocked: a bare executable
// name (with or without ".exe") that contains at least one letter or digit and
// no path separator. It rejects junk such as "", "." and ".exe", which used to
// reach the block list from apps whose exe path could not be resolved
// (filepath.Base("") is ".") and which the enforcer's substring metadata match
// would then apply to almost every process.
func ValidAppName(name string) bool {
	n := strings.TrimSpace(name)
	if strings.ContainsAny(n, `\/`) {
		return false
	}
	base := strings.TrimSuffix(strings.ToLower(n), ".exe")
	for _, r := range base {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r > 127 {
			return true
		}
	}
	return false
}

// ValidApps returns the trimmed, valid entries of apps (see ValidAppName).
func ValidApps(apps []string) []string {
	out := make([]string, 0, len(apps))
	for _, a := range apps {
		a = strings.TrimSpace(a)
		if ValidAppName(a) {
			out = append(out, a)
		}
	}
	return out
}
