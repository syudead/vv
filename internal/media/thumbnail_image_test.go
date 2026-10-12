package media

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// 適用する画像は動画の表示上の縦横比に合わせ、長辺 640px の JPEG にする
// （specs/045-comfyui-thumbnails/research.md R-7）。

func squarePNG(t *testing.T, size int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := range size {
		for x := range size {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestFitThumbnailImage(t *testing.T) {
	for _, tc := range []struct {
		name       string
		aspect     float64
		wantWidth  int
		wantHeight int
	}{
		{name: "16:9 の動画", aspect: 16.0 / 9.0, wantWidth: 640, wantHeight: 360},
		{name: "9:16 の動画", aspect: 9.0 / 16.0, wantWidth: 360, wantHeight: 640},
		{name: "大きさの分からない動画", aspect: 0, wantWidth: 640, wantHeight: 640},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fitted, err := FitThumbnailImage(context.Background(), squarePNG(t, 512), tc.aspect)
			if err != nil {
				t.Fatal(err)
			}
			config, err := jpeg.DecodeConfig(bytes.NewReader(fitted))
			if err != nil {
				t.Fatalf("JPEG として読めない: %v", err)
			}
			if config.Width != tc.wantWidth || config.Height != tc.wantHeight {
				t.Fatalf("大きさ = %dx%d, want %dx%d", config.Width, config.Height, tc.wantWidth, tc.wantHeight)
			}
		})
	}
}

// 提供元の出力の WebP も、PNG と同じく動画の縦横比の JPEG になる。
func TestFitThumbnailImageWebP(t *testing.T) {
	// 1×1 の可逆の WebP。
	webp := []byte("RIFF\x1a\x00\x00\x00WEBPVP8L\x0d\x00\x00\x00\x2f\x00\x00\x00\x10\x07\x10\x11\x11\x88\x88\xfe\x07\x00")
	fitted, err := FitThumbnailImage(context.Background(), webp, 16.0/9.0)
	if err != nil {
		t.Fatal(err)
	}
	config, err := jpeg.DecodeConfig(bytes.NewReader(fitted))
	if err != nil || config.Width != 640 || config.Height != 360 {
		t.Fatalf("fitted = %+v, %v", config, err)
	}
}

func TestFitThumbnailImageRejectsUndecodableData(t *testing.T) {
	_, err := FitThumbnailImage(context.Background(), []byte("not an image"), 16.0/9.0)
	if !errors.Is(err, domain.ErrThumbnailImageInvalid) {
		t.Fatalf("err = %v, want ErrThumbnailImageInvalid", err)
	}
}

func TestFitThumbnailSize(t *testing.T) {
	for _, tc := range []struct {
		aspect        float64
		width, height int
	}{
		{aspect: 16.0 / 9.0, width: 640, height: 360},
		{aspect: 4.0 / 3.0, width: 640, height: 480},
		{aspect: 1, width: 640, height: 640},
		{aspect: 2.39, width: 640, height: 268},
		{aspect: 9.0 / 16.0, width: 360, height: 640},
		{aspect: 1000, width: 640, height: 2},
	} {
		width, height := fitThumbnailSize(tc.aspect)
		if width != tc.width || height != tc.height {
			t.Errorf("fitThumbnailSize(%v) = %dx%d, want %dx%d", tc.aspect, width, height, tc.width, tc.height)
		}
	}
}
