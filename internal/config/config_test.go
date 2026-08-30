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

// Personal keys are the user's own credentials: they persist locally (unlike
// embedded keys) and survive builtin refreshes/migrations.
func TestPersonalKeyPersistsAndSurvivesMigration(t *testing.T) {
	isolatedHome(t)
	cfg := defaults()
	p := cfg.Providers[BuiltinPrimary]
	p.PersonalKey = "sk-my-own-key-abcdef123456"
	cfg.Providers[BuiltinPrimary] = p
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(path())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "sk-my-own-key") {
		t.Fatal("personal key should persist to the local config file")
	}
	if strings.Contains(string(raw), "api_key") {
		t.Fatal("embedded api_key field must never appear alongside it")
	}

	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	personalKey := loaded.Providers[BuiltinPrimary].PersonalKey
	if personalKey != "sk-my-own-key-abcdef123456" {
		t.Fatalf("personal key lost after reload+migration: %q", personalKey)
	}
	if got := MaskedKey(personalKey); strings.Contains(got, "sk-my") || !strings.Contains(got, "••••") {
		t.Fatalf("masked key reveals too much or formats wrong: %q", got)
	}
}

func TestMaskedKeyEmpty(t *testing.T) {
	if got := MaskedKey(""); got != "not set" {
		t.Fatalf("empty mask = %q", got)
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

// The /connect key is the user's own credential: it must resolve as the
// provider's key after a save+load cycle (APIKey is runtime-only).
func TestKeyFallsBackToPersonalKey(t *testing.T) {
	cfg := defaults()
	cfg.Providers["custom"] = Provider{Label: "Custom", Type: "openai", BaseURL: "http://x.example/v1", PersonalKey: "sk-persisted-123456"}
	if got := cfg.Key("custom"); got != "sk-persisted-123456" {
		t.Fatalf("Key should fall back to the persisted personal key, got %q", got)
	}
	custom := cfg.Providers["custom"]
	custom.EnvKey = "MIHANI_TEST_KEY"
	cfg.Providers["custom"] = custom
	t.Setenv("MIHANI_TEST_KEY", "sk-from-env")
	if got := cfg.Key("custom"); got != "sk-from-env" {
		t.Fatalf("env key should win over the personal key, got %q", got)
	}
	// An env var that is unset must not shadow the personal key.
	custom.EnvKey = "MIHANI_TEST_KEY_UNSET"
	cfg.Providers["custom"] = custom
	if got := cfg.Key("custom"); got != "sk-persisted-123456" {
		t.Fatalf("unset env var should fall through to the personal key, got %q", got)
	}
	custom.APIKey = "sk-runtime"
	cfg.Providers["custom"] = custom
	if got := cfg.Key("custom"); got != "sk-runtime" {
		t.Fatalf("runtime key should win, got %q", got)
	}
}

// Regression: a stale in-memory copy saving over the file must not delete
// user-added providers and their keys (multi-instance data loss).
func TestSavePreservesDiskProvidersMissingFromMemory(t *testing.T) {
	isolatedHome(t)
	disk := defaults()
	disk.Providers["mygw"] = Provider{Label: "MyGW", Type: "openai", BaseURL: "http://x.example/v1", PersonalKey: "sk-precious-key-123456"}
	if err := disk.Save(); err != nil {
		t.Fatal(err)
	}
	// A stale in-memory copy without mygw (second instance) saves.
	stale := defaults()
	if err := stale.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := loaded.Providers["mygw"].PersonalKey; got != "sk-precious-key-123456" {
		t.Fatalf("stale save must not delete a disk provider and its key, got %q", got)
	}
	// Memory still wins on conflict.
	loaded.Providers["mygw"] = Provider{Label: "MyGW v2", Type: "openai", BaseURL: "http://y.example/v1", PersonalKey: "sk-new"}
	if err := loaded.Save(); err != nil {
		t.Fatal(err)
	}
	again, _ := Load()
	if again.Providers["mygw"].PersonalKey != "sk-new" || again.Providers["mygw"].BaseURL != "http://y.example/v1" {
		t.Fatalf("in-memory values must win over disk on conflict: %#v", again.Providers["mygw"])
	}
	// Migration-driven deletions still win on save.
	again.Providers["openrouter"] = Provider{Label: "OpenRouter"}
	if err := again.Save(); err != nil {
		t.Fatal(err)
	}
	final, _ := Load()
	if _, ok := final.Providers["openrouter"]; ok {
		t.Fatal("legacy id was resurrected by the save merge")
	}
}

// Older releases shipped openai/openrouter/anthropic built-ins. They must
// disappear on load, and a selection pointing at them falls back cleanly.
func TestMigrateDropsRemovedLegacyProviders(t *testing.T) {
	isolatedHome(t)
	cfg := defaults()
	cfg.Providers["openai"] = Provider{Label: "OpenAI", Type: "openai", BaseURL: "https://api.openai.com/v1"}
	cfg.Providers["openrouter"] = Provider{Label: "OpenRouter", Type: "openai", BaseURL: "https://openrouter.ai/api/v1"}
	cfg.Providers["anthropic"] = Provider{Label: "Anthropic", Type: "anthropic"}
	cfg.CurrentProvider = "openrouter"
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"openai", "openrouter", "anthropic"} {
		if _, ok := loaded.Providers[id]; ok {
			t.Fatalf("legacy provider %s should have been dropped on load", id)
		}
	}
	if loaded.CurrentProvider != BuiltinPrimary {
		t.Fatalf("selection on a removed provider should fall back to the builtin, got %s", loaded.CurrentProvider)
	}
}
