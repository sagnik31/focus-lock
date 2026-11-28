// Package updater checks GitHub Releases for a newer Focus Lock, downloads the
// release's installer and verifies it against the published SHA-256 checksum,
// whose list is signed with the release key (signature.go).
//
// The existing installer is the updater: once downloaded and verified, Setup
// is started elevated, either by the "FocusLock Updater" task without a UAC
// prompt (apply.go) or, without that task, with one prompt (Launch), and does
// the usual stop
// enforcer -> replace exe -> re-register task -> relaunch. Nothing here
// replaces the running exe itself.
//
// Whether installing is allowed right now (no focus session) is decided by the
// caller (bridge.InstallUpdate) and again by the installer; this package only
// fetches and verifies.
package updater

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"focus-lock/backend/version"
)

const (
	// LatestReleaseURL is the GitHub API endpoint for the newest published
	// (non-draft, non-prerelease) release.
	LatestReleaseURL = "https://api.github.com/repos/sagnik31/focus-lock/releases/latest"

	// UserAgent is sent with every request (GitHub requires one). It is
	// deliberately generic and carries no user or machine data.
	UserAgent = "focus-lock-updater"

	// ChecksumsAsset is the checksum file published with every release.
	ChecksumsAsset = "SHA256SUMS.txt"

	// CheckInterval is how often the quiet start-up check may hit the network.
	CheckInterval = 24 * time.Hour

	maxReleaseJSON = 1 << 20   // 1 MiB
	maxChecksums   = 64 << 10  // 64 KiB
	maxSetupBytes  = 512 << 20 // sanity cap on the installer download
	lastCheckFile  = "last_check.json"
	partialSuffix  = ".part"
)

// SetupAssetName is the versioned installer file name for ver ("YYYY.M.D.N").
func SetupAssetName(ver string) string {
	return "FocusLock-Setup-" + ver + ".exe"
}

// Asset is one file attached to a release.
type Asset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
	Size        int64  `json:"size"`
}

// Release is the subset of the GitHub release object the updater uses.
type Release struct {
	TagName     string  `json:"tag_name"`
	Name        string  `json:"name"`
	Body        string  `json:"body"`
	PublishedAt string  `json:"published_at"`
	Draft       bool    `json:"draft"`
	Prerelease  bool    `json:"prerelease"`
	Assets      []Asset `json:"assets"`
}

// ParseRelease decodes a GitHub release JSON object and checks that its tag
// is a valid calendar version.
func ParseRelease(data []byte) (*Release, error) {
	var r Release
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("unexpected response from GitHub: %w", err)
	}
	if r.TagName == "" {
		return nil, errors.New("the latest release has no version tag")
	}
	if version.IsDev(r.TagName) {
		return nil, fmt.Errorf("the latest release has an unrecognised version %q", r.TagName)
	}
	return &r, nil
}

// Version is the release's bare version ("2026.11.17.0" for tag v2026.11.17.0).
func (r *Release) Version() string {
	return strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(r.TagName), "v"), "V")
}

// Asset returns the release asset with the given name.
func (r *Release) Asset(name string) (Asset, error) {
	for _, a := range r.Assets {
		if a.Name == name {
			return a, nil
		}
	}
	return Asset{}, fmt.Errorf("the release is missing %s", name)
}

// SetupAsset returns the versioned installer asset of the release.
func (r *Release) SetupAsset() (Asset, error) {
	return r.Asset(SetupAssetName(r.Version()))
}

// CheckResult is the outcome of comparing a release with the running version.
type CheckResult struct {
	Available   bool   `json:"available"`
	Latest      string `json:"latest"`
	Current     string `json:"current"`
	Notes       string `json:"notes"`
	PublishedAt string `json:"publishedAt"`
}

// Compare reports whether rel is newer than current. Drafts and pre-releases
// are never offered (GitHub's /latest already excludes them).
func Compare(current string, rel *Release) CheckResult {
	res := CheckResult{
		Latest:      rel.Version(),
		Current:     current,
		Notes:       rel.Body,
		PublishedAt: rel.PublishedAt,
	}
	res.Available = !rel.Draft && !rel.Prerelease && !version.IsDev(current) &&
		version.Compare(rel.Version(), current) > 0
	return res
}

// ParseChecksums finds the SHA-256 (lowercase hex) for name in a sha256sum-
// style file ("<hex>  <name>" per line; a leading '*' on the name, binary
// mode, is accepted).
func ParseChecksums(data []byte, name string) (string, error) {
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) != 2 {
			continue
		}
		if strings.TrimPrefix(fields[1], "*") != name {
			continue
		}
		sum := strings.ToLower(fields[0])
		if b, err := hex.DecodeString(sum); err != nil || len(b) != sha256.Size {
			return "", fmt.Errorf("%s has an invalid checksum for %s", ChecksumsAsset, name)
		}
		return sum, nil
	}
	return "", fmt.Errorf("%s has no checksum for %s", ChecksumsAsset, name)
}

