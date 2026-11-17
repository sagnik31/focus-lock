package browserpolicy

import (
	"reflect"
	"testing"
)

func TestPatterns(t *testing.T) {
	got := Patterns([]string{"https://www.YouTube.com/watch?v=1", "youtube.com", " monkeytype.com ", "", "m.facebook.com"})
	want := []string{"m.facebook.com", "monkeytype.com", "youtube.com"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Patterns = %v, want %v", got, want)
	}
}

func TestMergeKeepsOthersAndOnlyOwnsWhatItAdded(t *testing.T) {
	existing := []string{"corp-blocked.com", "youtube.com", "old-fl.com"}
	owned := []string{"old-fl.com"}
	desired := []string{"monkeytype.com", "youtube.com"}

	list, newOwned := merge(existing, owned, desired)

	// The user's entries stay (youtube.com was theirs already, so it is not
	// duplicated or claimed); our stale entry goes; the new one is added.
	if want := []string{"corp-blocked.com", "youtube.com", "monkeytype.com"}; !reflect.DeepEqual(list, want) {
		t.Fatalf("list = %v, want %v", list, want)
	}
	if want := []string{"monkeytype.com"}; !reflect.DeepEqual(newOwned, want) {
		t.Fatalf("owned = %v, want %v", newOwned, want)
	}

	// Clearing removes only what we own.
	if got, want := strip(list, newOwned), []string{"corp-blocked.com", "youtube.com"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("strip = %v, want %v", got, want)
	}
}

func TestMergeIsStable(t *testing.T) {
	desired := []string{"a.com", "b.com"}
	list, owned := merge(nil, nil, desired)
	list2, owned2 := merge(list, owned, desired)
	if !equal(list, list2) || !equal(owned, owned2) {
		t.Fatalf("second merge changed state: %v/%v -> %v/%v", list, owned, list2, owned2)
	}
}
