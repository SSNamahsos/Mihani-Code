package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreviewShowsFileChange(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte("old\n"), 0600); err != nil {
		t.Fatal(err)
	}
	preview := Preview("edit_file", map[string]any{"path": "main.go", "old_str": "old", "new_str": "new"}, root)
	if !strings.Contains(preview, "old") || !strings.Contains(preview, "new") {
		t.Fatalf("unexpected preview: %q", preview)
	}
}

func TestWriteCreatesSnapshot(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte("before"), 0600); err != nil {
		t.Fatal(err)
	}
	result := (Runner{Root: root}).Run(context.Background(), "write_file", map[string]any{"path": "main.go", "content": "after"})
	if !strings.HasPrefix(result, "OK:") {
		t.Fatal(result)
	}
	var found bool
	_ = filepath.Walk(filepath.Join(root, ".mihani", "snapshots"), func(path string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			found = true
		}
		return nil
	})
	if !found {
		t.Fatal("expected a pre-write snapshot")
	}
}
