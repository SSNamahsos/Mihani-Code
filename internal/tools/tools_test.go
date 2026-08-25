package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunnerRejectsPathsOutsideWorkspace(t *testing.T) {
	root := t.TempDir()
	got := (Runner{Root: root}).Run(context.Background(), "read_file", map[string]any{"path": filepath.Join(root, "..", "outside.txt")})
	if len(got) < 6 || got[:6] != "ERROR:" {
		t.Fatalf("expected path error, got %q", got)
	}
}

func TestRunnerWritesWithinWorkspace(t *testing.T) {
	root := t.TempDir()
	got := (Runner{Root: root}).Run(context.Background(), "write_file", map[string]any{"path": "nested/file.txt", "content": "hello"})
	if len(got) < 3 || got[:3] != "OK:" {
		t.Fatalf("expected write success, got %q", got)
	}
	b, err := os.ReadFile(filepath.Join(root, "nested", "file.txt"))
	if err != nil || string(b) != "hello" {
		t.Fatalf("unexpected file contents: %q, %v", b, err)
	}
}

// Regression: models send \n but Windows files use \r\n; edit_file must still match.
func TestEditFileMatchesAcrossLineEndings(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "win.txt")
	if err := os.WriteFile(path, []byte("first\r\nsecond\r\nthird\r\n"), 0644); err != nil {
		t.Fatal(err)
	}
	got := (Runner{Root: root}).Run(context.Background(), "edit_file",
		map[string]any{"path": "win.txt", "old_str": "first\nsecond", "new_str": "FIRST\nSECOND"})
	if !strings.HasPrefix(got, "OK:") {
		t.Fatalf("expected CRLF-tolerant edit to succeed, got %q", got)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "FIRST") || strings.Contains(string(b), "\n\n\n") {
		t.Fatalf("unexpected content after edit: %q", string(b))
	}
	if !strings.Contains(string(b), "\r\n") {
		t.Fatalf("original CRLF line endings were not preserved: %q", string(b))
	}
}

func TestEditFileRejectsAmbiguousMatch(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "dup.txt"), []byte("same\nsame\n"), 0644); err != nil {
		t.Fatal(err)
	}
	got := (Runner{Root: root}).Run(context.Background(), "edit_file",
		map[string]any{"path": "dup.txt", "old_str": "same", "new_str": "other"})
	if !strings.HasPrefix(got, "ERROR:") || !strings.Contains(got, "exactly once") {
		t.Fatalf("expected ambiguity error, got %q", got)
	}
}

func TestSearchSkipsBinaryAndOversizedFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "text.txt"), []byte("needle here\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "blob.bin"), []byte{'n', 0, 'e', 0, 'e', 0, 'd', 0}, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "huge.log"), []byte("needle"+strings.Repeat("x", 600*1024)), 0644); err != nil {
		t.Fatal(err)
	}
	out := (Runner{Root: root}).Run(context.Background(), "search_files", map[string]any{"pattern": "needle"})
	if !strings.Contains(out, "text.txt") {
		t.Fatalf("expected text match, got %q", out)
	}
	if strings.Contains(out, "blob.bin") || strings.Contains(out, "huge.log") {
		t.Fatalf("binary/oversized files should be skipped, got %q", out)
	}
}

func TestLimitTruncatesWithoutSplittingRunes(t *testing.T) {
	s := strings.Repeat("é", 100) // 200 bytes
	got := limit(s, 101)
	if strings.ContainsRune(got, '\uFFFD') {
		t.Fatalf("replacement rune leaked into truncation: %q", got)
	}
	if !strings.Contains(got, "(truncated)") {
		t.Fatalf("expected truncation marker, got %q", got)
	}
}
