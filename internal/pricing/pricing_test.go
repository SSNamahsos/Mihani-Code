package pricing

import (
	"math"
	"testing"
)

func TestRateForKnownPatterns(t *testing.T) {
	cases := map[string]Rate{
		"glm-5.3":            {Input: 0.60, Output: 2.50},
		"kat-coder-pro-v2.5": {Input: 0.50, Output: 2.00},
		"MiniMax-M3":         {Input: 0.30, Output: 1.20},
		"mimo-v2.5":          {Input: 0, Output: 0},
		"claude-opus-5":      {Input: 15.00, Output: 75.00},
		"gpt-5.6-sol":        {Input: 1.25, Output: 10.00},
		"grok-4-5":           {Input: 3.00, Output: 15.00},
	}
	for model, want := range cases {
		if got := RateFor(model); got != want {
			t.Errorf("%s: got %+v want %+v", model, got, want)
		}
	}
}

func TestFallbackForUnknownModel(t *testing.T) {
	rate := RateFor("totally-unknown-model")
	if rate != fallback {
		t.Fatalf("unexpected fallback rate: %+v", rate)
	}
}

func TestOverridesWinOverBuiltins(t *testing.T) {
	SetOverrides(map[string]Entry{"glm": {Input: 9.99, Output: 9.99}})
	defer SetOverrides(nil)
	if got := RateFor("glm-5.3"); got.Input != 9.99 || got.Output != 9.99 {
		t.Fatalf("override not applied: %+v", got)
	}
}

func TestCostMath(t *testing.T) {
	// glm-5.3: $0.60/M input, $2.50/M output.
	got := Cost("glm-5.3", 1_000_000, 1_000_000)
	if math.Abs(got-3.10) > 1e-9 {
		t.Fatalf("cost mismatch: %v", got)
	}
	if Cost("mimo-v2.5", 10_000_000, 10_000_000) != 0 {
		t.Fatal("free model must cost nothing")
	}
}
