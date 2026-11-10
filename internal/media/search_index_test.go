package media

import (
	"context"
	"math/bits"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// jpegFlippedBits allows for the sprite's JPEG, which moves luma by about one level and so can flip a DCT coefficient
// lying at the median; a different range, scale or trimming rule puts the hashes tens of bits apart.
const jpegFlippedBits = 2

// The fixture has a keyframe every 2 s, one still scene per 2-second segment between black bands, and black from 12 to
// 16 s, so the index keeps every keyframe but the black ones, each hashed as the sprite frame of the same scene is.
func TestKeyframeHashesMatchTheSpriteFingerprint(t *testing.T) {
	t.Parallel()
	const durationMs = 20_000
	video := mediaFixture(t, "keyframes-2s.mp4")
	keyframes, err := KeyframeHashes(context.Background(), video)
	if err != nil {
		t.Fatal(err)
	}
	entries := domain.NewSearchIndex(durationMs, keyframes).Entries
	times := make([]int64, len(entries))
	for i, entry := range entries {
		times[i] = entry.TimeMs
	}
	if want := []int64{0, 2000, 4000, 6000, 8000, 10_000, 16_000, 18_000}; !slices.Equal(times, want) {
		t.Fatalf("entry times = %v, want %v", times, want)
	}

	output, layout := generateSprite(t, video, durationMs)
	sheets := make([][]byte, layout.SheetCount)
	for i := range sheets {
		if sheets[i], err = os.ReadFile(filepath.Join(output, keyframeFileName(i, "jpg"))); err != nil {
			t.Fatal(err)
		}
	}
	sprite := domain.SeekSprite{SeekSpriteLayout: layout, FrameWidth: 320, FrameHeight: 240}
	fingerprint, err := SpriteFingerprint(sprite, sheets)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		frame := fingerprint.Frames[layout.FrameAt(entry.TimeMs)]
		if distance := bits.OnesCount64(frame.Hash ^ entry.Hash); distance > jpegFlippedBits {
			t.Errorf("at %d ms: index hash %016x, sprite hash %016x, %d bits apart", entry.TimeMs, entry.Hash,
				frame.Hash, distance)
		}
	}
}
