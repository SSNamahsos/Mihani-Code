package update

import (
	"context"
	"net/http"
	"net/http/httptest"
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
