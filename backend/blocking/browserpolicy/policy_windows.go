//go:build windows

package browserpolicy

import (
	"errors"
	"sort"
	"strconv"

	"golang.org/x/sys/windows/registry"
)

const (
	chromeKey    = `SOFTWARE\Policies\Google\Chrome`
	blocklistKey = chromeKey + `\URLBlocklist`

	// ownedKey/ownedValue record which URLBlocklist entries Focus Lock added,
	// so Clear removes exactly those and nothing the user configured.
	ownedKey   = `SOFTWARE\FocusLock`
	ownedValue = "ChromeBlocklistOwned"
)

// ApplyChrome makes Chrome's URLBlocklist contain the given sites (plus any
// entries that were there before Focus Lock touched it). It writes to HKLM, so
// it needs an elevated process (the enforcer). It is a no-op when the policy
// already matches, so it is safe to call on every enforcement tick.
func ApplyChrome(sites []string) error {
	desired := Patterns(sites)
	if len(desired) == 0 {
		return ClearChrome()
	}
	owned, err := readOwned()
	if err != nil {
		return err
	}
	existing, err := readList()
	if err != nil {
		return err
	}
	list, newOwned := merge(existing, owned, desired)
	if equal(list, existing) && equal(newOwned, owned) {
		return nil
	}
	// Record ownership of the union first: if we are interrupted between the
	// two writes, a later Clear still knows every entry we may have written.
	if err := writeOwned(union(owned, newOwned)); err != nil {
		return err
	}
	if err := writeList(list); err != nil {
		return err
	}
	return writeOwned(newOwned)
}

// ClearChrome removes the entries Focus Lock added to Chrome's URLBlocklist.
// When nothing else is left in the policy key it deletes the key too, which
// makes Chrome's "Managed by your organization" notice go away. Cheap when
// there is nothing to clear (one registry read), so it is safe to call on
// every idle tick.
func ClearChrome() error {
	owned, err := readOwned()
	if err != nil || len(owned) == 0 {
		return err
	}
	existing, err := readList()
	if err != nil {
		return err
	}
	if err := writeList(strip(existing, owned)); err != nil {
		return err
	}
	return writeOwned(nil)
}

func readList() ([]string, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, blocklistKey, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer k.Close()
	names, err := k.ReadValueNames(-1)
	if err != nil {
		return nil, err
	}
	// Chrome reads list policies as values named "1", "2", ... in numeric
	// order, so read them back the same way.
	type entry struct {
		n int
		v string
	}
	var entries []entry
	for _, name := range names {
		n, err := strconv.Atoi(name)
		if err != nil {
			continue
		}
		v, _, err := k.GetStringValue(name)
		if err != nil {
			return nil, err // rewriting the list without it would lose it
		}
		if v == "" {
			continue
		}
		entries = append(entries, entry{n, v})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].n < entries[j].n })
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.v)
	}
	return out, nil
}

func writeList(list []string) error {
	if len(list) == 0 {
		err := registry.DeleteKey(registry.LOCAL_MACHINE, blocklistKey)
		if err != nil && !errors.Is(err, registry.ErrNotExist) {
			return err
		}
		deleteIfEmpty(chromeKey)
		return nil
	}
	k, _, err := registry.CreateKey(registry.LOCAL_MACHINE, blocklistKey, registry.ALL_ACCESS)
	if err != nil {
		return err
	}
	defer k.Close()
	// Chrome stops reading at the first missing index, so the values must be a
	// contiguous 1..N run. Write that first and then drop the surplus: entries
	// only ever move to a lower index, so a failure part-way loses none.
	for i, v := range list {
		if err := k.SetStringValue(strconv.Itoa(i+1), v); err != nil {
			return err
		}
	}
	names, err := k.ReadValueNames(-1)
	if err != nil {
		return err
	}
	for _, name := range names {
		if n, err := strconv.Atoi(name); err == nil && (n < 1 || n > len(list) || name != strconv.Itoa(n)) {
			if err := k.DeleteValue(name); err != nil {
				return err
			}
		}
	}
	return nil
}

// deleteIfEmpty removes a policy key that has no values and no subkeys left.
func deleteIfEmpty(path string) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.QUERY_VALUE|registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return
	}
	info, err := k.Stat()
	k.Close()
	if err != nil || info.ValueCount != 0 || info.SubKeyCount != 0 {
		return
	}
	_ = registry.DeleteKey(registry.LOCAL_MACHINE, path)
}

func readOwned() ([]string, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, ownedKey, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer k.Close()
	v, _, err := k.GetStringsValue(ownedValue)
	if errors.Is(err, registry.ErrNotExist) {
		return nil, nil
	}
	return v, err
}

func writeOwned(owned []string) error {
	k, _, err := registry.CreateKey(registry.LOCAL_MACHINE, ownedKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if len(owned) == 0 {
		err := k.DeleteValue(ownedValue)
		if err != nil && !errors.Is(err, registry.ErrNotExist) {
			return err
		}
		return nil
	}
	return k.SetStringsValue(ownedValue, owned)
}

func union(a, b []string) []string {
	set := toSet(a)
	out := append([]string{}, a...)
	for _, e := range b {
		if !set[e] {
			out = append(out, e)
			set[e] = true
		}
	}
	return out
}
