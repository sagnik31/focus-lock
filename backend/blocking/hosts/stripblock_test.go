package hosts

import (
	"strings"
	"testing"
)

// G30: without the end marker, only lines Focus Lock wrote are removed.
func TestStripBlock(t *testing.T) {
	s, e := startMarker, endMarker
	cases := map[string][2]string{
		"complete":    {"a|" + s + "|127.0.0.1 x.com|::1 x.com|" + e + "|b", "a|b"},
		"none":        {"a|b|", "a|b|"},
		"no end":      {"a|" + s + "|127.0.0.1 x.com|::1 x.com|10.0.0.1 nas|b", "a|10.0.0.1 nas|b"},
		"own mapping": {"a|" + s + "|127.0.0.1 x.com|::1 x.com|127.0.0.1 dev.local|b", "a|127.0.0.1 dev.local|b"},
		"cut line":    {"a|" + s + "|127.0.0.1 x.com|::1 x.com|127.0.0.1 y.c", "a"},
		"cut pair":    {"a|" + s + "|127.0.0.1 x.com|::1 x.", "a"},
		"cut address": {"a|" + s + "|127.0.0.1 x.com|::1 x.com|127.0", "a"},
		"two starts":  {"a|" + s + "|127.0.0.1 x.com|::1 x.com|mine|" + s + "|127.0.0.1 x.com|::1 x.com|" + e, "a|mine"},
		"stray end":   {"a|" + e + "|b", "a|b"},
		"crlf":        {"a\r|" + s + "\r|127.0.0.1 x.com\r|::1 x.com\r|b\r", "a\r|b\r"},
	}
	for name, c := range cases {
		if got := strings.Join(stripBlock(strings.Split(c[0], "|")), "|"); got != c[1] {
			t.Errorf("%s: got %q, want %q", name, got, c[1])
		}
	}
}

// G50: "www." names the same site, and entries no layer can act on are invalid.
func TestBaseDomainAndValidSite(t *testing.T) {
	if got := BaseDomain("https://www.example.org/path"); got != "example.org" {
		t.Errorf("BaseDomain = %q", got)
	}
	if got := BaseDomain("www.com"); got != "www.com" {
		t.Errorf("BaseDomain(www.com) = %q", got)
	}
	for _, s := range []string{"example.org", " https://a.b/c ", "1.2.3.4"} {
		if !ValidSite(s) {
			t.Errorf("%q should be valid", s)
		}
	}
	for _, s := range []string{"", "youtube", "http://", "http:///x", "a b.com", "x.com\n1.2.3.4 y", "."} {
		if ValidSite(s) {
			t.Errorf("%q should be invalid", s)
		}
	}
}
