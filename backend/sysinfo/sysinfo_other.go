//go:build !windows

package sysinfo

func getAppsFromRegistry() ([]AppInfo, error) {
	return []AppInfo{}, nil
}

func getRunningProcesses() (map[string]bool, error) {
	return map[string]bool{}, nil
}
