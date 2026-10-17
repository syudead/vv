package media

import (
	"context"
	"os"
	"os/exec"
	"testing"
)

func TestGenerateSeekThumbnailSetWritesSpriteIntoOutputDir(t *testing.T) {
	if _, err := exec.LookPath(seekThumbnailCommand); err != nil {
		t.Skip("ffmpegが無いため実画像の生成を省略します")
	}

	videoPath := mediaFixture(t, "green.mp4")

	output := t.TempDir()
	if err := GenerateSeekThumbnailSet(context.Background(), videoPath, output, 11_000); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(output)
	if err != nil {
		t.Fatal(err)
	}
	// 11 秒の動画は 5 秒間隔の 3 コマで、1 枚のシートに収まる。
	if len(entries) != 1 || entries[0].Name() != "000.jpg" {
		t.Fatalf("出力が %v（000.jpg の 1 個のはず）", entries)
	}
}
