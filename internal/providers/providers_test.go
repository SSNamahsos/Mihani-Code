package providers

import "testing"

func TestNormalizeProviderTrimsURL(t *testing.T) {
	p := NormalizeProvider("local", "http://localhost:8000/v1/", "key", []string{"coder"})
	if p.BaseURL != "http://localhost:8000/v1" || p.APIKey != "key" || len(p.Models) != 1 {
		t.Fatalf("unexpected provider: %#v", p)
	}
}

func TestNormalizeBaseURL(t *testing.T) {
	cases := map[string]string{
		"https://seekai.cc":           "https://seekai.cc/v1",
		"https://seekai.cc/":          "https://seekai.cc/v1",
		"https://seekai.cc/v1":        "https://seekai.cc/v1",
		"http://localhost:11434/v1":   "http://localhost:11434/v1",
		"https://x.example/api/v1":    "https://x.example/api/v1",
		"https://x.example/custom/v1": "https://x.example/custom/v1",
		"":                            "",
	}
	for in, want := range cases {
		if got := NormalizeBaseURL(in); got != want {
			t.Fatalf("NormalizeBaseURL(%q) = %q, want %q", in, got, want)
		}
	}
}

// Regression: users paste the bare domain in /connect; chat requests must
// still hit the /v1 API path instead of the gateway's web app.
func TestNormalizeProviderAddsV1ForBareDomain(t *testing.T) {
	p := NormalizeProvider("seekai-like", "https://seekai.cc", "", []string{"m"})
	if p.BaseURL != "https://seekai.cc/v1" {
		t.Fatalf("bare domain must be normalized to the /v1 API path, got %s", p.BaseURL)
	}
}
