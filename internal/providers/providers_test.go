package providers

import "testing"

func TestNormalizeProviderTrimsURL(t *testing.T) {
	p := NormalizeProvider("local", "http://localhost:8000/v1/", "key", []string{"coder"})
	if p.BaseURL != "http://localhost:8000/v1" || p.APIKey != "key" || len(p.Models) != 1 {
		t.Fatalf("unexpected provider: %#v", p)
	}
}
