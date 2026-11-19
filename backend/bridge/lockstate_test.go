package bridge

import (
	"encoding/json"
	"testing"
	"time"

	"focus-lock/backend/clock"
	"focus-lock/backend/storage"
)

func decodeLock(t *testing.T, s string, err error) map[string]string {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("bad JSON %q: %v", s, err)
	}
	return m
}

func TestGetLockState(t *testing.T) {
	a := &App{Store: &storage.Store{}}
	s, err := a.GetLockState()
	if m := decodeLock(t, s, err); m["kind"] != "none" || m["end_time"] != "" {
		t.Fatalf("idle: %v", m)
	}

	end := clock.Default.Now().Add(time.Hour).Round(0)
	a.Store.Data.LockEndTime = end
	s, err = a.GetLockState()
	m := decodeLock(t, s, err)
	got, err := time.Parse(time.RFC3339Nano, m["end_time"])
	if m["kind"] != "manual" || err != nil || !got.Equal(end) {
		t.Fatalf("manual: %v (%v)", m, err)
	}
}

func TestLockStateJSONSchedule(t *testing.T) {
	end := time.Date(2026, 11, 3, 6, 0, 0, 0, time.UTC)
	s, err := lockStateJSON(storage.LockState{Kind: storage.LockSchedule, EndTime: end})
	m := decodeLock(t, s, err)
	if m["kind"] != "schedule" || m["end_time"] != "2026-11-03T06:00:00Z" {
		t.Fatalf("schedule: %v", m)
	}
}
