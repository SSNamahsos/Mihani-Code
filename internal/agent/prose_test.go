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
	"testing"

	"github.com/SSNamahsos/Mihani-Code/internal/config"
)

// Build mode: a reply that pastes a whole file as prose must be nudged back
// to the file tools (the "here, save this as index.html" failure mode).
func TestProseCodeDumpGetsNudged(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "text/event-stream")
		switch {
		case requests == 1:
			dump := "Here is the site:\n```html\n" + strings.Repeat("<div class='section'>Coffee</div>\n", 120) + "```\nSave it as index.html."
			payload, _ := json.Marshal(map[string]any{"choices": []map[string]any{{"delta": map[string]any{"content": dump}}}})
			fmt.Fprintf(w, "data: %s\n\n", payload)
			fmt.Fprint(w, "data: [DONE]\n\n")
		case requests == 2:
			// After the nudge the model actually uses the tool.
			args, _ := json.Marshal(map[string]any{"path": "index.html", "content": "<html></html>"})
			payload, _ := json.Marshal(map[string]any{
				"choices": []map[string]any{{"delta": map[string]any{"tool_calls": []map[string]any{{
					"index":    0,
					"id":       "c1",
					"function": map[string]any{"name": "write_file", "arguments": string(args)},
				}}}},
				}})
			fmt.Fprintf(w, "data: %s\n\n", payload)
			fmt.Fprint(w, "data: [DONE]\n\n")
		default:
			payload, _ := json.Marshal(map[string]any{"choices": []map[string]any{{"delta": map[string]any{"content": "Done - written with the tool."}}}})
			fmt.Fprintf(w, "data: %s\n\n", payload)
			fmt.Fprint(w, "data: [DONE]\n\n")
		}
	}))
	defer server.Close()
	cfg := config.Config{CurrentProvider: "test", CurrentModel: "test-model", MaxTokens: 100, Providers: map[string]config.Provider{"test": {Type: "openai", BaseURL: server.URL}}}
	root := t.TempDir()
	a := Agent{Cfg: cfg, Root: root, Client: server.Client()}
	var sawNudge bool
	if err := a.Send(context.Background(), "build a site", "build", func(string, map[string]any) bool { return true }, func(e Event) {
		if e.Kind == "activity" && strings.Contains(e.Text, "nudging") {
			sawNudge = true
		}
	}); err != nil {
		t.Fatalf("turn failed: %v", err)
	}
	if requests != 3 {
		t.Fatalf("expected dump + nudge + tool + done (3 requests), got %d", requests)
	}
	if !sawNudge {
		t.Fatal("expected a prose-dump nudge event")
	}
	if _, err := os.ReadFile(filepath.Join(root, "index.html")); err != nil {
		t.Fatalf("write_file after nudge was not executed: %v", err)
	}
}