// FileSHA256 returns the lowercase hex SHA-256 of a file.
func FileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ErrChecksumMismatch means a file does not match its published checksum.
var ErrChecksumMismatch = errors.New("the downloaded installer does not match its published checksum")

// ErrInstallerUnreadable means the downloaded installer could not be opened
// locked, read or resolved, for example because another program has it open
// for writing.
var ErrInstallerUnreadable = errors.New("the downloaded installer could not be read")

// VerifyFile checks path against the expected lowercase hex SHA-256.
func VerifyFile(path, wantSHA256 string) error {
	got, err := FileSHA256(path)
	if err != nil {
		return err
	}
	if !strings.EqualFold(got, wantSHA256) {
		return ErrChecksumMismatch
	}
	return nil
}

// Client talks to GitHub and manages the download folder.
type Client struct {
	HTTP *http.Client

	// StallTimeout ends an installer download that receives no data for this long.
	StallTimeout time.Duration
	APIURL       string            // the "latest release" endpoint
	Dir          string            // download + bookkeeping folder
	PublicKey    ed25519.PublicKey // release signing key (SHA256SUMS.txt.sig)
}

// New returns a Client for the real GitHub endpoint storing files in dir.
func New(dir string) *Client {
	return &Client{
		HTTP: &http.Client{Timeout: 10 * time.Second},

		StallTimeout: 60 * time.Second,
		APIURL:       LatestReleaseURL,
		Dir:          dir,
		PublicKey:    ReleasePublicKey(),
	}
}

// DefaultDir is %APPDATA%\FocusLock\update (per-user).
func DefaultDir() (string, error) {
	cfg, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cfg, "FocusLock", "update"), nil
}

func (c *Client) get(ctx context.Context, rawURL string, client *http.Client) (*http.Response, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" {
		return nil, fmt.Errorf("refusing non-HTTPS update URL %q", rawURL)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not reach GitHub (are you offline?): %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, statusError(resp)
	}
	return resp, nil
}

func statusError(resp *http.Response) error {
	switch {
	case (resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests) &&
		resp.Header.Get("X-RateLimit-Remaining") == "0":
		return errors.New("GitHub's rate limit was reached; try again in an hour")
	case resp.StatusCode == http.StatusNotFound:
		return errors.New("no release has been published yet")
	default:
		return fmt.Errorf("GitHub returned %s", resp.Status)
	}
}

// Latest fetches and parses the newest published release.
func (c *Client) Latest(ctx context.Context) (*Release, error) {
	resp, err := c.get(ctx, c.APIURL, c.HTTP)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxReleaseJSON))
	if err != nil {
		return nil, fmt.Errorf("reading the release info failed: %w", err)
	}
	return ParseRelease(data)
}

// ExpectedSHA256 downloads the release's SHA256SUMS.txt and its signature,
// checks the signature against the release key built into the app, and
// returns the checksum of the versioned installer. An unsigned or wrongly
// signed release is refused (ErrBadSignature).
func (c *Client) ExpectedSHA256(ctx context.Context, rel *Release) (string, error) {
	setup, err := rel.SetupAsset()
	if err != nil {
		return "", err
	}
	sums, err := rel.Asset(ChecksumsAsset)
	if err != nil {
		return "", err
	}
	sigAsset, err := rel.Asset(SignatureAsset)
	if err != nil {
		return "", ErrBadSignature
	}
	data, err := c.fetchSmall(ctx, sums.DownloadURL, ChecksumsAsset)
	if err != nil {
		return "", err
	}
	sig, err := c.fetchSmall(ctx, sigAsset.DownloadURL, SignatureAsset)
	if err != nil {
		return "", err
	}
	if err := VerifySignature(data, sig, c.PublicKey); err != nil {
		return "", err
	}
	return ParseChecksums(data, setup.Name)
}

// fetchSmall downloads a small release asset (checksums, signature).
func (c *Client) fetchSmall(ctx context.Context, rawURL, name string) ([]byte, error) {
	resp, err := c.get(ctx, rawURL, c.HTTP)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxChecksums))
	if err != nil {
		return nil, fmt.Errorf("downloading %s failed: %w", name, err)
	}
	return data, nil
}

// ErrDownloadStalled means the download stopped receiving data.
var ErrDownloadStalled = errors.New("the download stalled; check your connection and try again")

// Progress receives download progress; total is <= 0 when unknown.
type Progress func(done, total int64)

