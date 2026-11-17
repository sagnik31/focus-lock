package version

import "testing"

func TestParse(t *testing.T) {
	cases := []struct {
		in   string
		want [4]int
		ok   bool
	}{
		{"2026.11.17.0", [4]int{2026, 11, 17, 0}, true},
		{"v2026.11.17.2", [4]int{2026, 11, 17, 2}, true},
		{"V2026.1.2.3", [4]int{2026, 1, 2, 3}, true},
		{" 2026.11.17.1 ", [4]int{2026, 11, 17, 1}, true},
		{"2026.11.17", [4]int{2026, 11, 17, 0}, true},
		{"1.0.0", [4]int{1, 0, 0, 0}, true},
		{"1", [4]int{1, 0, 0, 0}, true},
		{"65535.0.0.0", [4]int{65535, 0, 0, 0}, true},
		{"65536.0.0.0", [4]int{}, false},
		{"dev", [4]int{}, false},
		{"", [4]int{}, false},
		{"v", [4]int{}, false},
		{"1.2.3.4.5", [4]int{}, false},
		{"1..2", [4]int{}, false},
		{"1.2.", [4]int{}, false},
		{"1.-2.3", [4]int{}, false},
		{"1.2a.3", [4]int{}, false},
		{"+1.2", [4]int{}, false},
	}
	for _, c := range cases {
		got, ok := Parse(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("Parse(%q) = %v, %v; want %v, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"2026.11.17.0", "2026.11.17.0", 0},
		{"v2026.11.17.0", "2026.11.17.0", 0},
		{"2026.11.17", "2026.11.17.0", 0},
		{"2026.11.17.0", "2026.11.17.1", -1},
		{"2026.11.17.1", "2026.11.17.0", 1},
		{"2026.11.9.0", "2026.11.17.0", -1}, // numeric, not lexical
		{"2026.12.1.0", "2026.11.30.5", 1},
		{"2027.1.1.0", "2026.12.31.9", 1},
		{"1.0.0", "2026.11.17.0", -1}, // the pre-calendar installer version
		{"dev", "2026.11.17.0", -1},
		{"2026.11.17.0", "dev", 1},
		{"dev", "", 0},
	}
	for _, c := range cases {
		if got := Compare(c.a, c.b); got != c.want {
			t.Errorf("Compare(%q, %q) = %d; want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestIsDevAndNormalize(t *testing.T) {
	if !IsDev("dev") || !IsDev("") || IsDev("2026.11.17.0") {
		t.Fatal("IsDev misclassified")
	}
	if got := Normalize("v2026.11.17"); got != "2026.11.17.0" {
		t.Fatalf("Normalize = %q", got)
	}
	if got := Normalize("dev"); got != "dev" {
		t.Fatalf("Normalize(dev) = %q", got)
	}
}
