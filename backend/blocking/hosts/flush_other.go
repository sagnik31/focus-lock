//go:build !windows

package hosts

// flushDNS is a no-op off Windows; the hosts logic here targets Windows.
func flushDNS() {}
