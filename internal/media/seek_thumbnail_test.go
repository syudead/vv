package media

import (
	"bytes"
	"context"
	"errors"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

func TestSeekThumbnailArgsBoundFrameWindow(t *testing.T) {
	forward := seekThumbnailArgs("/media/a.mp4", 9999, false)
	joined := strings.Join(forward, " ")
	for _, want := range []string{
		"-ss 9.999", "-t 1.000", "-frames:v 1", "scale=min(320\\,iw):-2,format=yuvj420p", "-f image2pipe", "pipe:1",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("通常抽出引数に %q が無い: %v", want, forward)
		}
	}

	tail := seekThumbnailArgs("/media/a.mp4", 9999, true)
	tailJoined := strings.Join(tail, " ")
	for _, want := range []string{"-ss 8.999", "-t 1.000", "reverse,scale=min(320\\,iw):-2,format=yuvj420p"} {
		if !strings.Contains(tailJoined, want) {
			t.Errorf("末尾抽出引数に %q が無い: %v", want, tail)
		}
	}
}

func TestSeekThumbnailExtractsAndFallsBackToTail(t *testing.T) {
	extractor := NewSeekThumbnailExtractor(nil)
	calls := 0
	extractor.commandContext = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		calls++
		mode := "image"
		if calls == 1 {
			mode = "empty"
		}
		return seekThumbnailHelperCommand(ctx, mode)
	}

	image, err := extractor.Extract(context.Background(), "/media/a.mp4", 9999)
	if err != nil {
		t.Fatal(err)
	}
	if string(image) != "jpeg" || calls != 2 {
		t.Errorf("image=%q calls=%d", image, calls)
	}
}

func TestSeekThumbnailClassifiesProcessFailures(t *testing.T) {
	extractor := NewSeekThumbnailExtractor(nil)
	extractor.commandContext = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return seekThumbnailHelperCommand(ctx, "failure")
	}
	if _, err := extractor.Extract(context.Background(), "/media/a.mp4", 1000); err == nil || errors.Is(err, domain.ErrSeekFrameUnavailable) {
		t.Fatalf("exit error=%v", err)
	}

	extractor.commandContext = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "definitely-not-a-real-command-seek-thumbnail")
	}
	if _, err := extractor.Extract(context.Background(), "/media/a.mp4", 1000); err == nil || errors.Is(err, domain.ErrSeekFrameUnavailable) {
		t.Fatalf("start error=%v", err)
	}
}

func TestSeekThumbnailStopsProcessWhenContextIsCanceled(t *testing.T) {
	extractor := NewSeekThumbnailExtractor(nil)
	extractor.commandContext = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return seekThumbnailHelperCommand(ctx, "wait")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	started := time.Now()
	_, err := extractor.Extract(ctx, "/media/a.mp4", 1000)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error=%v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("process終了まで%sかかった", elapsed)
	}
}

func TestSeekThumbnailStopsProcessWhenServerStops(t *testing.T) {
	serverDone := make(chan struct{})
	extractor := NewSeekThumbnailExtractor(serverDone)
	extractor.commandContext = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return seekThumbnailHelperCommand(ctx, "wait")
	}
	done := make(chan error, 1)
	go func() {
		_, err := extractor.Extract(context.Background(), "/media/a.mp4", 1000)
		done <- err
	}()

	close(serverDone)
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("server停止後もprocessが終了しない")
	}
}

func TestSeekThumbnailExtractsStartMiddleAndMediaEnd(t *testing.T) {
	if _, err := exec.LookPath(seekThumbnailCommand); err != nil {
		t.Skip("ffmpegが無いため実画像の抽出を省略します")
	}

	path := filepath.Join(t.TempDir(), "colors.mp4")
	args := []string{
		"-nostdin", "-v", "error",
		"-f", "lavfi", "-i", "color=c=red:s=64x64:r=30:d=1",
		"-f", "lavfi", "-i", "color=c=green:s=64x64:r=30:d=1",
		"-f", "lavfi", "-i", "color=c=blue:s=64x64:r=30:d=1",
		"-filter_complex", "[0:v][1:v][2:v]concat=n=3:v=1:a=0",
		"-c:v", "mpeg4", "-y", path,
	}
	if output, err := exec.Command(seekThumbnailCommand, args...).CombinedOutput(); err != nil {
		t.Fatalf("fixture生成に失敗しました: %v: %s", err, output)
	}

	extractor := NewSeekThumbnailExtractor(nil)
	tests := []struct {
		name       string
		positionMs int64
		dominant   byte
	}{
		{"start", 100, 'r'},
		{"middle", 1100, 'g'},
		{"end fallback", 2999, 'b'},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, err := extractor.Extract(context.Background(), path, tc.positionMs)
			if err != nil {
				t.Fatal(err)
			}
			image, err := jpeg.Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatalf("JPEGを読めません: %v", err)
			}
			if bounds := image.Bounds(); bounds.Dx() != 64 || bounds.Dy() != 64 {
				t.Fatalf("size=%dx%d want=64x64", bounds.Dx(), bounds.Dy())
			}
			r, g, b, _ := image.At(32, 32).RGBA()
			switch tc.dominant {
			case 'r':
				if r <= g || r <= b {
					t.Errorf("pixel=%d,%d,%d want red", r, g, b)
				}
			case 'g':
				if g <= r || g <= b {
					t.Errorf("pixel=%d,%d,%d want green", r, g, b)
				}
			case 'b':
				if b <= r || b <= g {
					t.Errorf("pixel=%d,%d,%d want blue", r, g, b)
				}
			}
		})
	}
}

func seekThumbnailHelperCommand(ctx context.Context, mode string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestSeekThumbnailHelperProcess")
	cmd.Env = append(os.Environ(), "GO_SEEK_THUMBNAIL_HELPER=1", "GO_SEEK_THUMBNAIL_MODE="+mode)
	return cmd
}

func TestSeekThumbnailHelperProcess(t *testing.T) {
	if os.Getenv("GO_SEEK_THUMBNAIL_HELPER") != "1" {
		return
	}
	switch os.Getenv("GO_SEEK_THUMBNAIL_MODE") {
	case "image":
		_, _ = os.Stdout.WriteString("jpeg")
	case "wait":
		time.Sleep(10 * time.Second)
	case "failure":
		_, _ = os.Stderr.WriteString("frame unavailable\n")
		os.Exit(2)
	}
	os.Exit(0)
}
