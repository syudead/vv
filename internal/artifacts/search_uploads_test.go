package artifacts

import (
	"os"
	"path/filepath"
	"testing"
)

// Startup empties the area, including uploads a killed search left, and the next search can still create one.
func TestSearchUploadsEmptyRemovesLeftUploads(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "search-uploads")
	uploads := NewSearchUploads(dir)
	left, err := uploads.Create()
	if err != nil {
		t.Fatal(err)
	}
	if err := left.Close(); err != nil {
		t.Fatal(err)
	}
	if err := uploads.Empty(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("the area survived emptying: %v", err)
	}
	next, err := uploads.Create()
	if err != nil {
		t.Fatal(err)
	}
	_ = next.Close()
	if err := uploads.Remove(next.Name()); err != nil {
		t.Fatal(err)
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Fatalf("entries after Remove = %v, %v", entries, err)
	}
}

func TestSearchUploadsRemoveRefusesOtherPaths(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	other := filepath.Join(root, "keep.mp4")
	if err := os.WriteFile(other, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := NewSearchUploads(filepath.Join(root, "search-uploads")).Remove(other); err == nil {
		t.Fatal("Remove deleted a file outside the area")
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatalf("the other file is gone: %v", err)
	}
}
