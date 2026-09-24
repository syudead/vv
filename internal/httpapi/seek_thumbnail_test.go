package httpapi

import (
	"errors"
	"io/fs"
	"net/http"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

func seekThumbnailServer(t *testing.T, reader ArtifactReader) (http.Handler, domain.Video) {
	t.Helper()
	mediaDir, video, _ := streamFixture(t, "a.mp4", 128)
	return newTestServer(t, Options{
		Videos:    &fakeLibrary{videos: map[int64]domain.Video{video.ID: video}, roots: []string{mediaDir}},
		Artifacts: reader,
	}), video
}

func TestSeekThumbnailReturnsJPEGAndImmutableCache(t *testing.T) {
	reader := &fakeArtifacts{image: []byte{0xff, 0xd8, 0xff, 0xd9}}
	handler, video := seekThumbnailServer(t, reader)
	rec := do(t, handler, http.MethodGet, "/api/videos/1/seek-thumbnail?positionMs=4000&v=abcdef012345")

	if rec.Code != http.StatusOK || rec.Body.String() != string(reader.image) {
		t.Fatalf("response = %d %x", rec.Code, rec.Body.Bytes())
	}
	if got := rec.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != cacheImmutable {
		t.Errorf("Cache-Control = %q", got)
	}
	if reader.positionMs != 4000 || reader.contentKey != video.ContentKey {
		t.Errorf("reader = %+v", reader)
	}
}

func TestSeekThumbnailWithoutVersionIsNotCached(t *testing.T) {
	handler, _ := seekThumbnailServer(t, &fakeArtifacts{image: []byte("jpeg")})
	rec := do(t, handler, http.MethodGet, "/api/videos/1/seek-thumbnail?positionMs=0")
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != cacheNoStore {
		t.Errorf("status=%d cache=%q", rec.Code, rec.Header().Get("Cache-Control"))
	}
}

func TestSeekThumbnailRejectsInvalidPositionAndProbe(t *testing.T) {
	reader := &fakeArtifacts{image: []byte("jpeg")}
	handler, video := seekThumbnailServer(t, reader)
	for _, target := range []string{
		"/api/videos/1/seek-thumbnail",
		"/api/videos/1/seek-thumbnail?positionMs=-1",
		"/api/videos/1/seek-thumbnail?positionMs=8533",
		"/api/videos/1/seek-thumbnail?positionMs=nope",
	} {
		if rec := do(t, handler, http.MethodGet, target); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status=%d body=%s", target, rec.Code, rec.Body.String())
		}
	}

	video.ProbeState = domain.ProbeStatePending
	mediaDir := t.TempDir()
	handler = newTestServer(t, Options{
		Videos:    &fakeLibrary{videos: map[int64]domain.Video{1: video}, roots: []string{mediaDir}},
		Artifacts: reader,
	})
	if rec := do(t, handler, http.MethodGet, "/api/videos/1/seek-thumbnail?positionMs=0"); rec.Code != http.StatusConflict {
		t.Errorf("probe pending: status=%d", rec.Code)
	}

	video.ProbeState = domain.ProbeStateDone
	video.VideoCodec = ""
	handler = newTestServer(t, Options{
		Videos:    &fakeLibrary{videos: map[int64]domain.Video{1: video}, roots: []string{mediaDir}},
		Artifacts: reader,
	})
	if rec := do(t, handler, http.MethodGet, "/api/videos/1/seek-thumbnail?positionMs=0"); rec.Code != http.StatusConflict {
		t.Errorf("video stream missing: status=%d", rec.Code)
	}
}

func TestSeekThumbnailMapsFileAndExtractionFailures(t *testing.T) {
	tests := []struct {
		name   string
		reader *fakeArtifacts
		want   int
	}{
		{"not generated", &fakeArtifacts{err: fs.ErrNotExist}, http.StatusConflict},
		{"read failure", &fakeArtifacts{err: errors.New("read failed")}, http.StatusInternalServerError},
		{"empty image", &fakeArtifacts{}, http.StatusInternalServerError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			handler, _ := seekThumbnailServer(t, tc.reader)
			rec := do(t, handler, http.MethodGet, "/api/videos/1/seek-thumbnail?positionMs=1000&v=version")
			if rec.Code != tc.want {
				t.Fatalf("status=%d want=%d body=%s", rec.Code, tc.want, rec.Body.String())
			}
			if rec.Header().Get("Cache-Control") != cacheNoStore {
				t.Errorf("error cache=%q", rec.Header().Get("Cache-Control"))
			}
		})
	}

}

func TestSeekThumbnailForMissingVideo(t *testing.T) {
	handler := newTestServer(t, Options{Videos: &fakeLibrary{}, Artifacts: &fakeArtifacts{}})
	rec := do(t, handler, http.MethodGet, "/api/videos/999/seek-thumbnail?positionMs=0")
	if rec.Code != http.StatusNotFound || decode[gen.Error](t, rec).Code != codeNotFound {
		t.Errorf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
