package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SSNamahsos/Mihani-Code/internal/config"
	"github.com/SSNamahsos/Mihani-Code/internal/secrets"
)

func TestOpenAIStreamCompletesAndKeepsHistory(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()
	cfg := config.Config{CurrentProvider: "test", CurrentModel: "test-model", MaxTokens: 100, Providers: map[string]config.Provider{"test": {Type: "openai", BaseURL: server.URL}}}
	a := Agent{Cfg: cfg, Root: t.TempDir(), Client: server.Client()}
	var text strings.Builder
	err := a.Send(context.Background(), "say hello", "build", func(string, map[string]any) bool { return true }, func(e Event) { text.WriteString(e.Text) })
	if err != nil {
		t.Fatal(err)
	}
	if requests != 1 || !strings.Contains(text.String(), "hello") {
		t.Fatalf("requests=%d text=%q", requests, text.String())
	}
	if len(a.history) != 3 {
		t.Fatalf("expected system, user, assistant history, got %d", len(a.history))
	}
}

// Regression: streamed parallel tool calls must be reassembled in index order.
func TestOpenAIToolCallsPreserveStreamOrder(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "text/event-stream")
		if requests > 1 {
			_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"done"}}]}` + "\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
			return
		}
		// Index 1 arrives before index 0 on the wire.
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"tool_calls":[{"index":1,"id":"call_B","function":{"name":"list_dir","arguments":"{}"}}]}}]}` + "\n\n"))
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_A","function":{"name":"read_file","arguments":"{\"path\":\"x\"}"}}]}}]}` + "\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"finish_reason\":\"tool_calls\",\"delta\":{}}],\"usage\":{\"total_tokens\":42}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()
	cfg := config.Config{CurrentProvider: "test", CurrentModel: "m", MaxTokens: 100, ContextWindow: 200000,
		Providers: map[string]config.Provider{"test": {Type: "openai", BaseURL: server.URL}}}
	a := Agent{Cfg: cfg, Root: t.TempDir(), Client: server.Client()}
	err := a.Send(context.Background(), "go", "build", func(string, map[string]any) bool { return true }, func(Event) {})
	if err != nil {
		t.Fatal(err)
	}
	var callIDs []string
	for _, msg := range a.history {
		if fmt.Sprint(msg["role"]) != "assistant" {
			continue
		}
		b, err := json.Marshal(msg)
		if err != nil {
			t.Fatal(err)
		}
		var decoded struct {
			ToolCalls []struct {
				ID string `json:"id"`
			} `json:"tool_calls"`
		}
		if json.Unmarshal(b, &decoded) == nil && len(decoded.ToolCalls) > 0 {
			callIDs = nil
			for _, c := range decoded.ToolCalls {
				callIDs = append(callIDs, c.ID)
			}
		}
	}
	if len(callIDs) != 2 {
		t.Fatalf("expected two stored tool calls, got %d", len(callIDs))
	}
	if callIDs[0] != "call_A" {
		t.Fatalf("tool call order not preserved by index, order=%v", callIDs)
	}
}

func TestAnthropicThinkingBlocksAreNotStored(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"type":"message_start","message":{"usage":{"input_tokens":10}}}` + "\n\n"))
		_, _ = w.Write([]byte(`data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}` + "\n\n"))
		_, _ = w.Write([]byte(`data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"hmm"}}` + "\n\n"))
		_, _ = w.Write([]byte(`data: {"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}` + "\n\n"))
		_, _ = w.Write([]byte(`data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"answer"}}` + "\n\n"))
		_, _ = w.Write([]byte(`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}` + "\n\n"))
	}))
	defer server.Close()
	cfg := config.Config{CurrentProvider: "test", CurrentModel: "m", MaxTokens: 100, ContextWindow: 200000,
		Providers: map[string]config.Provider{"test": {Type: "anthropic", BaseURL: server.URL}}}
	a := Agent{Cfg: cfg, Root: t.TempDir(), Client: server.Client()}
	err := a.Send(context.Background(), "q", "ask", func(string, map[string]any) bool { return true }, func(Event) {})
	if err != nil {
		t.Fatal(err)
	}
	assistant := a.history[len(a.history)-1]
	blocks, ok := assistant["content"].([]map[string]any)
	if !ok || len(blocks) != 1 {
		t.Fatalf("expected only non-thinking blocks stored, got %#v", assistant["content"])
	}
	if fmt.Sprint(blocks[0]["type"]) != "text" {
		t.Fatalf("unexpected stored block type: %v", blocks[0]["type"])
	}
}

func TestCompactionTrimsOldToolOutput(t *testing.T) {
	a := &Agent{}
	huge := strings.Repeat("x", 300_000)
	a.history = []map[string]any{
		{"role": "system", "content": "sys"},
		{"role": "user", "content": "hi"},
		{"role": "assistant", "content": nil, "tool_calls": []any{}},
		{"role": "tool", "tool_call_id": "1", "content": huge},
	}
	a.compactHistory()
	got := fmt.Sprint(a.history[3]["content"])
	if len(got) >= len(huge) {
		t.Fatalf("compaction did not trim old tool output (len=%d)", len(got))
	}
	if !strings.Contains(got, "truncated") {
		t.Fatalf("expected truncation marker, got suffix %q", got[len(got)-80:])
	}
}