// Download fetches the release's installer into Dir, verifying it against
// SHA256SUMS.txt. It returns the installer's path and checksum. A previously
// downloaded file that already matches is reused; a mismatching download is
// deleted. Older downloaded installers are removed on success.
func (c *Client) Download(ctx context.Context, rel *Release, progress Progress) (path, sum string, err error) {
	setup, err := rel.SetupAsset()
	if err != nil {
		return "", "", err
	}
	want, err := c.ExpectedSHA256(ctx, rel)
	if err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(c.Dir, 0o755); err != nil {
		return "", "", err
	}
	final := filepath.Join(c.Dir, setup.Name)
	if VerifyFile(final, want) == nil {
		c.removeOthers(setup.Name)
		if progress != nil {
			progress(setup.Size, setup.Size)
		}
		return final, want, nil
	}
	_ = os.Remove(final)

	// Give up when no data arrives for a while: a stalled connection would
	// otherwise keep every update action busy until the app restarts.
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	stall := time.AfterFunc(c.StallTimeout, func() { cancel(ErrDownloadStalled) })
	defer stall.Stop()

	// The installer download must not be cut off by the short API timeout,
	// so it uses the client's transport without an overall deadline (the
	// caller's ctx still cancels it).
	dl := &http.Client{Transport: c.HTTP.Transport, CheckRedirect: c.HTTP.CheckRedirect, Jar: c.HTTP.Jar}
	resp, err := c.get(ctx, setup.DownloadURL, dl)
	if err != nil {
		if errors.Is(context.Cause(ctx), ErrDownloadStalled) {
			err = ErrDownloadStalled
		}
		return "", "", err
	}
	defer resp.Body.Close()

	total := setup.Size
	if total <= 0 {
		total = resp.ContentLength
	}
	if total > maxSetupBytes {
		return "", "", errors.New("the installer is unexpectedly large")
	}

	part := final + partialSuffix
	f, err := os.Create(part)
	if err != nil {
		return "", "", err
	}
	h := sha256.New()
	w := &progressWriter{w: io.MultiWriter(f, h), total: total, fn: func(done, total int64) {
		stall.Reset(c.StallTimeout)
		if progress != nil {
			progress(done, total)
		}
	}}
	_, copyErr := io.Copy(w, io.LimitReader(resp.Body, maxSetupBytes+1))
	closeErr := f.Close()
	if copyErr == nil && w.done > maxSetupBytes {
		copyErr = errors.New("the installer is unexpectedly large")
	}
	if copyErr != nil && errors.Is(context.Cause(ctx), ErrDownloadStalled) {
		copyErr = ErrDownloadStalled
	}
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(part)
		if copyErr == nil {
			copyErr = closeErr
		}
		return "", "", fmt.Errorf("download failed: %w", copyErr)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		_ = os.Remove(part)
		return "", "", ErrChecksumMismatch
	}
	if err := os.Rename(part, final); err != nil {
		_ = os.Remove(part)
		return "", "", err
	}
	c.removeOthers(setup.Name)
	return final, want, nil
}

// removeOthers deletes downloaded installers (and partial downloads) other
// than keep.
func (c *Client) removeOthers(keep string) {
	entries, err := os.ReadDir(c.Dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || n == keep || !strings.HasPrefix(n, "FocusLock-Setup") {
			continue
		}
		_ = os.Remove(filepath.Join(c.Dir, n))
	}
}

type progressWriter struct {
	w     io.Writer
	done  int64
	total int64
	fn    Progress
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n, err := p.w.Write(b)
	p.done += int64(n)
	if p.fn != nil {
		p.fn(p.done, p.total)
	}
	return n, err
}

// LastCheck is the bookkeeping saved after each successful check, so the
// quiet start-up check runs at most once per CheckInterval and the "update
// available" badge survives a restart. It lives in Dir, not config.json, so
// strong mode and the request channel are not involved.
type LastCheck struct {
	CheckedAt time.Time   `json:"checkedAt"`
	Result    CheckResult `json:"result"`
}

// LoadLastCheck reads the last check record (ok=false if there is none).
func (c *Client) LoadLastCheck() (LastCheck, bool) {
	var lc LastCheck
	data, err := os.ReadFile(filepath.Join(c.Dir, lastCheckFile))
	if err != nil || json.Unmarshal(data, &lc) != nil || lc.CheckedAt.IsZero() {
		return LastCheck{}, false
	}
	return lc, true
}

// SaveLastCheck records a successful check.
func (c *Client) SaveLastCheck(lc LastCheck) error {
	if err := os.MkdirAll(c.Dir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(lc)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(c.Dir, lastCheckFile), data, 0o644)
}

// CheckDue reports whether a quiet check should run: no record yet, the
// record is at least CheckInterval old, or it is in the future (clock moved).
func CheckDue(last LastCheck, ok bool, now time.Time) bool {
	if !ok {
		return true
	}
	age := now.Sub(last.CheckedAt)
	return age < 0 || age >= CheckInterval
}
