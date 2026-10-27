package media

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func TestThumbnailReportsFirstFrameFallback(t *testing.T) {
	requireFFmpeg(t)
	videoPath := filepath.Join(t.TempDir(), "short.mp4")
	runFFmpeg(t, "-f", "lavfi", "-i", "color=c=white:s=64x64:r=10:d=2", "-c:v", "mpeg4", "-y", videoPath)

	// A fake hundred-second duration selects second ten, beyond the two-second source.
	output := filepath.Join(t.TempDir(), "thumbnail.jpg")
	firstFrame, err := Thumbnail(context.Background(), videoPath, 100_000, output, "")
	if err != nil {
		t.Fatal(err)
	}
	if !firstFrame {
		t.Fatal("先頭のコマで作ったのに、代用を返さない")
	}
	if info, err := os.Stat(output); err != nil || info.Size() == 0 {
		t.Fatalf("サムネイルが書かれていない: %v", err)
	}
}

func TestThumbnailReportsNoFallbackAtOffset(t *testing.T) {
	requireFFmpeg(t)
	videoPath := filepath.Join(t.TempDir(), "normal.mp4")
	runFFmpeg(t, "-f", "lavfi", "-i", "color=c=white:s=64x64:r=10:d=20", "-c:v", "mpeg4", "-y", videoPath)

	firstFrame, err := Thumbnail(context.Background(), videoPath, 20_000, filepath.Join(t.TempDir(), "thumbnail.jpg"), "")
	if err != nil {
		t.Fatal(err)
	}
	if firstFrame {
		t.Fatal("指定位置で作れたのに、代用を返した")
	}
}

// Video starts at second twelve while audio starts at zero, so no interval yields an image. Generation
// fails instead of decoding the whole video.
func TestGenerateSeekSpriteFailsWithoutIntervalImages(t *testing.T) {
	requireFFmpeg(t)
	dir := t.TempDir()
	video := filepath.Join(dir, "video.mkv")
	videoPath := filepath.Join(dir, "late-video.mkv")
	runFFmpeg(t, "-f", "lavfi", "-i", "color=c=white:s=64x64:r=10:d=3", "-c:v", "mpeg4", "-y", video)
	runFFmpeg(t, "-itsoffset", "12", "-i", video, "-f", "lavfi", "-t", "16", "-i", "anullsrc=r=8000:cl=mono",
		"-map", "0:v", "-map", "1:a", "-c:v", "copy", "-c:a", "mp2", "-y", videoPath)

	output := t.TempDir()
	if err := GenerateSeekSprite(context.Background(), videoPath, output, domain.NewSeekSpriteLayout(5_000)); err == nil {
		t.Fatal("どの区間にも画像が無いのに成功した")
	}
	if _, err := os.Stat(filepath.Join(output, "000.jpg")); !os.IsNotExist(err) {
		t.Fatalf("失敗したのにシートがある: %v", err)
	}
}
