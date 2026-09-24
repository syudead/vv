package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/syudead/vv/internal/media"
)

// 参照する動画が残っている内容の生成物は消さず、参照が無くなった内容の
// 生成物だけを消す。
func TestRemoveUnreferencedArtifacts(t *testing.T) {
	ctx, _, db, _ := missingSourceFixture(t)
	thumbnailsDir := t.TempDir()
	write := func(path string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	referenced := media.ThumbnailPath(thumbnailsDir, "missing-source")
	released := media.PreviewPath(thumbnailsDir, "released")
	write(referenced)
	write(released)

	if err := removeUnreferencedArtifacts(ctx, db, thumbnailsDir, "missing-source"); err != nil {
		t.Fatal(err)
	}
	if err := removeUnreferencedArtifacts(ctx, db, thumbnailsDir, "released"); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(referenced); err != nil {
		t.Errorf("参照されている内容の生成物が消えた: %v", err)
	}
	if _, err := os.Stat(released); !os.IsNotExist(err) {
		t.Errorf("参照の無い内容の生成物が残っている (err=%v)", err)
	}
}
