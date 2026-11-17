//go:build !windows

package browserpolicy

// ApplyChrome is a no-op off Windows (enforcement is Windows-only).
func ApplyChrome(sites []string) error { return nil }

// ClearChrome is a no-op off Windows.
func ClearChrome() error { return nil }
