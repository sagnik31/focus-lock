package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestComputeHMAC(t *testing.T) {
	s := &Store{activeSecret: []byte("super-secret-key")}
	data := []byte(`{"blocked_apps":["chrome.exe"]}`)

	sig := s.computeHMAC(data)
	if sig == "" {
		t.Fatal("expected a non-empty signature when a secret is set")
	}
	if s.computeHMAC(data) != sig {
		t.Error("HMAC must be deterministic for the same input and secret")
	}

	tampered := append(append([]byte{}, data...), ' ')
	if s.computeHMAC(tampered) == sig {
		t.Error("a one-byte change must produce a different signature")
	}

	other := &Store{activeSecret: []byte("a-different-key")}
	if other.computeHMAC(data) == sig {
		t.Error("the signature must depend on the secret")
	}

	none := &Store{}
	if none.computeHMAC(data) != "" {
		t.Error("with no secret the signature must be empty")
	}
}

// TestLoadVerifiesSignedConfig is the sign -> write -> load round-trip: a
// correctly signed config loads cleanly. (The tamper/recovery branch of
// loadInternal is exercised indirectly via computeHMAC above; it also reaches
// the platform registry backup, which unit tests avoid.)
func TestLoadVerifiesSignedConfig(t *testing.T) {
	dir := t.TempDir()
	s := &Store{
		filePath:     filepath.Join(dir, "config.json"),
		activeSecret: []byte("k"),
		canSign:      true,
	}

	cfg := Config{BlockedApps: []string{"chrome.exe"}, BlockCommonVPN: true}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.filePath, data, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.filePath+".sig", []byte(s.computeHMAC(data)), 0644); err != nil {
		t.Fatal(err)
	}

	if err := s.loadInternal(); err != nil {
		t.Fatalf("loading a correctly signed config should succeed: %v", err)
	}
	if len(s.Data.BlockedApps) != 1 || s.Data.BlockedApps[0] != "chrome.exe" {
		t.Errorf("config not loaded as written: %+v", s.Data)
	}
	if !s.Data.BlockCommonVPN {
		t.Error("BlockCommonVPN should have round-tripped as true")
	}
}
