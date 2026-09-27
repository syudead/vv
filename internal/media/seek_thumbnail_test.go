package media

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGenerateSeekThumbnailSetWritesSpriteIntoOutputDir(t *testing.T) {
	if _, err := exec.LookPath(seekThumbnailCommand); err != nil {
		t.Skip("ffmpegが無いため実画像の生成を省略します")
	}

	videoPath := filepath.Join(t.TempDir(), "green.mp4")
	args := []string{
		"-nostdin", "-v", "error",
		"-f", "lavfi", "-i", "color=c=green:s=64x64:r=30:d=11",
		"-c:v", "mpeg4", "-y", videoPath,
	}
	if output, err := exec.Command(seekThumbnailCommand, args...).CombinedOutput(); err != nil {
		t.Fatalf("fixture生成に失敗しました: %v: %s", err, output)
	}

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
