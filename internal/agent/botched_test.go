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
