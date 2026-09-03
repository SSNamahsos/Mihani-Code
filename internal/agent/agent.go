package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/SSNamahsos/Mihani-Code/internal/config"
	"github.com/SSNamahsos/Mihani-Code/internal/gitx"
	"github.com/SSNamahsos/Mihani-Code/internal/mcp"
	"github.com/SSNamahsos/Mihani-Code/internal/pricing"
	"github.com/SSNamahsos/Mihani-Code/internal/providers"
	"github.com/SSNamahsos/Mihani-Code/internal/secrets"
	"github.com/SSNamahsos/Mihani-Code/internal/skills"
	"github.com/SSNamahsos/Mihani-Code/internal/tools"
)

type Event struct {
	Kind       string
	Text       string
	Tool       string
	Input      map[string]any
	Tokens     int
	MaxTokens  int
	InputTok   int     // input tokens billed for the latest request
	OutputTok  int     // output tokens billed for the latest request
	CostUSD    float64 // USD billed for the latest request (delta, not cumulative)
	ToolCallID string
	ToolResult string
	Iteration  int
	Done       bool
	Approval   chan bool
	Answer     chan string // ask_user: UI delivers the user's answer over this channel
}

type Agent struct {
	Cfg           config.Config
	Root          string
	Client        *http.Client
	MaxIterations int
	history       []map[string]any
	mcp           map[string]*mcp.Client
	mcpTools      []map[string]any
	tokens        int
	lastFinish    string // finish_reason of the latest request ("stop", "length", "tool_calls")
	lengthNudges  int    // per-turn continuation nudges after a "length" finish
	proseNudges   int    // per-turn nudges when the model dumps file content as text
}

func (a *Agent) Reset()                           { a.history = nil; a.tokens = 0 }
func (a *Agent) History() []map[string]any        { return a.history }
func (a *Agent) Restore(history []map[string]any) { a.history = history }
func (a *Agent) Tokens() int                      { return a.tokens }

// TrimHistory reverts the stored conversation to n entries, dropping the
// partial work of a failed turn so a retry re-sends the prompt cleanly.
func (a *Agent) TrimHistory(n int) {
	if n < 0 {
		n = 0
	}
	if n < len(a.history) {
		a.history = a.history[:n]
	}
}

func (a *Agent) iterations() int {
	if a.MaxIterations > 0 {
		return a.MaxIterations
	}
	return 40
}

// Send runs one full agent turn. mode is one of build, plan, research, ask.
func (a *Agent) Send(ctx context.Context, prompt, mode string, approve func(string, map[string]any) bool, emit func(Event)) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	pricing.SetOverrides(a.Cfg.Pricing)
	a.tokens += estimateTokens(prompt)
	a.lengthNudges = 0 // continuation budget is per turn
	a.proseNudges = 0  // prose-dump nudge budget is per turn
	if err := a.ensureMCP(ctx, emit); err != nil {
		emit(Event{Kind: "activity", Text: "MCP unavailable: " + err.Error()})
	}
	p := a.Cfg.Providers[a.Cfg.CurrentProvider]
	if !p.UseNativeTools() {
		return a.sendPromptBased(ctx, p, prompt, mode, approve, emit)
	}
	if p.Type == "anthropic" {
		return a.sendAnthropic(ctx, p, prompt, mode, approve, emit)
	}
	return a.sendOpenAI(ctx, p, prompt, mode, approve, emit)
}

