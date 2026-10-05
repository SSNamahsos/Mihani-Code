package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConnectAcceptsNULPrefixedPastedURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"model-a"}]}`))
	}))
	defer srv.Close()

	m := newTestModel(80, 24)
	m.openConnect()
	m.connectFields[0] = "mihani-circle"
	m.connectFields[1] = "\x00" + srv.URL + "/api/v1" // exactly what conhost pastes
	m.connectFields[2] = "sk-test"
	m.connectInput.SetValue(m.connectFields[1])

	cmd := m.startConnect()
	if m.connecting != true {
		t.Fatalf("connect should have started, error = %q", m.connectError)
	}
	msg := cmd()
	res, ok := msg.(modelsMsg)
	if !ok {
		t.Fatalf("unexpected message %T", msg)
	}
	if res.err != nil {
		t.Fatalf("NUL-prefixed URL must work after sanitizing, got: %v", res.err)
	}
	if len(res.models) != 1 || res.models[0] != "model-a" {
		t.Fatalf("models = %v", res.models)
	}
}

func TestSanitizeFieldStripsControlChars(t *testing.T) {
	if got := sanitizeField("\x00https://api.example.com/v1\r\n"); got != "https://api.example.com/v1" {
		t.Fatalf("sanitizeField = %q", got)
	}
	if got := sanitizeField("sk-abc\x00\x01\x7fdef"); got != "sk-abcdef" {
		t.Fatalf("sanitizeField = %q", got)
	}
	if got := sanitizeField("  spaced  "); got != "spaced" {
		t.Fatalf("sanitizeField = %q", got)
	}
}

// A malformed base URL must produce a plain message, never a raw Go parse
// error leaking out of the HTTP layer.
func TestConnectRejectsBadURLWithReadableMessage(t *testing.T) {
	m := newTestModel(80, 24)
	m.openConnect()
	m.connectFields[0] = "bad"
	m.connectFields[1] = "api.example.com" // no scheme
	if cmd := m.startConnect(); cmd != nil {
		t.Fatal("startConnect should not proceed with an invalid URL")
	}
	if m.connectError == "" || !strings.Contains(strings.ToLower(m.connectError), "http") {
		t.Fatalf("error should mention the expected http(s) scheme, got %q", m.connectError)
	}
	if strings.Contains(m.connectError, "net/url") {
		t.Fatalf("raw net/url error leaked into the UI: %q", m.connectError)
	}
}
