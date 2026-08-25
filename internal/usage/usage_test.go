package usage

import (
	"testing"
	"time"
)

func isolatedHome(t *testing.T) {
	t.Helper()
	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv("HOME", t.TempDir())
}

func TestWindowSumAndReset(t *testing.T) {
	isolatedHome(t)
	Reset()

	Add(Entry{Provider: "hcnsec", CostUSD: 2.50, Time: time.Now().Add(-time.Hour)})
	Add(Entry{Provider: "hcnsec", CostUSD: 1.25, Time: time.Now()})
	Add(Entry{Provider: "seekai", CostUSD: 4.00, Time: time.Now()})

	if got := WindowSum("hcnsec"); got != 3.75 {
		t.Fatalf("per-provider sum = %v, want 3.75", got)
	}
	if got := WindowSum(""); got != 7.75 {
		t.Fatalf("global sum = %v, want 7.75", got)
	}

	reset := NextReset("hcnsec")
	want := time.Now().Add(-time.Hour).Add(window)
	if reset.IsZero() || absDuration(reset.Sub(want)) > time.Minute {
		t.Fatalf("reset time = %v, want ~%v", reset, want)
	}
	if !NextReset("missing").IsZero() {
		t.Fatal("unknown provider should have no reset time")
	}
}

func TestOldEntriesArePruned(t *testing.T) {
	isolatedHome(t)
	Reset()

	Add(Entry{Provider: "hcnsec", CostUSD: 9.00, Time: time.Now().Add(-25 * time.Hour)})
	Add(Entry{Provider: "hcnsec", CostUSD: 1.00, Time: time.Now()})

	if got := WindowSum("hcnsec"); got != 1.00 {
		t.Fatalf("stale entry still counted: %v", got)
	}
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