// Usage events must carry split token counts plus a computed USD delta.
func TestUsageEventsCarryCost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"ok"}}]}` + "\n\n"))
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{}}],"usage":{"prompt_tokens":1000,"completion_tokens":500,"total_tokens":1500}}` + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()
	cfg := config.Config{CurrentProvider: "p", CurrentModel: "glm-5.3", MaxTokens: 100, ContextWindow: 200000,
		Providers: map[string]config.Provider{"p": {Type: "openai", BaseURL: server.URL}}}
	a := Agent{Cfg: cfg, Root: t.TempDir(), Client: server.Client()}
	var usage *Event
	err := a.Send(context.Background(), "hi", "build", func(string, map[string]any) bool { return true },
		func(e Event) {
			if e.Kind == "usage" {
				u := e
				usage = &u
			}
		})
	if err != nil {
		t.Fatal(err)
	}
	if usage == nil || usage.InputTok != 1000 || usage.OutputTok != 500 {
		t.Fatalf("usage tokens wrong: %+v", usage)
	}
	// glm-5.3 rate: $0.60/M in, $2.50/M out → (1000*0.60 + 500*2.50)/1e6
	want := (1000*0.60 + 500*2.50) / 1_000_000
	if math.Abs(usage.CostUSD-want) > 1e-9 {
		t.Fatalf("cost = %v, want %v", usage.CostUSD, want)
	}
}

// Tool results pass through the redactor before reaching history or events.
func TestToolResultsAreRedactedEndToEnd(t *testing.T) {
	secret := "SKTEST-abc123-secret-value"
	secrets.Register(secret)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "leak.txt"), []byte("token="+secret), 0600); err != nil {
		t.Fatal(err)
	}

	var requests int
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		first := requests == 1
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		if !first {
			_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"done"}}]}` + "\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
			return
		}
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","function":{"name":"read_file","arguments":"{\"path\":\"leak.txt\"}"}}]}}]}` + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	cfg := config.Config{CurrentProvider: "p", CurrentModel: "m", MaxTokens: 100, ContextWindow: 200000,
		Providers: map[string]config.Provider{"p": {Type: "openai", BaseURL: server.URL}}}
	a := Agent{Cfg: cfg, Root: root, Client: server.Client()}
	var sawRedacted bool
	err := a.Send(context.Background(), "read it", "build", func(string, map[string]any) bool { return true },
		func(e Event) {
			if e.Kind == "tool_done" && strings.Contains(e.ToolResult, "[redacted]") {
				sawRedacted = true
			}
			if e.ToolResult != "" && strings.Contains(e.ToolResult, secret) {
				t.Errorf("secret leaked into event stream")
			}
		})
	if err != nil {
		t.Fatal(err)
	}
	if !sawRedacted {
		t.Fatal("expected tool result to be redacted")
	}
	for _, msg := range a.History() {
		b, _ := json.Marshal(msg)
		if strings.Contains(string(b), secret) {
			t.Fatal("secret leaked into stored history")
		}
	}
}

// ask_user pauses the tool loop and returns the UI's answer as the result.
func TestAskUserReturnsUIAnswer(t *testing.T) {
	a := &Agent{}
	out := make(chan string, 1)
	var askEv *Event
	go func() {
		result, err := a.runTool(context.Background(), "ask_user",
			map[string]any{"question": "pick one"}, "id1",
			func(string, map[string]any) bool { return false },
			func(e Event) {
				if e.Kind == "ask" {
					got := e
					askEv = &got
					go func() {
						time.Sleep(20 * time.Millisecond)
						got.Answer <- "yes"
					}()
				}
			})
		if err != nil {
			t.Error(err)
		}
		out <- result
	}()
	select {
	case r := <-out:
		if r != "User answered: yes" {
			t.Fatalf("ask_user result = %q, want %q", r, "User answered: yes")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("ask_user never returned")
	}
	if askEv == nil {
		t.Fatal("ask event was not emitted")
	}
}

// A cancelled context unblocks ask_user with an error result.
func TestAskUserCancelledContext(t *testing.T) {
	a := &Agent{}
	ctx, cancel := context.WithCancel(context.Background())
	out := make(chan string, 1)
	go func() {
		result, _ := a.runTool(ctx, "ask_user", map[string]any{"question": "q"}, "id2",
			func(string, map[string]any) bool { return true }, func(Event) {})
		out <- result
	}()
	time.Sleep(30 * time.Millisecond)
	cancel()
	select {
	case r := <-out:
		if !strings.Contains(r, "cancelled") {
			t.Fatalf("cancelled ask result = %q", r)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("ask_user did not unblock on cancellation")
	}
}
