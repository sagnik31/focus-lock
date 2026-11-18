package bridge

import (
	"testing"
	"time"

	"focus-lock/backend/storage"
	"focus-lock/backend/sysinfo"
)

// GetTopBlockedApps must reuse a recent installed-apps scan instead of
// rescanning (registry + folders + PowerShell) on every call.
func TestGetTopBlockedAppsUsesCachedScan(t *testing.T) {
	a := &App{Store: &storage.Store{}}
	a.Store.Data.Stats.BlockedDuration = map[string]int64{"fake-app.exe": 60}
	a.appsCache = []sysinfo.AppInfo{{Name: "Fake App", Exe: "fake-app.exe"}}
	a.appsAt = time.Now()

	got, err := a.GetTopBlockedApps()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "Fake App" {
		t.Fatalf("GetTopBlockedApps = %+v, want the cached Fake App (a real scan would not contain it)", got)
	}
}

func TestGetTopBlockedAppsSkipsScanWithoutStats(t *testing.T) {
	a := &App{Store: &storage.Store{}}
	got, err := a.GetTopBlockedApps()
	if err != nil || len(got) != 0 {
		t.Fatalf("GetTopBlockedApps = %v, %v; want empty, nil", got, err)
	}
	if a.appsCache != nil {
		t.Error("no stats: the installed apps should not have been scanned")
	}
}

func TestRankTopAppsMergesDurationAndKills(t *testing.T) {
	duration := map[string]int64{"steam.exe": 3600, "Discord.exe": 60}
	kills := map[string]int{"discord.exe": 9, "chrome.exe": 4, "slack.exe": 4, ".": 99}
	got := rankTopApps(duration, kills)
	want := []string{"steam.exe", "Discord.exe", "chrome.exe", "slack.exe"}
	if len(got) != len(want) {
		t.Fatalf("rankTopApps = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("rankTopApps = %v, want %v", got, want)
		}
	}
}

// Strong mode records only kill counts; top apps must still be suggested.
func TestGetTopBlockedAppsFromKillCountsOnly(t *testing.T) {
	a := &App{Store: &storage.Store{}}
	a.Store.Data.Stats.KillCounts = map[string]int{"Discord.exe": 3, "NordVPN.exe": 5}
	a.appsCache = []sysinfo.AppInfo{{Name: "Discord", Exe: "discord.exe"}}
	a.appsAt = time.Now()

	got, err := a.GetTopBlockedApps()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "Discord" {
		t.Fatalf("GetTopBlockedApps = %+v, want Discord (NordVPN isn't installed, so it is skipped)", got)
	}
}
