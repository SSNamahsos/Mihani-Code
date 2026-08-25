// Package pricing estimates USD cost from token usage. Rates are per million
// tokens and can be overridden per model pattern in config.json under
// "pricing". Unknown models fall back to a conservative default rate.
package pricing

import (
	"strings"
	"sync"
)

// Rate holds USD prices per one million tokens.
type Rate struct {
	Input  float64 `json:"input_per_m"`
	Output float64 `json:"output_per_m"`
}

// Entry is the config-facing shape: "pattern": {"input": x, "output": y}.
type Entry struct {
	Input  float64 `json:"input"`
	Output float64 `json:"output"`
}

var defaultRates = []struct {
	pattern string
	rate    Rate
}{
	{"glm-", Rate{Input: 0.60, Output: 2.50}},
	{"kat-coder", Rate{Input: 0.50, Output: 2.00}},
	{"minimax", Rate{Input: 0.30, Output: 1.20}},
	{"mimo", Rate{Input: 0.00, Output: 0.00}}, // free tier model
	{"claude-opus", Rate{Input: 15.00, Output: 75.00}},
	{"claude-sonnet", Rate{Input: 3.00, Output: 15.00}},
	{"claude-fable", Rate{Input: 3.00, Output: 15.00}},
	{"claude-haiku", Rate{Input: 0.80, Output: 4.00}},
	{"gpt-5", Rate{Input: 1.25, Output: 10.00}},
	{"gpt-4", Rate{Input: 2.50, Output: 10.00}},
	{"grok", Rate{Input: 3.00, Output: 15.00}},
}

var fallback = Rate{Input: 1.00, Output: 5.00}

var (
	mu       sync.RWMutex
	override = map[string]Rate{}
)

// SetOverrides installs user-configured rates keyed by lowercase substring.
func SetOverrides(entries map[string]Entry) {
	mu.Lock()
	defer mu.Unlock()
	override = make(map[string]Rate, len(entries))
	for pattern, e := range entries {
		override[strings.ToLower(pattern)] = Rate{Input: e.Input, Output: e.Output}
	}
}

// RateFor resolves the effective rate for a model id: config overrides win,
// then built-in patterns, then the fallback.
func RateFor(model string) Rate {
	lower := strings.ToLower(model)
	mu.RLock()
	defer mu.RUnlock()
	for pattern, rate := range override {
		if pattern != "" && strings.Contains(lower, pattern) {
			return rate
		}
	}
	for _, entry := range defaultRates {
		if strings.Contains(lower, entry.pattern) {
			return entry.rate
		}
	}
	return fallback
}

// Cost computes USD for the given token counts on a model.
func Cost(model string, inputTokens, outputTokens int) float64 {
	rate := RateFor(model)
	return (float64(inputTokens)/1_000_000)*rate.Input +
		(float64(outputTokens)/1_000_000)*rate.Output
}
