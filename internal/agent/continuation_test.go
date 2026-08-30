package agent

import (
	"context"
		"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SSNamahsos/Mihani-Code/internal/config"
)

// A 402 / credit-exhausted provider answer must NOT be retried by the
// reconnect loop — retrying repeats the same denial; the user has to reset
// the usage window or switch credential.
func TestCreditExhaustionNotRetriable(t *testing.T) {
	if Retriable(&providerCreditError{status: 402, message: "x"}) {
		t.Fatal("credit exhaustion must not be retriable")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(402)
		fmt.Fprint(w, `{"error":{"message":"insufficient credits, top up your account","type":"billing"}}`)
	}))
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	err = providerError(resp)
	if err == nil {
		t.Fatal("expected error")
	}
	if Retriable(err) {
		t.Fatalf("402 billing error must not be retriable: %v", err)
	}
	if !strings.Contains(err.Error(), "Reset usage window") {
		t.Fatalf("credit error should point at the fix, got: %v", err)
	}
}

// Regression: when the response hits the max output token limit (finish
// "length") mid tool-call, the turn must continue in place with a
// continuation nudge instead of dying with an unparseable-arguments error.
func TestTruncatedReplyContinuesInTurn(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "text/event-stream")
		if requests == 1 {
			// tool call arguments cut off mid-JSON + stream ends at length.
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"function\":{\"name\":\"write_file\",\"arguments\":\"{\\\"path\\\":\\\"big.html\\\",\\\"content\\\":\\\"<html>\"}}]}}]}\n\n")
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"length\"}]}\n\n")
			fmt.Fprint(w, "data: [DONE]\n\n")
			return
		}
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"done in smaller chunks\"}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	cfg := config.Config{CurrentProvider: "test", CurrentModel: "test-model", MaxTokens: 100, Providers: map[string]config.Provider{"test": {Type: "openai", BaseURL: server.URL}}}
	a := Agent{Cfg: cfg, Root: t.TempDir(), Client: server.Client()}
	var text strings.Builder
	err := a.Send(context.Background(), "write a big file", "build", func(string, map[string]any) bool { return true }, func(e Event) { text.WriteString(e.Text) })
	if err != nil {
		t.Fatalf("turn should continue after length-truncation, got: %v", err)
	}
	if requests != 2 {
		t.Fatalf("expected 2 requests (truncated + continued), got %d", requests)
	}
	if !strings.Contains(text.String(), "done in smaller chunks") {
		t.Fatalf("missing continuation output: %q", text.String())
	}
	// The continuation nudge must be in history so the model sees why it
	// should split work into smaller chunks.
	found := false
	for _, h := range a.history {
		if c, ok := h["content"].(string); ok && strings.Contains(c, "token limit") {
			found = true
		}
	}
	if !found {
		t.Fatal("expected continuation nudge in history")
	}
}

// An empty reply that ended at the token limit also continues instead of
// surfacing "provider returned an empty response".
func TestEmptyLengthReplyContinues(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "text/event-stream")
		if requests == 1 {
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"length\"}]}\n\n")
			fmt.Fprint(w, "data: [DONE]\n\n")
			return
		}
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"continued\"}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	cfg := config.Config{CurrentProvider: "test", CurrentModel: "test-model", MaxTokens: 100, Providers: map[string]config.Provider{"test": {Type: "openai", BaseURL: server.URL}}}
	a := Agent{Cfg: cfg, Root: t.TempDir(), Client: server.Client()}
	var text strings.Builder
	if err := a.Send(context.Background(), "go", "build", func(string, map[string]any) bool { return true }, func(e Event) { text.WriteString(e.Text) }); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if requests != 2 || !strings.Contains(text.String(), "continued") {
		t.Fatalf("requests=%d text=%q", requests, text.String())
	}
}

// Repeated truncation (nudge budget of 3) must still fail rather than loop
// forever burning tokens.
func TestTruncationNudgeBudgetExhausts(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"c\",\"function\":{\"name\":\"write_file\",\"arguments\":\"{\\\"path\\\":\\\"x\\\",\\\"content\\\":\\\"<\"}}]}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"length\"}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	cfg := config.Config{CurrentProvider: "test", CurrentModel: "test-model", MaxTokens: 100, Providers: map[string]config.Provider{"test": {Type: "openai", BaseURL: server.URL}}}
	a := Agent{Cfg: cfg, Root: t.TempDir(), Client: server.Client()}
	err := a.Send(context.Background(), "go", "build", func(string, map[string]any) bool { return true }, func(Event) {})
	if err == nil {
		t.Fatal("expected the turn to fail after the nudge budget")
	}
	if requests > 4 {
		t.Fatalf("nudge budget exceeded: %d requests", requests)
	}
}

// Defense in depth: even in native-tools mode, some models emit the call as
// literal JSON text inside the content. The agent must parse and execute it
// instead of ending the turn with a JSON blob as the reply.
func TestNativeModeParsesTextToolCall(t *testing.T) {
	var requests int
	var wroteFile bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "text/event-stream")
		if requests == 1 {
			payload := "{\"choices\":[{\"delta\":{\"content\":\"Writing the file. \\n ```tool_call\\n {\\\"name\\\": \\\"write_file\\\", \\\"arguments\\\": {\\\"path\\\": \\\"note.txt\\\", \\\"content\\\": \\\"hello\\\"}}\\n ```\"}}]}"
			fmt.Fprintf(w, "data: %s\n\n", payload)
			fmt.Fprint(w, "data: [DONE]\n\n")
			return
		}
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"All done.\"}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	cfg := config.Config{CurrentProvider: "test", CurrentModel: "test-model", MaxTokens: 100, Providers: map[string]config.Provider{"test": {Type: "openai", BaseURL: server.URL}}}
	root := t.TempDir()
	a := Agent{Cfg: cfg, Root: root, Client: server.Client()}
	var text strings.Builder
	if err := a.Send(context.Background(), "write note.txt with hello", "build", func(string, map[string]any) bool { return true }, func(e Event) { text.WriteString(e.Text) }); err != nil {
		t.Fatalf("turn failed: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "note.txt"))
	if err != nil {
		t.Fatalf("write_file from text was not executed: %v", err)
	}
	if strings.TrimSpace(string(data)) != "hello" {
		t.Fatalf("file content wrong: %q", data)
	}
	wroteFile = true
	if !wroteFile || !strings.Contains(text.String(), "All done.") {
		t.Fatalf("expected continuation after text tool call, text=%q", text.String())
	}
}

func TestLooksLikeCodeDump(t *testing.T) {
	if looksLikeCodeDump("short answer") {
		t.Fatal("short prose must not match")
	}
	if !looksLikeCodeDump("here you go:\n```html\n" + strings.Repeat("<div></div>\n", 300)) {
		t.Fatal("fenced code dump must match")
	}
}