func (a *Agent) sendOpenAI(ctx context.Context, p config.Provider, prompt, mode string, approve func(string, map[string]any) bool, emit func(Event)) error {
	system := map[string]any{"role": "system", "content": SystemPrompt(mode, a.Root)}
	if len(a.history) == 0 || a.history[0]["role"] != "system" {
		a.history = append([]map[string]any{system}, a.history...)
	} else {
		a.history[0] = system
	}
	a.history = append(a.history, map[string]any{"role": "user", "content": prompt})
	for iteration := 1; iteration <= a.iterations(); iteration++ {
		emit(Event{Kind: "activity", Text: "thinking", Iteration: iteration})
		a.compactHistory()
		assistant, calls, apiIn, apiOut, responseChars, err := a.openAIRequest(ctx, p, true, emit)
		if err != nil {
			// The response hit the max output token limit: the model was cut
			// off (often mid tool call, leaving unparseable arguments that
			// used to fail the whole turn). Continue the same turn with a
			// nudge to split work into smaller chunks — the user sees the
			// agent carry on instead of a dead red error.
			if errors.Is(err, errTruncated) && a.lastFinish == "length" && a.lengthNudges < 3 {
				a.lengthNudges++
				a.history = append(a.history, map[string]any{"role": "user", "content": "Your previous reply was cut off at the maximum output token limit. " +
					"Continue the task from where it stopped. Split large file writes into several smaller write_file/edit_file calls so one reply never exceeds the limit."})
				emit(Event{Kind: "activity", Text: "continuing (previous reply hit the token limit)"})
				continue
			}
			return err
		}
		a.trackUsage(apiIn, apiOut, responseChars, emit)
		a.history = append(a.history, assistant)
		if len(calls) == 0 {
			// Defense in depth #2 (build mode): some models still dump file
			// content as plain text instead of calling write_file — the exact
			// "here, save this as index.html" failure mode. A reply cut off
			// at the output token limit is treated the same: it almost always
			// means the model is writing a whole file in one giant reply.
			// Nudge them back to the tools in smaller chunks; the bounded
			// budget prevents nudge loops.
			if c, _ := assistant["content"].(string); c != "" && mode == "build" && a.proseNudges < 3 &&
				(looksLikeCodeDump(c) || (a.lastFinish == "length" && len(c) > 1000)) {
				a.proseNudges++
				reason := "it looks like pasted file content"
				if a.lastFinish == "length" {
					reason = "it was cut off at the maximum output token limit"
				}
				a.history = append(a.history, map[string]any{"role": "user", "content": "Your previous reply " + reason + ". Do not paste file content in chat. " +
					"Use the write_file / edit_file tools to create or modify files instead, " +
					"one file per call, in smaller chunks for large files. Continue the task."})
				emit(Event{Kind: "activity", Text: "nudging: use file tools instead of pasting code"})
				continue
			}
			if c, _ := assistant["content"].(string); c != "" {
				if textCalls, _ := extractToolCalls(c); len(textCalls) > 0 {
					emit(Event{Kind: "activity", Text: "parsing tool call from reply text"})
					var results strings.Builder
					for _, call := range textCalls {
						result, runErr := a.runTool(ctx, call.Name, call.Input, call.ID, approve, emit)
						if runErr != nil {
							return runErr
						}
						status := "ok"
						if strings.HasPrefix(result, "ERROR") || strings.Contains(result, "User declined") {
							status = "error"
						}
						fmt.Fprintf(&results, "<tool_result name=%q id=%q status=%q>\n%s\n</tool_result>\n",
							call.Name, call.ID, status, clip(result, historyToolLimit))
					}
					a.history = append(a.history, map[string]any{"role": "user", "content": results.String()})
					continue
				}
			}
			if c, _ := assistant["content"].(string); strings.TrimSpace(c) == "" {
				// Empty stream that ended at the token limit: continue the
				// turn with a nudge instead of a dead "empty response" error.
				if a.lastFinish == "length" && a.lengthNudges < 3 {
					a.lengthNudges++
					a.history = append(a.history, map[string]any{"role": "user", "content": "Your previous reply was empty and the stream ended at the maximum output token limit. " +
						"Continue: finish the task in smaller steps so each reply stays under the limit."})
					emit(Event{Kind: "activity", Text: "continuing (previous reply hit the token limit)"})
					continue
				}
				// A completed stream with no text and no tool calls means the
				// upstream dropped the response; retry via the reconnect loop
				// instead of showing a silent blank reply.
				return errEmptyResponse
			}
			emit(Event{Kind: "done", Done: true})
			return nil
		}
		for _, call := range calls {
			result, err := a.runTool(ctx, call.Name, call.Input, call.ID, approve, emit)
			if err != nil {
				return err
			}
			a.history = append(a.history, map[string]any{"role": "tool", "tool_call_id": call.ID, "content": clip(result, historyToolLimit)})
		}
	}
	return fmt.Errorf("agent stopped after %d tool iterations", a.iterations())
}

type toolCall struct {
	ID, Name string
	Input    map[string]any
}

