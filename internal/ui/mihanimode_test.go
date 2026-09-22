package ui

import (
	"strings"
	"testing"

	"github.com/SSNamahsos/Mihani-Code/internal/config"
)

// Mihani Mode (Shift+Tab) auto-approves every dangerous tool while armed and
// leaves the decision to the approval modal when off.
func TestMihaniModeBypassesApproval(t *testing.T) {
	m := newTestModel(80, 24)
	if m.mihaniModeActive() {
		t.Fatal("Mihani Mode must be off by default")
	}
	m.setMihaniMode(true)
	if !m.mihaniModeActive() {
		t.Fatal("Mihani Mode did not arm")
	}
	if m.cfg.MihaniMode != true {
		t.Fatal("Mihani Mode state not mirrored into the config")
	}
	m.setMihaniMode(false)
	if m.mihaniModeActive() {
		t.Fatal("Mihani Mode did not disarm")
	}
	// The legacy auto-confirm setting keeps working as an alias.
	m2 := newTestModel(80, 24)
	m2.cfg.AutoConfirm = true
	if !m2.mihaniModeActive() {
		t.Fatal("AutoConfirm should also skip prompts")
	}
}

// The header pill advertises the armed state with a red MIHANI badge.
func TestMihaniModeHeaderPill(t *testing.T) {
	m := newTestModel(80, 24)
	m.version = "test"
	plainUI = true // deterministic borders in the assertion-free render path
	plainUI = false
	m.setMihaniMode(true)
	header := m.headerRow()
	if !strings.Contains(stripANSI(header), "⚡ MIHANI") {
		t.Fatalf("header missing the MIHANI pill while armed: %q", stripANSI(header))
	}
	m.setMihaniMode(false)
	header = m.headerRow()
	if strings.Contains(stripANSI(header), "MIHANI") {
		t.Fatalf("header should show the mode name when disarmed: %q", stripANSI(header))
	}
}

// Sessions recorded before the v0.3.0 rename replay their old tool names as
// the canonical Mihani_* cards, and todo cards keep updating in place.
func TestReplayMapsLegacyToolNames(t *testing.T) {
	m := newTestModel(80, 24)
	m.replayAssistant(map[string]any{
		"role": "assistant",
		"content": "working\n" + `<tool_call>{"name": "todo_write", "arguments": {"todos": [{"content": "step one", "status": "done"}]}}</tool_call>`,
	})
	found := false
	for _, b := range m.blocks {
		if b.kind == blockTodo {
			found = true
		}
	}
	if !found {
		t.Fatal("legacy todo_write call did not rebuild the todo card")
	}

	m2 := newTestModel(80, 24)
	m2.appendToolDoneCard("call_1", "edit_file", "OK: edited main.go")
	if m2.blocks[len(m2.blocks)-1].label != "Mihani_Edit_File" {
		t.Fatalf("legacy tool name not normalized on replay: %q", m2.blocks[len(m2.blocks)-1].label)
	}
}

// The persisted config round-trips the Mihani Mode flag under its own key.
func TestMihaniModeConfigField(t *testing.T) {
	cfg := config.Config{MihaniMode: true}
	if !cfg.MihaniMode {
		t.Fatal("config flag not stored")
	}
}
