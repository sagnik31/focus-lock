package updater

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const olderVer = "2026.11.10.0"

func TestPrepareApplyDownloadsVerifiedSetup(t *testing.T) {
	f := newFake(t)
	c := f.client(t)
	path, err := c.PrepareApply(context.Background(), olderVer, "")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != c.Dir || filepath.Base(path) != SetupAssetName(testVer) {
		t.Fatalf("setup at %s, want it in %s", path, c.Dir)
	}
	if err := VerifyFile(path, sha(setupBytes)); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareApplyReusesUserDownload(t *testing.T) {
	f := newFake(t)
	c := f.client(t)
	userDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(userDir, SetupAssetName(testVer)), setupBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	path, err := c.PrepareApply(context.Background(), olderVer, userDir)
	if err != nil {
		t.Fatal(err)
	}
	if f.hitCount("/dl/"+SetupAssetName(testVer)) != 0 {
		t.Fatal("matching user download was fetched again")
	}
	if filepath.Dir(path) != c.Dir {
		t.Fatalf("setup must run from the secure folder, got %s", path)
	}
}

func TestPrepareApplyIgnoresTamperedUserDownload(t *testing.T) {
	f := newFake(t)
	c := f.client(t)
	userDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(userDir, SetupAssetName(testVer)), []byte("planted"), 0o644); err != nil {
		t.Fatal(err)
	}
	path, err := c.PrepareApply(context.Background(), olderVer, userDir)
	if err != nil {
		t.Fatal(err)
	}
	if f.hitCount("/dl/"+SetupAssetName(testVer)) != 1 {
		t.Fatal("tampered user download was not replaced by a fresh download")
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, setupBytes) {
		t.Fatal("the planted file was kept")
	}
}

func TestPrepareApplyRefusals(t *testing.T) {
	cases := []struct {
		name    string
		current string
		setup   func(f *fakeGitHub)
		want    int
	}{
		{"same version", testVer, nil, ApplyNotNewer},
		{"downgrade", "2026.12.1.0", nil, ApplyNotNewer},
		{"dev build", "dev", nil, ApplyDevBuild},
		{"bad signature", olderVer, func(f *fakeGitHub) { f.badSig = true }, ApplyVerifyFailed},
		{"tampered release", olderVer, func(f *fakeGitHub) { f.setup = []byte("other") }, ApplyVerifyFailed},
		{"offline", olderVer, func(f *fakeGitHub) { f.status = 500 }, ApplyNetwork},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFake(t)
			if tc.setup != nil {
				tc.setup(f)
			}
			c := f.client(t)
			_, err := c.PrepareApply(context.Background(), tc.current, "")
			if got := ApplyCode(err); got != tc.want {
				t.Fatalf("code %d (%v), want %d", got, err, tc.want)
			}
			if ApplyExitMessage(tc.want) == "" {
				t.Fatal("refusal has no message")
			}
		})
	}
}

func TestApplyExitMessage(t *testing.T) {
	if ApplyExitMessage(ApplyStarted) != "" {
		t.Fatal("success should have no message")
	}
	if ApplyExitMessage(99) == "" || ApplyExitMessage(1) == "" {
		t.Fatal("unknown codes need a message")
	}
}

func TestParseTaskStatus(t *testing.T) {
	st, err := ParseTaskStatus("Ready|22|2026-11-20T10:15:30.0000000Z")
	if err != nil {
		t.Fatal(err)
	}
	if st.Running || st.LastResult != 22 || !st.LastRun.Equal(time.Date(2026, 11, 20, 10, 15, 30, 0, time.UTC)) {
		t.Fatalf("parsed %+v", st)
	}
	if st, err := ParseTaskStatus("Running|267009|2026-11-20T10:15:30Z"); err != nil || !st.Running {
		t.Fatalf("running: %+v %v", st, err)
	}
	for _, bad := range []string{"", "Ready|x|2026-11-20T10:15:30Z", "Ready|0|11/30/1999", "Ready|0"} {
		if _, err := ParseTaskStatus(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
