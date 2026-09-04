//go:build windows

package update

import (
	"os"
	"path/filepath"
	"testing"
)

// The Windows swap renames the running exe aside, installs the new binary in
// its place, and removes the old file — all in-process (a running .exe is
// opened with FILE_SHARE_DELETE so it can be renamed). This test exercises
// that logic with a stubbed relaunch so no real window opens.
func TestSwapBinaryReplacesInPlace(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "mihani.exe")
	tmp := filepath.Join(exe) + ".update.tmp"

	// standing-in "current binary" and "downloaded" binary
	if err := os.WriteFile(exe, []byte("OLD-VERSION"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tmp, []byte("NEW-VERSION"), 0700); err != nil {
		t.Fatal(err)
	}

	started := 0
	oldStart := startNewWindow
	startNewWindow = func(path string) error {
		started++
		return nil
	}
	t.Cleanup(func() { startNewWindow = oldStart })

	note, willRestart, err := swapBinary(exe, tmp, "v9.9.9")
	if err != nil {
		t.Fatalf("swapBinary: %v", err)
	}
	if !willRestart {
		t.Fatal("expected willRestart=true when a new window opened")
	}
	if started != 1 {
		t.Fatalf("expected relaunch once, got %d", started)
	}
	// The new binary is now at the canonical path.
	b, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "NEW-VERSION" {
		t.Fatalf("canonical exe = %q, want NEW-VERSION", string(b))
	}
	// The old file and the temp are gone.
	if _, err := os.Stat(exe + ".old"); !os.IsNotExist(err) {
		t.Fatal("leftover .old file should be removed")
	}
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Fatal("leftover .update.tmp should be consumed")
	}
	if note == "" {
		t.Fatal("expected a non-empty user note")
	}
}
