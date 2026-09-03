package skills

import (
	"strings"
	"testing"
)

func TestParseDescriptionFrontmatter(t *testing.T) {
	cases := []struct {
		name, raw, want string
	}{
		{
			name: "inline description",
			raw:  "---\nname: x\ndescription: Does the thing.\n---\n# X\nbody",
			want: "Does the thing.",
		},
		{
			name: "quoted description",
			raw:  "---\nname: x\ndescription: \"Quoted with, commas.\"\n---\nbody",
			want: "Quoted with, commas.",
		},
		{
			name: "block description",
			raw:  "---\nname: x\ndescription: >\n  Multi line\n  description text\n---\nbody",
			want: "Multi line description text",
		},
		{
			name: "no frontmatter falls back to first line",
			raw:  "# My Skill\nThis does stuff.",
			want: "My Skill",
		},
	}
	for _, c := range cases {
		if got := ParseDescription(c.raw); got != c.want {
			t.Errorf("%s: ParseDescription = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestParseDescriptionCapsLength(t *testing.T) {
	long := strings.Repeat("word ", 200)
	got := ParseDescription("description: " + long)
	if r := []rune(got); len(r) > 301 {
		t.Fatalf("description not capped: %d runes", len(r))
	}
}

func TestParseDescriptionEmptyFrontmatterLine(t *testing.T) {
	// A leading "---" with no description key should not crash and falls back.
	got := ParseDescription("---\nname: x\n---\n# Title here")
	if got == "" {
		t.Fatal("expected a fallback description")
	}
}
