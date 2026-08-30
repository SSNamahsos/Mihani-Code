package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestGlobMatchesNestedAndSkipsVendor(t *testing.T) {
	root := t.TempDir()
	mustWrite := func(rel, content string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("main.go", "x")
	mustWrite("sub/inner.go", "x")
	mustWrite("sub/notes.md", "x")
	mustWrite("node_modules/pkg/x.go", "x")

	out := Runner{Root: root}.Run(context.Background(), "glob", map[string]any{"pattern": "**/*.go"})
	if !strings.Contains(out, "main.go") || !strings.Contains(out, filepath.Join("sub", "inner.go")) {
		t.Fatalf("glob **/*.go missed files: %s", out)
	}
	if strings.Contains(out, "node_modules") {
		t.Fatalf("glob must skip node_modules: %s", out)
	}
	out = Runner{Root: root}.Run(context.Background(), "glob", map[string]any{"pattern": "sub/*.md"})
	if !strings.Contains(out, "md") {
		t.Fatalf("glob sub/*.md missed: %s", out)
	}
}

func TestGlobMatchUnit(t *testing.T) {
	cases := []struct{ pat, path string; want bool }{
		{"**/*.go", "a/b/c.go", true},
		{"**/*.go", "a/b/c.txt", false},
		{"*.md", "readme.md", true},
		{"*.md", "sub/readme.md", false},
		{"src/**", "src/a/b/c.go", true},
		{"**", "a/b/c.go", true},
	}
	for _, c := range cases {
		if got := globMatch(c.pat, c.path); got != c.want {
			t.Errorf("globMatch(%q, %q) = %v, want %v", c.pat, c.path, got, c.want)
		}
	}
}

func TestHtmlToTextStripsNoise(t *testing.T) {
	in := `<html><head><style>body{color:red}</style></head>
<body>
<script src="evil.js">var x = "should vanish";</script>
<h1>Coffee Shop</h1>
<p class="lead">Est. 2020 &mdash; good beans &amp; better people.</p>
<!-- a comment -->
</body></html>`
	got := htmlToText(in)
	if strings.Contains(got, "style") || strings.Contains(got, "evil.js") || strings.Contains(got, "should vanish") {
		t.Fatalf("script/style content leaked: %q", got)
	}
	if !strings.Contains(got, "Coffee Shop") || !strings.Contains(got, "good beans") {
		t.Fatalf("visible text lost: %q", got)
	}
}

func TestResolveDDGLink(t *testing.T) {
	if got := resolveDDGLink("/l/?uddg=https%3A%2F%2Fexample.com%2Fpage&rut=xyz"); got != "https://example.com/page" {
		t.Fatalf("redirect unwrap failed: %q", got)
	}
	if got := resolveDDGLink("https://example.com/ok"); got != "https://example.com/ok" {
		t.Fatalf("absolute link mangled: %q", got)
	}
}

func TestWebFetchSavesBinaryFile(t *testing.T) {
	payload := []byte{0x89, 'P', 'N', 'G', 0, 1, 2, 3, 4}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(payload)
	}))
	defer srv.Close()
	root := t.TempDir()
	out := Runner{Root: root}.Run(context.Background(), "web_fetch", map[string]any{
		"url":     srv.URL,
		"save_to": "img/pixel.png",
	})
	if !strings.HasPrefix(out, "OK: saved") {
		t.Fatalf("expected save confirmation, got: %s", out)
	}
	got, err := os.ReadFile(filepath.Join(root, "img", "pixel.png"))
	if err != nil || len(got) != len(payload) {
		t.Fatalf("saved file wrong: err=%v len=%d", err, len(got))
	}
}

func TestWebFetchRejectsNonHTTP(t *testing.T) {
	out := Runner{Root: t.TempDir()}.Run(context.Background(), "web_fetch", map[string]any{"url": "file:///etc/passwd"})
	if !strings.HasPrefix(out, "ERROR") {
		t.Fatalf("expected error for non-http url, got: %s", out)
	}
}

func TestBashTimeoutParameter(t *testing.T) {
	var command string
	if runtime.GOOS == "windows" {
		command = "ping -n 4 127.0.0.1 > nul"
	} else {
		command = "sleep 3"
	}
	out := Runner{Root: t.TempDir()}.Run(context.Background(), "bash", map[string]any{
		"command": command,
		"timeout": 2,
	})
	if !strings.Contains(out, "timed out after 2s") {
		t.Fatalf("expected 2s timeout message, got: %s", out)
	}
}
