package sysinfo

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// Registry locations to check
// 1. Standard Uninstall keys (Control Panel apps)
var uninstallKeys = []struct {
	Key  registry.Key
	Path string
}{
	{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`},
	{registry.LOCAL_MACHINE, `SOFTWARE\Wow6432Node\Microsoft\Windows\CurrentVersion\Uninstall`},
	{registry.CURRENT_USER, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`},
}

// 2. App Paths (Executables registered globally, e.g., "excel.exe")
const appPathsKey = `SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths`

// 3. Store Apps (User specific)
const storeAppsKey = `Software\Classes\Local Settings\Software\Microsoft\Windows\CurrentVersion\AppModel\Repository\Packages`

// getAppsFromRegistry scans the system (Registry + Store) for installed apps.
// It returns the list to the main sysinfo.go which adds icons.
func getAppsFromRegistry() ([]AppInfo, error) {
	uniqueApps := make(map[string]AppInfo)

	// Phase 1: Scan Standard Uninstall Keys
	scanUninstallKeys(uniqueApps)

	// Phase 2: Scan "App Paths" (High reliability for .exe paths)
	// REMOVED: User requested only "Installed Apps" list style. App Paths often includes helper exes.
	// scanAppPaths(uniqueApps)

	// Phase 3: Scan Windows Store Apps
	scanStoreApps(uniqueApps)

	// Convert map to slice
	var result []AppInfo
	for _, app := range uniqueApps {
		result = append(result, app)
	}

	// Sort by Name
	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})

	return result, nil
}

func scanUninstallKeys(apps map[string]AppInfo) {
	for _, keyLoc := range uninstallKeys {
		k, err := registry.OpenKey(keyLoc.Key, keyLoc.Path, registry.READ)
		if err != nil {
			continue
		}

		subkeys, err := k.ReadSubKeyNames(-1)
		k.Close()
		if err != nil {
			continue
		}

		for _, subkeyName := range subkeys {
			skPath := keyLoc.Path + `\` + subkeyName
			sk, err := registry.OpenKey(keyLoc.Key, skPath, registry.READ)
			if err != nil {
				continue
			}

			name, _, err := sk.GetStringValue("DisplayName")
			sk.Close() // Close early to avoid leaks in loop

			if err != nil || name == "" {
				continue
			}

			// Filter out System Components (Registry flag)
			sysComp, _, errSys := sk.GetIntegerValue("SystemComponent")
			if errSys == nil && sysComp == 1 {
				continue
			}

			// Filter out by Name Blacklist
			if isSystemApp(name) {
				continue
			}

			// Try to find path from InstallLocation or DisplayIcon
			// Re-open to read other values if needed, or read all at once above.
			// simplified for brevity:
			sk, _ = registry.OpenKey(keyLoc.Key, skPath, registry.READ)
			installLoc, _, _ := sk.GetStringValue("InstallLocation")
			displayIcon, _, _ := sk.GetStringValue("DisplayIcon")
			sk.Close()

			exePath := determineExePath(displayIcon, installLoc)

			// Even if exePath is empty, we add the app because the user might want to know it exists.
			// We use the Name as the key if path is missing.
			key := strings.ToLower(exePath)
			if key == "" {
				key = "nopath:" + strings.ToLower(name)
			}

			if _, exists := apps[key]; !exists {
				apps[key] = AppInfo{
					Name:     name,
					Exe:      filepath.Base(exePath),
					FullPath: exePath,
					Source:   "Registry",
				}
			}
		}
	}
}

// scanAppPaths is removed as per user request to match "Installed Apps" list.
// func scanAppPaths(apps map[string]AppInfo) { ... }

func scanStoreApps(apps map[string]AppInfo) {
	// Accessing HKCU Store apps registry
	k, err := registry.OpenKey(registry.CURRENT_USER, storeAppsKey, registry.READ)
	if err != nil {
		return
	}
	defer k.Close()

	subkeys, err := k.ReadSubKeyNames(-1)
	if err != nil {
		return
	}

	for _, subkeyName := range subkeys {
		skPath := storeAppsKey + `\` + subkeyName
		sk, err := registry.OpenKey(registry.CURRENT_USER, skPath, registry.READ)
		if err != nil {
			continue
		}

		displayName, _, _ := sk.GetStringValue("DisplayName")
		packageRoot, _, _ := sk.GetStringValue("PackageRootFolder")
		sk.Close()

		// Store apps often have a DisplayName that is a reference string (e.g., @{Microsoft.App...})
		// Resolving that requires calling a Windows DLL, which is complex.
		// STRICT FILTERING: Use only human-readable names.
		if packageRoot != "" {
			// If DisplayName is cryptic or empty, SKIP IT.
			// We do NOT want to show @{...} or ms-resource:...
			if displayName == "" || strings.HasPrefix(displayName, "@{") || strings.HasPrefix(displayName, "ms-resource:") {
				continue
			}

			// Filter out by Name Blacklist
			if isSystemApp(displayName) {
				continue
			}

			// Find an executable inside the package root
			exePath := findLargestExe(packageRoot)

			if exePath != "" {
				key := strings.ToLower(exePath)
				apps[key] = AppInfo{
					Name:     displayName,
					Exe:      filepath.Base(exePath),
					FullPath: exePath,
					Source:   "Store",
				}
			}
		}
	}
}

// Helpers

func determineExePath(displayIcon, installLocation string) string {
	// 1. Try DisplayIcon (clean it up)
	if displayIcon != "" {
		parts := strings.Split(displayIcon, ",")
		path := strings.Trim(parts[0], `"`)
		if strings.HasSuffix(strings.ToLower(path), ".exe") {
			return path
		}
	}
	// 2. Try InstallLocation scan
	if installLocation != "" {
		path := strings.Trim(installLocation, `"`)
		return findLargestExe(path)
	}
	return ""
}

func findLargestExe(dir string) string {
	var bestExe string
	var maxSize int64

	// Shallow scan (or deep if preferred, but shallow is faster)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".exe") {
			info, err := entry.Info()
			if err == nil {
				if info.Size() > maxSize {
					maxSize = info.Size()
					bestExe = filepath.Join(dir, entry.Name())
				}
			}
		}
	}
	return bestExe
}

// isSystemApp checks if the app name contains keywords indicating it's a system component/driver/runtime.
func isSystemApp(name string) bool {
	lowerName := strings.ToLower(name)
	keywords := []string{
		"runtime",
		"framework",
		"redistributable",
		"middleware",
		"interop",
		"bios",
		"driver",
		"uefi",
		"support",
		"helper",
		"service",
		"updater",
		"installer",
		"language pack",
		"physx",
		"directx",
		"vulkan",
		"opengl",
		"opencl",
	}

	for _, kw := range keywords {
		if strings.Contains(lowerName, kw) {
			return true
		}
	}
	// Specific checks for common noise
	if strings.Contains(lowerName, "windows") && strings.Contains(lowerName, "sdk") {
		return true
	}
	if strings.Contains(lowerName, "microsoft") && strings.Contains(lowerName, "visual c++") {
		return true
	}

	return false
}
