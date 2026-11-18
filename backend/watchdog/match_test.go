package watchdog

import "testing"

func TestDeepMatch(t *testing.T) {
	blocked := []string{"chrome.exe", "Steam"}

	cases := []struct {
		name     string
		prodName string
		fileDesc string
		want     bool
	}{
		{"product-name substring, case-insensitive", "Google Chrome", "", true},
		{"file-description substring, case-insensitive", "", "Steam Client Bootstrapper", true},
		{"no match", "Mozilla Firefox", "Firefox Browser", false},
		{"both metadata fields empty", "", "", false},
		{"blocked name stripped of .exe before match", "chrome", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := deepMatch(c.prodName, c.fileDesc, blocked); got != c.want {
				t.Errorf("deepMatch(%q, %q) = %v, want %v", c.prodName, c.fileDesc, got, c.want)
			}
		})
	}
}

func TestDeepMatchIgnoresEmptyBlockedName(t *testing.T) {
	// strings.Contains(x, "") is always true, so a stray "" or ".exe" in the
	// block list must be skipped rather than matching every process.
	if deepMatch("Notepad", "Windows Notepad", []string{""}) {
		t.Error(`an empty blocked name must not match`)
	}
	if deepMatch("Notepad", "Windows Notepad", []string{".exe"}) {
		t.Error(`a ".exe"-only blocked name must not match`)
	}
}

func TestDeepMatchIgnoresJunkAndShortNames(t *testing.T) {
	// "." used to reach the block list for apps with no resolved exe and
	// matched every process with a dot in its metadata.
	if deepMatch("Microsoft .NET", ".NET Host", []string{"."}) {
		t.Error(`"." must not match`)
	}
	if deepMatch("Node.js", "Node.js JavaScript Runtime", []string{"..exe"}) {
		t.Error(`"..exe" must not match`)
	}
	// Very short names are left to the exact-name fast loop.
	if deepMatch("Tencent QQ", "QQ", []string{"qq.exe"}) {
		t.Error("a two-letter name must not be substring-matched")
	}
	if !deepMatch("Discord", "", []string{"discord.exe", "."}) {
		t.Error("a real name next to junk must still match")
	}
}
