package mediafs

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveDiskFailsForAnUnreadableFolder(t *testing.T) {
	root := t.TempDir()
	disk, err := New().ResolveDisk(root, false)
	if err != nil || disk.Key == "" {
		t.Fatalf("readable folder: disk = %+v, err = %v; want a keyed disk", disk, err)
	}
	if _, err := New().ResolveDisk(filepath.Join(root, "missing"), false); err == nil {
		t.Fatal("missing folder: want an error")
	}
}

// An unmounted mount point is a readable, empty directory, so an empty folder counts as unreadable only while the
// library holds videos under it.
func TestResolveDiskTreatsAnEmptyPopulatedFolderAsUnmounted(t *testing.T) {
	for _, tc := range []struct {
		name      string
		populated bool
		entry     bool
		unmounted bool
	}{
		{name: "empty and populated", populated: true, unmounted: true},
		{name: "empty new folder"},
		{name: "populated with an entry", populated: true, entry: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if tc.entry {
				if err := os.WriteFile(filepath.Join(root, "movie.mp4"), nil, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			_, err := New().ResolveDisk(root, tc.populated)
			if errors.Is(err, ErrMediaFolderUnmounted) != tc.unmounted || (err != nil && !tc.unmounted) {
				t.Fatalf("err = %v, want unmounted %v", err, tc.unmounted)
			}
		})
	}
}
