//go:build !windows

package sysinfo

func getAppsFromRegistry() ([]AppInfo, error) {
	return []AppInfo{}, nil
}
