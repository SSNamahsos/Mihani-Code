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

const releasesURL = "https://api.github.com/repos/" + Repo + "/releases/latest"

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

// Latest fetches the newest published release for Repo.
func Latest(ctx context.Context) (*Release, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releasesURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "mihani-code-updater")
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, fmt.Errorf("no published release found for %s", Repo)
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("github returned %d while checking for updates", resp.StatusCode)
	}
	var raw ghRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&raw); err != nil {
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
	client := &http.Client{Timeout: 10 * time.Second}
	url := "https://raw.githubusercontent.com/" + Repo + "/main/CHANGELOG.md"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "mihani-code-updater")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github changelog returned %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	return changelogSection(string(raw), tag), nil
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
	dir := filepath.Dir(exe)
	tmp := filepath.Join(dir, filepath.Base(exe)+".update.tmp")

	client := &http.Client{Timeout: 3 * time.Minute}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, dlURL, nil)
	if err != nil {
		return "", false, err
	}
	req.Header.Set("User-Agent", "mihani-code-updater")
	resp, err := client.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", false, fmt.Errorf("download failed: github %d", resp.StatusCode)
	}
	f, err := os.Create(tmp)
	if err != nil {
		return "", false, fmt.Errorf("could not write the new binary: %w", err)
	}
	n, err := io.Copy(f, io.LimitReader(resp.Body, 300<<20))
	closeErr := f.Close()
	if err != nil {
		os.Remove(tmp)
		return "", false, fmt.Errorf("download interrupted: %w", err)
	}
	if closeErr != nil {
		os.Remove(tmp)
		return "", false, fmt.Errorf("could not finish writing the new binary: %w", closeErr)
	}
	if n < 4096 {
		os.Remove(tmp)
		return "", false, fmt.Errorf("downloaded file is only %d bytes — not a valid binary", n)
	}
	// Do NOT delete tmp here: on Windows the swap happens only after this
	// process exits (a running .exe is locked), so the helper still needs tmp.
	// swapBinary consumes tmp (renames it on Unix, the helper moves it on
	// Windows, and it is removed on failure).
	return swapBinary(exe, tmp, r.Tag)
}
