package watcher

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeAt(t *testing.T, path string, mtime time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

func TestNewestFileChange(t *testing.T) {
	root := t.TempDir()
	base := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	writeAt(t, filepath.Join(root, "a.mp4"), base)
	writeAt(t, filepath.Join(root, "sub", "b.mp4"), base.Add(5*time.Second))
	writeAt(t, filepath.Join(root, ".hidden", "c.mp4"), base.Add(time.Hour))
	writeAt(t, filepath.Join(root, "@eaDir", "d.mp4"), base.Add(time.Hour))

	t.Run("非 recursive は直下のファイルだけを見る", func(t *testing.T) {
		got, err := NewestFileChange(root, false)
		if err != nil || !got.Equal(base) {
			t.Fatalf("got %v, %v; want %v", got, err, base)
		}
	})
	t.Run("recursive は除外のディレクトリを除いて下も見る", func(t *testing.T) {
		got, err := NewestFileChange(root, true)
		if err != nil || !got.Equal(base.Add(5*time.Second)) {
			t.Fatalf("got %v, %v; want %v", got, err, base.Add(5*time.Second))
		}
	})
	t.Run("無いディレクトリとファイルの無いディレクトリはゼロ値", func(t *testing.T) {
		got, err := NewestFileChange(filepath.Join(root, "missing"), true)
		if err != nil || !got.IsZero() {
			t.Fatalf("missing: got %v, %v", got, err)
		}
		empty := t.TempDir()
		got, err = NewestFileChange(empty, true)
		if err != nil || !got.IsZero() {
			t.Fatalf("empty: got %v, %v", got, err)
		}
	})
}
