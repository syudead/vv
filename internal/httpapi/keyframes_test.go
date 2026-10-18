package httpapi

import (
	"net/http"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func TestTranscodeUsesOnlyMatchingKeyframeSample(t *testing.T) {
	for _, matching := range []bool{true, false} {
		library, source := transcodeMetadataFixture(t)
		library.keyframes = domain.KeyframeProbe{Version: domain.KeyframeProbeVersion, Source: source, Spacing: domain.KeyframesSparse}
		if !matching {
			library.keyframes.Source.SizeBytes++
		}
		fake := &fakeTranscoder{body: "fragmented-mp4", beforeReturn: func(request domain.LiveTranscodeRequest) {
			if request.SparseKeyframes != matching {
				t.Errorf("SparseKeyframes=%v, want %v", request.SparseKeyframes, matching)
			}
		}}
		handler := newTestServer(t, Options{Videos: library, Transcoder: fake})
		rec := do(t, handler, http.MethodGet, "/api/videos/7/transcode.mp4?startMs=1000")
		if rec.Code != http.StatusOK || !slices.Equal(library.keyframeReads, []int64{7}) {
			t.Fatalf("status=%d reads=%v: %s", rec.Code, library.keyframeReads, rec.Body)
		}
	}
}
