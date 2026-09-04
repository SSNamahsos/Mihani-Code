package agent

import (
	"testing"
)

// The exact (malformed) tool call the user reported: an aliased tag
// <Longcat_tool_call> with a closing </ask_user> and invalid JSON. It parses to
// ZERO valid tool calls, but must be recognized as a botched tool call so the
// turn gets a corrective nudge instead of silently stopping.
func TestBotchedToolCallDetection(t *testing.T) {
	sample := "<Longcat_tool_call>ask_user options\": [\"A markdown guide (technical but readable)\", " +
		"\"A screenplay/documentary script format\", \"A code-heavy tutorial with examples\", " +
		"\"A high-level overview document\"]</ask_user>"

	// It is NOT a valid tool call.
	calls, _ := extractToolCalls(sample)
	if len(calls) != 0 {
		t.Fatalf("malformed tool call parsed to %d calls, want 0", len(calls))
	}
	// But it looks like a botched tool call.
	if !looksLikeBotchedToolCall(sample) {
		t.Fatal("the malformed <Longcat_tool_call>...</ask_user> reply should be detected as a botched tool call")
	}
}

func TestLooksLikeBotchedToolCallCases(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{"I created the file.</write_file>", true},                 // closing tag for a known tool
		{"<Longcat_tool_call>{\"name\":\"bash\"}</Longcat_tool_call>", true}, // aliased tag
		{"Sure, here is a summary of the research.", false},         // normal prose
		{"The ask_user tool asks the user a question.", false},      // tool name in prose, no tag
	}
	for i, c := range cases {
		if got := looksLikeBotchedToolCall(c.text); got != c.want {
			t.Fatalf("case %d (%q): looksLikeBotchedToolCall = %v, want %v", i, c.text, got, c.want)
		}
	}
}

// Research mode must now permit writing (the user wants research to be able to
// read AND create files). Only the system-prompt guidance encodes this; verify
// it no longer forbids modification.
func TestResearchModeAllowsWriting(t *testing.T) {
	got, ok := modeGuidance["research"]
	if !ok {
		t.Fatal("missing research mode guidance")
	}
	if got == "Do NOT modify files. Investigate code, docs, and options; report findings with references." {
		t.Fatal("research mode still forbids modifying files")
	}
}

// Read-only modes (ask, plan) must hide the file/shell tools from the model;
// build and research keep the full set. Verified across the native, anthropic,
// and prompt-based tool listings.
func TestModeGatesFileTools(t *testing.T) {
	fileTools := []string{"write_file", "edit_file", "delete_file", "bash"}
	has := func(list []map[string]any, name string) bool {
		for _, m := range list {
			if n, _ := m["name"].(string); n == name {
				return true
			}
			if f, ok := m["function"].(map[string]any); ok {
				if n, _ := f["name"].(string); n == name {
					return true
				}
			}
		}
		return false
	}
	hasEntry := func(a *Agent, name string) bool {
		for _, e := range toolEntries(a) {
			if e.Name == name {
				return true
			}
		}
		return false
	}

	for _, ro := range []string{"ask", "plan"} {
		a := &Agent{mode: ro}
		for _, ft := range fileTools {
			if !a.hidesTool(ft) {
				t.Fatalf("mode %s should hide %s", ro, ft)
			}
			if has(a.openAITools(), ft) || has(a.anthropicTools(), ft) || hasEntry(a, ft) {
				t.Fatalf("mode %s still lists %s in the tool set", ro, ft)
			}
		}
		// Non-file tools stay available.
		if a.hidesTool("read_file") || a.hidesTool("ask_user") || a.hidesTool("web_search") {
			t.Fatalf("mode %s must keep read/search/ask tools", ro)
		}
		if !has(a.openAITools(), "read_file") || !hasEntry(a, "read_file") {
			t.Fatalf("mode %s should still list read_file", ro)
		}
	}

	for _, writable := range []string{"build", "research"} {
		a := &Agent{mode: writable}
		for _, ft := range fileTools {
			if a.hidesTool(ft) {
				t.Fatalf("mode %s should NOT hide %s", writable, ft)
			}
			if !has(a.openAITools(), ft) || !hasEntry(a, ft) {
				t.Fatalf("mode %s should list %s", writable, ft)
			}
		}
	}
}
