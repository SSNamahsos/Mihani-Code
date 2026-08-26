package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/SSNamahsos/Mihani-Code/internal/config"
	"github.com/SSNamahsos/Mihani-Code/internal/tools"
)

// Text-based tool protocol for endpoints that do not support OpenAI function
// calling. The model replies with <tool_call> JSON blocks inside ordinary
// messages; Mihani executes them locally and feeds back <tool_result> blocks,
// looping until the model answers without calling anything.

const promptToolProtocol = `# Tool use protocol

You control real local tools. To run one, end your reply with exactly one block:

<tool_call>{"name": "tool_name", "arguments": { ... }}</tool_call>

Rules:
- The block must contain valid JSON on a single line.
- One tool call per reply. Wait for the result before deciding the next step.
- Every call receives a reply in the form:
  <tool_result name="..." status="ok|error">output</tool_result>
- Never invent results. Never fabricate a tool_result yourself.
- When the task is complete (or no tool is needed), answer in plain prose and
  include NO tool_call block.`

// toolCatalog renders the available tools for prompt-based providers.
func (a *Agent) toolCatalog() string {
	var b strings.Builder
	for _, t := range toolEntries(a) {
		params, _ := json.Marshal(t.Schema)
		fmt.Fprintf(&b, "- %s: %s\n  arguments: %s\n", t.Name, t.Description, truncateForPrompt(string(params), 300))
	}
	return b.String()
}

type namedTool struct {
	Name        string
	Description string
	Schema      map[string]any
}

func toolEntries(a *Agent) []namedTool {
	out := make([]namedTool, 0, len(tools.Registry)+len(a.mcpTools))
	for _, t := range tools.Registry {
		out = append(out, namedTool{Name: t.Name, Description: t.Description, Schema: t.Schema})
	}
	for _, t := range a.mcpTools {
		name, _ := t["name"].(string)
		desc, _ := t["description"].(string)
		schema, _ := t["input_schema"].(map[string]any)
		out = append(out, namedTool{Name: name, Description: desc, Schema: schema})
	}
	return out
}

func truncateForPrompt(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// openAIChatText streams a plain-chat completion (no tools parameter) and
// returns the assistant text plus billed token counts.
func (a *Agent) openAIChatText(ctx context.Context, p config.Provider, emit func(Event)) (string, int, int, int, error) {
	assistant, _, apiIn, apiOut, responseChars, err := a.openAIRequest(ctx, p, false, emit)
	if err != nil {
		return "", 0, 0, 0, err
	}
	content, _ := assistant["content"].(string)
	return content, apiIn, apiOut, responseChars, nil
}

var (
	taggedCallRe   = regexp.MustCompile(`(?s)<tool_call>\s*(\{.*?\})\s*</tool_call>`)
	fencedCallRe   = regexp.MustCompile("(?s)```(?:tool_call|json)?\\s*\n(\\{.*?\\})\n```")
	looseJSONRe    = regexp.MustCompile(`(?s)\{\s*"name"\s*:\s*"[^"]+"\s*,\s*"arguments"\s*:\s*\{.*?\}\s*\}`)
	whitespaceOnly = regexp.MustCompile(`^\s*$`)
)

// extractToolCalls pulls every well-formed tool call out of an assistant
// message, merging tagged and fenced blocks in the order they appear, then
// falling back to a bare JSON object heuristic.
func extractToolCalls(text string) ([]toolCall, error) {
	blocks := orderedMatches([]matcher{
		{re: taggedCallRe, group: 1},
		{re: fencedCallRe, group: 1},
	}, text)
	if len(blocks) == 0 {
		trimmed := strings.TrimSpace(stripTags(text))
		if looseJSONRe.MatchString(trimmed) {
			blocks = looseJSONRe.FindAllString(trimmed, -1)
		}
	}
	if len(blocks) == 0 {
		return nil, nil
	}
	calls := make([]toolCall, 0, len(blocks))
	var errs []string
	for i, block := range blocks {
		var payload struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal([]byte(block), &payload); err != nil || payload.Name == "" {
			errs = append(errs, fmt.Sprintf("call %d: invalid JSON or missing name", i+1))
			continue
		}
		args := payload.Arguments
		if args == nil {
			args = map[string]any{}
		}
		calls = append(calls, toolCall{ID: fmt.Sprintf("prompt_%d", i+1), Name: payload.Name, Input: args})
	}
	if len(calls) == 0 && len(errs) > 0 {
		return nil, fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return calls, nil
}

type matcher struct {
	re    *regexp.Regexp
	group int
}

type matchHit struct {
	start   int
	payload string
}

// orderedMatches collects capture-group matches from every pattern and sorts
// them by position so multi-style messages keep their sequence.
func orderedMatches(patterns []matcher, text string) []string {
	var hits []matchHit
	for _, p := range patterns {
		for _, m := range p.re.FindAllStringSubmatchIndex(text, -1) {
			gi := p.group * 2
			if gi+1 >= len(m) || m[gi] < 0 {
				continue
			}
			hits = append(hits, matchHit{start: m[gi], payload: text[m[gi]:m[gi+1]]})
		}
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].start < hits[j].start })
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.payload)
	}
	return out
}

