package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func isolatedHome(t *testing.T) {
	t.Helper()
	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv("HOME", t.TempDir())
}

// Hard requirement: API keys must never be written to disk.
func TestSaveNeverPersistsAPIKeys(t *testing.T) {
	isolatedHome(t)
	cfg := defaults()
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path())
	if err != nil {
		t.Fatal(err)
	}
	content := string(raw)
	if strings.Contains(content, "sk-") {
		t.Fatal("an api key leaked into config.json")
	}
	if strings.Contains(content, "api_key") {
		t.Fatal("api_key field present in config.json")
	}
	for _, key := range []string{
		defaults().Providers[BuiltinPrimary].APIKey,
		defaults().Providers[BuiltinSecondary].APIKey,
	} {
		if key == "" {
			continue // placeholder blob: source-only build without credentials
		}
	}
}

// The product must never expose upstream endpoint names to users.
func TestBuiltinsUseNeutralBranding(t *testing.T) {
	cfg := defaults()
	for _, id := range []string{BuiltinPrimary, BuiltinSecondary} {
		p, ok := cfg.Providers[id]
		if !ok {
			t.Fatalf("missing built-in provider %s", id)
		}
		if !strings.Contains(p.Label, "Mihani") {
			t.Fatalf("built-in label leaks branding rules: %q", p.Label)
		}
	}
	if cfg.CurrentProvider != BuiltinPrimary || cfg.CurrentModel != "glm-5.3" {
		t.Fatalf("unexpected defaults: %s/%s", cfg.CurrentProvider, cfg.CurrentModel)
	}
	if label := cfg.ProviderLabel(); !strings.Contains(label, "Mihani") {
		t.Fatalf("ProviderLabel is not neutral: %q", label)
	}
}

func TestDefaultsContainShippedModels(t *testing.T) {
	cfg := defaults()
	want := map[string][]string{
		BuiltinPrimary:   {"glm-5.2", "glm-5.3", "kat-coder-pro-v2.5", "MiniMax-M3", "mimo-v2.5"},
		BuiltinSecondary: {"claude-opus-5", "claude-opus-4-8", "claude-fable-5", "claude-sonnet-5", "gpt-5.6-sol", "grok-4-5"},
	}
	for id, models := range want {
		got := cfg.Providers[id].Models
		if len(got) != len(models) {
			t.Fatalf("%s models = %v, want %v", id, got, models)
		}
	}
}

// Old config files (pre-builtins or legacy providers) must migrate away from
// endpoint-named ids while keeping whatever the user added.
func TestLoadMigratesBuiltinsAndKeepsCustom(t *testing.T) {
	isolatedHome(t)
	legacy := `{
	  "version": 1,
	  "current_provider": "anthropic",
	  "current_model": "whatever",
	  "providers": {"custom": {"label": "Mine", "type": "openai", "base_url": "https://x/v1", "models": ["a"]}}
	}`
	if err := os.MkdirAll(filepath.Dir(path()), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path(), []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Providers[BuiltinPrimary]; !ok {
		t.Fatal("primary builtin was not migrated in")
	}
	if _, ok := cfg.Providers[BuiltinSecondary]; !ok {
		t.Fatal("secondary builtin was not migrated in")
	}
	if _, ok := cfg.Providers["custom"]; !ok {
		t.Fatal("user provider was dropped during migration")
	}
	// Unknown current provider falls back to the primary built-in.
	if cfg.CurrentProvider != BuiltinPrimary {
		t.Fatalf("current provider not repointed: %q", cfg.CurrentProvider)
	}
	if cfg.Budget() != DefaultDailyBudgetUSD {
		t.Fatalf("default budget not applied: %v", cfg.Budget())
	}
}

// A config that still names an old endpoint id must be silently renamed and
// the legacy entry removed so it never appears in overlays again.
func TestLoadRenamesLegacyEndpointIDs(t *testing.T) {
	isolatedHome(t)
	legacy := `{
	  "version": 1,
	  "current_provider": "seekai",
	  "current_model": "claude-opus-5",
	  "providers": {}
	}`
	if err := os.MkdirAll(filepath.Dir(path()), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path(), []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CurrentProvider != BuiltinSecondary {
		t.Fatalf("seekai was not renamed to %s: %q", BuiltinSecondary, cfg.CurrentProvider)
	}
	if cfg.CurrentModel != "claude-opus-5" {
		t.Fatalf("valid current model should survive migration: %q", cfg.CurrentModel)
	}
	for _, legacyID := range []string{"hcnsec", "seekai"} {
		if _, exists := cfg.Providers[legacyID]; exists {
			t.Fatalf("legacy id %s still present after migration", legacyID)
		}
	}
}

func TestBudgetZeroMeansDefaultAndNegativeDisables(t *testing.T) {
	cfg := Config{}
	if cfg.Budget() != DefaultDailyBudgetUSD {
		t.Fatal("zero budget should fall back to the default")
	}
	cfg.BudgetUSD = -1
	if cfg.Budget() > 0 {
		t.Fatal("negative budget should disable enforcement")
	}
}

// The shared $10 credit belongs to the embedded keys: only built-in Mihani
// endpoints are capped; user-connected providers are never throttled.
func TestBudgetEnforcedOnlyForBuiltins(t *testing.T) {
	cfg := defaults()
	if !cfg.IsBuiltinProvider(BuiltinPrimary) || !cfg.IsBuiltinProvider(BuiltinSecondary) {
		t.Fatal("built-in ids not recognized")
	}
	if cfg.IsBuiltinProvider("my-own-provider") {
		t.Fatal("custom provider must not count as built-in")
	}
	if got := cfg.BudgetEnforced(BuiltinPrimary); got != DefaultDailyBudgetUSD {
		t.Fatalf("built-in should be capped at default, got %v", got)
	}
	if got := cfg.BudgetEnforced("my-own-provider"); got != 0 {
		t.Fatalf("custom provider should have no cap, got %v", got)
	}
}
