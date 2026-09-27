// Package update checks the public GitHub releases for a newer Mihani Code
// build and can install it over the running binary. It talks to the GitHub
// REST API directly — no model call, no API key — so an update check costs
// zero tokens and never touches a provider.
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Repo is the GitHub owner/name the releases live under.
const Repo = "SSNamahsos/Mihani-Code"

// HomePage is where the update is published and browsed.
const HomePage = "https://github.com/" + Repo + "/releases"

// ManualInstallHint is the copy-pasteable one-liner that installs the latest
// release. It is shown whenever the in-app update cannot reach GitHub, so a
// flaky route never leaves the user with a bare timeout and no way forward.
func ManualInstallHint() string {
	if runtime.GOOS == "windows" {
		return "irm https://raw.githubusercontent.com/" + Repo + "/main/install.ps1 | iex"
	}
	return "curl -fsSL https://raw.githubusercontent.com/" + Repo + "/main/install.sh | sh"
}

// releasesBase is the API root; a var so tests can point it at a local server.
var releasesBase = "https://api.github.com/repos/" + Repo

func releasesURL() string { return releasesBase + "/releases/latest" }

// rawBase is the raw.githubusercontent root for the CHANGELOG read.
var rawBase = "https://raw.githubusercontent.com/" + Repo + "/main"

// Tuning for flaky consumer networks. GitHub is routinely slow, throttled or
// intermittently unreachable from some regions: a single 10-second shot used
// to fail the whole update even though a retry seconds later succeeded, and
// the 3-minute download cap killed large binaries on a slow link.
const (
	checkAttempts   = 3
	checkTimeout    = 20 * time.Second
	retryBackoff    = 2 * time.Second
	downloadTimeout = 10 * time.Minute
)

// fetchWithRetry performs a GET, retrying transient failures (timeouts,
// connection resets, 5xx) with a growing backoff, and returns the response
// body. The body is fully read inside each attempt so the attempt context is
// always cancelled — no context leak on the success path.
func fetchWithRetry(ctx context.Context, url, accept string, attempts int) (int, []byte, error) {
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if attempt > 1 {
			select {
			case <-ctx.Done():
				return 0, nil, ctx.Err()
			case <-time.After(time.Duration(attempt-1) * retryBackoff):
			}
		}
		attemptCtx, cancel := context.WithTimeout(ctx, checkTimeout)
		req, err := http.NewRequestWithContext(attemptCtx, http.MethodGet, url, nil)
		if err != nil {
			cancel()
			return 0, nil, err
		}
		req.Header.Set("User-Agent", "mihani-code-updater")
		if accept != "" {
			req.Header.Set("Accept", accept)
		}
		client := &http.Client{Timeout: checkTimeout}
		resp, err := client.Do(req)
		if err == nil && resp.StatusCode < 500 {
			body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			resp.Body.Close()
			cancel()
			if readErr != nil {
				lastErr = readErr
				continue
			}
			return resp.StatusCode, body, nil
		}
		if err == nil {
			lastErr = fmt.Errorf("github returned %s", resp.Status)
			resp.Body.Close()
		} else {
			lastErr = err
		}
		cancel()
	}
	return 0, nil, lastErr
}

// unreachableError describes a failed GitHub round trip in plain language and
// points at the manual installer, which uses a different fetch path and often
// succeeds when the app's own request does not.
func unreachableError(what string, err error) error {
	reason := "could not reach GitHub"
	lower := strings.ToLower(err.Error())
	switch {
	case strings.Contains(lower, "timeout"), strings.Contains(lower, "deadline exceeded"),
		strings.Contains(lower, "timed out"):
		reason = "timed out reaching GitHub"
	}
	return fmt.Errorf("%s (%s) after %d attempts — this network may block or throttle GitHub · install the same version manually: %s",
		reason, what, checkAttempts, ManualInstallHint())
}

