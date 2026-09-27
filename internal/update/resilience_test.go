package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// A flaky GitHub route must not fail the update check: transient 5xx responses
// are retried and a later success wins.
func TestLatestRetriesTransientFailures(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&hits, 1) < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v9.9.9","name":"nine","assets":[{"name":"mihani-windows-amd64.exe","browser_download_url":"https://example.invalid/mihani-windows-amd64.exe"}]}`))
	}))
	defer srv.Close()

	old := releasesBase
	releasesBase = srv.URL
	defer func() { releasesBase = old }()

	rel, err := Latest(context.Background())
	if err != nil {
		t.Fatalf("expected the retry to succeed, got %v", err)
	}
	if rel.Tag != "v9.9.9" {
		t.Fatalf("tag = %q", rel.Tag)
	}
	if atomic.LoadInt32(&hits) != 3 {
		t.Fatalf("expected 3 attempts, got %d", hits)
	}
}

// When GitHub is unreachable the error must explain itself and hand over the
// exact manual-install command instead of leaking a bare timeout.
func TestLatestUnreachableErrorIsActionable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close() // nothing is listening: connection refused
	old := releasesBase
	releasesBase = srv.URL
	defer func() { releasesBase = old }()

	_, err := Latest(context.Background())
	if err == nil {
		t.Fatal("expected an error when the host is down")
	}
	msg := err.Error()
	if !strings.Contains(msg, "attempts") {
		t.Fatalf("error should mention the retries, got: %v", msg)
	}
	if !strings.Contains(msg, "install.ps1") && !strings.Contains(msg, "install.sh") {
		t.Fatalf("error should include the manual installer, got: %v", msg)
	}
}

func TestManualInstallHintMatchesPlatform(t *testing.T) {
	hint := ManualInstallHint()
	if hint == "" || !strings.Contains(hint, Repo) {
		t.Fatalf("unexpected hint %q", hint)
	}
	if strings.Contains(hint, "curl") && !strings.Contains(hint, "install.sh") {
		t.Fatalf("non-windows hint should use install.sh, got %q", hint)
	}
}

// A release with no binary for this platform must point at the release page
// and the installer rather than failing cryptically.
func TestApplyWithoutAssetIsActionable(t *testing.T) {
	_, _, err := Apply(context.Background(), &Release{Tag: "v1.0.0", URL: HomePage})
	if err == nil {
		t.Fatal("expected an error when no asset matches this platform")
	}
	if !strings.Contains(err.Error(), HomePage) {
		t.Fatalf("error should point at the release page, got: %v", err)
	}
}

func TestFetchWithRetryGivesUpAfterAttempts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	start := time.Now()
	_, _, err := fetchWithRetry(context.Background(), srv.URL, "", 2)
	if err == nil {
		t.Fatal("expected an error after exhausting attempts")
	}
	// Backoff between two attempts must be bounded and short.
	if time.Since(start) > 20*time.Second {
		t.Fatalf("retries took too long: %v", time.Since(start))
	}
}

// Field report: a download cut short by a dropped connection keeps its PE
// header, so it passed the "is it a real binary?" check and was installed —
// Windows then refused it with "not a valid application for this OS platform".
// The published asset size must be enforced before the swap.
func TestDownloadRejectsTruncatedBinary(t *testing.T) {
	full := make([]byte, 200000)
	for i := range full {
		full[i] = byte(i % 251)
	}
	// The real-world truncation: a proxy/CDN cut the transfer but the client
	// still received a clean, complete HTTP body — just fewer bytes than the
	// release advertises.
	partial := make([]byte, 120000)
	copy(partial, full[:120000])
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(partial)
	}))
	defer srv.Close()

	tmp := filepath.Join(t.TempDir(), "mihani.exe")
	err := downloadTo(context.Background(), &http.Client{Timeout: 30 * time.Second}, srv.URL, tmp, int64(len(full)))
	if err == nil {
		t.Fatal("a truncated download must be rejected")
	}
	if !strings.Contains(err.Error(), "incomplete download") {
		t.Fatalf("error should name the truncation, got: %v", err)
	}
	if _, statErr := os.Stat(tmp); statErr == nil {
		t.Fatal("the truncated file must be deleted, not left for the swap")
	}
}

// A complete download of the expected size passes.
func TestDownloadAcceptsCompleteBinary(t *testing.T) {
	full := make([]byte, 100000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(full)
	}))
	defer srv.Close()

	tmp := filepath.Join(t.TempDir(), "mihani.exe")
	if err := downloadTo(context.Background(), &http.Client{Timeout: 30 * time.Second}, srv.URL, tmp, int64(len(full))); err != nil {
		t.Fatalf("complete download rejected: %v", err)
	}
	got, err := os.Stat(tmp)
	if err != nil || got.Size() != int64(len(full)) {
		t.Fatalf("file not written correctly: %v", err)
	}
}

func TestAssetSizeLookup(t *testing.T) {
	rel := &Release{AssetSizes: map[string]int64{AssetName(): 19194880}}
	if got := AssetSize(rel); got != 19194880 {
		t.Fatalf("AssetSize = %d", got)
	}
	if got := AssetSize(&Release{}); got != 0 {
		t.Fatalf("missing sizes should mean 0 (no check), got %d", got)
	}
}
