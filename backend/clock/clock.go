// Package clock provides a "trusted" time source that resists the user moving
// the system wall clock to escape a focus session.
//
// The problem: enforcement decisions ("is this lock still running?", "is a
// schedule active right now?") used the raw system clock, so advancing the
// Windows time-of-day by a few hours ended a manual lock early and let a
// schedule window be skipped.
//
// The fix: anchor to NTP once, then advance using Go's MONOTONIC clock. Go's
// time.Now() carries a monotonic reading sourced from the OS performance
// counter, which the time-of-day slider cannot move; time.Since therefore
// measures real elapsed time regardless of wall-clock changes. So while the
// process runs, moving the system clock forward or backward has no effect on
// the trusted time. On a fresh start (e.g. the Ghost relaunching) NTP re-anchors
// to real time. When NTP is unreachable we fall back, best-effort, to monotonic
// time over the system clock and keep enforcing — a focus session is never
// broken just because the network is down.
package clock

import (
	"sync"
	"time"

	"focus-lock/backend/ntp"
)

// fetchNTP is the network-time source. It is a package variable so tests can
// substitute a deterministic fake.
var fetchNTP = ntp.GetNetworkTime

// Clock reports a trusted time (see package doc). The zero value is not usable;
// construct one with New.
type Clock struct {
	mu       sync.Mutex
	anchored bool      // true once NTP has anchored at least once
	baseWall time.Time // trusted wall time at the anchor instant (monotonic reading stripped)
	baseMono time.Time // reading captured at the same instant, kept for its monotonic component
}

// New returns a Clock anchored to the current system time. Until Sync (or
// Start) upgrades the anchor to NTP, the result is best-effort: it still
// advances monotonically, so tampering after this point is ignored, but the
// absolute value is only as correct as the system clock was at construction.
func New() *Clock {
	n := time.Now()
	return &Clock{
		baseWall: n.Round(0), // Round(0) strips the monotonic reading -> a pure wall anchor
		baseMono: n,          // keep the monotonic reading -> the elapsed-time reference
	}
}

// Default is the process-wide trusted clock. Each process (UI and Ghost) has
// its own; they independently anchor to NTP.
var Default = New()

// Now returns the current trusted time. It is baseWall plus the real time
// elapsed since the anchor, measured on the monotonic clock.
func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.baseWall.Add(time.Since(c.baseMono))
}

// Anchored reports whether NTP has successfully anchored the clock at least
// once. When false, timing is best-effort and should be treated as unverified.
func (c *Clock) Anchored() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.anchored
}

// Sync fetches NTP time and re-anchors to it. On failure it returns the error
// and leaves the existing anchor (and its monotonic progression) untouched, so
// a transient network failure never disturbs a running session.
func (c *Clock) Sync() error {
	nt, err := fetchNTP()
	if err != nil {
		return err
	}
	mono := time.Now()

	c.mu.Lock()
	defer c.mu.Unlock()
	c.baseWall = nt.Round(0) // NTP time has no monotonic reading; normalise defensively
	c.baseMono = mono
	c.anchored = true
	return nil
}

// Start anchors the clock immediately and then re-syncs every interval for the
// life of the process, in its own goroutine. It returns at once; the first sync
// happens in the background, so Now() calls before it completes use the
// system-anchored (best-effort) value. onResult, if non-nil, is called after
// each attempt with the outcome (nil on success) for logging.
func (c *Clock) Start(interval time.Duration, onResult func(error)) {
	go func() {
		report := func(err error) {
			if onResult != nil {
				onResult(err)
			}
		}
		report(c.Sync())
		t := time.NewTicker(interval)
		defer t.Stop()
		for range t.C {
			report(c.Sync())
		}
	}()
}
