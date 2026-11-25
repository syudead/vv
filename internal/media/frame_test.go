package media

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func TestFrameAtKeepsSmallVideoSizeAndPosition(t *testing.T) {
	requireFFmpeg(t)
	videoPath := mediaFixture(t, "frame-clock.mp4")

	for _, positionMs := range []int64{3_000, 22_000} {
		frame, err := FrameAt(context.Background(), videoPath, positionMs, domain.ThumbnailFrameMaxSide)
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
		// Limited-range luma 16+7t becomes (7t)*255/219 in full-range PNG.
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

	frame, err := FrameAt(context.Background(), videoPath, 500, domain.ThumbnailFrameMaxSide)
	if err != nil {
		t.Fatal(err)
	}
	if frame.Width != 2048 || frame.Height != 1152 {
		t.Fatalf("size = %dx%d, want 2048x1152", frame.Width, frame.Height)
	}
}

func TestFrameAtFitsTheRequestedLongSideWithoutEnlarging(t *testing.T) {
	t.Parallel()
	requireFFmpeg(t)
	videoPath := filepath.Join(t.TempDir(), "wide.mp4")
	generate := exec.Command("ffmpeg", "-nostdin", "-v", "error",
		"-f", "lavfi", "-i", "testsrc=size=320x180:rate=2:duration=1", "-c:v", "mpeg4", videoPath)
	if out, err := generate.CombinedOutput(); err != nil {
		t.Fatalf("cannot generate the video: %v: %s", err, out)
	}

	for _, tc := range []struct{ maxSide, width, height int }{
		{160, 160, 90},
		{640, 320, 180},
	} {
		frame, err := FrameAt(context.Background(), videoPath, 0, tc.maxSide)
		if err != nil {
			t.Fatalf("FrameAt(maxSide %d) = %v", tc.maxSide, err)
		}
		config, err := png.DecodeConfig(bytes.NewReader(frame.PNG))
		if err != nil {
			t.Fatalf("FrameAt(maxSide %d) is not a PNG: %v", tc.maxSide, err)
		}
		if config.Width != tc.width || config.Height != tc.height || frame.Width != tc.width || frame.Height != tc.height {
			t.Errorf("FrameAt(maxSide %d) = %dx%d PNG reported as %dx%d, want %dx%d",
				tc.maxSide, config.Width, config.Height, frame.Width, frame.Height, tc.width, tc.height)
		}
	}
}

func TestFrameAtFailsPastTheEnd(t *testing.T) {
	requireFFmpeg(t)
	videoPath := mediaFixture(t, "short-clock.mp4")

	if _, err := FrameAt(context.Background(), videoPath, 20_000, domain.ThumbnailFrameMaxSide); err == nil {
		t.Fatal("FrameAt past the end succeeded")
	}
}

func grayAt(img image.Image) uint8 {
	bounds := img.Bounds()
	r, g, b, _ := img.At(bounds.Dx()/2, bounds.Dy()/2).RGBA()
	return uint8(((r + g + b) / 3) >> 8)
}
