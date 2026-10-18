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
	firstFrame, err := Thumbnail(context.Background(), videoPath, 100_000, output)
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

	firstFrame, err := Thumbnail(context.Background(), videoPath, 20_000, filepath.Join(t.TempDir(), "thumbnail.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	if firstFrame {
		t.Fatal("指定位置で作れたのに、代用を返した")
	}
}

// Video starts at second twelve while audio starts at zero; segment seeking yields no frames, but
// full decoding can start at the first available frame.
func TestGenerateSeekSpriteReportsFullDecode(t *testing.T) {
	requireFFmpeg(t)
	dir := t.TempDir()
	video := filepath.Join(dir, "video.mkv")
	videoPath := filepath.Join(dir, "late-video.mkv")
	runFFmpeg(t, "-f", "lavfi", "-i", "color=c=white:s=64x64:r=10:d=3", "-c:v", "mpeg4", "-y", video)
	runFFmpeg(t, "-itsoffset", "12", "-i", video, "-f", "lavfi", "-t", "16", "-i", "anullsrc=r=8000:cl=mono",
		"-map", "0:v", "-map", "1:a", "-c:v", "copy", "-c:a", "mp2", "-y", videoPath)

	layout := domain.NewSeekSpriteLayout(5_000)
	if err := generateSeekSpriteParallel(context.Background(), videoPath, t.TempDir(), layout); err == nil {
		t.Fatal("前提: 区間ごとの抽出が成功した")
	}
	output := t.TempDir()
	fullDecode, err := GenerateSeekSprite(context.Background(), videoPath, output, layout)
	if err != nil {
		t.Fatal(err)
	}
	if !fullDecode {
		t.Fatal("全編から作ったのに、代用を返さない")
	}
	if len(readSheets(t, output)) != 1 {
		t.Fatal("シートが 1 枚ではない")
	}
}

func TestGenerateSeekSpriteDoesNotCountPerIntervalExtraction(t *testing.T) {
	requireFFmpeg(t)
	videoPath := filepath.Join(t.TempDir(), "mpeg4.mp4")
	runFFmpeg(t, "-f", "lavfi", "-i", timeGraySource("64x64", "12"), "-c:v", "mpeg4", "-y", videoPath)

	fullDecode, err := GenerateSeekSprite(context.Background(), videoPath, t.TempDir(), domain.NewSeekSpriteLayout(12_000))
	if err != nil {
		t.Fatal(err)
	}
	if fullDecode {
		t.Fatal("区間ごとの抽出で作れたのに、代用を返した")
	}
}