func (a *Agent) openAIRequest(ctx context.Context, p config.Provider, useTools bool, emit func(Event)) (map[string]any, []toolCall, int, int, int, error) {
	body := map[string]any{
		"model":          a.Cfg.CurrentModel,
		"max_tokens":     a.Cfg.MaxTokens,
		"stream":         true,
		"messages":       a.history,
		"stream_options": map[string]any{"include_usage": true},
	}
	if effort := a.Cfg.CurrentEffort(); effort != "" {
		body["reasoning_effort"] = effort
	}
	if useTools {
		body["tools"] = a.openAITools()
	}
	b, err := json.Marshal(body)
	if err != nil {
		return nil, nil, 0, 0, 0, err
	}
	base := providers.NormalizeBaseURL(p.BaseURL)
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/chat/completions", bytes.NewReader(b))
	if err != nil {
		return nil, nil, 0, 0, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if key := a.Cfg.Key(a.Cfg.CurrentProvider); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := a.client().Do(req)
	if err != nil {
		return nil, nil, 0, 0, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, nil, 0, 0, 0, providerError(resp)
	}
	// Gateways whose base URL is missing the API prefix answer chat requests
	// with their web app (200 + HTML) instead of a JSON stream. Failing loudly
	// beats a silent empty reply that looks like a cancelled turn.
	if ct := resp.Header.Get("Content-Type"); strings.Contains(ct, "text/html") {
		return nil, nil, 0, 0, 0, fmt.Errorf("provider answered with a web page instead of a model stream — check the base URL for %s (it should point at the API, usually ending in /v1)", a.Cfg.CurrentProvider)
	}
	var content strings.Builder
	calls := map[int]*toolCallState{}
	promptTokens, completionTokens, totalTokens := 0, 0, 0
	responseChars := 0
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			continue
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Role             string `json:"role"`
					Content          string `json:"content"`
					ReasoningContent string `json:"reasoning_content"`
					Reasoning        string `json:"reasoning"`
					ToolCalls        []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Usage struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal([]byte(data), &chunk) != nil {
			continue
		}
		if chunk.Usage.TotalTokens > 0 || chunk.Usage.PromptTokens > 0 {
			promptTokens = chunk.Usage.PromptTokens
			completionTokens = chunk.Usage.CompletionTokens
			totalTokens = chunk.Usage.TotalTokens
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta
		if chunk.Choices[0].FinishReason != "" {
			a.lastFinish = chunk.Choices[0].FinishReason
		}
		if delta.ReasoningContent != "" || delta.Reasoning != "" {
			part := delta.ReasoningContent + delta.Reasoning
			responseChars += len(part)
			emit(Event{Kind: "reasoning", Text: part})
		}
		if delta.Content != "" {
			content.WriteString(delta.Content)
			responseChars += len(delta.Content)
			emit(Event{Kind: "text", Text: delta.Content})
		}
		for _, deltaCall := range delta.ToolCalls {
			state := calls[deltaCall.Index]
			if state == nil {
				state = &toolCallState{}
				calls[deltaCall.Index] = state
			}
			if deltaCall.ID != "" {
				state.ID = deltaCall.ID
			}
			if deltaCall.Function.Name != "" && state.Name == "" {
				state.Name = deltaCall.Function.Name
			}
			state.Arguments += deltaCall.Function.Arguments
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, 0, 0, 0, err
	}
	assistant := map[string]any{"role": "assistant", "content": nil}
	if content.Len() > 0 {
		assistant["content"] = content.String()
	}
	// Preserve tool call order: Go maps are unordered, so sort by index.
	indexes := make([]int, 0, len(calls))
	for index := range calls {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	var result []toolCall
	var rawCalls []map[string]any
	for _, index := range indexes {
		state := calls[index]
		input := map[string]any{}
		if err := json.Unmarshal([]byte(state.Arguments), &input); err != nil {
			// The stream ended mid tool call: partial JSON arguments are not
			// a valid payload for any tool. A sentinel error lets the caller
			// decide: "length" finish gets an in-turn continuation nudge,
			// anything else retries via the reconnect loop.
			return nil, nil, 0, 0, 0, fmt.Errorf("%w: %s", errTruncated, state.Name)
		}
		id := state.ID
		if id == "" {
			id = fmt.Sprintf("call_%d_%d", index, time.Now().UnixNano())
		}
		result = append(result, toolCall{ID: id, Name: state.Name, Input: input})
		rawCalls = append(rawCalls, map[string]any{"id": id, "type": "function", "function": map[string]any{"name": state.Name, "arguments": state.Arguments}})
	}
	if len(rawCalls) > 0 {
		assistant["tool_calls"] = rawCalls
	}
	if totalTokens == 0 {
		totalTokens = promptTokens + completionTokens
	}
	return assistant, result, promptTokens, completionTokens, responseChars, nil
}

type toolCallState struct{ ID, Name, Arguments string }

func (a *Agent) sendAnthropic(ctx context.Context, p config.Provider, prompt, mode string, approve func(string, map[string]any) bool, emit func(Event)) error {
	if len(a.history) == 0 {
		a.history = []map[string]any{}
	}
	a.history = append(a.history, map[string]any{"role": "user", "content": prompt})
	for iteration := 1; iteration <= a.iterations(); iteration++ {
		emit(Event{Kind: "activity", Text: "thinking", Iteration: iteration})
		a.compactHistory()
		message, calls, apiIn, apiOut, responseChars, err := a.anthropicRequest(ctx, p, mode, emit)
		if err != nil {
			return err
		}
		a.trackUsage(apiIn, apiOut, responseChars, emit)
		a.history = append(a.history, message)
		if len(calls) == 0 {
			emit(Event{Kind: "done", Done: true})
			return nil
		}
		results := make([]map[string]any, 0, len(calls))
		for _, call := range calls {
			result, runErr := a.runTool(ctx, call.Name, call.Input, call.ID, approve, emit)
			if runErr != nil {
				return runErr
			}
			results = append(results, map[string]any{"type": "tool_result", "tool_use_id": call.ID, "content": clip(result, historyToolLimit)})
		}
		a.history = append(a.history, map[string]any{"role": "user", "content": results})
	}
	return fmt.Errorf("agent stopped after %d tool iterations", a.iterations())
}

func (a *Agent) anthropicRequest(ctx context.Context, p config.Provider, mode string, emit func(Event)) (map[string]any, []toolCall, int, int, int, error) {
	body := map[string]any{
		"model":      a.Cfg.CurrentModel,
		"max_tokens": a.Cfg.MaxTokens,
		"stream":     true,
		"system":     SystemPrompt(mode, a.Root),
		"messages":   a.history,
		"tools":      a.anthropicTools(),
	}
	b, err := json.Marshal(body)
	if err != nil {
		return nil, nil, 0, 0, 0, err
	}
	base := strings.TrimRight(p.BaseURL, "/")
	if base == "" {
		base = "https://api.anthropic.com/v1"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/messages", bytes.NewReader(b))
	if err != nil {
		return nil, nil, 0, 0, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", a.Cfg.Key(a.Cfg.CurrentProvider))
	req.Header.Set("anthropic-version", "2023-06-01")
	resp, err := a.client().Do(req)
	if err != nil {
		return nil, nil, 0, 0, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, nil, 0, 0, 0, providerError(resp)
	}
	var blocks []map[string]any
	var text strings.Builder
	var current *map[string]any
	responseChars := 0
	inputTokens, outputTokens := 0, 0
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		var event struct {
			Type         string         `json:"type"`
			Index        int            `json:"index"`
			ContentBlock map[string]any `json:"content_block"`
			Delta        map[string]any `json:"delta"`
			Message      struct {
				Usage struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			} `json:"message"`
			Usage struct {
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
			Error *struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal([]byte(data), &event) != nil {
			continue
		}
		switch event.Type {
		case "message_start":
			inputTokens = event.Message.Usage.InputTokens
		case "message_delta":
			if event.Usage.OutputTokens > 0 {
				outputTokens = event.Usage.OutputTokens
			}
		case "error":
			if event.Error != nil {
				return nil, nil, 0, 0, 0, fmt.Errorf("provider returned %s: %s", event.Error.Type, event.Error.Message)
			}
			return nil, nil, 0, 0, 0, fmt.Errorf("provider returned a stream error")
		case "content_block_start":
			block := event.ContentBlock
			blocks = append(blocks, block)
			current = &blocks[len(blocks)-1]
		case "content_block_delta":
			if event.Delta["type"] == "thinking_delta" {
				emit(Event{Kind: "reasoning", Text: fmt.Sprint(event.Delta["thinking"])})
			}
			if event.Delta["type"] == "text_delta" {
				part := fmt.Sprint(event.Delta["text"])
				text.WriteString(part)
				responseChars += len(part)
				emit(Event{Kind: "text", Text: part})
			}
			if event.Delta["type"] == "input_json_delta" && current != nil {
				(*current)["input_json"] = fmt.Sprint((*current)["input_json"]) + fmt.Sprint(event.Delta["partial_json"])
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, 0, 0, 0, err
	}
	// Drop unsigned thinking/redacted_thinking blocks: replaying them is a 400.
	stored := make([]map[string]any, 0, len(blocks))
	for _, block := range blocks {
		switch block["type"] {
		case "thinking", "redacted_thinking":
			continue
		}
		delete(block, "input_json") // never persist partial-json scratch fields
		stored = append(stored, block)
	}
	message := map[string]any{"role": "assistant", "content": stored}
	if len(stored) == 0 {
		message["content"] = text.String()
	}
	var calls []toolCall
	for _, block := range blocks {
		if block["type"] != "tool_use" {
			continue
		}
		input := map[string]any{}
		if raw, ok := block["input"].(map[string]any); ok {
			input = raw
		} else if raw := fmt.Sprint(block["input_json"]); raw != "<nil>" && raw != "" {
			_ = json.Unmarshal([]byte(raw), &input)
		}
		calls = append(calls, toolCall{ID: fmt.Sprint(block["id"]), Name: fmt.Sprint(block["name"]), Input: input})
	}
	return message, calls, inputTokens, outputTokens, responseChars, nil
}

const historyToolLimit = 6000

func (a *Agent) runTool(ctx context.Context, name string, input map[string]any, id string, approve func(string, map[string]any) bool, emit func(Event)) (string, error) {
	emit(Event{Kind: "tool_start", Tool: name, Input: input, ToolCallID: id})
	if name == "ask_user" {
		return a.askUser(ctx, input, emit)
	}
	definition := tools.Lookup(name)
	if preview := tools.Preview(name, input, a.Root); preview != "" {
		emit(Event{Kind: "tool_preview", Tool: name, ToolResult: secrets.Redact(preview), ToolCallID: id})
	}
	if definition.Dangerous && !approve(name, input) {
		result := "User declined to run this tool."
		emit(Event{Kind: "tool_done", Tool: name, ToolResult: result, ToolCallID: id, Input: input})
		return result, nil
	}
	var result string
	if strings.HasPrefix(name, "mcp.") {
		parts := strings.SplitN(strings.TrimPrefix(name, "mcp."), ".", 2)
		if len(parts) != 2 {
			result = "ERROR: invalid MCP tool name"
		} else {
			client := a.mcp[parts[0]]
			if client == nil {
				result = "ERROR: MCP server not connected"
			} else {
				data, err := client.Call(ctx, parts[1], input)
				if err != nil {
					result = "ERROR: " + err.Error()
				} else {
					result = string(data)
				}
			}
		}
	} else {
		result = tools.Runner{Root: a.Root}.Run(ctx, name, input)
	}
	// Never let a secret reach the transcript, the model history, or logs.
	result = secrets.Redact(result)
	emit(Event{Kind: "tool_done", Tool: name, ToolResult: result, ToolCallID: id, Input: input})
	return result, nil
}

// askUser pauses the agent loop and hands the question to the UI, which
// renders an answer menu. The user's choice is returned as the tool result so
// the model can continue the same turn (and ask again if it needs to).
func (a *Agent) askUser(ctx context.Context, input map[string]any, emit func(Event)) (string, error) {
	question, _ := input["question"].(string)
	if strings.TrimSpace(question) == "" {
		return "ERROR: ask_user requires a non-empty question", nil
	}
	answer := make(chan string, 1)
	emit(Event{Kind: "ask", Text: question, Input: input, Answer: answer})
	select {
	case a := <-answer:
		return "User answered: " + a, nil
	case <-ctx.Done():
		return "ERROR: the user cancelled the question; do not ask it again this turn", nil
	}
}

// compactHistory trims the oldest stored tool outputs once the raw history
// grows past its budget so long sessions do not overflow provider context.
func (a *Agent) compactHistory() {
	const budget = 240_000 // ~60k tokens of raw characters
	total := 0
	for _, m := range a.history {
		total += contentSize(m["content"])
	}
	if total <= budget {
		return
	}
	target := budget / 2
	for i := 0; i < len(a.history) && total > target; i++ {
		m := a.history[i]
		role, _ := m["role"].(string)
		if role == "tool" {
			if c, ok := m["content"].(string); ok && len(c) > 400 {
				total -= len(c) - 400
				m["content"] = clip(c, 400) + "\n[older tool output truncated to save context]"
			}
			continue
		}
		if role == "user" {
			for _, r := range toolResults(m["content"]) {
				if fmt.Sprint(r["type"]) != "tool_result" {
					continue
				}
				if c, ok := r["content"].(string); ok && len(c) > 400 {
					total -= len(c) - 400
					r["content"] = clip(c, 400) + "\n[older tool output truncated to save context]"
				}
			}
		}
	}
}

// toolResults normalizes Anthropic user content (either []map[string]any built
// in-process or []any decoded from a restored session) into its item maps.
func toolResults(v any) []map[string]any {
	var out []map[string]any
	switch c := v.(type) {
	case []map[string]any:
		out = c
	case []any:
		for _, item := range c {
			if m, ok := item.(map[string]any); ok {
				out = append(out, m)
			}
		}
	}
	return out
}

func contentSize(v any) int {
	switch c := v.(type) {
	case string:
		return len(c)
	case []map[string]any:
		n := 0
		for _, item := range c {
			n += contentSize(item["content"])
		}
		return n
	case []any:
		n := 0
		for _, item := range c {
			if m, ok := item.(map[string]any); ok {
				n += contentSize(m["content"])
			}
		}
		return n
	}
	return 0
}

// trackUsage meters context size and emits the billed delta for the latest
// request. Authoritative provider totals win over character estimates.
func (a *Agent) trackUsage(apiIn, apiOut, responseChars int, emit func(Event)) {
	if apiIn+apiOut > 0 {
		// The latest request's prompt+completion reflects current context size.
		a.tokens = apiIn + apiOut
	} else {
		a.tokens += responseChars / 4
	}
	window := a.Cfg.ContextWindow
	if window <= 0 {
		window = 200_000
	}
	cost := pricing.Cost(a.Cfg.CurrentModel, apiIn, apiOut)
	emit(Event{Kind: "usage", Tokens: a.tokens, MaxTokens: window, InputTok: apiIn, OutputTok: apiOut, CostUSD: cost})
}

func (a *Agent) client() *http.Client {
	if a.Client != nil {
		return a.Client
	}
	// Timeout so a hung upstream can never leave the UI "thinking" forever;
	// the request context still cancels immediately on user interrupt.
	return &http.Client{Timeout: 3 * time.Minute}
}

func (a *Agent) ensureMCP(ctx context.Context, emit func(Event)) error {
	if a.mcp != nil {
		return nil
	}
	a.mcp = map[string]*mcp.Client{}
	for _, server := range mcp.Discover(a.Root) {
		if server.Command == "" {
			continue
		}
		emit(Event{Kind: "activity", Text: "connecting MCP " + server.Name})
		client, err := mcp.Start(ctx, server)
		if err != nil {
			return fmt.Errorf("%s: %w", server.Name, err)
		}
		a.mcp[server.Name] = client
		list, err := client.Tools()
		if err != nil {
			return fmt.Errorf("%s tools: %w", server.Name, err)
		}
		for _, item := range list {
			a.mcpTools = append(a.mcpTools, map[string]any{"name": "mcp." + server.Name + "." + item.Name, "description": item.Description, "input_schema": item.InputSchema})
		}
	}
	return nil
}

func (a *Agent) Close() {
	for _, client := range a.mcp {
		_ = client.Close()
	}
	a.mcp = nil
}

// providerHTTPError wraps a non-2xx provider response so the UI can tell a
// retriable server failure (5xx/429/408) from an instant one (bad key, bad
// request) without parsing error strings.
type providerHTTPError struct {
	status  int
	message string
}

func (e *providerHTTPError) Error() string { return e.message }

// providerCreditError marks a provider response that says the account budget
// or credits are exhausted (402, or a 4xx whose body names credits/balance/
// quota). Never retriable: retrying repeats the same denial; the user must
// reset the local usage window, add a personal key, or switch provider.
type providerCreditError struct {
	status  int
	message string
}

func (e *providerCreditError) Error() string { return e.message }

// Sentinel stream failures. errTruncated is wrapped by errors.Is checks so
// the caller can branch on the finish reason instead of parsing strings.
var (
	errTruncated     = errors.New("provider response truncated mid tool-call")
	errEmptyResponse = errors.New("provider returned an empty response")
)

var creditWords = []string{"credit", "balance", "quota", "insufficient", "billing", "payment", "subscription", "arrear"}

func looksLikeCreditExhausted(status int, body string) bool {
	if status == 402 {
		return true
	}
	if status != 400 && status != 403 {
		return false
	}
	b := strings.ToLower(body)
	for _, w := range creditWords {
		if strings.Contains(b, w) {
			return true
		}
	}
	return false
}

// Retriable reports whether a failed turn is worth retrying. Transport
// failures (connection refused, timeout, reset) and server-side responses
// are; user cancellation and client errors (4xx: auth, model, payload) are
// not — retrying those just repeats the same failure ten times.
func Retriable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var creditErr *providerCreditError
	if errors.As(err, &creditErr) {
		return false // budget denial: retrying repeats the same failure
	}
	var httpErr *providerHTTPError
	if errors.As(err, &httpErr) {
		return httpErr.status == 408 || httpErr.status == 429 || httpErr.status >= 500
	}
	return true
}

func providerError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	bodyStr := string(body)
	var payload struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &payload) == nil && payload.Error.Message != "" {
		label := payload.Error.Type
		if label != "" {
			label += ": "
		}
		msg := fmt.Sprintf("provider returned %s — %s%s", resp.Status, label, payload.Error.Message)
		if looksLikeCreditExhausted(resp.StatusCode, msg) {
			return &providerCreditError{status: resp.StatusCode, message: msg +
				" The provider reports exhausted credits/budget — open /settings → “Reset usage window”, add your personal API key, or switch provider with /providers"}
		}
		return &providerHTTPError{status: resp.StatusCode, message: msg}
	}
	excerpt := strings.TrimSpace(bodyStr)
	msg := fmt.Sprintf("provider returned %s", resp.Status)
	if excerpt != "" {
		msg = fmt.Sprintf("provider returned %s — %s", resp.Status, clip(excerpt, 300))
	}
	if looksLikeCreditExhausted(resp.StatusCode, msg) {
		return &providerCreditError{status: resp.StatusCode, message: msg +
			" The provider reports exhausted credits/budget — open /settings → “Reset usage window”, add your personal API key, or switch provider with /providers"}
	}
	return &providerHTTPError{status: resp.StatusCode, message: msg}
}

func estimateTokens(s string) int {
	n := len(s) / 4
	if n < 1 && s != "" {
		return 1
	}
	return n
}

// clip truncates a string to at most n characters without cutting a rune in half.
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return tools.Truncate(s, n) + "\n…[truncated]"
}

// EffortLevels lists the reasoning-effort levels a model actually exposes.
// Plain non-reasoning models get only "none": offering levels they cannot
// use would be a fake knob.
func EffortLevels(model string) []string {
	if supportsEffort(model) {
		return []string{"none", "low", "medium", "high"}
	}
	return []string{"none"}
}

// supportsEffort recognizes current-generation reasoning model families by
// name prefix (any provider prefix before "/" is ignored).
func supportsEffort(model string) bool {
	m := strings.ToLower(model)
	if i := strings.LastIndex(m, "/"); i >= 0 {
		m = m[i+1:]
	}
	for _, prefix := range []string{
		"gpt-5", "o1", "o3", "o4", "o5", "o-",
		"deepseek-r1", "r1",
		"claude", "grok-4", "kimi-k",
		"glm-4.6", "glm-4.7", "glm-5",
		"minimax-m2", "minimax-m3",
		"qwen3", "qwq", "mimo-v", "kat-coder",
	} {
		if strings.HasPrefix(m, prefix) {
			return true
		}
	}
	return false
}

// looksLikeCodeDump heuristically detects a reply that pastes file content
// as prose (fenced code blocks or raw markup/JS long enough to be a real
// file) rather than using the file tools.
func looksLikeCodeDump(c string) bool {
	if len(c) < 3000 {
		return false
	}
	for _, marker := range []string{"```", "<!DOCTYPE", "<html", "function ", "const ", "def ", "import "} {
		if strings.Contains(c, marker) {
			return true
		}
	}
	return false
}

var modeGuidance = map[string]string{
	"":         "",
	"build":    "You may edit files and run commands to complete the task directly.",
	"plan":     "Do NOT modify files or run mutating commands. Explore the codebase, then propose a concrete implementation plan.",
	"research": "Do NOT modify files. Investigate code, docs, and options; report findings with references.",
	"ask":      "Answer questions only. Do NOT modify files or run mutating commands.",
}

// SystemPrompt builds the per-turn system prompt including environment facts,
// workspace context, and mode-specific guidance.
func SystemPrompt(mode, root string) string {
	var b strings.Builder
	b.WriteString(basePrompt)
	if guidance, ok := modeGuidance[strings.ToLower(mode)]; ok && guidance != "" {
		b.WriteString("\n\nMode: " + strings.ToLower(mode) + ". " + guidance)
	}
	if cwd, err := os.Getwd(); err == nil {
		fmt.Fprintf(&b, "\n\n# Environment\n- Working directory: %s", cwd)
	}
	fmt.Fprintf(&b, "\n- Platform: %s/%s", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(&b, "\n- Date: %s", time.Now().Format("2006-01-02"))
	if root != "" {
		ctx2, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if branch, err := gitx.Branch(ctx2, root); err == nil && branch != "" {
			fmt.Fprintf(&b, "\n- Git branch: %s", branch)
		}
	}
	if root != "" {
		b.WriteString(workspaceContext(root))
	}
	return b.String()
}

const basePrompt = "You are Mihani Code, a concise terminal coding agent. Inspect before editing. Explain changes briefly. Never claim a change was made unless a tool succeeded. Keep output practical. Prefer edit_file over rewriting whole files. NEVER paste full file content in your reply — create or modify files only with the write_file/edit_file tools (several smaller calls for large files); chat text is for explanations and summaries. Use markdown formatting in responses. For multi-step work (3+ steps), create a visible task list with the todo_write tool up front and update item statuses (pending, in_progress, done) as you progress. When a decision genuinely needs the user, use the ask_user tool with concrete options instead of guessing. You have exactly the tools listed in the tool section: never claim a tool is unavailable, never pretend to have tools that are not listed, and if a tool call is rejected, correct the arguments against the listed schema and continue rather than giving up."

func workspaceContext(root string) string {
	var b strings.Builder
	if found := skills.Discover(root); len(found) > 0 {
		b.WriteString("\n\n# Skills\nThe following skills are installed. When a task matches a skill's description, use the read_file tool to open its SKILL.md path and follow the instructions inside it before proceeding; do not guess at what a skill does.")
		for _, skill := range found {
			b.WriteString("\n- " + skill.Name + ": " + skill.Description + "  (file: " + skill.Path + ")")
		}
	}
	if servers := mcp.Discover(root); len(servers) > 0 {
		b.WriteString("\n\nMCP servers configured:")
		for _, server := range servers {
			b.WriteString("\n- " + server.Name)
		}
	}
	return b.String()
}

func (a *Agent) openAITools() []map[string]any {
	result := []map[string]any{}
	for _, tool := range tools.Registry {
		result = append(result, map[string]any{"type": "function", "function": map[string]any{"name": tool.Name, "description": tool.Description, "parameters": tool.Schema}})
	}
	for _, tool := range a.mcpTools {
		result = append(result, map[string]any{"type": "function", "function": map[string]any{"name": tool["name"], "description": tool["description"], "parameters": tool["input_schema"]}})
	}
	return result
}

func (a *Agent) anthropicTools() []map[string]any {
	result := []map[string]any{}
	for _, tool := range tools.Registry {
		result = append(result, map[string]any{"name": tool.Name, "description": tool.Description, "input_schema": tool.Schema})
	}
	result = append(result, a.mcpTools...)
	return result
}
