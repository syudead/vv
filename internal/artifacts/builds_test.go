package artifacts

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// sizedSheetWriter writes the layout's sheets at the given size, so a test can tell builds apart by frame width.
func sizedSheetWriter(t *testing.T, width, height int) func(string) (bool, error) {
	t.Helper()
	sheet := sheetJPEG(t, width, height)
	return func(dir string) (bool, error) {
		for i := range spriteLayout.SheetCount {
			if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%03d.jpg", i)), sheet, 0o644); err != nil {
				return false, err
			}
		}
		return false, nil
	}
}

func readPreview(t *testing.T, store *Store) string {
	t.Helper()
	file, _, err := store.PreviewFile(key)
	if err != nil {
		t.Fatalf("PreviewFile: %v", err)
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func frameWidth(t *testing.T, store *Store) int {
	t.Helper()
	sprite, err := store.SeekSprite(key)
	if err != nil {
		t.Fatalf("SeekSprite: %v", err)
	}
	return sprite.FrameWidth
}

// A forced sprite publish keeps the published build readable while it writes, then switches to the new one.
func TestForcedSeekPublishSwitchesBuild(t *testing.T) {
	root := t.TempDir()
	store := New(root)
	if _, err := store.PublishSeekThumbnails(key, spriteLayout, false, sizedSheetWriter(t, 320, 180)); err != nil {
		t.Fatal(err)
	}
	oldSheet, err := store.SeekSpriteSheet(key, 0)
	if err != nil {
		t.Fatal(err)
	}

	called := false
	_, err = store.PublishSeekThumbnails(key, spriteLayout, true, func(dir string) (bool, error) {
		called = true
		if got := frameWidth(t, store); got != 32 {
			t.Errorf("書き込み中の SeekSprite の幅 = %d, want 32 (古い build)", got)
		}
		if sheet, err := store.SeekSpriteSheet(key, 0); err != nil || !bytes.Equal(sheet, oldSheet) {
			t.Errorf("書き込み中の SeekSpriteSheet: err=%v, 古い build と一致=%v", err, bytes.Equal(sheet, oldSheet))
		}
		return sizedSheetWriter(t, 640, 360)(dir)
	})
	if err != nil || !called {
		t.Fatalf("強制した公開: err=%v, write=%v", err, called)
	}
	if got := frameWidth(t, store); got != 64 {
		t.Fatalf("公開後の幅 = %d, want 64 (新しい build)", got)
	}
	if sheet, err := store.SeekSpriteSheet(key, 0); err != nil || bytes.Equal(sheet, oldSheet) {
		t.Fatalf("公開後の SeekSpriteSheet: err=%v, 古いまま=%v", err, bytes.Equal(sheet, oldSheet))
	}
	if exists(seekDirOf(root)) {
		t.Fatal("古い build のスプライトが残っている")
	}

	// An unforced publish adopts the current build, whichever build the pointer names.
	if _, err := store.PublishSeekThumbnails(key, spriteLayout, false, func(string) (bool, error) {
		t.Error("完成したスプライトの上で write を呼んだ")
		return false, nil
	}); err != nil {
		t.Fatal(err)
	}

	if err := store.RemoveContent(key); err != nil {
		t.Fatal(err)
	}
	if store.SeekThumbnailsAvailable(key) {
		t.Fatal("RemoveContent の後もスプライトが読める")
	}
	if leftovers, _ := filepath.Glob(filepath.Join(root, "seek", "ab", ".*", "*")); len(leftovers) != 0 {
		t.Fatalf("RemoveContent の後に残ったファイル: %v", leftovers)
	}
}

// A forced preview publish keeps the published pair readable while it writes, then switches to the new one.
func TestForcedPreviewPublishSwitchesBuild(t *testing.T) {
	root := t.TempDir()
	store := New(root)
	ctx := context.Background()
	if err := store.PublishPreview(ctx, key, false, writer([]byte("old")), nil); err != nil {
		t.Fatal(err)
	}

	called := false
	err := store.PublishPreview(ctx, key, true, func(output string) error {
		called = true
		if got := readPreview(t, store); got != "old" {
			t.Errorf("書き込み中の PreviewFile = %q, want old", got)
		}
		if !store.PreviewAvailable(key) {
			t.Error("書き込み中に PreviewAvailable が false")
		}
		return os.WriteFile(output, []byte("new"), 0o644)
	}, nil)
	if err != nil || !called {
		t.Fatalf("強制した公開: err=%v, write=%v", err, called)
	}
	if got := readPreview(t, store); got != "new" || !store.PreviewAvailable(key) {
		t.Fatalf("公開後の PreviewFile = %q, PreviewAvailable=%v", got, store.PreviewAvailable(key))
	}
	_, _, _, preview, manifest := existingLayout(root)
	if exists(preview) || exists(manifest) {
		t.Fatal("古い build のプレビューが残っている")
	}

	if err := store.PublishPreview(ctx, key, false, func(string) error {
		t.Error("完成したプレビューの上で write を呼んだ")
		return nil
	}, nil); err != nil {
		t.Fatal(err)
	}

	if err := store.RemoveContent(key); err != nil {
		t.Fatal(err)
	}
	if store.PreviewAvailable(key) {
		t.Fatal("RemoveContent の後もプレビューが読める")
	}
	if leftovers, _ := filepath.Glob(filepath.Join(root, "preview", "ab", ".*", "*")); len(leftovers) != 0 {
		t.Fatalf("RemoveContent の後に残ったファイル: %v", leftovers)
	}
}

// A failing forced write leaves the published build current.
func TestForcedPublishFailureKeepsBuild(t *testing.T) {
	failure := errors.New("generation failed")
	t.Run("sprite", func(t *testing.T) {
		store := New(t.TempDir())
		if _, err := store.PublishSeekThumbnails(key, spriteLayout, false, sizedSheetWriter(t, 320, 180)); err != nil {
			t.Fatal(err)
		}
		_, err := store.PublishSeekThumbnails(key, spriteLayout, true, func(dir string) (bool, error) {
			_, _ = sizedSheetWriter(t, 640, 360)(dir)
			return false, failure
		})
		if !errors.Is(err, failure) {
			t.Fatalf("err = %v, want %v", err, failure)
		}
		if got := frameWidth(t, store); got != 32 {
			t.Fatalf("失敗後の幅 = %d, want 32 (古い build)", got)
		}
	})
	t.Run("preview", func(t *testing.T) {
		store := New(t.TempDir())
		ctx := context.Background()
		if err := store.PublishPreview(ctx, key, false, writer([]byte("old")), nil); err != nil {
			t.Fatal(err)
		}
		err := store.PublishPreview(ctx, key, true, func(output string) error {
			_ = os.WriteFile(output, []byte("new"), 0o644)
			return failure
		}, nil)
		if !errors.Is(err, failure) {
			t.Fatalf("err = %v, want %v", err, failure)
		}
		if got := readPreview(t, store); got != "old" {
			t.Fatalf("失敗後の PreviewFile = %q, want old", got)
		}
	})
}

// A read that misses the build it resolved reads the build the pointer names now.
func TestReadResolvedFollowsSwitchedPointer(t *testing.T) {
	store := New(t.TempDir())
	var reads []int
	got, err := readResolved(store, previewDirName, key, func(build int) (string, error) {
		reads = append(reads, build)
		if build == 0 {
			if err := store.switchBuild(previewDirName, key, 1); err != nil {
				t.Fatal(err)
			}
			return "", fs.ErrNotExist
		}
		return "build 1", nil
	})
	if err != nil || got != "build 1" || len(reads) != 2 {
		t.Fatalf("readResolved = %q, %v; reads %v", got, err, reads)
	}
}
