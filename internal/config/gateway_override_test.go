package config

import (
	"testing"
)

// applyGatewayOverride must re-point the two built-ins at the gateway and set a
// client token. When MIHANI_GATEWAY is unset, the shipped default gateway is
// used so built-in providers work out of the box.
func TestApplyGatewayOverride(t *testing.T) {
	// Unset: defaults to the shipped gateway.
	t.Setenv("MIHANI_GATEWAY", "")
	t.Setenv("MIHANI_GATEWAY_TOKEN", "")
	c := defaults()
	applyGatewayOverride(&c)
	if got := c.Providers[BuiltinPrimary].BaseURL; got != defaultGateway+"/cloud/v1" {
		t.Fatalf("cloud base = %q, want %q", got, defaultGateway+"/cloud/v1")
	}
	if got := c.Providers[BuiltinSecondary].BaseURL; got != defaultGateway+"/pro/v1" {
		t.Fatalf("pro base = %q, want %q", got, defaultGateway+"/pro/v1")
	}
	if got := c.Key(BuiltinSecondary); got != defaultGatewayToken {
		t.Fatalf("pro key should be the default client token, got %q", got)
	}

	// Set explicitly: both built-ins point at the custom gateway with the client token.
	t.Setenv("MIHANI_GATEWAY", "https://gw.example.com/")
	t.Setenv("MIHANI_GATEWAY_TOKEN", "client-tok")
	c = defaults()
	applyGatewayOverride(&c)

	if got := c.Providers[BuiltinPrimary].BaseURL; got != "https://gw.example.com/cloud/v1" {
		t.Fatalf("cloud base = %q, want .../cloud/v1", got)
	}
	if got := c.Providers[BuiltinSecondary].BaseURL; got != "https://gw.example.com/pro/v1" {
		t.Fatalf("pro base = %q, want .../pro/v1", got)
	}
	if got := c.Key(BuiltinSecondary); got != "client-tok" {
		t.Fatalf("pro key should be the client token, got %q", got)
	}
	if got := c.Key(BuiltinPrimary); got != "client-tok" {
		t.Fatalf("cloud key should be the client token, got %q", got)
	}
}

// The override must not touch custom (user-added) providers.
func TestApplyGatewayOverrideLeavesCustomProviders(t *testing.T) {
	t.Setenv("MIHANI_GATEWAY", "https://gw.example.com")
	t.Setenv("MIHANI_GATEWAY_TOKEN", "client-tok")
	c := defaults()
	c.Providers["ollama"] = Provider{Label: "Ollama", BaseURL: "http://localhost:11434/v1", Models: []string{"x"}}
	applyGatewayOverride(&c)
	if got := c.Providers["ollama"].BaseURL; got != "http://localhost:11434/v1" {
		t.Fatalf("custom provider base must be untouched, got %q", got)
	}
	if c.Key("ollama") != "" {
		t.Fatalf("custom provider key must be untouched, got %q", c.Key("ollama"))
	}
}
