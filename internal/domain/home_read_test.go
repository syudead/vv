package domain

import (
	"errors"
	"slices"
	"testing"
)

// Canonical folder keys round-trip.
func TestParseHomeShelfKey(t *testing.T) {
	for _, value := range []string{"folder:12"} {
		key, err := ParseHomeShelfKey(value)
		if err != nil || key.String() != value {
			t.Errorf("ParseHomeShelfKey(%q) = %+v, %v", value, key, err)
		}
	}
	if key, _ := ParseHomeShelfKey("folder:12"); key != FolderShelfKey(12) {
		t.Errorf("folder:12 = %+v", key)
	}
	for _, value := range []string{"", "folder:", "folder:0", "folder:-1", "folder:01", "folder:x", "folder", "later"} {
		if _, err := ParseHomeShelfKey(value); !errors.Is(err, ErrInvalidHomeShelfKey) {
			t.Errorf("ParseHomeShelfKey(%q) error = %v, want ErrInvalidHomeShelfKey", value, err)
		}
	}
}

// 鍵の並びは 1 から MaxHomeShelfKeys 個で、同じ鍵は最初の 1 つだけにする。
func TestParseHomeShelfKeys(t *testing.T) {
	keys, err := ParseHomeShelfKeys([]string{"folder:2", "folder:3", "folder:2"})
	if err != nil || !slices.Equal(keys, []HomeShelfKey{FolderShelfKey(2), FolderShelfKey(3)}) {
		t.Fatalf("keys = %+v, %v", keys, err)
	}
	nine := slices.Repeat([]string{"folder:2"}, MaxHomeShelfKeys+1)
	for _, values := range [][]string{nil, nine, {"folder:2", "x"}} {
		if _, err := ParseHomeShelfKeys(values); !errors.Is(err, ErrInvalidHomeShelfKey) {
			t.Errorf("ParseHomeShelfKeys(%v) error = %v, want ErrInvalidHomeShelfKey", values, err)
		}
	}
}
