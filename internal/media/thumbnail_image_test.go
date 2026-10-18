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

func squarePNG(t *testing.T, size int) []byte {
	t.Helper()
	return rectPNG(t, size, size)
}

func rectPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
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

func TestFitThumbnailImageWebP(t *testing.T) {
	// One-pixel lossless WebP.
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

func TestFitThumbnailSource(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name                  string
		width, height         int
		aspect                float64
		wantWidth, wantHeight int
	}{
		{name: "16:9 の動画に 4000×3000", width: 4000, height: 3000, aspect: 16.0 / 9.0, wantWidth: 2048, wantHeight: 1152},
		{name: "16:9 の動画に 1280×720", width: 1280, height: 720, aspect: 16.0 / 9.0, wantWidth: 1280, wantHeight: 720},
		{name: "大きさの分からない動画に 1001×1000", width: 1001, height: 1000, aspect: 0, wantWidth: 1001, wantHeight: 1000},
		{name: "16:9 の動画に 16×9", width: 16, height: 9, aspect: 16.0 / 9.0, wantWidth: 16, wantHeight: 9},
		{name: "16:9 の動画に 1×1", width: 1, height: 1, aspect: 16.0 / 9.0, wantWidth: 1, wantHeight: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fitted, err := FitThumbnailSource(context.Background(), rectPNG(t, tc.width, tc.height), tc.aspect)
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

func TestFitThumbnailSourceRejectsUndecodableData(t *testing.T) {
	_, err := FitThumbnailSource(context.Background(), []byte("not an image"), 16.0/9.0)
	if !errors.Is(err, domain.ErrThumbnailImageInvalid) {
		t.Fatalf("err = %v, want ErrThumbnailImageInvalid", err)
	}
}

func TestFitThumbnailSourceSize(t *testing.T) {
	for _, tc := range []struct {
		width, height int
		aspect        float64
		wantW, wantH  int
	}{
		{width: 4000, height: 3000, aspect: 16.0 / 9.0, wantW: 2048, wantH: 1152},
		{width: 1280, height: 720, aspect: 16.0 / 9.0, wantW: 1280, wantH: 720},
		{width: 1000, height: 1000, aspect: 16.0 / 9.0, wantW: 1000, wantH: 562},
		{width: 3000, height: 4000, aspect: 9.0 / 16.0, wantW: 1152, wantH: 2048},
		{width: 1920, height: 1080, aspect: 9.0 / 16.0, wantW: 607, wantH: 1080},
		{width: 5000, height: 2500, aspect: 0, wantW: 2048, wantH: 1024},
		{width: 640, height: 480, aspect: 0, wantW: 640, wantH: 480},
		{width: 1001, height: 1000, aspect: 0, wantW: 1001, wantH: 1000},
		{width: 1001, height: 1001, aspect: 16.0 / 9.0, wantW: 1001, wantH: 563},
		{width: 1125, height: 2001, aspect: 9.0 / 16.0, wantW: 1125, wantH: 2000},
		{width: 16, height: 9, aspect: 16.0 / 9.0, wantW: 16, wantH: 9},
		{width: 9, height: 16, aspect: 9.0 / 16.0, wantW: 9, wantH: 16},
		{width: 17, height: 9, aspect: 16.0 / 9.0, wantW: 16, wantH: 9},
		// A one-pixel minimum may make the requested aspect unrepresentable without upscaling.
		{width: 1, height: 1, aspect: 16.0 / 9.0, wantW: 1, wantH: 1},
		{width: 1, height: 1, aspect: 0, wantW: 1, wantH: 1},
		{width: 3, height: 1, aspect: 9.0 / 16.0, wantW: 1, wantH: 1},
	} {
		w, h := fitThumbnailSourceSize(tc.width, tc.height, tc.aspect)
		if w != tc.wantW || h != tc.wantH {
			t.Errorf("fitThumbnailSourceSize(%d, %d, %v) = %dx%d, want %dx%d",
				tc.width, tc.height, tc.aspect, w, h, tc.wantW, tc.wantH)
		}
	}
}
