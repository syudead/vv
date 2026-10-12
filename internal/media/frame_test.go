package media

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"path/filepath"
	"testing"
)

// 編集するコマは指定の位置の場面の PNG で、長辺は動画の長辺、上限 2048px である
// （specs/045-comfyui-thumbnails/research.md R-8）。

func TestFrameAtKeepsSmallVideoSizeAndPosition(t *testing.T) {
	requireFFmpeg(t)
	dir := t.TempDir()
	videoPath := filepath.Join(dir, "clock.mp4")
	runFFmpeg(t, "-f", "lavfi", "-i", timeGraySource("64x48", "30"), "-c:v", "mpeg4", "-q:v", "2", "-g", "50", "-y", videoPath)

	for _, positionMs := range []int64{3_000, 22_000} {
		frame, err := FrameAt(context.Background(), videoPath, positionMs)
		if err != nil {
			t.Fatalf("FrameAt(%d) = %v", positionMs, err)
		}
		if frame.Width != 64 || frame.Height != 48 {
			t.Fatalf("FrameAt(%d) size = %dx%d, want 64x48", positionMs, frame.Width, frame.Height)
		}
		decoded, err := png.Decode(bytes.NewReader(frame.PNG))
		if err != nil {
			t.Fatalf("not a PNG: %v", err)
		}
		// 制限範囲の 16+7t の輝度は、PNG の全範囲では (7t)*255/219 になる。
		got := float64(grayAt(decoded)) * 219 / 255 / 7
		want := float64(positionMs) / 1000
		if got < want-0.6 || got > want+0.6 {
			t.Errorf("FrameAt(%d) shows %.2f s, want %.0f s", positionMs, got, want)
		}
	}
}

func TestFrameAtCapsLongSide(t *testing.T) {
	requireFFmpeg(t)
	dir := t.TempDir()
	videoPath := filepath.Join(dir, "large.mp4")
	runFFmpeg(t, "-f", "lavfi", "-i", "color=c=gray:s=2560x1440:r=1:d=2", "-c:v", "mpeg4", "-q:v", "10", "-y", videoPath)

	frame, err := FrameAt(context.Background(), videoPath, 500)
	if err != nil {
		t.Fatal(err)
	}
	if frame.Width != 2048 || frame.Height != 1152 {
		t.Fatalf("size = %dx%d, want 2048x1152", frame.Width, frame.Height)
	}
}

func TestFrameAtFailsPastTheEnd(t *testing.T) {
	requireFFmpeg(t)
	dir := t.TempDir()
	videoPath := filepath.Join(dir, "short.mp4")
	runFFmpeg(t, "-f", "lavfi", "-i", timeGraySource("64x64", "3"), "-c:v", "mpeg4", "-q:v", "2", "-y", videoPath)

	if _, err := FrameAt(context.Background(), videoPath, 20_000); err == nil {
		t.Fatal("FrameAt past the end succeeded")
	}
}

// grayAt は画像の中央の画素の明るさ（0〜255）を返す。
func grayAt(img image.Image) uint8 {
	bounds := img.Bounds()
	r, g, b, _ := img.At(bounds.Dx()/2, bounds.Dy()/2).RGBA()
	return uint8(((r + g + b) / 3) >> 8)
}
