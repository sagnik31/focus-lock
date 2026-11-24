package storage

import "strings"

// ValidAppName reports whether name can usefully be blocked: a bare executable
// name (with or without ".exe") that contains at least one letter or digit and
// no path separator. It rejects junk such as "", "." and ".exe", which used to
// reach the block list from apps whose exe path could not be resolved
// (filepath.Base("") is ".") and which the enforcer's substring metadata match
// would then apply to almost every process.
func ValidAppName(name string) bool {
	n := strings.TrimSpace(name)
	if strings.ContainsAny(n, `\/`) {
		return false
	}
	base := strings.TrimSuffix(strings.ToLower(n), ".exe")
	for _, r := range base {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r > 127 {
			return true
		}
	}
	return false
}

// protectedApps must never be terminated: Windows' own session, shell and
// security processes (ending some of them bugchecks or signs the user out),
// and Focus Lock with the WebView2 runtime its window runs in.
var protectedApps = func() map[string]bool {
	m := map[string]bool{"secure system": true, "memory compression": true}
	for _, n := range strings.Fields(`
		focus-lock focuslockghost msedgewebview2
		system registry smss csrss wininit winlogon services lsass lsaiso svchost
		fontdrvhost dwm logonui userinit sihost taskhostw dllhost
		runtimebroker conhost ctfmon explorer
		shellexperiencehost startmenuexperiencehost searchhost
		textinputhost applicationframehost lockapp
		audiodg spoolsv wudfhost wmiprvse trustedinstaller
		msmpeng nissrv securityhealthservice consent`) {
		m[n] = true
	}
	return m
}()

// ProtectedApp reports whether name (with or without ".exe") is a process
// that Focus Lock refuses to block.
func ProtectedApp(name string) bool {
	return protectedApps[strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), ".exe")]
}

// ValidApps returns the trimmed entries of apps that can be blocked (see
// ValidAppName and ProtectedApp). The enforcer applies it too, so a protected
// name saved by an older build is ignored.
func ValidApps(apps []string) []string {
	out := make([]string, 0, len(apps))
	for _, a := range apps {
		a = strings.TrimSpace(a)
		if ValidAppName(a) && !ProtectedApp(a) {
			out = append(out, a)
		}
	}
	return out
}
