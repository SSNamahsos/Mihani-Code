package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/SSNamahsos/Mihani-Code/internal/pricing"
	"github.com/SSNamahsos/Mihani-Code/internal/secrets"
)

type Provider struct {
	Label   string   `json:"label"`
	Type    string   `json:"type"`
	BaseURL string   `json:"base_url"`
	EnvKey  string   `json:"env_key,omitempty"`
	APIKey  string   `json:"-"` // runtime only: never marshaled, never persisted
	Models  []string `json:"models"`
	// NativeTools marks whether the endpoint supports OpenAI function calling.
	// nil/true = native; false = Mihani drives tools via a prompt protocol
	// (for gateways that strip the tools parameter, e.g. some proxies).
	NativeTools *bool `json:"native_tools,omitempty"`
}

// UseNativeTools reports whether tool calls should use the API's function
// calling rather than the text-based fallback protocol.
func (p Provider) UseNativeTools() bool {
	return p.NativeTools == nil || *p.NativeTools
}

// DefaultDailyBudgetUSD is enforced per provider over a rolling 24 hours.
const DefaultDailyBudgetUSD = 10.0

type Config struct {
	Version         int                      `json:"version"`
	CurrentProvider string                   `json:"current_provider"`
	CurrentModel    string                   `json:"current_model"`
	MaxTokens       int                      `json:"max_tokens"`
	ContextWindow   int                      `json:"context_window,omitempty"`
	BudgetUSD       float64                  `json:"budget_usd,omitempty"`
	Pricing         map[string]pricing.Entry `json:"pricing,omitempty"`
	AutoConfirm     bool                     `json:"auto_confirm"`
	MaxIterations   int                      `json:"max_iterations,omitempty"`
	Workspace       string                   `json:"workspace,omitempty"`
	Providers       map[string]Provider      `json:"providers"`
	Permissions     map[string]string        `json:"permissions,omitempty"`
}

// Built-in provider ids. These are internal keys only: every user-facing
// surface shows the neutral Mihani Code branding, never an endpoint name.
const (
	BuiltinPrimary   = "mihani"
	BuiltinSecondary = "mihani-pro"
)

// legacyBuiltinIDs maps ids from earlier releases to their replacements so
// old config files migrate cleanly and endpoint names disappear.
var legacyBuiltinIDs = map[string]string{
	"hcnsec": BuiltinPrimary,
	"seekai": BuiltinSecondary,
}

func defaults() Config {
	return Config{
		Version:         2,
		CurrentProvider: BuiltinPrimary,
		CurrentModel:    "glm-5.3",
		MaxTokens:       8192,
		ContextWindow:   200_000,
		BudgetUSD:       DefaultDailyBudgetUSD,
		Permissions:     map[string]string{"read": "allow", "write": "ask", "shell": "ask", "network": "ask"},
		Providers: map[string]Provider{
			BuiltinPrimary: {
				Label:   "Mihani Cloud",
				Type:    "openai",
				BaseURL: "https://api.hcnsec.cn/v1",
				APIKey:  secrets.Primary(),
				Models:  []string{"glm-5.2", "glm-5.3", "kat-coder-pro-v2.5", "MiniMax-M3", "mimo-v2.5"},
			},
			BuiltinSecondary: {
				Label:   "Mihani Pro",
				Type:    "openai",
				BaseURL: "https://seekai.cc/v1",
				APIKey:  secrets.Secondary(),
				Models:  []string{"claude-opus-5", "claude-opus-4-8", "claude-fable-5", "claude-sonnet-5", "gpt-5.6-sol", "grok-4-5"},
				// This gateway strips the tools parameter, so Mihani drives
				// file/shell tools through the text protocol instead.
				NativeTools: boolPtr(false),
			},
		},
	}
}

func path() string { h, _ := os.UserHomeDir(); return filepath.Join(h, ".mihani", "config.json") }

func Load() (Config, error) {
	c := defaults()
	b, e := os.ReadFile(path())
	if os.IsNotExist(e) {
		return c, nil
	}
	if e != nil {
		return c, e
	}
	if e = json.Unmarshal(b, &c); e != nil {
		return c, e
	}
	if c.Providers == nil {
		c.Providers = defaults().Providers
	}
	migrateBuiltins(&c)
	if c.MaxTokens == 0 {
		c.MaxTokens = 8192
	}
	if c.ContextWindow <= 0 {
		c.ContextWindow = 200_000
	}
	if c.BudgetUSD == 0 {
		c.BudgetUSD = DefaultDailyBudgetUSD
	}
	return c, nil
}

// migrateBuiltins injects the shipped endpoints, removes ids from earlier
// releases (so endpoint names never linger in config or overlays), and
// repoints the active selection when its id was renamed.
func migrateBuiltins(c *Config) {
	for name, builtin := range defaults().Providers {
		c.Providers[name] = builtin
	}
	for legacy := range legacyBuiltinIDs {
		delete(c.Providers, legacy)
	}
	if replacement, ok := legacyBuiltinIDs[c.CurrentProvider]; ok {
		c.CurrentProvider = replacement
		if p := c.Providers[replacement]; len(p.Models) > 0 && !containsModel(p.Models, c.CurrentModel) {
			c.CurrentModel = p.Models[0]
		}
	}
	if _, ok := c.Providers[c.CurrentProvider]; !ok {
		c.CurrentProvider = BuiltinPrimary
		if p := c.Providers[BuiltinPrimary]; len(p.Models) > 0 && !containsModel(p.Models, c.CurrentModel) {
			c.CurrentModel = p.Models[0]
		}
	}
}

func containsModel(models []string, model string) bool {
	for _, m := range models {
		if m == model {
			return true
		}
	}
	return false
}

func boolPtr(v bool) *bool { return &v }

// LegacyProviderID resolves an id from an earlier release to its current
// built-in replacement. ok is false for unknown or non-legacy ids.
func LegacyProviderID(id string) (replacement string, ok bool) {
	replacement, ok = legacyBuiltinIDs[id]
	return replacement, ok
}

// Label returns the display name for the active provider. Built-ins always
// present as Mihani branding; unknown selections fall back to the product name.
func (c Config) ProviderLabel() string {
	if p, ok := c.Providers[c.CurrentProvider]; ok && p.Label != "" {
		return p.Label
	}
	return "Mihani Code"
}

func (c Config) Save() error {
	p := path()
	if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		return e
	}
	b, e := json.MarshalIndent(c, "", "  ")
	if e != nil {
		return e
	}
	return os.WriteFile(p, b, 0600)
}

func (c Config) Key(name string) string {
	p := c.Providers[name]
	if p.APIKey != "" {
		return p.APIKey
	}
	if p.EnvKey != "" {
		return os.Getenv(p.EnvKey)
	}
	return ""
}

// Budget returns the effective daily USD budget (<=0 disables enforcement).
func (c Config) Budget() float64 {
	if c.BudgetUSD == 0 {
		return DefaultDailyBudgetUSD
	}
	return c.BudgetUSD
}

// IsBuiltinProvider reports whether a provider id is one of the shipped
// Mihani endpoints. The daily credit budget applies only to these — user-added
// providers run on their own credentials and are never capped here.
func (c Config) IsBuiltinProvider(name string) bool {
	return name == BuiltinPrimary || name == BuiltinSecondary
}

// BudgetEnforced returns the daily cap for the active provider, or 0 when no
// cap applies (non-built-in providers, or budgeting disabled).
func (c Config) BudgetEnforced(current string) float64 {
	if !c.IsBuiltinProvider(current) {
		return 0
	}
	return c.Budget()
}
