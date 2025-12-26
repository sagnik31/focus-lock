package protection

// ProtectProcess attempts to modify the current process's permissions
// to prevent termination by the user (e.g., via Task Manager).
// On non-Windows systems, this is a no-op.
func ProtectProcess() error {
	return protectProcess()
}

// SetCritical sets the process as a critical system process (BSOD on termination).
// On non-Windows systems, this is a no-op.
func SetCritical(enable bool) error {
	return setCritical(enable)
}
