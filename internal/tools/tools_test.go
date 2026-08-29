package tools

import (
	"context"
	"fmt"
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

// Large files must be paged with offset/limit instead of hitting a wall.
func TestReadFileOffsetLimitPaging(t *testing.T) {
	root := t.TempDir()
	var sb strings.Builder
	for i := 1; i <= 300; i++ {
		fmt.Fprintf(&sb, "line-%d\n", i)
	}
	if err := os.WriteFile(filepath.Join(root, "big.txt"), []byte(sb.String()), 0644); err != nil {
		t.Fatal(err)
	}
	r := Runner{Root: root}

	got := r.Run(context.Background(), "read_file", map[string]any{"path": "big.txt", "offset": 120, "limit": 5})
	if !strings.Contains(got, "line-120") || !strings.Contains(got, "line-124") || strings.Contains(got, "line-125\n") {
		t.Fatalf("window wrong: %q", got)
	}
	if !strings.Contains(got, "lines 120-124 of 300") {
		t.Fatalf("range annotation missing: %q", got)
	}

	got = r.Run(context.Background(), "read_file", map[string]any{"path": "big.txt", "offset": 400})
	if !strings.Contains(got, "past the end") {
		t.Fatalf("expected past-end notice: %q", got)
	}

	// Whole-file reads are line-paged and report the total so the model can
	// read everything (half = total/2) or any range.
	var many strings.Builder
	for i := 1; i <= 1000; i++ {
		fmt.Fprintf(&many, "row-%d\n", i)
	}
	if err := os.WriteFile(filepath.Join(root, "rows.txt"), []byte(many.String()), 0644); err != nil {
		t.Fatal(err)
	}
	got = r.Run(context.Background(), "read_file", map[string]any{"path": "rows.txt"})
	if !strings.Contains(got, "of 1000") || !strings.Contains(got, "half point: offset 500") {
		t.Fatalf("read should reveal total lines and half point: %q", tailOf(got, 200))
	}
	if !strings.Contains(got, "[lines 1-400 of 1000]") {
		t.Fatalf("expected a bounded first page: %q", tailOf(got, 200))
	}
	// Half-file read: offset near total/2.
	got = r.Run(context.Background(), "read_file", map[string]any{"path": "rows.txt", "offset": 500, "limit": 100})
	if !strings.Contains(got, "row-500") || !strings.Contains(got, "of 1000") {
		t.Fatalf("half-window read wrong: %q", tailOf(got, 200))
	}
}

func tailOf(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

func TestRunnerDeletesFilesAndDirectories(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "junk.txt")
	if err := os.WriteFile(file, []byte("bye"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := (Runner{Root: root}).Run(context.Background(), "delete_file", map[string]any{"path": "junk.txt"}); !strings.HasPrefix(got, "OK:") {
		t.Fatalf("file delete failed: %q", got)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("file was not deleted")
	}

	dir := filepath.Join(root, "folder")
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "deep.txt"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := (Runner{Root: root}).Run(context.Background(), "delete_file", map[string]any{"path": "folder"}); !strings.Contains(got, "deleted directory") {
		t.Fatalf("directory delete failed: %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "sub", "deep.txt")); !os.IsNotExist(err) {
		t.Fatal("directory tree was not deleted recursively")
	}

	// Deleting the workspace root itself must be refused.
	if got := (Runner{Root: root}).Run(context.Background(), "delete_file", map[string]any{"path": "."}); !strings.Contains(got, "refusing") {
		t.Fatalf("root delete should be refused, got %q", got)
	}
}

func TestTodoWriteFormatsList(t *testing.T) {
	in := map[string]any{"todos": []any{
		map[string]any{"content": "read code", "status": "done"},
		map[string]any{"content": "rewrite", "status": "in_progress"},
		map[string]any{"content": "tests"},
	}}
	got := (Runner{Root: t.TempDir()}).Run(context.Background(), "todo_write", in)
	if !strings.HasPrefix(got, "OK: 1/3 done") {
		t.Fatalf("unexpected result header: %q", got)
	}
	for _, want := range []string{"✓ read code", "◐ rewrite", "○ tests"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing line %q in: %q", want, got)
		}
	}
}

func TestParseTodoListRejectsBadInput(t *testing.T) {
	if _, err := ParseTodoList([]any{}); err == nil {
		t.Fatal("empty list should be rejected")
	}
	if _, err := ParseTodoList([]any{map[string]any{"status": "done"}}); err == nil {
		t.Fatal("item without content should be rejected")
	}
	list, err := ParseTodoList([]any{map[string]any{"content": "x", "status": "BANANA"}})
	if err != nil || list[0].Status != "pending" {
		t.Fatalf("unknown status should normalize to pending, got %+v err=%v", list, err)
	}
}
