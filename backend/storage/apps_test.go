package storage

import (
	"testing"
	"time"
)

func TestValidAppName(t *testing.T) {
	valid := []string{"discord.exe", "Steam", "qq.exe", " chrome.exe ", "Mullvad VPN.exe", "微信.exe"}
	invalid := []string{"", " ", ".", ".exe", "..", "-.exe", `C:\x\y.exe`, "a/b.exe"}
	for _, n := range valid {
		if !ValidAppName(n) {
			t.Errorf("ValidAppName(%q) = false, want true", n)
		}
	}
	for _, n := range invalid {
		if ValidAppName(n) {
			t.Errorf("ValidAppName(%q) = true, want false", n)
		}
	}
}

func TestPolicyRejectsJunkAppNames(t *testing.T) {
	now := time.Now()

	// AddApp refuses "." outright.
	if err := ApplyOperation(idleConfig(), Operation{Type: OpAddApp, Name: "."}, now); err == nil {
		t.Error(`AddApp "." should be refused`)
	}

	// SetApps drops junk, and dropping a stored junk entry mid-lock is allowed
	// (it never blocked anything), while real apps still can't be removed.
	cfg := lockedConfig(now)
	cfg.BlockedApps = append(cfg.BlockedApps, ".")
	if err := ApplyOperation(cfg, Operation{Type: OpSetApps, Apps: []string{"chrome.exe", "steam.exe", "."}}, now); err != nil {
		t.Fatalf("SetApps keeping the real apps should be allowed mid-lock: %v", err)
	}
	for _, a := range cfg.BlockedApps {
		if a == "." {
			t.Errorf(`SetApps kept "." in %v`, cfg.BlockedApps)
		}
	}
	if err := ApplyOperation(cfg, Operation{Type: OpSetApps, Apps: []string{"chrome.exe", "."}}, now); err == nil {
		t.Error("SetApps dropping steam.exe mid-lock should still be refused")
	}

	// Import skips junk.
	cfg = idleConfig()
	if err := ApplyOperation(cfg, Operation{Type: OpImport, Apps: []string{".", "", "slack.exe"}}, now); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"chrome.exe": true, "slack.exe": true}
	if len(cfg.BlockedApps) != len(want) {
		t.Errorf("after import BlockedApps = %v, want only %v", cfg.BlockedApps, want)
	}
	for _, a := range cfg.BlockedApps {
		if !want[a] {
			t.Errorf("unexpected blocked app %q after import", a)
		}
	}
}
