package config

import "testing"

func TestModelForAndSetSelectedModel(t *testing.T) {
	c := defaults()
	pro := c.Providers[BuiltinPrimary]
	proModels := pro.Models
	if len(proModels) == 0 {
		t.Fatal("primary provider has no models")
	}

	// No stored selection -> first model.
	if got := c.ModelFor(BuiltinPrimary); got != proModels[0] {
		t.Fatalf("ModelFor default = %q, want %q", got, proModels[0])
	}

	// Remember a choice, then it round-trips.
	if len(proModels) > 1 {
		want := proModels[1]
		c.SetSelectedModel(BuiltinPrimary, want)
		if got := c.ModelFor(BuiltinPrimary); got != want {
			t.Fatalf("ModelFor after set = %q, want %q", got, want)
		}
	}

	// A stored model that is no longer listed falls back to the first.
	c.SelectedModels[BuiltinSecondary] = "a-model-that-was-removed"
	got := c.ModelFor(BuiltinSecondary)
	if got != c.Providers[BuiltinSecondary].Models[0] {
		t.Fatalf("ModelFor removed-model fallback = %q, want %q", got, c.Providers[BuiltinSecondary].Models[0])
	}

	// SetSelectedModel on the active provider also updates CurrentModel.
	active := c.CurrentProvider
	c.SetSelectedModel(active, c.ModelFor(active))
	if c.CurrentModel != c.ModelFor(active) {
		t.Fatalf("SetSelectedModel should track CurrentModel for the active provider")
	}

	// Unknown provider -> "".
	if c.ModelFor("nope") != "" {
		t.Fatal("ModelFor(unknown) should be empty")
	}
}
