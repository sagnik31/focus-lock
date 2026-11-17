package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const testVer = "2026.11.17.1"

var setupBytes = []byte("pretend this is FocusLock-Setup.exe\x00\x01\x02")

func sha(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// fakeGitHub serves a "latest release" API response plus its assets.
type fakeGitHub struct {
	srv       *httptest.Server
	release   map[string]any
	sums      string
	setup     []byte
	status    int // for the API endpoint; 0 = 200
	rateLimit bool
	mu        sync.Mutex
	hits      map[string]int
	userAgent string
}

func newFake(t *testing.T) *fakeGitHub {
	t.Helper()
	f := &fakeGitHub{setup: setupBytes, hits: map[string]int{}}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	name := SetupAssetName(testVer)
	f.sums = sha(setupBytes) + "  " + name + "\n" + sha(setupBytes) + "  FocusLock-Setup.exe\n"
	f.release = map[string]any{
		"tag_name":     "v" + testVer,
		"name":         "Focus Lock " + testVer,
		"body":         "What's changed",
		"published_at": "2026-11-17T02:10:00Z",
		"assets": []map[string]any{
			{"name": name, "browser_download_url": f.srv.URL + "/dl/" + name, "size": len(setupBytes)},
			{"name": "FocusLock-Setup.exe", "browser_download_url": f.srv.URL + "/dl/FocusLock-Setup.exe", "size": len(setupBytes)},
			{"name": ChecksumsAsset, "browser_download_url": f.srv.URL + "/dl/" + ChecksumsAsset, "size": 10},
		},
	}
	return f
}

func (f *fakeGitHub) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.hits[r.URL.Path]++
	f.userAgent = r.Header.Get("User-Agent")
	f.mu.Unlock()
	switch {
	case r.URL.Path == "/latest":
		if f.rateLimit {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if f.status != 0 {
			w.WriteHeader(f.status)
			return
		}
		_ = json.NewEncoder(w).Encode(f.release)
	case r.URL.Path == "/dl/"+ChecksumsAsset:
		_, _ = w.Write([]byte(f.sums))
	case strings.HasPrefix(r.URL.Path, "/dl/FocusLock-Setup"):
		_, _ = w.Write(f.setup)
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeGitHub) hitCount(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hits[path]
}

func (f *fakeGitHub) client(t *testing.T) *Client {
	c := New(t.TempDir())
	c.HTTP = f.srv.Client()
	c.HTTP.Timeout = 5 * time.Second
	c.APIURL = f.srv.URL + "/latest"
	return c
}

func TestLatestAndCompare(t *testing.T) {
	f := newFake(t)
	c := f.client(t)
	rel, err := c.Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if f.userAgent != UserAgent {
		t.Fatalf("User-Agent = %q", f.userAgent)
	}
	if rel.Version() != testVer {
		t.Fatalf("Version = %q", rel.Version())
	}
	if res := Compare("2026.11.10.0", rel); !res.Available || res.Latest != testVer || res.Notes != "What's changed" {
		t.Fatalf("older current: %+v", res)
	}
	if res := Compare(testVer, rel); res.Available {
		t.Fatal("same version offered as update")
	}
	if res := Compare("2026.12.1.0", rel); res.Available {
		t.Fatal("older release offered as update")
	}
	if res := Compare("dev", rel); res.Available {
		t.Fatal("dev build offered an update")
	}
	rel.Prerelease = true
	if res := Compare("2026.11.10.0", rel); res.Available {
		t.Fatal("prerelease offered")
	}
}

func TestLatestErrors(t *testing.T) {
	f := newFake(t)
	c := f.client(t)

	f.rateLimit = true
	if _, err := c.Latest(context.Background()); err == nil || !strings.Contains(err.Error(), "rate limit") {
		t.Fatalf("rate limit: %v", err)
	}
	f.rateLimit = false

	f.status = http.StatusNotFound
	if _, err := c.Latest(context.Background()); err == nil || !strings.Contains(err.Error(), "no release") {
		t.Fatalf("404: %v", err)
	}
	f.status = http.StatusInternalServerError
	if _, err := c.Latest(context.Background()); err == nil {
		t.Fatal("500 accepted")
	}
	f.status = 0

	f.release["tag_name"] = "nightly"
	if _, err := c.Latest(context.Background()); err == nil {
		t.Fatal("non-calendar tag accepted")
	}

	// Offline: nothing listening.
	c.APIURL = "https://127.0.0.1:1/latest"
	if _, err := c.Latest(context.Background()); err == nil {
		t.Fatal("offline accepted")
	}
	// Plain HTTP is refused before any request.
	c.APIURL = strings.Replace(f.srv.URL, "https://", "http://", 1) + "/latest"
	if _, err := c.Latest(context.Background()); err == nil || !strings.Contains(err.Error(), "non-HTTPS") {
		t.Fatalf("http: %v", err)
	}
}

func TestParseRelease(t *testing.T) {
	if _, err := ParseRelease([]byte("not json")); err == nil {
		t.Fatal("bad JSON accepted")
	}
	if _, err := ParseRelease([]byte(`{"tag_name":""}`)); err == nil {
		t.Fatal("empty tag accepted")
	}
	rel, err := ParseRelease([]byte(`{"tag_name":"v2026.1.2.0","assets":[{"name":"SHA256SUMS.txt"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rel.SetupAsset(); err == nil {
		t.Fatal("missing setup asset not reported")
	}
	if a, err := rel.Asset(ChecksumsAsset); err != nil || a.Name != ChecksumsAsset {
		t.Fatalf("Asset: %v %v", a, err)
	}
}

func TestParseChecksums(t *testing.T) {
	h := sha(setupBytes)
	data := []byte("# comment\n" + strings.ToUpper(h) + " *FocusLock-Setup-1.exe\r\n" + h + "  FocusLock-Setup.exe\n")
	if got, err := ParseChecksums(data, "FocusLock-Setup-1.exe"); err != nil || got != h {
		t.Fatalf("binary-mode line: %q %v", got, err)
	}
	if got, err := ParseChecksums(data, "FocusLock-Setup.exe"); err != nil || got != h {
		t.Fatalf("text-mode line: %q %v", got, err)
	}
	if _, err := ParseChecksums(data, "other.exe"); err == nil {
		t.Fatal("missing entry accepted")
	}
	if _, err := ParseChecksums([]byte("abcd  x.exe\n"), "x.exe"); err == nil {
		t.Fatal("short hash accepted")
	}
}

func TestDownloadVerifiesAndCleans(t *testing.T) {
	f := newFake(t)
	c := f.client(t)
	// A stale installer and partial download from an older release.
	_ = os.WriteFile(filepath.Join(c.Dir, "FocusLock-Setup-2026.11.10.0.exe"), []byte("old"), 0o644)
	_ = os.WriteFile(filepath.Join(c.Dir, "FocusLock-Setup-2026.11.10.0.exe.part"), []byte("old"), 0o644)
	_ = os.WriteFile(filepath.Join(c.Dir, lastCheckFile), []byte("{}"), 0o644)

	rel, err := c.Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var lastDone, lastTotal int64
	path, sum, err := c.Download(context.Background(), rel, func(d, tot int64) { lastDone, lastTotal = d, tot })
	if err != nil {
		t.Fatal(err)
	}
	if sum != sha(setupBytes) || filepath.Base(path) != SetupAssetName(testVer) {
		t.Fatalf("path=%s sum=%s", path, sum)
	}
	if lastDone != int64(len(setupBytes)) || lastTotal != int64(len(setupBytes)) {
		t.Fatalf("progress %d/%d", lastDone, lastTotal)
	}
	if err := VerifyFile(path, sum); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(c.Dir)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != 2 { // the new setup + last_check.json
		t.Fatalf("old downloads not cleaned: %v", names)
	}

	// A second download reuses the verified file.
	before := f.hitCount("/dl/" + SetupAssetName(testVer))
	if _, _, err := c.Download(context.Background(), rel, nil); err != nil {
		t.Fatal(err)
	}
	if f.hitCount("/dl/"+SetupAssetName(testVer)) != before {
		t.Fatal("verified download was fetched again")
	}

	// Tampering after download is caught by VerifyFile (InstallUpdate re-checks).
	_ = os.WriteFile(path, []byte("tampered"), 0o644)
	if err := VerifyFile(path, sum); !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("tampered file: %v", err)
	}
}

func TestDownloadChecksumMismatch(t *testing.T) {
	f := newFake(t)
	f.setup = []byte("a different (tampered) installer")
	c := f.client(t)
	rel, err := c.Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.Download(context.Background(), rel, nil); !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("want checksum mismatch, got %v", err)
	}
	entries, _ := os.ReadDir(c.Dir)
	if len(entries) != 0 {
		t.Fatalf("mismatching download left behind: %v", entries)
	}
}

func TestDownloadMissingChecksumEntry(t *testing.T) {
	f := newFake(t)
	f.sums = sha(setupBytes) + "  something-else.exe\n"
	c := f.client(t)
	rel, err := c.Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.Download(context.Background(), rel, nil); err == nil {
		t.Fatal("download without a checksum accepted")
	}
	if f.hitCount("/dl/"+SetupAssetName(testVer)) != 0 {
		t.Fatal("installer fetched before its checksum was known")
	}
}

func TestLastCheck(t *testing.T) {
	c := New(t.TempDir())
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	lc, ok := c.LoadLastCheck()
	if ok || !CheckDue(lc, ok, now) {
		t.Fatal("no record should be due")
	}
	want := LastCheck{CheckedAt: now, Result: CheckResult{Available: true, Latest: testVer}}
	if err := c.SaveLastCheck(want); err != nil {
		t.Fatal(err)
	}
	got, ok := c.LoadLastCheck()
	if !ok || !got.CheckedAt.Equal(now) || got.Result.Latest != testVer || !got.Result.Available {
		t.Fatalf("round trip: %+v %v", got, ok)
	}
	if CheckDue(got, true, now.Add(23*time.Hour)) {
		t.Fatal("due before 24h")
	}
	if !CheckDue(got, true, now.Add(24*time.Hour)) {
		t.Fatal("not due after 24h")
	}
	if !CheckDue(got, true, now.Add(-time.Hour)) {
		t.Fatal("future record should be due")
	}
}
