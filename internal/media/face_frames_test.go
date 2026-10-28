package media

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// rotatedFaceVideo writes a 2 s, 2 fps video stored as 320 x 160 with its left half white and a 90-degree display
// rotation, so it plays as 160 x 320 with the white half at the bottom.
func rotatedFaceVideo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	stored, path := filepath.Join(dir, "stored.mp4"), filepath.Join(dir, "rotated.mp4")
	// The rotation is added by remuxing; on an encoded input ffmpeg would turn the pixels instead.
	for _, args := range [][]string{
		{"-f", "lavfi", "-i", "color=black:s=320x160:r=2:d=2,drawbox=x=0:y=0:w=160:h=160:color=white:t=fill",
			"-c:v", "mpeg4", stored},
		{"-display_rotation", "90", "-i", stored, "-c", "copy", path},
	} {
		if out, err := exec.Command("ffmpeg", append([]string{"-nostdin", "-v", "error"}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("cannot generate the rotated video: %v: %s", err, out)
		}
	}
	return path
}

func blueAt(pix []byte, x, y int) byte {
	return pix[(y*domain.FaceFrameSide+x)*3]
}

// The face POC sized frames from the stored dimensions, which stretched phone videos and lost every face.
func TestFaceFramesAreUprightAndFittedIntoTheSquare(t *testing.T) {
	t.Parallel()
	requireFFmpeg(t)
	path := rotatedFaceVideo(t)

	var frames int
	err := FaceFrames(context.Background(), path, func(pix []byte) error {
		frames++
		if len(pix) != domain.FaceFrameSide*domain.FaceFrameSide*3 {
			t.Fatalf("frame of %d bytes, want a %d px square of BGR", len(pix), domain.FaceFrameSide)
		}
		// Displayed 160 x 320 fits as 640 x 1280 at the left; the rest is black padding.
		if top, bottom, pad := blueAt(pix, 320, 320), blueAt(pix, 320, 960), blueAt(pix, 960, 960); top > 16 || bottom < 240 || pad > 16 {
			t.Fatalf("top %d, bottom %d, padding %d; want an upright frame with its white half at the bottom", top, bottom, pad)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if frames != 2 {
		t.Fatalf("frames = %d, want 2 for 2 s at %v fps", frames, domain.FaceSampleFPS)
	}
}

func TestFaceFramesStopsWhenTheCallbackFails(t *testing.T) {
	t.Parallel()
	requireFFmpeg(t)
	path := rotatedFaceVideo(t)
	stop := errors.New("stop")

	calls := 0
	err := FaceFrames(context.Background(), path, func([]byte) error {
		calls++
		return stop
	})
	if !errors.Is(err, stop) || calls != 1 {
		t.Fatalf("FaceFrames = %v after %d calls, want the callback's error after one", err, calls)
	}
}

// The failed video's activity entry shows ffmpeg's reason, read from its bounded stderr, and ffmpeg's invalid-data
// error fails the job at once instead of being retried.
func TestFaceFramesReportsWhyFFmpegFailed(t *testing.T) {
	t.Parallel()
	requireFFmpeg(t)
	path := filepath.Join(t.TempDir(), "broken.mp4")
	if err := os.WriteFile(path, []byte("not a video"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := FaceFrames(context.Background(), path, func([]byte) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "moov atom not found") || !errors.Is(err, domain.ErrFailureRepeats) {
		t.Fatalf("FaceFrames = %v, want ffmpeg's reason marked as a repeating failure", err)
	}
}
