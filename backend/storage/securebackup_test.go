package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// secureTestStore is a strong-mode signer whose secure copy lives in its own
// temp dir (standing in for %ProgramData%\FocusLock).
func secureTestStore(t *testing.T) *Store {
	t.Helper()
	s := testSigningStore(t)
	s.strong = true
	s.secureDir = t.TempDir()
	return s
}

func secureLockedConfig() Config {
	return Config{
		BlockedApps:  []string{"discord.exe"},
		BlockedSites: []string{"youtube.com"},
		Schedules:    []Schedule{{ID: "s1", Name: "Work", Days: []string{"Mon"}, StartTime: "09:00", EndTime: "17:00", Enabled: true}},
		LockEndTime:  time.Now().Add(time.Hour).UTC().Truncate(time.Second),
	}
}

// G33: the first strong-mode load adopts the reduced-mode config once.
func TestAdoptLegacyConfigOnce(t *testing.T) {
	old := testSigningStore(t)
	old.Data = secureLockedConfig()
	if err := old.Save(); err != nil {
		t.Fatal(err)
	}
	s := &Store{filePath: old.filePath, activeSecret: []byte("hklm"), strong: true, canSign: true,
		secureDir: t.TempDir(), legacySecret: []byte("test-secret")}
	if got := reload(t, s); len(got.BlockedApps) != 1 || got.LockEndTime.IsZero() {
		t.Fatalf("reduced-mode config not adopted: %+v", got)
	}
	if s.legacySecret != nil {
		t.Error("the legacy secret must not be accepted after the first save")
	}
	if got := reload(t, s); len(got.BlockedApps) != 1 {
		t.Errorf("config not re-signed: %+v", got)
	}
}

func TestLegacyConfigRejectedWithoutMigration(t *testing.T) {
	old := testSigningStore(t)
	old.Data = secureLockedConfig()
	if err := old.Save(); err != nil {
		t.Fatal(err)
	}
	s := &Store{filePath: old.filePath, activeSecret: []byte("hklm"), strong: true, canSign: true}
	if err := s.Load(); err == nil {
		t.Fatal("a config signed with another secret was accepted")
	}
	if _, err := os.Stat(old.filePath + ".rejected"); err != nil {
		t.Error("rejected config not kept")
	}
}

// G36: once the secure copy is written the save is committed.
func TestSaveCommittedBySecureCopy(t *testing.T) {
	s := secureTestStore(t)
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	// A folder in place of config.json makes its replacement fail.
	if err := os.Remove(s.filePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(s.filePath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyRequest(Operation{Type: OpStartFocus, Seconds: 3600}, time.Now()); err != nil {
		t.Fatalf("committed save reported as failed: %v", err)
	}
	if got := reload(t, s); got.LockEndTime.IsZero() {
		t.Error("lock lost")
	}
}

func reload(t *testing.T, s *Store) Config {
	t.Helper()
	s.Data = Config{}
	if err := s.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	return s.Data
}

// G10: deleting every user-writable file (config, sig, .bak, .bak.sig) must
// not lose the block lists or schedules: the secure copy restores them and the
// primary is rewritten.
func TestSecureCopySurvivesDeletedUserFiles(t *testing.T) {
	s := secureTestStore(t)
	want := secureLockedConfig()
	s.Data = want
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"", ".sig", ".bak", ".bak.sig"} {
		if err := os.Remove(s.filePath + suffix); err != nil {
			t.Fatal(err)
		}
	}

	got := reload(t, s)
	if len(got.BlockedApps) != 1 || len(got.BlockedSites) != 1 || len(got.Schedules) != 1 {
		t.Fatalf("lists not restored: %+v", got)
	}
	if !got.LockEndTime.Equal(want.LockEndTime) {
		t.Errorf("LockEndTime = %v", got.LockEndTime)
	}
	if _, err := os.Stat(s.filePath); err != nil {
		t.Errorf("primary not healed: %v", err)
	}
}

// G10: putting back an older, validly signed config.json (an empty list, no
// lock) must not win over the newer secure copy, and the primary is healed.
func TestSecureCopyBeatsReplayedOlderPrimary(t *testing.T) {
	s := secureTestStore(t)
	if err := s.Save(); err != nil { // old state: nothing blocked, unlocked
		t.Fatal(err)
	}
	oldData, _ := os.ReadFile(s.filePath)
	oldSig, _ := os.ReadFile(s.filePath + ".sig")

	s.Data = secureLockedConfig()
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	newData, _ := os.ReadFile(s.filePath)

	// Replay the old signed pair over the primary and the .bak.
	for _, p := range []string{s.filePath, s.filePath + ".bak"} {
		_ = os.WriteFile(p, oldData, 0o644)
		_ = os.WriteFile(p+".sig", oldSig, 0o644)
	}

	got := reload(t, s)
	if len(got.BlockedSites) != 1 || got.LockEndTime.IsZero() {
		t.Fatalf("older config won: %+v", got)
	}
	healed, _ := os.ReadFile(s.filePath)
	if string(healed) != string(newData) {
		t.Error("primary was not rewritten from the secure copy")
	}
}

// A secure copy whose signature doesn't verify is ignored; the primary is used.
func TestTamperedSecureCopyFallsBackToPrimary(t *testing.T) {
	s := secureTestStore(t)
	s.Data = secureLockedConfig()
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	sp := s.securePath()
	_ = os.WriteFile(sp, []byte(`{"blocked_sites":[]}`), 0o644)

	got := reload(t, s)
	if len(got.BlockedSites) != 1 {
		t.Fatalf("tampered secure copy was trusted: %+v", got)
	}
}

// Without a secure dir (reduced mode, or the folder couldn't be secured)
// nothing is written outside %APPDATA% and loading works as before.
func TestNoSecureDirKeepsOldBehaviour(t *testing.T) {
	s := testSigningStore(t)
	s.Data = secureLockedConfig()
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	if got := reload(t, s); len(got.BlockedSites) != 1 {
		t.Fatalf("got %+v", got)
	}
}

// The folder ACL must parse and grant only SYSTEM and Administrators, with
// inheritance from ProgramData switched off.
func TestSecureDirSDDL(t *testing.T) {
	sd, err := windows.SecurityDescriptorFromString(secureDirSDDL)
	if err != nil {
		t.Fatal(err)
	}
	ctl, _, err := sd.Control()
	if err != nil {
		t.Fatal(err)
	}
	if ctl&windows.SE_DACL_PROTECTED == 0 {
		t.Error("DACL is not protected")
	}
	str := sd.String()
	for _, want := range []string{"O:BA", "(A;OICI;FA;;;SY)", "(A;OICI;FA;;;BA)"} {
		if !strings.Contains(str, want) {
			t.Errorf("SDDL %q missing %q", str, want)
		}
	}
	if strings.Count(str, "(A;") != 2 {
		t.Errorf("expected exactly two allow ACEs, got %q", str)
	}
}

// protectDir must apply cleanly to a real folder (as a non-admin test run it
// may lack the right to set the owner; skip rather than fail then).
func TestProtectDirAppliesACL(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "FocusLock")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := protectDir(dir); err != nil {
		t.Skipf("protectDir needs admin rights here: %v", err)
	}
	sd, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	if str := sd.String(); strings.Contains(str, ";;;BU)") || strings.Contains(str, ";;;AU)") {
		t.Errorf("users still have access: %q", str)
	}
}
