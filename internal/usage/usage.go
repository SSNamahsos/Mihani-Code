// Package usage persists per-request spend and answers "how much was used in
// the last 24 hours" so the app can enforce a daily budget per provider.
package usage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const window = 24 * time.Hour

// Entry records one provider request.
type Entry struct {
	Time     time.Time `json:"time"`
	Provider string    `json:"provider"`
	Model    string    `json:"model"`
	Input    int       `json:"input_tokens"`
	Output   int       `json:"output_tokens"`
	CostUSD  float64   `json:"cost_usd"`
	// KeyKind distinguishes shared embedded-key usage ("", the historical
	// default, and "embedded") from a user's own "personal" key.
	KeyKind string `json:"key_kind,omitempty"`
}

// Key kinds.
const (
	Embedded = "embedded"
	Personal = "personal"
)

type store struct {
	Entries []Entry `json:"entries"`
}

var (
	mu     sync.Mutex
	loaded bool
)

func path() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".mihani", "usage.json")
}

func load() store {
	var s store
	data, err := os.ReadFile(path())
	if err == nil {
		_ = json.Unmarshal(data, &s)
	}
	return prune(s)
}

func prune(s store) store {
	cutoff := time.Now().Add(-window)
	kept := s.Entries[:0]
	for _, e := range s.Entries {
		if e.Time.After(cutoff) {
			kept = append(kept, e)
		}
	}
	s.Entries = kept
	return s
}

func persist(s store) {
	p := path()
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(p, data, 0600)
}

// Add records a request and drops entries older than the window.
func Add(e Entry) {
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	mu.Lock()
	defer mu.Unlock()
	s := load()
	s.Entries = append(s.Entries, e)
	persist(prune(s))
}

// WindowSum returns total USD spent on a provider within the last 24 hours.
// An empty provider sums across all providers. All key kinds are included —
// use WindowSumFor when the shared-cap math needs embedded usage only.
func WindowSum(provider string) float64 {
	return WindowSumFor(provider, "")
}

// WindowSumFor totals USD for a provider filtered by key kind. kind "" means
// no filter; "embedded" also matches historical entries written before the
// personal-key feature existed.
func WindowSumFor(provider, kind string) float64 {
	mu.Lock()
	defer mu.Unlock()
	s := prune(load())
	sum := 0.0
	for _, e := range s.Entries {
		if provider != "" && e.Provider != provider {
			continue
		}
		if kind == Embedded {
			if e.KeyKind != "" && e.KeyKind != Embedded {
				continue
			}
		} else if kind != "" && e.KeyKind != kind {
			continue
		}
		sum += e.CostUSD
	}
	return sum
}

// NextReset reports when the oldest entry inside the current window leaves it,
// i.e. when budget headroom next increases. Only shared (embedded) usage
// counts — personal keys have their own quota. Zero means nothing is tracked.
func NextReset(provider string) time.Time {
	mu.Lock()
	defer mu.Unlock()
	s := prune(load())
	var oldest time.Time
	for _, e := range s.Entries {
		if provider != "" && e.Provider != provider {
			continue
		}
		if e.KeyKind == Personal {
			continue
		}
		if oldest.IsZero() || e.Time.Before(oldest) {
			oldest = e.Time
		}
	}
	if oldest.IsZero() {
		return time.Time{}
	}
	return oldest.Add(window)
}

// Reset clears the whole store (used by tests and /settings).
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	persist(store{})
}
