package ui

import (
	"testing"

	"github.com/SSNamahsos/Mihani-Code/internal/agent"
	"github.com/SSNamahsos/Mihani-Code/internal/config"
)

// Mode routing: build work goes to the tool-capable provider (Mihani Cloud),
// read-only chat (plan/research/ask) goes to the chat provider (Mihani Pro),
// and a deliberately-chosen custom provider is never yanked by a mode switch.
func TestApplyModeRouting(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	m := newTestModel(100, 30)
	m.cfg = cfg
	m.agent = &agent.Agent{}

	m.modeIndex = modeIndex("build")
	m.applyMode()
	if m.cfg.CurrentProvider != config.BuiltinPrimary {
		t.Fatalf("build should route to Mihani Cloud, got %q", m.cfg.CurrentProvider)
	}

	m.modeIndex = modeIndex("ask")
	m.applyMode()
	if m.cfg.CurrentProvider != config.BuiltinSecondary {
		t.Fatalf("ask should route to Mihani Pro, got %q", m.cfg.CurrentProvider)
	}
	if want := cfg.ModelFor(config.BuiltinSecondary); m.cfg.CurrentModel != want {
		t.Fatalf("ask model = %q, want Pro's %q", m.cfg.CurrentModel, want)
	}

	// Custom provider is preserved across a mode change.
	cfg.Providers["ollama"] = config.Provider{
		Label: "Ollama", Type: "openai", BaseURL: "http://localhost:11434/v1", Models: []string{"llama"},
	}
	m.cfg = cfg
	m.cfg.CurrentProvider = "ollama"
	m.cfg.CurrentModel = "llama"
	m.modeIndex = modeIndex("build")
	m.applyMode()
	if m.cfg.CurrentProvider != "ollama" {
		t.Fatalf("custom provider must be preserved, got %q", m.cfg.CurrentProvider)
	}
}

func modeIndex(name string) int {
	for i, mo := range modes {
		if mo.name == name {
			return i
		}
	}
	return 0
}
