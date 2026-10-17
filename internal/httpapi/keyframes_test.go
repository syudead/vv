package httpapi

import (
	"context"
	"os"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func TestTranscodeUsesOnlyMatchingKeyframeSample(t *testing.T) {
	for _, name := range []string{"bare", "ingested"} {
		t.Run(name, func(t *testing.T) {
			env := newTranscodeProbeEnv(t)
			info, err := os.Stat(env.paths[name])
			if err != nil {
				t.Fatal(err)
			}
			p := domain.KeyframeProbe{Version: domain.KeyframeProbeVersion, Source: domain.FileStampOf(info), Spacing: domain.KeyframesSparse}
			probe := domain.Probe{DurationMs: 300000, VideoCodec: "h264", Keyframes: &p}
			if err := env.db.Ingest().ApplyProbe(context.Background(), env.ids[name], probe, domain.Playability{}); err != nil {
				t.Fatal(err)
			}
			want := true
			env.fake.beforeReturn = func(request domain.LiveTranscodeRequest) {
				if request.SparseKeyframes != want {
					t.Errorf("SparseKeyframes = %v, want %v", request.SparseKeyframes, want)
				}
			}
			env.transcode(t, name, "?startMs=290000")
			if err := os.WriteFile(env.paths[name], []byte("replacement file of a different size"), 0o600); err != nil {
				t.Fatal(err)
			}
			want = false
			env.transcode(t, name, "?startMs=290000")
		})
	}
}
