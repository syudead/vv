package media

import (
	"context"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSeekThumbnailArgsGenerateFiveSecondFrames(t *testing.T) {
	args := seekThumbnailArgs("/media/a.mp4", "/cache/%06d.jpg")
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"-i /media/a.mp4",
		"-map 0:V:0?",
		"select='isnan(prev_selected_t)+gt(floor(t/5)\\,floor(prev_selected_t/5))'",
		"scale=min(320\\,iw):-2",
		"-start_number 0",
		"/cache/%06d.jpg",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("生成引数に %q が無い: %v", want, args)
		}
	}
}

func TestGenerateSeekThumbnailsUsesFixedBucketsAtFractionalFrameRate(t *testing.T) {
	if _, err := exec.LookPath(seekThumbnailCommand); err != nil {
		t.Skip("ffmpegが無いため実画像の生成を省略します")
	}

	videoPath := filepath.Join(t.TempDir(), "fractional.mp4")
	args := []string{
		"-nostdin", "-v", "error",
		"-f", "lavfi", "-i", "color=c=green:s=64x64:r=30000/1001:d=16",
		"-c:v", "mpeg4", "-y", videoPath,
	}
	if output, err := exec.Command(seekThumbnailCommand, args...).CombinedOutput(); err != nil {
		t.Fatalf("fixture生成に失敗しました: %v: %s", err, output)
	}

	thumbnailsDir := t.TempDir()
	if err := GenerateSeekThumbnails(context.Background(), videoPath, thumbnailsDir, "fractional:1"); err != nil {
		t.Fatal(err)
	}
	cache := NewSeekThumbnailCache(thumbnailsDir)
	for _, position := range []int64{0, 5000, 10_000, 15_000, 15_999} {
		if _, err := cache.Read(context.Background(), "fractional:1", position); err != nil {
			t.Fatalf("position %d: %v", position, err)
		}
	}
}

func TestSeekThumbnailPathUsesFiveSecondBucket(t *testing.T) {
	root := filepath.Join("cache", "seek")
	first := SeekThumbnailPath(root, "abcdef:12", 4999)
	second := SeekThumbnailPath(root, "abcdef:12", 5000)
	if filepath.Base(first) != "000000.jpg" || filepath.Base(second) != "000001.jpg" {
		t.Fatalf("paths = %q, %q", first, second)
	}
	if !strings.Contains(first, filepath.Join("ab", "abcdef_12")) {
		t.Fatalf("content path = %q", first)
	}
}

func TestGenerateAndReadSeekThumbnails(t *testing.T) {
	if _, err := exec.LookPath(seekThumbnailCommand); err != nil {
		t.Skip("ffmpegが無いため実画像の生成を省略します")
	}

	videoPath := filepath.Join(t.TempDir(), "colors.mp4")
	args := []string{
		"-nostdin", "-v", "error",
		"-f", "lavfi", "-i", "color=c=red:s=64x64:r=30:d=3",
		"-f", "lavfi", "-i", "color=c=blue:s=64x64:r=30:d=3",
		"-filter_complex", "[0:v][1:v]concat=n=2:v=1:a=0",
		"-c:v", "mpeg4", "-y", videoPath,
	}
	if output, err := exec.Command(seekThumbnailCommand, args...).CombinedOutput(); err != nil {
		t.Fatalf("fixture生成に失敗しました: %v: %s", err, output)
	}

	thumbnailsDir := t.TempDir()
	if err := GenerateSeekThumbnails(context.Background(), videoPath, thumbnailsDir, "abcdef:12"); err != nil {
		t.Fatal(err)
	}
	cache := NewSeekThumbnailCache(thumbnailsDir)
	for _, position := range []int64{0, 4999, 5000, 5999} {
		data, err := cache.Read(context.Background(), "abcdef:12", position)
		if err != nil {
			t.Fatalf("position %d: %v", position, err)
		}
		if _, err := jpeg.Decode(strings.NewReader(string(data))); err != nil {
			t.Fatalf("position %d JPEG: %v", position, err)
		}
	}
}

func TestSeekThumbnailCacheReportsMissingFrame(t *testing.T) {
	cache := NewSeekThumbnailCache(t.TempDir())
	_, err := cache.Read(context.Background(), "missing:1", 0)
	if !os.IsNotExist(err) {
		t.Fatalf("error = %v", err)
	}
}

func TestRemoveSeekThumbnails(t *testing.T) {
	thumbnailsDir := t.TempDir()
	target := SeekThumbnailDir(filepath.Join(thumbnailsDir, "seek"), "remove:1")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := RemoveSeekThumbnails(thumbnailsDir, "remove:1"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("removed cache error = %v", err)
	}
}
