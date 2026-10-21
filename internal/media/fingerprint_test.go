package media

import (
	"bytes"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// spriteFingerprintFixture reads captured sprite sheets, so fingerprint tests
// exercise JPEG decoding, hashing and comparison without generating a video.
func spriteFingerprintFixture(t *testing.T, name string, intervalMs int64, frameCount int) domain.Fingerprint {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "fingerprints", name+".jpg"))
	if err != nil {
		t.Fatal(err)
	}
	sheet, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	// These dimensions and intervals describe the stored inputs, independently
	// of the current sprite generator and layout policy.
	sprite := domain.SeekSprite{
		SeekSpriteLayout: domain.SeekSpriteLayout{
			IntervalMs: intervalMs, FrameCount: frameCount, Columns: 9, Rows: 9, SheetCount: 1,
		},
		FrameWidth: sheet.Width / 9, FrameHeight: sheet.Height / 9,
	}
	fingerprint, err := SpriteFingerprint(sprite, [][]byte{data})
	if err != nil {
		t.Fatal(err)
	}
	if fingerprint.Version != domain.FingerprintVersion || fingerprint.IntervalMs != intervalMs || len(fingerprint.Frames) != frameCount {
		t.Fatalf("fingerprint = version %d, interval %d, frames %d", fingerprint.Version, fingerprint.IntervalMs, len(fingerprint.Frames))
	}
	return fingerprint
}

// Log distances to show margin against the threshold.
func logDistance(t *testing.T, name string, a, b domain.Fingerprint) {
	t.Helper()
	distance, ok := domain.CompareFingerprints(a, b)
	t.Logf("%s: 中央値 %d（比べられた %v、閾値 %d）", name, distance, ok, domain.FingerprintMatchMaxDistance)
}

func TestSpriteFingerprintMatchesReencodes(t *testing.T) {
	t.Parallel()

	large := spriteFingerprintFixture(t, "short", 1000, 60)
	small := spriteFingerprintFixture(t, "letterbox", 1000, 60)
	other := spriteFingerprintFixture(t, "other", 1000, 60)

	logDistance(t, "再エンコードどうし", large, small)
	logDistance(t, "別の動画", large, other)
	if distance, ok := domain.CompareFingerprints(large, small); !ok || distance > domain.FingerprintMatchMaxDistance {
		t.Errorf("再エンコードどうし: distance=%d ok=%v", distance, ok)
	}
	if distance, ok := domain.CompareFingerprints(large, other); !ok || distance <= domain.FingerprintMatchMaxDistance {
		t.Errorf("尺だけ同じ別の動画: distance=%d ok=%v", distance, ok)
	}
}

// The 410 s and 410.4 s fixtures have different sampling intervals; pairing
// frames by time must still match them in either comparison direction.
func TestSpriteFingerprintMatchesLongReencodesWithDifferentIntervals(t *testing.T) {
	t.Parallel()

	a := spriteFingerprintFixture(t, "long", 5062, 81)
	b := spriteFingerprintFixture(t, "long-reencoded", 5067, 81)
	if a.IntervalMs == b.IntervalMs {
		t.Fatalf("間隔が同じ %d", a.IntervalMs)
	}
	for _, pair := range [][2]domain.Fingerprint{{a, b}, {b, a}} {
		logDistance(t, "間隔の違う再エンコード", pair[0], pair[1])
		if distance, ok := domain.CompareFingerprints(pair[0], pair[1]); !ok || distance > domain.FingerprintMatchMaxDistance {
			t.Errorf("間隔 %d と %d: distance=%d ok=%v", pair[0].IntervalMs, pair[1].IntervalMs, distance, ok)
		}
	}
	other := spriteFingerprintFixture(t, "long-other", 5062, 81)
	logDistance(t, "別の動画", a, other)
	if domain.FingerprintsMatch(a, other) {
		t.Error("尺だけ同じ別の動画が一致する")
	}
}

// Low-frequency 32x32 luma should tolerate both legacy 160px/q4 and current 320px/q2 sprites.
func TestSpriteFingerprintMatchesAcrossSpriteSizes(t *testing.T) {
	t.Parallel()

	old := spriteFingerprintFixture(t, "old-size", 1000, 60)
	current := spriteFingerprintFixture(t, "short", 1000, 60)
	other := spriteFingerprintFixture(t, "other", 1000, 60)

	logDistance(t, "160px と 320px", old, current)
	logDistance(t, "160px と別の動画の 320px", old, other)
	if distance, ok := domain.CompareFingerprints(old, current); !ok || distance > domain.FingerprintMatchMaxDistance/2 {
		t.Errorf("160px と 320px: distance=%d ok=%v（閾値の半分 %d 以下のはず）", distance, ok, domain.FingerprintMatchMaxDistance/2)
	}
	if domain.FingerprintsMatch(old, other) {
		t.Error("尺だけ同じ別の動画が一致する")
	}
}

func TestSpriteFingerprintRejectsMismatchedSheets(t *testing.T) {
	layout := domain.NewSeekSpriteLayout(60_000)
	sprite := domain.SeekSprite{SeekSpriteLayout: layout, FrameWidth: 160, FrameHeight: 90}
	if _, err := SpriteFingerprint(sprite, nil); err == nil {
		t.Error("シートが無いのに誤りにならない")
	}
	var small bytes.Buffer
	if err := jpeg.Encode(&small, image.NewGray(image.Rect(0, 0, 160, 90)), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := SpriteFingerprint(sprite, [][]byte{small.Bytes()}); err == nil {
		t.Error("コマがシートの外なのに誤りにならない")
	}
	if _, err := SpriteFingerprint(sprite, [][]byte{[]byte("not a jpeg")}); err == nil {
		t.Error("読めないシートで誤りにならない")
	}
}
