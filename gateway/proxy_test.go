package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// fakeUpstream records the Authorization header it received and echoes a body,
// optionally as an SSE stream. It reports the path it saw so tests can assert
// routing.
type fakeUpstream struct {
	mu        sync.Mutex
	lastAuth  string
	lastPath  string
	respond   func(w http.ResponseWriter)
	calls     int32
}

func (f *fakeUpstream) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.lastAuth = r.Header.Get("Authorization")
		f.lastPath = r.URL.Path
		f.mu.Unlock()
		atomic.AddInt32(&f.calls, 1)
		if f.respond != nil {
			f.respond(w)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}
}

func (f *fakeUpstream) auth() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastAuth
}
func (f *fakeUpstream) path() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastPath
}

func newTestGateway(t *testing.T, proKey, cloudKey, clientTokens string) (*Server, *fakeUpstream, *fakeUpstream) {
	t.Helper()
	pro := &fakeUpstream{}
	cloud := &fakeUpstream{}
	proSrv := httptest.NewServer(pro.handler())
	cloudSrv := httptest.NewServer(cloud.handler())
	t.Cleanup(proSrv.Close)
	t.Cleanup(cloudSrv.Close)

	srv := NewServer(map[string]Upstream{
		"pro":   {Name: "pro", Base: proSrv.URL + "/v1", APIKey: proKey},
		"cloud": {Name: "cloud", Base: cloudSrv.URL + "/v1", APIKey: cloudKey},
	}, splitTokens(clientTokens), 8, 30, 8, nil)
	return srv, pro, cloud
}

func doReq(t *testing.T, srv http.Handler, method, path, token string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rd)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

func TestRoutesToCorrectUpstream(t *testing.T) {
	srv, pro, cloud := newTestGateway(t, "SECRET-PRO", "SECRET-CLOUD", "")
	h := srv.Handler()

	doReq(t, h, "POST", "/pro/chat/completions", "", []byte(`{}`))
	if !strings.Contains(pro.path(), "/chat/completions") {
		t.Fatalf("pro route hit %q, want /chat/completions", pro.path())
	}
	if pro.auth() != "Bearer SECRET-PRO" {
		t.Fatalf("pro upstream got auth %q, want the pro secret", pro.auth())
	}

	doReq(t, h, "POST", "/cloud/chat/completions", "", []byte(`{}`))
	if cloud.auth() != "Bearer SECRET-CLOUD" {
		t.Fatalf("cloud upstream got auth %q, want the cloud secret", cloud.auth())
	}
}

func TestUpstreamKeyNotLeakedToClient(t *testing.T) {
	// The response body must not echo the upstream secret.
	srv, pro, _ := newTestGateway(t, "SECRET-PRO", "SECRET-CLOUD", "")
	pro.respond = func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}
	rec := doReq(t, srv.Handler(), "POST", "/pro/chat/completions", "", []byte(`{}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "SECRET-PRO") {
		t.Fatal("upstream secret leaked into the client response")
	}
}

func TestClientTokenAuth(t *testing.T) {
	srv, pro, _ := newTestGateway(t, "SECRET-PRO", "SECRET-CLOUD", "goodtoken")
	h := srv.Handler()

	// No token -> 401, upstream not called.
	rec := doReq(t, h, "POST", "/pro/chat/completions", "", []byte(`{}`))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no token: status %d, want 401", rec.Code)
	}
	if atomic.LoadInt32(&pro.calls) != 0 {
		t.Fatal("upstream must not be called when auth fails")
	}

	// Wrong token -> 401.
	rec = doReq(t, h, "POST", "/pro/chat/completions", "wrong", []byte(`{}`))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token: status %d, want 401", rec.Code)
	}

	// Right token -> 200 and upstream receives the SECRET, not the client token.
	rec = doReq(t, h, "POST", "/pro/chat/completions", "goodtoken", []byte(`{}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("good token: status %d, want 200", rec.Code)
	}
	if pro.auth() != "Bearer SECRET-PRO" {
		t.Fatalf("upstream got %q, want the pro secret (client token must not be forwarded)", pro.auth())
	}
}

func TestStreamingPassthrough(t *testing.T) {
	srv, pro, _ := newTestGateway(t, "K", "K", "")
	pro.respond = func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: one\n\n"))
		_, _ = w.Write([]byte("data: two\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}
	rec := doReq(t, srv.Handler(), "POST", "/pro/chat/completions", "", []byte(`{}`))
	got := rec.Body.String()
	if !strings.Contains(got, "data: one") || !strings.Contains(got, "data: two") || !strings.Contains(got, "data: [DONE]") {
		t.Fatalf("streamed body not passed through: %q", got)
	}
}

func TestRateLimit(t *testing.T) {
	// Tiny burst to force 429 quickly; single IP.
	srv, _, _ := newTestGateway(t, "K", "K", "")
	srv.burst = 2
	srv.refillPerSec = 0 // no refill during the test
	h := srv.Handler()

	var codes []int
	for i := 0; i < 5; i++ {
		rec := doReq(t, h, "POST", "/pro/chat/completions", "", []byte(`{}`))
		codes = append(codes, rec.Code)
	}
	limited := 0
	for _, c := range codes {
		if c == http.StatusTooManyRequests {
			limited++
		}
	}
	// With burst 2 and no refill, at least some of 5 requests must be limited.
	if limited == 0 {
		t.Fatalf("expected at least one 429, got %v", codes)
	}
}
