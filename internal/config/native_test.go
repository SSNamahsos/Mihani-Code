package config

import (
	"os"
	"path/filepath"
	"testing"
)

// Regression: Mihani Pro (seekai) now supports native OpenAI function
// calling (verified against the gateway 2026-08-30). The old
// native_tools:false made opus models answer in prose instead of using
// tools ("I can't write files here").
func TestMihaniProUsesNativeToolsByDefault(t *testing.T) {
	p := defaults().Providers[BuiltinSecondary]
	if !p.UseNativeTools() {
		t.Fatal("mihani-pro must use native function calling now")
	}
	if NativeToolsDefault("https://seekai.cc/v1") != nil {
		t.Fatal("seekai must no longer be on the strips-tools list")
	}
	// hcnsec stays on the prompt protocol until its gateway proves support.
	if NativeToolsDefault("https://api.hcnsec.cn/v1") == nil {
		t.Fatal("hcnsec should stay prompt-based for now")
	}
}

// Stored configs from older releases carry native_tools:false for seekai
// hosts and the old 8192 output budget. Load must upgrade both.
func TestLoadUpgradesStaleSeekaiConfig(t *testing.T) {
	isolatedHome(t)
	stale := `{
	  "version": 2,
	  "current_provider": "mihani-pro",
	  "current_model": "claude-opus-5",
	  "max_tokens": 8192,
	  "providers": {
	    "seekai-custom": {"label": "SeekAI", "type": "openai", "base_url": "https://seekai.cc/v1", "models": ["claude-opus-5"], "native_tools": false}
	  }
	}`
	if err := os.MkdirAll(os.TempDir(), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path()), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path(), []byte(stale), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	p := cfg.Providers["seekai-custom"]
	if p.NativeTools != nil && !*p.NativeTools {
		t.Fatal("stale native_tools:false for seekai host should be upgraded to native")
	}
	if !p.UseNativeTools() {
		t.Fatal("seekai-custom should use native tools after migration")
	}
	if cfg.MaxTokens != defaultMaxTokens {
		t.Fatalf("stale 8192 max_tokens should upgrade to %d, got %d", defaultMaxTokens, cfg.MaxTokens)
	}
}
