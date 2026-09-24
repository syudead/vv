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

	output := t.TempDir()
	if err := GenerateSeekThumbnails(context.Background(), videoPath, filepath.Join(output, "%06d.jpg")); err != nil {
		t.Fatal(err)
	}
	// 16 秒の動画は 0・5・10・15 秒の4つの区間に1枚ずつになる。
	for _, name := range []string{"000000.jpg", "000001.jpg", "000002.jpg", "000003.jpg"} {
		if _, err := os.Stat(filepath.Join(output, name)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
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

	output := t.TempDir()
	if err := GenerateSeekThumbnails(context.Background(), videoPath, filepath.Join(output, "%06d.jpg")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"000000.jpg", "000001.jpg"} {
		data, err := os.ReadFile(filepath.Join(output, name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if _, err := jpeg.Decode(strings.NewReader(string(data))); err != nil {
			t.Fatalf("%s JPEG: %v", name, err)
		}
	}
}
