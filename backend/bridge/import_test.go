package bridge

import (
	"testing"

	"focus-lock/backend/sysinfo"
)

func TestResolveAppNameIgnoresAppsWithoutExe(t *testing.T) {
	installed := []sysinfo.AppInfo{
		{Name: "Some Suite", Exe: "."}, // older builds' value for "no exe found"
		{Name: "Another Tool", Exe: ""},
		{Name: "Steam", Exe: "steam.exe"},
	}
	cases := map[string]string{
		"slack.exe":        "slack.exe", // not installed: kept as typed, not mapped to "."
		"Steam":            "steam.exe", // display-name match still works
		"steam.exe":        "steam.exe",
		"steam-helper.exe": "steam-helper.exe", // G49: an explicit name is kept
		"st":               "st",               // too short for fuzzy matching
		"":                 "",
	}
	for in, want := range cases {
		if got := resolveAppName(in, installed); got != want {
			t.Errorf("resolveAppName(%q) = %q, want %q", in, got, want)
		}
	}
}
