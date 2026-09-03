package update

import (
	"runtime"
	"testing"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		candidate, current string
		want               bool
	}{
		{"v0.2.18", "v0.2.17", true},
		{"v0.2.17", "v0.2.17", false},
		{"v0.2.17", "v0.2.18", false},
		{"v0.3.0", "v0.2.99", true},
		{"v10.0.0", "v9.9.9", true}, // numeric, not lexicographic
		{"v0.2.10", "v0.2.9", true},
		{"0.2.18", "v0.2.17", true}, // v prefix optional
		{"", "v0.2.17", false},
		{"v0.2.17", "", false},
	}
	for _, c := range cases {
		if got := Newer(c.candidate, c.current); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.candidate, c.current, got, c.want)
		}
	}
}

func TestCompareVersionsOrder(t *testing.T) {
	if compareVersions("v0.2.17", "v0.2.17") != 0 {
		t.Fatal("equal versions should compare 0")
	}
	if compareVersions("v0.2.17", "v0.2.18") >= 0 {
		t.Fatal("17 should be less than 18")
	}
	if compareVersions("v0.2.18", "v0.2.17") <= 0 {
		t.Fatal("18 should be greater than 17")
	}
}

func TestAssetForURL(t *testing.T) {
	base := "mihani-" + runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		base += ".exe"
	}
	r := &Release{Assets: []string{
		"https://github.com/SSNamahsos/Mihani-Code/releases/download/v0.2.18/mihani-linux-amd64",
		"https://github.com/SSNamahsos/Mihani-Code/releases/download/v0.2.18/" + base,
		"https://github.com/SSNamahsos/Mihani-Code/releases/download/v0.2.18/mihani-darwin-arm64",
	}}
	if got := AssetForURL(r); got != "https://github.com/SSNamahsos/Mihani-Code/releases/download/v0.2.18/"+base {
		t.Fatalf("AssetForURL picked %q", got)
	}
	if got := AssetForURL(&Release{}); got != "" {
		t.Fatalf("empty release should yield no asset, got %q", got)
	}
}
