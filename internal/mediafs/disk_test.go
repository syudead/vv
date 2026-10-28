package mediafs

import (
	"path/filepath"
	"testing"
)

func TestResolveDiskFailsForAnUnreadableFolder(t *testing.T) {
	root := t.TempDir()
	disk, err := New().ResolveDisk(root)
	if err != nil || disk.Key == "" {
		t.Fatalf("readable folder: disk = %+v, err = %v; want a keyed disk", disk, err)
	}
	if _, err := New().ResolveDisk(filepath.Join(root, "missing")); err == nil {
		t.Fatal("missing folder: want an error")
	}
}
