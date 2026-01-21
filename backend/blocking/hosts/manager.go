package hosts

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	startMarker  = "#### FOCUS LOCK START ####"
	endMarker    = "#### FOCUS LOCK END ####"
	redirectIP   = "127.0.0.1"
	redirectIPv6 = "::1"
)

// Popular sites map for robust subdomain blocking
var popularSites = map[string][]string{
	"facebook.com":  {"www.facebook.com", "m.facebook.com", "touch.facebook.com", "l.facebook.com", "static.xx.fbcdn.net"},
	"instagram.com": {"www.instagram.com", "m.instagram.com", "l.instagram.com", "api.instagram.com"},
	"twitter.com":   {"www.twitter.com", "m.twitter.com", "mobile.twitter.com", "api.twitter.com"},
	"x.com":         {"www.x.com", "m.x.com", "mobile.x.com", "api.x.com"},
	"youtube.com":   {"www.youtube.com", "m.youtube.com", "music.youtube.com"},
	"tiktok.com":    {"www.tiktok.com", "m.tiktok.com", "v16-web.tiktok.com"},
	"reddit.com":    {"www.reddit.com", "old.reddit.com", "new.reddit.com", "i.reddit.com"},
	"netflix.com":   {"www.netflix.com", "api-global.netflix.com"},
}

// Block writes the given domains to the hosts file between our markers.
// It backs up the existing block if possible (not implemented here for simplicity, but good practice).
func Block(domains []string) error {
	hostsPath := getHostsPath()

	// Ensure we can write to it (remove ReadOnly if set)
	if err := ensureWritable(hostsPath); err != nil {
		return fmt.Errorf("failed to make hosts writable: %w", err)
	}

	content, err := os.ReadFile(hostsPath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(content), "\n")
	var newLines []string
	inBlock := false

	// Filter out existing block
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == startMarker {
			inBlock = true
			continue
		}
		if trimmed == endMarker {
			inBlock = false
			continue
		}
		if !inBlock {
			newLines = append(newLines, line)
		}
	}

	// Generate new block
	expanded := ExpandDomains(domains)
	block := []string{startMarker}
	for _, domain := range expanded {
		block = append(block, fmt.Sprintf("%s %s", redirectIP, domain))
		block = append(block, fmt.Sprintf("%s %s", redirectIPv6, domain))
	}
	block = append(block, endMarker)

	// Append block to clean lines
	finalContent := strings.Join(append(newLines, block...), "\n")

	// Write back
	// Atomic write is hard with system files because of permissions/attributes, so strictly truncate and write.
	if err := os.WriteFile(hostsPath, []byte(finalContent), 0644); err != nil {
		return err
	}

	// Flush DNS Cache
	_ = exec.Command("ipconfig", "/flushdns").Run()
	return nil
}

// Unblock removes our section from the hosts file.
func Unblock() error {
	hostsPath := getHostsPath()

	// Ensure we can write to it
	if err := ensureWritable(hostsPath); err != nil {
		return fmt.Errorf("failed to make hosts writable: %w", err)
	}

	content, err := os.ReadFile(hostsPath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(content), "\n")
	var newLines []string
	inBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == startMarker {
			inBlock = true
			continue
		}
		if trimmed == endMarker {
			inBlock = false
			continue
		}
		if !inBlock {
			newLines = append(newLines, line)
		}
	}

	finalContent := strings.Join(newLines, "\n")
	if err := os.WriteFile(hostsPath, []byte(finalContent), 0644); err != nil {
		return err
	}

	// Flush DNS Cache
	_ = exec.Command("ipconfig", "/flushdns").Run()
	return nil
}

func ensureWritable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	// Check if ReadOnly bit is set
	if info.Mode().Perm()&0200 == 0 {
		// Attempt to add Write permission (0644 or just add 0200)
		return os.Chmod(path, 0644)
	}
	return nil
}

// ExpandDomains takes a list of input domains and returns a comprehensive list including subdomains.
func ExpandDomains(inputs []string) []string {
	unique := make(map[string]bool)

	for _, raw := range inputs {
		domain := cleanDomain(raw)
		if domain == "" {
			continue
		}

		// 1. Generic Expansion
		unique[domain] = true
		unique["www."+domain] = true
		unique["m."+domain] = true
		unique["mobile."+domain] = true

		// 2. Popular Site Expansion
		// Check if the domain (or its root) matches our popular list
		// Simple check: does the domain end with a key in popularSites?
		for root, subs := range popularSites {
			if strings.HasSuffix(domain, root) {
				for _, sub := range subs {
					unique[sub] = true
				}
			}
		}
	}

	var result []string
	for k := range unique {
		result = append(result, k)
	}
	return result
}

func cleanDomain(input string) string {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "http") {
		input = "http://" + input
	}
	u, err := url.Parse(input)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

func getHostsPath() string {
	system32 := os.Getenv("SystemRoot") + "\\System32"
	return filepath.Join(system32, "drivers", "etc", "hosts")
}
