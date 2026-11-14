package media

import (
	"bytes"
	"context"
	"errors"
	"image/png"
	"math/bits"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// The fixture holds one scene per 2-second segment and black from 12 to 16 s, so frames sampled once a second pair
// up within a segment, differ across segments, and match the index's keyframe of their segment.
func TestQueryVideoFramesSamplesOnceASecondAsTheIndexHashes(t *testing.T) {
	t.Parallel()
	video := mediaFixture(t, "keyframes-2s.mp4")
	durationMs, err := QueryVideoDuration(context.Background(), video)
	if err != nil || durationMs != 20_000 {
		t.Fatalf("QueryVideoDuration = %d, %v, want 20000", durationMs, err)
	}
	frames, err := QueryVideoFrames(context.Background(), video, domain.ImageSearchQueryMaxFrames)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 20 {
		t.Fatalf("frames = %d, want one per second of 20 s", len(frames))
	}
	keyframes := map[int64]uint64{}
	if err := KeyframeHashes(context.Background(), video, func(k domain.KeyframeHash) {
		keyframes[k.TimeMs] = k.Frame.Hash
	}); err != nil {
		t.Fatal(err)
	}
	for i, frame := range frames {
		black := i >= 12 && i < 16
		if frame.Flat != black {
			t.Errorf("frame %d: flat = %v, want %v", i, frame.Flat, black)
		}
		if black {
			continue
		}
		segment := int64(i/2) * 2000
		if d := bits.OnesCount64(frame.Hash ^ keyframes[segment]); d > domain.FingerprintMatchMaxDistance {
			t.Errorf("frame %d is %d bits from the keyframe at %d ms", i, d, segment)
		}
		if i%2 == 0 && i+2 < len(frames) && !frames[i+2].Flat &&
			bits.OnesCount64(frame.Hash^frames[i+2].Hash) <= domain.FingerprintMatchMaxDistance {
			t.Errorf("frames %d and %d of different segments match", i, i+2)
		}
	}
	if _, err := QueryVideoFrames(context.Background(), "testdata/SHA256SUMS", 10); !errors.Is(err, domain.ErrImageSearchUnreadable) {
		t.Fatalf("frames of a text file: %v, want ErrImageSearchUnreadable", err)
	}
	if _, err := QueryVideoDuration(context.Background(), "testdata/SHA256SUMS"); !errors.Is(err, domain.ErrImageSearchUnreadable) {
		t.Fatalf("duration of a text file: %v, want ErrImageSearchUnreadable", err)
	}
}

// The limit and the whole-or-part rule apply to the footage, so audio running past the video does not count.
func TestQueryVideoDurationIsTheVideoStreams(t *testing.T) {
	t.Parallel()
	requireFFmpeg(t)
	video := filepath.Join(t.TempDir(), "long-audio.mp4")
	runFFmpeg(t, "-f", "lavfi", "-i", "color=c=white:s=64x64:r=10:d=3", "-f", "lavfi", "-t", "8",
		"-i", "anullsrc=r=8000:cl=mono", "-c:v", "mpeg4", "-c:a", "aac", "-y", video)
	durationMs, err := QueryVideoDuration(context.Background(), video)
	if err != nil || durationMs != 3_000 {
		t.Fatalf("QueryVideoDuration = %d, %v, want the video stream's 3000", durationMs, err)
	}
}

// The fixture scrolls 60 pixels a second with keyframes at 0 and 8 s: a PNG cut 6 s after the first keyframe is too
// far from it to match, and refining the gap to the next keyframe finds it at 6 s.
func TestRefineFrameHashesFindsAFrameBetweenKeyframes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	video := mediaFixture(t, "moving-8s.mp4")
	cut, err := exec.CommandContext(ctx, thumbnailCommand, "-v", "error", "-ss", "6", "-i", video, "-frames:v", "1",
		"-f", "image2pipe", "-c:v", "png", "pipe:1").Output()
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(cut))
	if err != nil {
		t.Fatal(err)
	}
	query := ImageFrameHash(img).Hash
	var keyframes []domain.KeyframeHash
	if err := KeyframeHashes(ctx, video, func(k domain.KeyframeHash) { keyframes = append(keyframes, k) }); err != nil {
		t.Fatal(err)
	}
	if len(keyframes) != 2 || keyframes[0].TimeMs != 0 || keyframes[1].TimeMs != 8000 {
		t.Fatalf("keyframes = %+v, want 0 and 8000 ms", keyframes)
	}
	if d := bits.OnesCount64(query ^ keyframes[0].Frame.Hash); d <= domain.FingerprintMatchMaxDistance {
		t.Fatalf("the cut frame is %d bits from the keyframe at 0 ms, want over %d", d, domain.FingerprintMatchMaxDistance)
	}

	frames, err := RefineFrameHashes(ctx, video, 0, 8000)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 32 {
		t.Fatalf("refined %d frames, want 4 a second over 8 s", len(frames))
	}
	match, ok := domain.MatchRefinedFrames(query, domain.ImageSearchRefinement{VideoID: 1, StartMs: 0, EndMs: 8000}, frames)
	if !ok || match.PositionMs < 5750 || match.PositionMs > 6250 {
		t.Fatalf("refined match = %+v, %v, want one within 250 ms of 6000 ms", match, ok)
	}
}
