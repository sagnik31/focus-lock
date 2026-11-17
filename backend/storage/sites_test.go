package storage

import (
	"reflect"
	"testing"
	"time"
)

func TestRecentSitesTrackAddsAndRemovals(t *testing.T) {
	now := time.Now()
	cfg := idleConfig()

	must := func(op Operation) {
		t.Helper()
		if err := ApplyOperation(cfg, op, now); err != nil {
			t.Fatalf("%s: %v", op.Type, err)
		}
	}
	must(Operation{Type: OpAddSite, URL: "youtube.com"})
	must(Operation{Type: OpAddSites, Sites: []string{"a.com", "b.com"}})
	must(Operation{Type: OpAddSite, URL: "youtube.com"}) // re-add moves to front
	must(Operation{Type: OpRemoveSite, URL: "reddit.com"})

	want := []string{"reddit.com", "youtube.com", "a.com", "b.com"}
	if !reflect.DeepEqual(cfg.RecentSites, want) {
		t.Fatalf("recent = %v, want %v", cfg.RecentSites, want)
	}

	must(Operation{Type: OpForgetRecent, URL: "a.com"})
	if want := []string{"reddit.com", "youtube.com", "b.com"}; !reflect.DeepEqual(cfg.RecentSites, want) {
		t.Fatalf("after forget = %v, want %v", cfg.RecentSites, want)
	}
}

func TestRecentSitesCapped(t *testing.T) {
	cfg := idleConfig()
	for i := 0; i < maxRecentSites+5; i++ {
		_ = ApplyOperation(cfg, Operation{Type: OpAddSite, URL: string(rune('a'+i)) + ".com"}, time.Now())
	}
	if len(cfg.RecentSites) != maxRecentSites {
		t.Fatalf("len = %d, want %d", len(cfg.RecentSites), maxRecentSites)
	}
}

func TestSiteGroupsAllowedDuringLockAndValidated(t *testing.T) {
	now := time.Now()
	cfg := lockedConfig(now)

	ok := Operation{Type: OpSaveSiteGroups, SiteGroups: []SiteGroup{
		{ID: "g1", Name: "  News ", Sites: []string{"bbc.com", " bbc.com", "", "cnn.com"}},
	}}
	if err := ApplyOperation(cfg, ok, now); err != nil {
		t.Fatalf("saving groups during a lock should be allowed: %v", err)
	}
	if got := cfg.SiteGroups[0]; got.Name != "News" || !reflect.DeepEqual(got.Sites, []string{"bbc.com", "cnn.com"}) {
		t.Fatalf("group not cleaned: %+v", got)
	}
	// Saving groups never changes what is blocked.
	if !reflect.DeepEqual(cfg.BlockedSites, []string{"reddit.com"}) {
		t.Fatalf("blocked sites changed: %v", cfg.BlockedSites)
	}

	bad := [][]SiteGroup{
		{{ID: "", Name: "x"}},
		{{ID: "g1", Name: "  "}},
		{{ID: "g1", Name: "News"}, {ID: "g2", Name: "news"}},
	}
	for _, groups := range bad {
		if err := ApplyOperation(cfg, Operation{Type: OpSaveSiteGroups, SiteGroups: groups}, now); err == nil {
			t.Errorf("groups %+v should be refused", groups)
		}
	}
}

func TestImportMergesGroupsAndRecent(t *testing.T) {
	now := time.Now()
	cfg := lockedConfig(now) // import must work mid-lock (append-only)
	cfg.SiteGroups = []SiteGroup{{ID: "g1", Name: "News", Sites: []string{"bbc.com"}}}
	cfg.RecentSites = []string{"youtube.com"}

	op := Operation{
		Type: OpImport,
		SiteGroups: []SiteGroup{
			{ID: "x1", Name: "news", Sites: []string{"cnn.com", "bbc.com"}}, // merges by name
			{ID: "x2", Name: "Games", Sites: []string{"roblox.com"}},        // new group
		},
		RecentSites: []string{"monkeytype.com", "youtube.com"},
	}
	if err := ApplyOperation(cfg, op, now); err != nil {
		t.Fatalf("import refused: %v", err)
	}

	want := []SiteGroup{
		{ID: "g1", Name: "News", Sites: []string{"bbc.com", "cnn.com"}},
		{ID: "x2", Name: "Games", Sites: []string{"roblox.com"}},
	}
	if !reflect.DeepEqual(cfg.SiteGroups, want) {
		t.Fatalf("groups = %+v, want %+v", cfg.SiteGroups, want)
	}
	// The user's own history stays first; imports are appended, de-duplicated.
	if want := []string{"youtube.com", "monkeytype.com"}; !reflect.DeepEqual(cfg.RecentSites, want) {
		t.Fatalf("recent = %v, want %v", cfg.RecentSites, want)
	}
	// Groups and history never change what is blocked.
	if !reflect.DeepEqual(cfg.BlockedSites, []string{"reddit.com"}) {
		t.Fatalf("blocked sites changed: %v", cfg.BlockedSites)
	}
}
