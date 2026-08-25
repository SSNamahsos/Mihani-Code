package secrets

import (
	"strings"
	"testing"
)

// The embedded credentials must decode to real keys and redaction must scrub
// them from arbitrary text — this is the core of "users cannot read the key".
// Source-only builds (placeholder blob) skip the credential-specific checks.
func TestEmbeddedKeysDecodeAndRedact(t *testing.T) {
	if Primary() == "" && Secondary() == "" {
		t.Skip("placeholder blob.bin — no embedded credentials in this source build")
	}
	for _, key := range []string{Primary(), Secondary()} {
		if !strings.HasPrefix(key, "sk-") || len(key) < 20 {
			t.Fatalf("embedded credential did not decode correctly: %q", Redact(key))
		}
		leaked := "tool output contains " + key + " inside logs"
		got := Redact(leaked)
		if strings.Contains(got, key) {
			t.Fatal("Redact failed to remove an embedded secret")
		}
		if !strings.Contains(got, "[redacted]") {
			t.Fatalf("expected [redacted] marker, got %q", got)
		}
	}
}

func TestRegisterAddsAndDeduplicates(t *testing.T) {
	Register("TESTSECRET-1234567890")
	Register("TESTSECRET-1234567890") // duplicate is a no-op
	got := Redact("value=TESTSECRET-1234567890")
	if strings.Contains(got, "TESTSECRET") {
		t.Fatalf("registered secret leaked: %q", got)
	}
}

func TestShortValuesAreIgnored(t *testing.T) {
	before := len([]rune(Redact("abc")))
	Register("abc")
	if after := len([]rune(Redact("abc"))); after != before {
		t.Fatal("short values must not be registered")
	}
}

func TestPlainTextPassesThrough(t *testing.T) {
	in := "ordinary output with no secrets at all"
	if out := Redact(in); out != in {
		t.Fatalf("clean text was modified: %q", out)
	}
}