// StripToolCalls removes call blocks so prose can be shown cleanly.
func StripToolCalls(text string) string {
	out := taggedCallRe.ReplaceAllString(text, "")
	out = fencedCallRe.ReplaceAllString(out, "")
	return out
}

func stripTags(text string) string {
	return strings.ReplaceAll(strings.ReplaceAll(text, "<tool_call>", ""), "</tool_call>", "")
}

// sendPromptBased runs the full ReAct-style loop over a plain-chat endpoint:
// request → parse tool calls from text → execute locally → feed results back.
func (a *Agent) sendPromptBased(ctx context.Context, p config.Provider, prompt, mode string, approve func(string, map[string]any) bool, emit func(Event)) error {
	system := SystemPrompt(mode, a.Root) + "\n\n" + promptToolProtocol + "\n\nAvailable tools:\n" + a.toolCatalog()
	a.history = append(a.history,
		map[string]any{"role": "system", "content": system},
		map[string]any{"role": "user", "content": prompt},
	)
	for iteration := 1; iteration <= a.iterations(); iteration++ {
		emit(Event{Kind: "activity", Text: "thinking", Iteration: iteration})
		a.compactHistory()
		content, apiIn, apiOut, responseChars, err := a.openAIChatText(ctx, p, emit)
		if err != nil {
			return err
		}
		a.trackUsage(apiIn, apiOut, responseChars, emit)

		calls, parseErr := extractToolCalls(content)
		if len(calls) == 0 {
			// No usable call: if the model attempted one and botched it, tell
			// it how to fix that; otherwise the turn is complete.
			if parseErr != nil && !whitespaceOnly.MatchString(content) {
				hint := "Reply with exactly one <tool_call>{\"name\": ..., \"arguments\": {...}}</tool_call> block."
				if strings.Contains(content, "node -e") || strings.Contains(content, "python -c") {
					hint = "Inline scripts with nested quotes break JSON. Instead: use write_file to save a temp .js/.py file, then run it with bash."
				} else if strings.Count(content, "\\\"") > 0 {
					hint = "Escape quotes carefully or prefer single quotes inside command strings; simpler commands parse more reliably."
				}
				a.history = append(a.history,
					map[string]any{"role": "assistant", "content": content},
					map[string]any{"role": "user", "content": fmt.Sprintf(
						"<tool_result status=\"error\">your tool call could not be parsed: %s\nYou wrote:\n%s\n%s</tool_result>",
						parseErr, clip(content, 400), hint)})
				continue
			}
			emit(Event{Kind: "done", Done: true})
			return nil
		}

		a.history = append(a.history, map[string]any{"role": "assistant", "content": content})
		var results strings.Builder
		for _, call := range calls {
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
		a.tokens += estimateTokens(results.String())
		a.history = append(a.history, map[string]any{"role": "user", "content": results.String()})
	}
	return fmt.Errorf("agent stopped after %d tool iterations", a.iterations())
}
