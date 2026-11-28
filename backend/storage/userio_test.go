//go:build windows

package storage

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"

	"focus-lock/backend/userfs"
)

func enableUserfs(t *testing.T) {
	t.Helper()
	if err := userfs.Enable(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(userfs.Disable)
}

// With the enforcer's restricted user token, saving, loading and the request
// channel all still work in the user's own folder.
func TestStoreWorksWithUserRights(t *testing.T) {
	enableUserfs(t)
	s := testSigningStore(t)
	s.Data.BlockedSites = []string{"example.com"}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	s.Data.BlockedSites = nil
	if err := s.Load(); err != nil {
		t.Fatal(err)
	}
	if len(s.Data.BlockedSites) != 1 {
		t.Fatalf("loaded %v", s.Data.BlockedSites)
	}

	dir := s.RequestDir()
	if err := writeJSONAtomic(requestPath(dir), Request{ID: "r1"}); err != nil {
		t.Fatal(err)
	}
	req, ok := TakePendingRequest(dir)
	if !ok || req.ID != "r1" {
		t.Fatalf("took %+v, %v", req, ok)
	}
	WriteResult(dir, Result{ID: "r1", OK: true})
	if res, ok := readResult(dir); !ok || res.ID != "r1" {
		t.Fatalf("result %+v, %v", res, ok)
	}
}

// G25: a junction planted in place of the config folder can't make the
// enforcer write where only administrators may (meaningful when the test runs
// elevated, as on CI; unelevated it is refused anyway).
func TestJunctionedConfigFolderIsNotWritten(t *testing.T) {
	enableUserfs(t)
	win, err := windows.GetWindowsDirectory()
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "FocusLock")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, win).CombinedOutput(); err != nil {
		t.Skipf("can't create a junction: %v %s", err, out)
	}
	defer os.Remove(link)

	s := &Store{filePath: filepath.Join(link, "focuslock-g25-test.json"), activeSecret: []byte("x"), canSign: true}
	err = s.Save()
	target := filepath.Join(win, "focuslock-g25-test.json")
	if _, statErr := os.Stat(target); statErr == nil {
		_ = os.Remove(target)
		_ = os.Remove(target + ".sig")
		t.Fatal("the save went through the junction into the Windows folder")
	}
	if err == nil {
		t.Error("the save reported success")
	}
	WriteResult(link, Result{ID: "x"})
	if _, statErr := os.Stat(filepath.Join(win, "result.json")); statErr == nil {
		t.Error("a result was written through the junction")
	}
}
