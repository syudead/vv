package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/media"
)

// 参照する動画が残っている内容の生成物は消さず、参照が無くなった内容の
// 生成物だけを消す。
func TestReleaseRemovesOnlyUnreferencedArtifacts(t *testing.T) {
	_, _, db, _ := missingSourceFixture(t)
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

	assets := newArtifacts(db, thumbnailsDir, slog.Default())
	assets.release([]string{"missing-source", "released"})
	assets.wait()

	if _, err := os.Stat(referenced); err != nil {
		t.Errorf("参照されている内容の生成物が消えた: %v", err)
	}
	if _, err := os.Stat(released); !os.IsNotExist(err) {
		t.Errorf("参照の無い内容の生成物が残っている (err=%v)", err)
	}
}

// 削除は同じ内容の生成と直列になる。生成の側が錠を持っている間に同じ内容の
// 動画が取り込まれて完了すれば、そのあとに走る削除は参照を見て消さない。
func TestReleaseWaitsForGenerationOfSameContent(t *testing.T) {
	ctx, _, db, _ := missingSourceFixture(t)
	thumbnailsDir := t.TempDir()
	assets := newArtifacts(db, thumbnailsDir, slog.Default())
	path := media.ThumbnailPath(thumbnailsDir, "readded")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 生成の側が錠を持っている。
	unlock := assets.lock("readded")
	assets.release([]string{"readded"})
	time.Sleep(50 * time.Millisecond)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("錠を待たずに消した: %v", err)
	}

	// 錠の中で同じ内容の動画が取り込まれ、既存の生成物を採用して完了する。
	folders, err := db.ListMediaFolders(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertVideo(ctx, domain.VideoFile{
		Path: filepath.Join(folders[0].Path, "readded.mp4"), Title: "readded", ContentKey: "readded",
		SizeBytes: 1, MTime: time.Unix(1, 0), Container: "mp4",
	}); err != nil {
		t.Fatal(err)
	}
	unlock()
	assets.wait()

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("取り込み直した動画の生成物が消えた: %v", err)
	}
}
