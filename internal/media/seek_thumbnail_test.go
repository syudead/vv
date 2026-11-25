package media

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"testing"

	"github.com/syudead/vv/internal/domain"
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
	if len(entries) != 1 || entries[0].Name() != "000.jpg" {
		t.Fatalf("出力が %v（000.jpg の 1 個のはず）", entries)
	}
}

func TestSeekSpriteOutcomeKeepsSuccessAfterTheDeadline(t *testing.T) {
	expired, cancel := context.WithCancel(context.Background())
	cancel()
	if err := seekSpriteOutcome(context.Background(), expired, nil); err != nil {
		t.Fatalf("finished generation = %v, want success", err)
	}
	failed := errors.New("ffmpeg was killed")
	if err := seekSpriteOutcome(context.Background(), expired, failed); !errors.Is(err, domain.ErrFailureRepeats) {
		t.Fatalf("failure after the deadline = %v, want ErrFailureRepeats", err)
	}
}
