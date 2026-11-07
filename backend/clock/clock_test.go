package clock

import (
	"errors"
	"testing"
	"time"
)

// withFetch swaps the NTP source for the duration of a test.
func withFetch(f func() (time.Time, error)) func() {
	prev := fetchNTP
	fetchNTP = f
	return func() { fetchNTP = prev }
}

func TestNewIsUnanchoredButUsable(t *testing.T) {
	c := New()
	if c.Anchored() {
		t.Error("a fresh clock must not report itself anchored")
	}
	// Before NTP, Now() tracks the system clock (best-effort).
	if delta := time.Since(c.Now()); delta < -time.Second || delta > time.Second {
		t.Errorf("unanchored Now() should be near system time, off by %v", delta)
	}
}

func TestSyncAnchorsToNTP(t *testing.T) {
	// A fake NTP server 10 years in the "future": if Now() reflects it, the
	// anchor (not the system clock) is driving the result.
	fake := time.Date(2036, 1, 2, 3, 4, 5, 0, time.UTC)
	defer withFetch(func() (time.Time, error) { return fake, nil })()

	c := New()
	if err := c.Sync(); err != nil {
		t.Fatalf("Sync should succeed with a healthy NTP source: %v", err)
	}
	if !c.Anchored() {
		t.Error("clock should report anchored after a successful Sync")
	}
	// Now() must be at/after the anchor and within a small window of it.
	got := c.Now()
	if got.Before(fake) {
		t.Errorf("Now() %v is before the NTP anchor %v", got, fake)
	}
	if got.Sub(fake) > 5*time.Second {
		t.Errorf("Now() %v drifted too far from the NTP anchor %v", got, fake)
	}
}

func TestSyncFailureLeavesClockUsable(t *testing.T) {
	defer withFetch(func() (time.Time, error) { return time.Time{}, errors.New("network down") })()

	c := New()
	if err := c.Sync(); err == nil {
		t.Fatal("Sync should return the NTP error")
	}
	if c.Anchored() {
		t.Error("a failed Sync must not mark the clock anchored")
	}
	// Best-effort: Now() still returns a sane, system-tracking time.
	if delta := time.Since(c.Now()); delta < -time.Second || delta > time.Second {
		t.Errorf("after a failed Sync, Now() should still track system time, off by %v", delta)
	}
}

func TestNowAdvancesMonotonically(t *testing.T) {
	fake := time.Date(2030, 6, 1, 0, 0, 0, 0, time.UTC)
	defer withFetch(func() (time.Time, error) { return fake, nil })()

	c := New()
	if err := c.Sync(); err != nil {
		t.Fatal(err)
	}
	t1 := c.Now()
	time.Sleep(20 * time.Millisecond)
	t2 := c.Now()

	elapsed := t2.Sub(t1)
	if elapsed <= 0 {
		t.Errorf("Now() must advance; got t1=%v t2=%v", t1, t2)
	}
	// The advance should match real elapsed time (monotonic), not jump around.
	if elapsed < 15*time.Millisecond || elapsed > 500*time.Millisecond {
		t.Errorf("elapsed %v does not match the ~20ms slept", elapsed)
	}
}
