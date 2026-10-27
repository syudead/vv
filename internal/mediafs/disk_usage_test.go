package mediafs

import (
	"path/filepath"
	"testing"
)

func TestDiskUsageReadsTheVolumeOrFails(t *testing.T) {
	root := t.TempDir()
	got, err := New().DiskUsage(root)
	if err != nil {
		t.Fatal(err)
	}
	if got.TotalBytes <= 0 || got.FreeBytes < 0 || got.FreeBytes > got.TotalBytes {
		t.Fatalf("usage = %+v, want 0 <= free <= total and total > 0", got)
	}
	if _, err := New().DiskUsage(filepath.Join(root, "missing")); err == nil {
		t.Fatal("missing path: want an error")
	}
}
