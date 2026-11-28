//go:build windows

package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func writeSetup(t *testing.T, dir string) (path, sum string) {
	t.Helper()
	path = filepath.Join(dir, "FocusLock-Setup-2026.11.29.0.exe")
	data := []byte("setup bytes")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(data)
	return path, hex.EncodeToString(h[:])
}

// G24: while the verified installer is open it can't be changed, renamed or
// deleted, and neither can its folder.
func TestOpenVerifiedLocksTheFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "update")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path, sum := writeSetup(t, dir)
	f, resolved, err := openVerified(path, sum)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(resolved, path) {
		t.Errorf("resolved %q, want %q", resolved, path)
	}
	if err := os.WriteFile(path, []byte("evil"), 0o644); err == nil {
		t.Error("the locked installer was overwritten")
	}
	if err := os.Rename(path, path+".old"); err == nil {
		t.Error("the locked installer was renamed")
	}
	if err := os.Remove(path); err == nil {
		t.Error("the locked installer was deleted")
	}
	if err := os.Rename(dir, dir+"2"); err == nil {
		t.Error("the installer's folder was renamed")
	}
	f.Close()
	if err := os.WriteFile(path, []byte("ok"), 0o644); err != nil {
		t.Errorf("the file stayed locked after close: %v", err)
	}
}

func TestOpenVerifiedRefusesWrongChecksum(t *testing.T) {
	path, _ := writeSetup(t, t.TempDir())
	if _, _, err := openVerified(path, strings.Repeat("0", 64)); !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("got %v, want ErrChecksumMismatch", err)
	}
	// Not left locked.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
}

// A file someone else holds open for writing is refused, not verified.
func TestOpenVerifiedRefusesFileOpenForWriting(t *testing.T) {
	path, sum := writeSetup(t, t.TempDir())
	w, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if _, _, err := openVerified(path, sum); !errors.Is(err, ErrInstallerUnreadable) {
		t.Fatalf("got %v, want ErrInstallerUnreadable", err)
	}
}

// The launched path has no links left in it.
func TestOpenVerifiedResolvesJunctions(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "target")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
		t.Skipf("can't create a junction: %v %s", err, out)
	}
	path, sum := writeSetup(t, target)
	f, resolved, err := openVerified(filepath.Join(link, filepath.Base(path)), sum)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	want, _ := filepath.EvalSymlinks(path)
	if !strings.EqualFold(resolved, want) && !strings.EqualFold(resolved, path) {
		t.Errorf("resolved %q, want %q", resolved, path)
	}
	if strings.Contains(strings.ToLower(resolved), `\link\`) {
		t.Errorf("resolved path still goes through the junction: %q", resolved)
	}
}

func TestTrimLongPathPrefix(t *testing.T) {
	for in, want := range map[string]string{
		`\\?\C:\Users\x\a.exe`:    `C:\Users\x\a.exe`,
		`\\?\UNC\srv\share\a.exe`: `\\srv\share\a.exe`,
		`C:\a.exe`:                `C:\a.exe`,
	} {
		if got := trimLongPathPrefix(in); got != want {
			t.Errorf("trimLongPathPrefix(%q) = %q, want %q", in, got, want)
		}
	}
}
