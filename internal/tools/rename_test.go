package tools

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The v0.3.0 rename gave every tool a Mihani_* name; legacy spellings from
// restored sessions or older models must still resolve to the same tool.
func TestNormalizeLegacyNames(t *testing.T) {
	cases := []struct{ in, want string }{
		{"read_file", ToolReadFile},
		{"todo_write", ToolTodoWrite},
		{"ask_user", ToolAskUser},
		{"bash", ToolBash},
		{"glob", ToolGlob},
		{"write_file", ToolWriteFile},
		{"mihani_bash", ToolBash},
		{"MIHANI_BASH", ToolBash},
		{"Mihani_Bash", ToolBash},
		{"  Mihani_Read_File  ", ToolReadFile},
		{"mcp.docs.search", "mcp.docs.search"}, // MCP names pass through
		{"something_else", "something_else"},
	}
	for _, c := range cases {
		if got := Normalize(c.in); got != c.want {
			t.Fatalf("Normalize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestGrepReturnsPathLineMatches(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "app.go"), []byte("package main\n\nfunc HandleRequest() {}\nfunc unused() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes.md"), []byte("# notes\nHandleRequest is the entry point\n"), 0644); err != nil {
		t.Fatal(err)
	}

	out := Runner{Root: root}.Run(context.Background(), ToolGrep, map[string]any{"pattern": "HandleRequest"})
	if !strings.Contains(out, filepath.Join("src", "app.go")+":3") {
		t.Fatalf("grep did not report the match with line number: %q", out)
	}
	if !strings.Contains(out, "notes.md:2") {
		t.Fatalf("grep missed a second file: %q", out)
	}

	// Case-insensitive flag.
	out = Runner{Root: root}.Run(context.Background(), ToolGrep, map[string]any{"pattern": "handlerequest", "ignore_case": true})
	if !strings.Contains(out, filepath.Join("src", "app.go")+":3") {
		t.Fatalf("ignore_case not honored: %q", out)
	}

	// No matches.
	out = Runner{Root: root}.Run(context.Background(), ToolGrep, map[string]any{"pattern": "zzz-not-there"})
	if !strings.Contains(out, "No matches") {
		t.Fatalf("expected no-match message, got %q", out)
	}

	// Invalid regex surfaces an error instead of crashing.
	out = Runner{Root: root}.Run(context.Background(), ToolGrep, map[string]any{"pattern": "("})
	if !strings.HasPrefix(out, "ERROR:") {
		t.Fatalf("expected regex error, got %q", out)
	}
}

func TestImageReaderReportsFormatAndSize(t *testing.T) {
	root := t.TempDir()
	img := image.NewRGBA(image.Rect(0, 0, 7, 5))
	img.Set(3, 2, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tiny.png"), buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}

	out := Runner{Root: root}.Run(context.Background(), ToolImageReader, map[string]any{"path": "tiny.png"})
	if !strings.Contains(out, "png") || !strings.Contains(out, "7x5") {
		t.Fatalf("image reader output missing format/dimensions: %q", out)
	}
	if !strings.Contains(out, "bytes") {
		t.Fatalf("image reader output missing byte size: %q", out)
	}

	// A non-image returns an error, not a crash.
	if err := os.WriteFile(filepath.Join(root, "text.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	out = Runner{Root: root}.Run(context.Background(), ToolImageReader, map[string]any{"path": "text.txt"})
	if !strings.HasPrefix(out, "ERROR:") {
		t.Fatalf("expected error for non-image, got %q", out)
	}
}
