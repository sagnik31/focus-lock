// Package browserpolicy blocks websites through Chrome's enterprise policy
// (URLBlocklist) while a focus session is enforced.
//
// Why this exists alongside the hosts file: Chrome with Secure DNS
// (DNS-over-HTTPS) resolves names itself and does not reliably honour the
// hosts file, and an offline-capable site (a service-worker PWA) loads from
// cache without any DNS lookup at all. URLBlocklist is checked by Chrome on
// every navigation, independent of DNS, cache or service workers, so a
// refresh or a new visit to a blocked site shows Chrome's "blocked" page.
//
// The policy only exists while enforcement is active: the enforcer writes it
// when blocking starts and removes it when the session ends or is paused, so
// Chrome's "Managed by your organization" notice only shows during sessions.
// Entries the user (or an admin) already had in URLBlocklist are preserved;
// Focus Lock records which entries it added and only ever removes those.
package browserpolicy

import (
	"sort"

	"focus-lock/backend/blocking/hosts"
)

// Patterns converts blocked-site inputs into URLBlocklist patterns. A bare
// host pattern ("youtube.com") matches the domain and all its subdomains, so a
// leading "www." is dropped to cover the whole site. Output is de-duplicated
// and sorted so repeated calls produce identical lists (no registry churn).
func Patterns(sites []string) []string {
	seen := make(map[string]bool)
	out := []string{}
	for _, s := range sites {
		host := hosts.BaseDomain(s)
		if host == "" || seen[host] {
			continue
		}
		seen[host] = true
		out = append(out, host)
	}
	sort.Strings(out)
	return out
}

// merge computes the URLBlocklist to write and the set of entries Focus Lock
// owns afterwards. existing is the current list, owned is what Focus Lock
// added previously, desired is what it wants blocked now. Entries that are in
// existing but not owned belong to someone else and are kept, in order.
// Entries that someone else already had are not claimed as owned, so a later
// clear never removes them.
func merge(existing, owned, desired []string) (list, newOwned []string) {
	ownedSet := toSet(owned)
	var others []string
	for _, e := range existing {
		if !ownedSet[e] {
			others = append(others, e)
		}
	}
	othersSet := toSet(others)
	list = append([]string{}, others...)
	newOwned = []string{}
	for _, d := range desired {
		if othersSet[d] {
			continue
		}
		list = append(list, d)
		newOwned = append(newOwned, d)
	}
	return list, newOwned
}

// strip removes the owned entries from existing, keeping everything else.
func strip(existing, owned []string) []string {
	ownedSet := toSet(owned)
	out := []string{}
	for _, e := range existing {
		if !ownedSet[e] {
			out = append(out, e)
		}
	}
	return out
}

func toSet(list []string) map[string]bool {
	m := make(map[string]bool, len(list))
	for _, e := range list {
		m[e] = true
	}
	return m
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
