package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// isolatedHome points the session store at a temp directory for the test.
func isolatedHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
}

func TestIDIsStable(t *testing.T) {
	if ID("C:\\project") != ID("C:\\project") {
		t.Fatal("session ID is not stable")
	}
}

func TestSaveLoadListAndLatest(t *testing.T) {
	isolatedHome(t)

	first := Record{ID: NewID(), Workspace: "ws-a", Title: "first task", Model: "m1", History: []map[string]any{{"role": "user", "content": "hi"}}}
	if err := Save(first); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	second := Record{ID: NewID(), Workspace: "ws-b", Title: "other workspace", Model: "m2"}
	if err := Save(second); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(first.ID)
	if err != nil || loaded.Title != "first task" || len(loaded.History) != 1 {
		t.Fatalf("load round-trip failed: %#v %v", loaded, err)
	}
	if loaded.CreatedAt.IsZero() || loaded.UpdatedAt.IsZero() {
		t.Fatal("timestamps were not populated")
	}

	records, err := List()
	if err != nil || len(records) != 2 {
		t.Fatalf("expected 2 records, got %d (%v)", len(records), err)
	}
	if records[0].ID != second.ID {
		t.Fatalf("list should be newest first, got %v then %v", records[0].ID, records[1].ID)
	}

	latest, err := LatestForWorkspace("ws-a")
	if err != nil || latest.ID != first.ID {
		t.Fatalf("latest-for-workspace mismatch: %#v %v", latest, err)
	}
	if _, err := LatestForWorkspace("missing"); err == nil {
		t.Fatal("expected error for unknown workspace")
	}

	dir, _ := dir()
	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Fatalf("unexpected files in session dir: %d", len(entries))
	}
	_ = filepath.Separator
}