// Release is the subset of a GitHub release the UI needs.
type Release struct {
	Tag    string   // e.g. "v0.2.18"
	Name   string   // human release title
	Body   string   // release notes / changelog body
	URL    string   // human release page on GitHub
	Assets []string // browser_download_url list
}

type ghRelease struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	Body    string `json:"body"`
	HTMLURL string `json:"html_url"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

// Latest fetches the newest published release for Repo. Transient network
// failures are retried, and an exhausted retry reports the manual installer
// instead of a raw timeout.
func Latest(ctx context.Context) (*Release, error) {
	status, body, err := fetchWithRetry(ctx, releasesURL(), "application/vnd.github+json", checkAttempts)
	if err != nil {
		return nil, unreachableError("update check", err)
	}
	switch {
	case status == http.StatusNotFound:
		return nil, fmt.Errorf("no published release found for %s", Repo)
	case status != http.StatusOK:
		return nil, fmt.Errorf("github returned %d while checking for updates", status)
	}
	var raw ghRelease
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	r := &Release{Tag: raw.TagName, Name: raw.Name, Body: raw.Body, URL: raw.HTMLURL}
	for _, a := range raw.Assets {
		if a.URL != "" {
			r.Assets = append(r.Assets, a.URL)
		}
	}
	if r.Tag == "" {
		return nil, fmt.Errorf("release response had no tag")
	}
	return r, nil
}

// Newer reports whether candidate (a tag like "v0.2.18") is strictly newer
// than current. Equal or older tags report false; empty values report false.
func Newer(candidate, current string) bool {
	if candidate == "" || current == "" {
		return false
	}
	return compareVersions(candidate, current) > 0
}

// compareVersions orders two dotted versions. Non-numeric segments parse as 0.
func compareVersions(a, b string) int {
	a = stripVersion(a)
	b = stripVersion(b)
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		x, y := 0, 0
		if i < len(as) {
			x, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			y, _ = strconv.Atoi(bs[i])
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

func stripVersion(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	return s
}

// AssetForURL returns the download URL that matches the running OS/arch
// (matching the release build names, e.g. mihani-windows-amd64.exe), or "".
func AssetForURL(r *Release) string {
	if r == nil {
		return ""
	}
	base := "mihani-" + runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		base += ".exe"
	}
	for _, u := range r.Assets {
		if strings.HasSuffix(u, "/"+base) {
			return u
		}
	}
	return ""
}

// CleanupStale removes a leftover <exe>.old file next to the running binary.
// That file is the pre-update binary during an in-place swap; if a session
// was hard-killed mid-swap it can remain, so a fresh launch clears it. Best
// effort and never fatal.
func CleanupStale() {
	exe, err := currentBinary()
	if err != nil {
		return
	}
	_ = os.Remove(exe + ".old")
}

// OpenURL opens a URL in the system default browser (best effort).
func OpenURL(u string) error {
	if u == "" {
		return fmt.Errorf("empty url")
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	case "darwin":
		cmd = exec.Command("open", u)
	default:
		cmd = exec.Command("xdg-open", u)
	}
	return cmd.Start()
}

// Changelog fetches the detailed changelog section for tag from the repo's
// CHANGELOG.md on GitHub (a raw, unauthenticated file read — no model, no
// tokens). The GitHub release body is auto-generated and sparse, so the real
// "what's new" lives in the CHANGELOG. It returns the section for tag, or the
// newest section if tag is not found, or ("", nil) when nothing is usable.
func Changelog(ctx context.Context, tag string) (string, error) {
	status, body, err := fetchWithRetry(ctx, rawBase+"/CHANGELOG.md", "", checkAttempts)
	if err != nil {
		return "", unreachableError("changelog fetch", err)
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("github changelog returned %d", status)
	}
	return changelogSection(string(body), tag), nil
}

// changelogSection extracts "## <tag>" up to the next "## " header; if tag is
// absent it falls back to the first version section.
func changelogSection(doc, tag string) string {
	want := "## " + strings.TrimSpace(tag)
	var lines []string
	var found bool
	inWant := false
	inFirst := false
	for _, line := range strings.Split(doc, "\n") {
		if strings.HasPrefix(line, "## ") {
			if line == want {
				found = true
				inWant = true
				continue
			}
			if inWant {
				break // hit the next version's header
			}
			if !inFirst && !found {
				// first "## " header that is not the wanted one = fallback top
				inFirst = true
				continue
			}
			continue
		}
		if inWant || inFirst {
			lines = append(lines, line)
		}
	}
	if found {
		lines = append([]string{want}, lines...)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// currentBinary returns the path of the running executable, resolved through
// any symlinks so the swap lands on the real file.
func currentBinary() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if real, e := filepath.EvalSymlinks(exe); e == nil {
		exe = real
	}
	return exe, nil
}

// Apply downloads the release binary for this platform and swaps it over the
// running executable. It returns a human summary of what happened (shown in
// the UI) or an error. The download is capped at 300 MB; a file smaller than
// 4 KB is rejected as clearly not a real binary. ctx may be cancelled by the
// user (esc) to abort the download.
func Apply(ctx context.Context, r *Release) (string, bool, error) {
	if r == nil {
		return "", false, fmt.Errorf("no release to install")
	}
	dlURL := AssetForURL(r)
	if dlURL == "" {
		return "", false, fmt.Errorf("no release binary is published for %s/%s — install from %s", runtime.GOOS, runtime.GOARCH, r.URL)
	}
	exe, err := currentBinary()
	if err != nil {
		return "", false, fmt.Errorf("could not locate the running binary: %w", err)
	}
	// The release host (objects.githubusercontent.com) is frequently slow or
	// throttled on consumer networks; a 3-minute cap killed large binaries
	// mid-download. Give it room, and retry once from scratch before giving
	// up with an actionable message.
	dir := filepath.Dir(exe)
	tmp := filepath.Join(dir, filepath.Base(exe)+".update.tmp")
	client := &http.Client{Timeout: downloadTimeout}

	var lastErr error
	for attempt := 1; attempt <= 2; attempt++ {
		err := downloadTo(ctx, client, dlURL, tmp)
		if err == nil {
			// Do NOT delete tmp: on Windows the swap happens only after this
			// process exits (a running .exe is locked), so the helper still
			// needs tmp. swapBinary consumes it.
			return swapBinary(exe, tmp, r.Tag)
		}
		lastErr = err
		os.Remove(tmp)
		if ctx.Err() != nil {
			return "", false, ctx.Err()
		}
	}
	return "", false, fmt.Errorf("%w · the release host did not respond — install manually: %s", lastErr, ManualInstallHint())
}

// downloadTo streams the release binary to tmp and only reports success once
// the whole body arrived. ctx lets the user cancel with esc.
func downloadTo(ctx context.Context, client *http.Client, dlURL, tmp string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, dlURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "mihani-code-updater")
	resp, err := client.Do(req)
	if err != nil {
		lower := strings.ToLower(err.Error())
		if os.IsTimeout(err) || strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline") {
			return fmt.Errorf("the download timed out after %v (GitHub's release host is slow or blocked on this network)", client.Timeout)
		}
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: github %d", resp.StatusCode)
	}
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("could not write the new binary: %w", err)
	}
	n, err := io.Copy(f, io.LimitReader(resp.Body, 300<<20))
	closeErr := f.Close()
	if err != nil {
		os.Remove(tmp)
		return fmt.Errorf("download interrupted after %d bytes: %w", n, err)
	}
	if closeErr != nil {
		os.Remove(tmp)
		return fmt.Errorf("could not finish writing the new binary: %w", closeErr)
	}
	if n < 4096 {
		os.Remove(tmp)
		return fmt.Errorf("downloaded file is only %d bytes — not a valid binary", n)
	}
	return nil
}
