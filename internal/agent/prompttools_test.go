package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/SSNamahsos/Mihani-Code/internal/config"
)

func TestExtractToolCallsTagged(t *testing.T) {
	text := "Let me look at that.\n" + tagged("read_file", map[string]any{"path": "main.go"})
	calls, err := extractToolCalls(text)
	if err != nil || len(calls) != 1 {
		t.Fatalf("expected one call, got %d (%v)", len(calls), err)
	}
	if calls[0].Name != "read_file" || calls[0].Input["path"] != "main.go" {
		t.Fatalf("wrong call: %#v", calls[0])
	}
}

func TestExtractToolCallsFencedAndMultiple(t *testing.T) {
	text := "step 1\n```json\n" + `{"name": "list_dir", "arguments": {"path": "."}}` + "\n```\nthen\n" +
		tagged("bash", map[string]any{"command": "dir"})
	calls, err := extractToolCalls(text)
	if err != nil || len(calls) != 2 {
		t.Fatalf("expected two calls, got %d (%v)", len(calls), err)
	}
	if calls[0].Name != "list_dir" || calls[1].Name != "bash" {
		t.Fatalf("order wrong: %s then %s", calls[0].Name, calls[1].Name)
	}
}

func TestExtractToolCallsNone(t *testing.T) {
	if calls, _ := extractToolCalls("Just a plain answer with no tools."); len(calls) != 0 {
		t.Fatalf("plain prose produced calls: %#v", calls)
	}
}

// Regression: a stream that cut off inside a tool_call block (opening tag
// present, closing tag missing) must be reported as a parse error so the
// turn gets a corrective hint — never a silent early end.
func TestExtractToolCallsTruncatedBlockReported(t *testing.T) {
	truncated := "Running it now.\n" + toolCallOpenTag + `{"name":"bash","arguments":{"command":"curl -s https://example.com?x=`
	calls, err := extractToolCalls(truncated)
	if err == nil {
		t.Fatal("truncated tool_call block must be reported as a parse error")
	}
	if len(calls) != 0 {
		t.Fatalf("no complete call may be extracted from a truncated block, got %d", len(calls))
	}
	// Complete blocks still parse normally.
	ok := tagged("bash", map[string]any{"command": "dir"})
	calls, err = extractToolCalls("done.\n" + ok)
	if err != nil || len(calls) != 1 {
		t.Fatalf("complete block must still parse, got %d calls (%v)", len(calls), err)
	}
}

func TestExtractToolCallsMalformedReportsError(t *testing.T) {
	if _, err := extractToolCalls("<tool_call>{not json}</tool_call>"); err == nil {
		t.Fatal("malformed call must surface an error so the model can retry")
	}
}

func TestStripToolCallsCleansProse(t *testing.T) {
	got := StripToolCalls("Working on it.\n" + tagged("x", map[string]any{}) + "\nDone soon.")
	if strings.Contains(got, "tool_call") {
		t.Fatalf("block not stripped: %q", got)
	}
}

func tagged(name string, args map[string]any) string {
	payload, _ := json.Marshal(map[string]any{"name": name, "arguments": args})
	return "<tool_call>" + string(payload) + "</tool_call>"
}

// sseChunk builds one OpenAI-style SSE data line from a delta payload.
func sseChunk(t *testing.T, delta map[string]any) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": delta}}})
	if err != nil {
		t.Fatal(err)
	}
	return "data: " + string(b) + "\n\n"
}

// End-to-end: a gateway without function calling still gets full tool access
// through the text protocol — model asks, Mihani executes, history loops.
func TestPromptBasedToolsEndToEnd(t *testing.T) {
	root := t.TempDir()
	marker := "secret-content-marker-7f3a"
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte(marker), 0644); err != nil {
		t.Fatal(err)
	}

	var step int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		switch atomic.AddInt32(&step, 1) {
		case 1:
			content := "Checking the file now.\n" + tagged("read_file", map[string]any{"path": "hello.txt"})
			fmt.Fprint(w, sseChunk(t, map[string]any{"content": content}))
			fmt.Fprint(w, "data: [DONE]\n\n")
		default:
			content := "The file contains " + marker + ". All done."
			fmt.Fprint(w, sseChunk(t, map[string]any{"content": content}))
			fmt.Fprint(w, "data: [DONE]\n\n")
		}
	}))
	defer server.Close()

	no := false
	cfg := config.Config{
		CurrentProvider: "p", CurrentModel: "m", MaxTokens: 100, ContextWindow: 200000,
		Providers: map[string]config.Provider{"p": {
			Type: "openai", BaseURL: server.URL,
			NativeTools: &no,
		}},
	}
	a := Agent{Cfg: cfg, Root: root, Client: server.Client()}

	sawResult := false
	err := a.Send(context.Background(), "read hello.txt", "build",
		func(string, map[string]any) bool { return true },
		func(e Event) {
			if e.Kind == "tool_done" && strings.Contains(e.ToolResult, marker) {
				sawResult = true
			}
		})
	if err != nil {
		t.Fatal(err)
	}
	if !sawResult {
		t.Fatal("tool never executed via the text protocol")
	}

	found := false
	for _, msg := range a.History() {
		if fmt.Sprint(msg["role"]) != "user" {
			continue
		}
		if s, ok := msg["content"].(string); ok &&
			strings.Contains(s, "<tool_result") && strings.Contains(s, marker) {
			found = true
		}
	}
	if !found {
		t.Fatal("tool_result was never fed back into the conversation")
	}
}

// The protocol system prompt must advertise the tools so non-native models know what exists.
func TestPromptBasedSystemPromptListsTools(t *testing.T) {
	a := &Agent{}
	catalog := a.toolCatalog()
	for _, want := range []string{"read_file", "write_file", "edit_file", "bash"} {
		if !strings.Contains(catalog, want) {
			t.Fatalf("catalog missing %s:\n%s", want, catalog)
		}
	}
	if !strings.Contains(promptToolProtocol, "<tool_call>") {
		t.Fatal("protocol does not describe the tool_call format")
	}
}
