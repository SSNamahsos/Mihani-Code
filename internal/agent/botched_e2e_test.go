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

// botchedPayload is the first model reply: a malformed tool call under an
// aliased tag (the exact shape from the user report) with invalid JSON inside
// the message content. The SSE chunk itself must be valid JSON, so the inner
// quotes are escaped. After unescaping, the model's content is:
//   <Longcat_tool_call>ask_user options": ["A", "B"]</ask_user>
const botchedPayload = "{\"choices\":[{\"delta\":{\"content\":\"<Longcat_tool_call>ask_user options\\\": [\\\"A\\\", \\\"B\\\"]</ask_user>\"}}]}"

// donePayload is the second reply: a normal prose completion after the nudge.
const donePayload = "{\"choices\":[{\"delta\":{\"content\":\"done\"}}]}"

// End-to-end (native path): the model first emits a malformed tool call (aliased
// tag), Mihani must NOT stop the turn — it nudges for a resend, and the turn
// completes on the following valid response.
func TestSendRecoversFromBotchedToolCall(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "text/event-stream")
		if requests == 1 {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", botchedPayload)
		} else {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", donePayload)
		}
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

// Same recovery on the PROMPT-BASED path (native_tools: false) — the path your
// model (longcat-2.0 / hcnsec) actually uses. The model emits the aliased tag in
// plain text; extractToolCalls returns no calls and no parse error, so the
// aliased-tag detector must catch it.
func TestSendPromptBasedRecoversFromBotchedToolCall(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "text/event-stream")
		if requests == 1 {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", botchedPayload)
		} else {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", donePayload)
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	noNative := false
	cfg := config.Config{CurrentProvider: "t", CurrentModel: "m", MaxTokens: 200, Providers: map[string]config.Provider{"t": {Type: "openai", BaseURL: server.URL, NativeTools: &noNative}}}
	if cfg.Providers["t"].UseNativeTools() {
		t.Fatal("test must force the prompt-based path")
	}
	a := Agent{Cfg: cfg, Root: t.TempDir(), Client: server.Client()}
	var activity strings.Builder
	err := a.Send(context.Background(), "research something", "research", func(string, map[string]any) bool { return true },
		func(e Event) { activity.WriteString(e.Text + "\n") })
	if err != nil {
		t.Fatalf("Send should recover and complete on the prompt-based path, got %v", err)
	}
	if requests != 2 {
		t.Fatalf("expected 2 requests (botched reply then nudge-driven resend), got %d", requests)
	}
	if !strings.Contains(activity.String(), "nudging: resend tool call") {
		t.Fatalf("expected a botched-tool-call nudge activity, got:\n%s", activity.String())
	}
}
