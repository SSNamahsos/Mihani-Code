package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SSNamahsos/Mihani-Code/internal/agent"
)

// "! command" runs in the workspace and appends the output as a block.
func TestShellPassthrough(t *testing.T) {
	m := newTestModel(80, 24)
	m.root = "."
	cmd := m.runShellPassthrough("echo mihani-shell-check")
	if cmd == nil {
		t.Fatal("expected a command to run")
	}
	msg := cmd()
	done, ok := msg.(shellDoneMsg)
	if !ok {
		t.Fatalf("expected shellDoneMsg, got %T", msg)
	}
	if !strings.Contains(done.output, "mihani-shell-check") {
		t.Fatalf("shell output missing: %q", done.output)
	}
	// Feeding the result through Update appends the output block.
	m.Update(done)
	last := m.blocks[len(m.blocks)-1]
	if !strings.Contains(stripANSI(last.renderInner(80, "")), "mihani-shell-check") {
		t.Fatalf("output block missing: %q", last.content)
	}
}

// "# note" appends the line to .mihani.md in the workspace.
func TestProjectMemoryAppend(t *testing.T) {
	dir := t.TempDir()
	m := newTestModel(80, 24)
	m.root = dir
	m.appendProjectMemory("always run go vet before committing")
	data, err := os.ReadFile(filepath.Join(dir, ".mihani.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "always run go vet before committing") {
		t.Fatalf("memory note missing: %q", string(data))
	}
	m.appendProjectMemory("second line")
	data, _ = os.ReadFile(filepath.Join(dir, ".mihani.md"))
	if !strings.Contains(string(data), "second line") {
		t.Fatalf("second note missing: %q", string(data))
	}
}

// @path mentions inline file contents for the model, capped and redacted.
func TestExpandMentions(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("the answer is 42"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "big.bin"), []byte(strings.Repeat("x", 20*1024)), 0644); err != nil {
		t.Fatal(err)
	}

	got := expandMentions("explain @notes.txt please", root)
	if !strings.Contains(got, `<file path="notes.txt">`) || !strings.Contains(got, "the answer is 42") {
		t.Fatalf("file content not inlined: %q", got)
	}

	// Oversized files stay as plain mentions.
	if got := expandMentions("read @big.bin", root); strings.Contains(got, "<file") {
		t.Fatalf("oversized file should not be inlined: %q", got)
	}

	// Unknown mentions stay untouched.
	if got := expandMentions("mail me at a@b.com", root); got != "mail me at a@b.com" {
		t.Fatalf("unknown mention mutated: %q", got)
	}
}

// ctrl+up / ctrl+down walk submitted prompts, preserving the draft.
func TestPromptHistory(t *testing.T) {
	m := newTestModel(80, 24)
	m.recordPrompt("first prompt")
	m.recordPrompt("second prompt")

	m.historyPrev()
	if got := m.input.Value(); got != "second prompt" {
		t.Fatalf("historyPrev = %q", got)
	}
	m.historyPrev()
	if got := m.input.Value(); got != "first prompt" {
		t.Fatalf("historyPrev again = %q", got)
	}
	m.input.SetValue("draft in progress")
	m.promptHistoryPos = -1
	m.promptDraft = "draft in progress"
	m.historyPrev()
	m.historyNext()
	if got := m.input.Value(); got != "draft in progress" {
		t.Fatalf("draft not restored: %q", got)
	}
}

// /compact force-trims stored tool output.
func TestCompactCommand(t *testing.T) {
	a := &agent.Agent{}
	a.Restore([]map[string]any{
		{"role": "tool", "content": strings.Repeat("x", 5000)},
	})
	summary := a.CompactNow()
	if !strings.Contains(summary, "freed") {
		t.Fatalf("unexpected summary: %q", summary)
	}
	total := 0
	for _, msg := range a.History() {
		if c, ok := msg["content"].(string); ok {
			total += len(c)
		}
	}
	if total > 600 {
		t.Fatalf("tool output not trimmed: %d chars", total)
	}
}

// /todos prints the latest todo card.
func TestTodosCommand(t *testing.T) {
	m := newTestModel(80, 24)
	m.command("/todos")
	last := m.blocks[len(m.blocks)-1]
	if !strings.Contains(last.content, "no todo list yet") {
		t.Fatalf("empty-state message missing: %q", last.content)
	}
	m.blocks = append(m.blocks, &block{kind: blockTodo, content: "✓ step one\ndetail: 1/1 done"})
	m.command("/todos")
	last = m.blocks[len(m.blocks)-1]
	if !strings.Contains(last.content, "step one") {
		t.Fatalf("todos not shown: %q", last.content)
	}
}
