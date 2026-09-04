package agent

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
)

// The exact failure the user reported: a provider timeout that wraps
// context.DeadlineExceeded inside a *url.Error carrying the endpoint URL.
func providerTimeoutErr() error {
	return &url.Error{
		Op:  "Post",
		URL: "https://api.hcnsec.cn/v1/chat/completions",
		Err: context.DeadlineExceeded,
	}
}

func TestClassifyProviderTimeoutIsRetriable(t *testing.T) {
	ctx := context.Background()
	got := classifyTransportError(ctx, providerTimeoutErr())

	var te *providerTransportError
	if !errors.As(got, &te) {
		t.Fatalf("expected providerTransportError, got %T", got)
	}
	if te.kind != "timeout" {
		t.Fatalf("kind = %q, want timeout", te.kind)
	}
	if !Retriable(got) {
		t.Fatal("a provider timeout must be retriable (so the reconnect loop engages)")
	}
	// The message shown to the user must not leak the endpoint or URL.
	msg := DescribeError(got)
	for _, bad := range []string{"http", "hcnsec", "api.", "//"} {
		if strings.Contains(strings.ToLower(msg), bad) {
			t.Fatalf("timeout message leaks endpoint detail: %q", msg)
		}
	}
}

func TestClassifyUserCancelIsNotRetriable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := &url.Error{Op: "Post", URL: "https://x.example/v1", Err: context.Canceled}
	got := classifyTransportError(ctx, err)
	if Retriable(got) {
		t.Fatal("a user cancellation must not be retried")
	}
	if DescribeError(got) != "request cancelled" {
		t.Fatalf("cancel message = %q, want 'request cancelled'", DescribeError(got))
	}
}

func TestDescribeErrorNeverLeaksURL(t *testing.T) {
	cases := []error{
		providerTimeoutErr(),
		&url.Error{Op: "Post", URL: "https://seekai.cc/v1/chat/completions", Err: errors.New("connection refused")},
		&url.Error{Op: "Post", URL: "https://seekai.cc/v1/chat/completions", Err: errors.New("dial tcp: lookup: no such host")},
		errors.New(`Post "https://api.hcnsec.cn/v1/chat/completions": context deadline exceeded (Client.Timeout exceeded while awaiting headers)`),
	}
	for _, err := range cases {
		msg := DescribeError(err)
		if msg == "" {
			t.Fatalf("empty description for %v", err)
		}
		if strings.Contains(msg, "hcnsec") || strings.Contains(msg, "seekai") || strings.Contains(msg, "api.") || strings.Contains(msg, "://") || strings.Contains(msg, "chat/completions") {
			t.Fatalf("DescribeError leaks endpoint detail: %q", msg)
		}
		if !Retriable(err) {
			t.Fatalf("network/timeout error should be retriable: %v", err)
		}
	}
}

func TestDescribeErrorKnownKinds(t *testing.T) {
	if got := DescribeError(context.Canceled); got != "request cancelled" {
		t.Fatalf("cancel = %q", got)
	}
	if got := DescribeError(&providerHTTPError{status: 502, message: "provider returned 502 Bad Gateway"}); got != "provider returned 502 Bad Gateway" {
		t.Fatalf("http = %q", got)
	}
	if got := DescribeError(errEmptyResponse); got == "" {
		t.Fatal("empty-response description should be non-empty")
	}
}
