package media

import (
	"bytes"
	"context"
	"image/jpeg"
	"path/filepath"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func TestFramePairImagesPutsBothFilesSideBySide(t *testing.T) {
	requireFFmpeg(t)
	paths := [2]string{
		filepath.Join("testdata", "fingerprints", "base.mp4"),
		filepath.Join("testdata", "fingerprints", "letterbox.mp4"),
	}
	images, err := FramePairImages(context.Background(), paths,
		[]domain.VersionEvidenceFrame{{AMs: 5000, BMs: 5000}, {AMs: 30000, BMs: 29000}})
	if err != nil || len(images) != 2 {
		t.Fatalf("FramePairImages = %d 枚, %v", len(images), err)
	}
	config, err := jpeg.DecodeConfig(bytes.NewReader(images[0]))
	if err != nil || config.Width != 640 || config.Height != 180 {
		t.Errorf("画像 = %dx%d, %v, want 左 320x180 の 2 倍の幅", config.Width, config.Height, err)
	}
	if _, err := FramePairImages(context.Background(), [2]string{paths[0], filepath.Join(t.TempDir(), "x.mp4")},
		[]domain.VersionEvidenceFrame{{}}); err == nil {
		t.Error("無いファイルで誤りにならない")
	}
}
