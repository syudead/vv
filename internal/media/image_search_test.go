package media

import (
	"context"
	"errors"
	"math/bits"
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
