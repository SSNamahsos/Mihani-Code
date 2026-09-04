package agent

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SSNamahsos/Mihani-Code/internal/config"
)

// End-to-end: the model first emits a malformed tool call (aliased tag, the
// exact shape from the user report), Mihani must NOT stop the turn — it nudges
// for a resend, and the turn completes on the following valid response. This is
// what used to silently stop with a half-call shown to the user.
func TestSendRecoversFromBotchedToolCall(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "text/event-stream")
		if requests == 1 {
			// First reply: a botched tool call (aliased tag + closing </ask_user>,
			// invalid JSON) and no native tool_calls.
			payload := `{"choices":[{"delta":{"content":"<Longcat_tool_call>ask_user options\": [\"A\", \"B\"]</ask_user>"}}]}`
			_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
			return
		}
		// Second reply: a normal prose completion after the nudge.
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"done\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	cfg := config.Config{CurrentProvider: "t", CurrentModel: "m", MaxTokens: 200, Providers: map[string]config.Provider{"t": {Type: "openai", BaseURL: server.URL}}}
	a := Agent{Cfg: cfg, Root: t.TempDir(), Client: server.Client()}
	var activity strings.Builder
	err := a.Send(context.Background(), "research something", "research", func(string, map[string]any) bool { return true },
		func(e Event) { activity.WriteString(e.Text + "\n") })
	if err != nil {
		t.Fatalf("Send should recover and complete, got %v", err)
	}
	if requests != 2 {
		t.Fatalf("expected 2 requests (botched reply then nudge-driven resend), got %d", requests)
	}
	if !strings.Contains(activity.String(), "nudging: resend tool call") {
		t.Fatalf("expected a botched-tool-call nudge activity, got:\n%s", activity.String())
	}
}
