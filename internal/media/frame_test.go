package media

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"testing"
)

// 編集するコマは指定の位置の場面の PNG で、長辺は動画の長辺、上限 2048px である
//

func TestFrameAtKeepsSmallVideoSizeAndPosition(t *testing.T) {
	requireFFmpeg(t)
	videoPath := mediaFixture(t, "frame-clock.mp4")

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
	videoPath := mediaFixture(t, "large.mp4")

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
	videoPath := mediaFixture(t, "short-clock.mp4")

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
