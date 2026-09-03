package ui

import (
	"strings"
	"testing"

	"github.com/SSNamahsos/Mihani-Code/internal/config"
	"github.com/SSNamahsos/Mihani-Code/internal/update"
)

// A detected newer release must surface in the home banner and header, and
// /update must render a modal that shows the version jump, what's new, and an
// install action. This is a render-only smoke test (no terminal, no network).
func TestUpdateUIRenders(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	m := newTestModel(110, 34)
	m.cfg = cfg
	m.version = "v0.2.17"
	m.root = t.TempDir()
	m.sessionID = "test-session"

	// Up to date: no banner anywhere.
	if got := m.welcome(); strings.Contains(stripANSI(got), "is available") {
		t.Fatal("banner shown with no update")
	}

	// A newer release is pending.
	m.updateLatest = &update.Release{
		Tag:  "v0.2.18",
		Name: "v0.2.18",
		Body: "auto-generated compare link only",
		URL:  "https://github.com/SSNamahsos/Mihani-Code/releases/tag/v0.2.18",
	}
	if !m.updateReady() {
		t.Fatal("updateReady should be true for a newer tag")
	}
	if got := m.welcome(); !strings.Contains(stripANSI(got), "is available") {
		t.Fatal("home banner missing the update line")
	}
	if got := m.headerRow(); !strings.Contains(stripANSI(got), "v0.2.18") {
		t.Fatal("header should carry the pending-update marker")
	}

	// The modal: current → latest, what's new (detailed changelog), install.
	m.openUpdateModal()
	m.updateChangelog = "## v0.2.18\n\n### Added\n- automatic update check and self-update"
	m.updateVp.SetContent(m.updateChangelogText())
	view := stripANSI(m.updateView())
	for _, want := range []string{"v0.2.17", "v0.2.18", "What's new", "automatic update check", "install", "github.com"} {
		if !strings.Contains(view, want) {
			t.Fatalf("update modal missing %q\n---\n%s", want, view)
		}
	}

	// Dismissing clears the ready state and the banner.
	m.updateDismissed = true
	if m.updateReady() {
		t.Fatal("dismissed update should not be ready")
	}
	if got := m.welcome(); strings.Contains(stripANSI(got), "is available") {
		t.Fatal("banner should be gone after dismiss")
	}
}
