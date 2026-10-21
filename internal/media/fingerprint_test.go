package media

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func fingerprintFixture(t *testing.T, name string) domain.Fingerprint {
	t.Helper()
	fingerprint, err := VideoFingerprint(context.Background(), filepath.Join("testdata", "fingerprints", name+".mp4"))
	if err != nil {
		t.Fatal(err)
	}
	if fingerprint.Version != domain.FingerprintVersion || fingerprint.IntervalMs != 1000 || len(fingerprint.Frames) < 60 {
		t.Fatalf("%s: version %d, interval %d, frames %d", name, fingerprint.Version, fingerprint.IntervalMs, len(fingerprint.Frames))
	}
	return fingerprint
}

// Versions an owner would bundle match the base; the shifted start is found as a one-second offset.
func TestVideoFingerprintMatchesVersionsAnOwnerWouldBundle(t *testing.T) {
	requireFFmpeg(t)
	t.Parallel()
	base := fingerprintFixture(t, "base")
	for _, c := range []struct {
		name     string
		offsetMs int64
	}{
		{"letterbox", 0},
		{"watermark", 0},
		{"shifted", -1000},
	} {
		other := fingerprintFixture(t, c.name)
		distance, ok := domain.CompareFingerprints(base, other)
		pairs, _ := domain.ComparableFramePairs(base, other)
		t.Logf("%s: 中央値 %d（閾値 %d）", c.name, distance, domain.FingerprintMatchMaxDistance)
		if !ok || distance > domain.FingerprintMatchMaxDistance {
			t.Errorf("%s: distance=%d ok=%v, want 一致", c.name, distance, ok)
		}
		if got := int64(pairs[0].B-pairs[0].A) * 1000; got != c.offsetMs {
			t.Errorf("%s: ずれ %d ms, want %d ms", c.name, got, c.offsetMs)
		}
	}
	other := fingerprintFixture(t, "other")
	if distance, ok := domain.CompareFingerprints(base, other); !ok || distance <= domain.FingerprintMatchMaxDistance {
		t.Errorf("尺だけ同じ別の動画: distance=%d ok=%v", distance, ok)
	}
}

func TestVideoFingerprintFailsOnAMissingFile(t *testing.T) {
	requireFFmpeg(t)
	if _, err := VideoFingerprint(context.Background(), filepath.Join(t.TempDir(), "missing.mp4")); err == nil {
		t.Error("無いファイルで誤りにならない")
	}
}
