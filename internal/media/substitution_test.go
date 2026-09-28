package media

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// 代用は、生成の関数が値として返す（specs/024-import-progress/research.md R-7）。

// 指定位置にコマの無い入力では、先頭のコマで作り、そのことを返す。
func TestThumbnailReportsFirstFrameFallback(t *testing.T) {
	requireFFmpeg(t)
	videoPath := filepath.Join(t.TempDir(), "short.mp4")
	runFFmpeg(t, "-f", "lavfi", "-i", "color=c=white:s=64x64:r=10:d=2", "-c:v", "mpeg4", "-y", videoPath)

	// 尺を 100 秒と偽ると、抽出位置は 10 秒になり、2 秒の入力にはコマが無い。
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

// 指定位置でコマを取れた入力では、代用を返さない。
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

// 区間ごとの抽出に失敗する入力では、全編から作り、そのことを返す。
//
// 映像が 12 秒目から始まり、音声が 0 秒から始まる入力は、先頭の区間（0〜5 秒）に
// 映像のコマが無いので、区間ごとの抽出ではどのコマも取れない。全編の復号は最初の
// コマから作れる。
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

// 索引から作れない入力が区間ごとの抽出で作れたときは、通常の経路なので代用に数えない。
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
