//go:build !windows

package protection

func protectProcess() error {
	// No-op on non-Windows
	return nil
}

func setCritical(enable bool) error {
	return nil
}
