package hosts

import (
	"strings"
	"testing"
)

func TestCleanDomain(t *testing.T) {
	cases := map[string]string{
		"youtube.com":                       "youtube.com",
		"https://www.youtube.com/watch?v=x": "www.youtube.com",
		"http://reddit.com/r/all":           "reddit.com",
		"  facebook.com  ":                  "facebook.com",
		"":                                  "",
		"httpbin.org":                       "httpbin.org", // starts with "http" but has no scheme
		"httpstatus.io/404":                 "httpstatus.io",
		"HTTPS://WWW.YouTube.com/watch":     "www.youtube.com",
		"YouTube.com":                       "youtube.com",
	}
	for in, want := range cases {
		if got := cleanDomain(in); got != want {
			t.Errorf("cleanDomain(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExpandDomainsGenericAndPopular(t *testing.T) {
	got := ExpandDomains([]string{"youtube.com"})
	set := make(map[string]bool, len(got))
	for _, d := range got {
		set[d] = true
	}
	want := []string{
		"youtube.com",        // the domain itself
		"www.youtube.com",    // generic expansion
		"m.youtube.com",      // generic expansion
		"mobile.youtube.com", // generic expansion
		"music.youtube.com",  // popular-site expansion
	}
	for _, w := range want {
		if !set[w] {
			t.Errorf("ExpandDomains(youtube.com) missing %q; got %v", w, got)
		}
	}
}

func TestExpandDomainsSkipsEmptyAndDedups(t *testing.T) {
	got := ExpandDomains([]string{"reddit.com", "reddit.com", "", "   "})
	if len(got) == 0 {
		t.Fatal("expected expansions for reddit.com")
	}
	seen := make(map[string]int, len(got))
	for _, d := range got {
		if d == "" {
			t.Error("ExpandDomains must not emit an empty domain")
		}
		seen[d]++
	}
	for d, n := range seen {
		if n > 1 {
			t.Errorf("domain %q appeared %d times; expected de-duplication", d, n)
		}
	}
}

// The hosts block must be byte-identical across calls, or Block's no-op check
// never matches and the enforcer rewrites the hosts file on every tick.
func TestExpandDomainsDeterministic(t *testing.T) {
	in := []string{"youtube.com", "monkeytype.com", "facebook.com", "reddit.com"}
	first := ExpandDomains(in)
	for i := 0; i < 20; i++ {
		got := ExpandDomains(in)
		if len(got) != len(first) {
			t.Fatalf("length changed: %d vs %d", len(got), len(first))
		}
		for j := range got {
			if got[j] != first[j] {
				t.Fatalf("order changed at %d: %q vs %q", j, got[j], first[j])
			}
		}
	}
}

// Popular-site expansion must respect the dot boundary: blocking netflix.com
// or dropbox.com must not pull in x.com's subdomains, nor notreddit.com
// reddit's.
func TestExpandDomainsPopularNeedsDotBoundary(t *testing.T) {
	for _, in := range []string{"netflix.com", "dropbox.com", "roblox.com", "notreddit.com"} {
		for _, d := range ExpandDomains([]string{in}) {
			if strings.HasSuffix(d, ".x.com") || d == "x.com" {
				t.Errorf("ExpandDomains(%q) includes %q (X)", in, d)
			}
			if in != "notreddit.com" {
				continue
			}
			if strings.HasSuffix(d, ".reddit.com") && !strings.HasSuffix(d, ".notreddit.com") {
				t.Errorf("ExpandDomains(%q) includes %q (reddit)", in, d)
			}
		}
	}
	// A subdomain of a popular root still expands.
	set := map[string]bool{}
	for _, d := range ExpandDomains([]string{"www.youtube.com"}) {
		set[d] = true
	}
	if !set["music.youtube.com"] {
		t.Error("www.youtube.com should still expand to music.youtube.com")
	}
}
