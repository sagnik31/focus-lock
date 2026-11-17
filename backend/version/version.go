// Package version holds the app's build version and compares calendar versions.
//
// Releases use calendar versions YYYY.M.D.N (tag vYYYY.M.D.N), which are also
// valid 4-part Windows file versions. Version is set at build time:
//
//	wails build -ldflags "-X focus-lock/backend/version.Version=2026.11.17.0"
//
// Local builds keep the default "dev" and never offer updates.
package version

import (
	"strconv"
	"strings"
)

// Dev is the version of a build that was not stamped by the release pipeline.
const Dev = "dev"

// Version is the running build's version (overridden via -ldflags -X).
var Version = Dev

// IsDev reports whether v is an unstamped/unparseable development version.
func IsDev(v string) bool {
	_, ok := Parse(v)
	return !ok
}

// Parse parses "YYYY.M.D.N" (an optional leading "v"; missing trailing parts
// count as 0) into its four numeric parts. Each part must be a non-negative
// integer no larger than 65535 (the Windows file-version limit). ok is false
// for anything else, including "dev" and "".
func Parse(v string) (parts [4]int, ok bool) {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(strings.TrimPrefix(v, "v"), "V")
	if v == "" {
		return parts, false
	}
	fields := strings.Split(v, ".")
	if len(fields) > 4 {
		return parts, false
	}
	for i, f := range fields {
		if f == "" {
			return parts, false
		}
		for _, r := range f {
			if r < '0' || r > '9' {
				return parts, false
			}
		}
		n, err := strconv.Atoi(f)
		if err != nil || n > 65535 {
			return parts, false
		}
		parts[i] = n
	}
	return parts, true
}

// Compare returns -1, 0 or +1 as a is older than, equal to or newer than b.
// An unparseable version (e.g. "dev") sorts before every valid version, and
// two unparseable versions compare equal.
func Compare(a, b string) int {
	pa, oka := Parse(a)
	pb, okb := Parse(b)
	switch {
	case !oka && !okb:
		return 0
	case !oka:
		return -1
	case !okb:
		return 1
	}
	for i := range pa {
		if pa[i] < pb[i] {
			return -1
		}
		if pa[i] > pb[i] {
			return 1
		}
	}
	return 0
}

// Normalize returns v as a bare 4-part "A.B.C.D" string, or v unchanged if it
// does not parse.
func Normalize(v string) string {
	p, ok := Parse(v)
	if !ok {
		return v
	}
	s := make([]string, len(p))
	for i, n := range p {
		s[i] = strconv.Itoa(n)
	}
	return strings.Join(s, ".")
}
