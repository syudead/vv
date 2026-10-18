package media

import (
	"context"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
)

func TestThumbnailAtWritesFrameAtPosition(t *testing.T) {
	requireFFmpeg(t)
	dir := t.TempDir()
	videoPath := filepath.Join(dir, "clock.mp4")
	runFFmpeg(t, "-f", "lavfi", "-i", timeGraySource("64x64", "30"), "-c:v", "mpeg4", "-q:v", "2", "-g", "50", "-y", videoPath)

	for _, positionMs := range []int64{3_000, 22_000} {
		output := filepath.Join(dir, "thumb.jpg")
		if err := ThumbnailAt(context.Background(), videoPath, positionMs, output); err != nil {
			t.Fatalf("ThumbnailAt(%d) = %v", positionMs, err)
		}
		got := jpegSeconds(t, output)
		want := float64(positionMs) / 1000
		if got < want-0.6 || got > want+0.6 {
			t.Errorf("ThumbnailAt(%d) の場面は %.2f 秒（%.0f 秒のはず）", positionMs, got, want)
		}
	}
}

func TestThumbnailAtFailsWithoutFrameAtPosition(t *testing.T) {
	requireFFmpeg(t)
	dir := t.TempDir()
	videoPath := filepath.Join(dir, "clock.mp4")
	runFFmpeg(t, "-f", "lavfi", "-i", timeGraySource("64x64", "3"), "-c:v", "mpeg4", "-q:v", "2", "-y", videoPath)

	output := filepath.Join(dir, "thumb.jpg")
	if err := ThumbnailAt(context.Background(), videoPath, 10_000, output); err == nil {
		t.Fatal("尺の外の位置で ThumbnailAt が成功した")
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("失敗したのに画像が残っている: %v", err)
	}
}

// Interpret full-range JPEG luma using the source's limited-range 16+7t time encoding.
func jpegSeconds(t *testing.T, path string) float64 {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	img, err := jpeg.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	ycbcr, ok := img.(*image.YCbCr)
	if !ok {
		t.Fatalf("JPEG の色の形式が %T", img)
	}
	b := ycbcr.Bounds()
	luma := float64(ycbcr.YCbCrAt(b.Min.X+b.Dx()/2, b.Min.Y+b.Dy()/2).Y)
	limited := luma*219/255 + 16
	return (limited - 16) / 7
}
