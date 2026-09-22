package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

type fakeSeekThumbnailExtractor struct {
	image       []byte
	err         error
	path        string
	positionMs  int64
	deadlineSet bool
	started     chan struct{}
}

func (f *fakeSeekThumbnailExtractor) Extract(ctx context.Context, path string, positionMs int64) ([]byte, error) {
	f.path, f.positionMs = path, positionMs
	_, f.deadlineSet = ctx.Deadline()
	if f.started != nil {
		close(f.started)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return f.image, f.err
}

func seekThumbnailServer(t *testing.T, extractor SeekThumbnailExtractor) (http.Handler, domain.Video) {
	t.Helper()
	mediaDir, video, _ := streamFixture(t, "a.mp4", 128)
	return newTestServer(t, Options{
		Videos:         &fakeLibrary{videos: map[int64]domain.Video{video.ID: video}, roots: []string{mediaDir}},
		SeekThumbnails: extractor,
	}), video
}

func TestSeekThumbnailReturnsJPEGAndImmutableCache(t *testing.T) {
	extractor := &fakeSeekThumbnailExtractor{image: []byte{0xff, 0xd8, 0xff, 0xd9}}
	handler, _ := seekThumbnailServer(t, extractor)
	rec := do(t, handler, http.MethodGet, "/api/videos/1/seek-thumbnail?positionMs=4000&v=abcdef012345")

	if rec.Code != http.StatusOK || rec.Body.String() != string(extractor.image) {
		t.Fatalf("response = %d %x", rec.Code, rec.Body.Bytes())
	}
	if got := rec.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != cacheImmutable {
		t.Errorf("Cache-Control = %q", got)
	}
	if extractor.positionMs != 4000 || extractor.path == "" || !extractor.deadlineSet {
		t.Errorf("extractor = %+v", extractor)
	}
}

func TestSeekThumbnailWithoutVersionIsNotCached(t *testing.T) {
	handler, _ := seekThumbnailServer(t, &fakeSeekThumbnailExtractor{image: []byte("jpeg")})
	rec := do(t, handler, http.MethodGet, "/api/videos/1/seek-thumbnail?positionMs=0")
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != cacheNoStore {
		t.Errorf("status=%d cache=%q", rec.Code, rec.Header().Get("Cache-Control"))
	}
}

func TestSeekThumbnailRejectsInvalidPositionAndProbe(t *testing.T) {
	extractor := &fakeSeekThumbnailExtractor{image: []byte("jpeg")}
	handler, video := seekThumbnailServer(t, extractor)
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
		Videos:         &fakeLibrary{videos: map[int64]domain.Video{1: video}, roots: []string{mediaDir}},
		SeekThumbnails: extractor,
	})
	if rec := do(t, handler, http.MethodGet, "/api/videos/1/seek-thumbnail?positionMs=0"); rec.Code != http.StatusConflict {
		t.Errorf("probe pending: status=%d", rec.Code)
	}

	video.ProbeState = domain.ProbeStateDone
	video.VideoCodec = ""
	handler = newTestServer(t, Options{
		Videos:         &fakeLibrary{videos: map[int64]domain.Video{1: video}, roots: []string{mediaDir}},
		SeekThumbnails: extractor,
	})
	if rec := do(t, handler, http.MethodGet, "/api/videos/1/seek-thumbnail?positionMs=0"); rec.Code != http.StatusConflict {
		t.Errorf("video stream missing: status=%d", rec.Code)
	}
}

func TestSeekThumbnailMapsFileAndExtractionFailures(t *testing.T) {
	tests := []struct {
		name      string
		extractor *fakeSeekThumbnailExtractor
		want      int
	}{
		{"frame unavailable", &fakeSeekThumbnailExtractor{err: domain.ErrSeekFrameUnavailable}, http.StatusConflict},
		{"process failure", &fakeSeekThumbnailExtractor{err: errors.New("start failed")}, http.StatusInternalServerError},
		{"empty image", &fakeSeekThumbnailExtractor{}, http.StatusInternalServerError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			handler, _ := seekThumbnailServer(t, tc.extractor)
			rec := do(t, handler, http.MethodGet, "/api/videos/1/seek-thumbnail?positionMs=1000&v=version")
			if rec.Code != tc.want {
				t.Fatalf("status=%d want=%d body=%s", rec.Code, tc.want, rec.Body.String())
			}
			if rec.Header().Get("Cache-Control") != cacheNoStore {
				t.Errorf("error cache=%q", rec.Header().Get("Cache-Control"))
			}
		})
	}

	mediaDir, video, _ := streamFixture(t, "gone.mp4", 16)
	if err := removeFixture(video.Path); err != nil {
		t.Fatal(err)
	}
	handler := newTestServer(t, Options{
		Videos:         &fakeLibrary{videos: map[int64]domain.Video{1: video}, roots: []string{mediaDir}},
		SeekThumbnails: &fakeSeekThumbnailExtractor{image: []byte("jpeg")},
	})
	if rec := do(t, handler, http.MethodGet, "/api/videos/1/seek-thumbnail?positionMs=0"); rec.Code != http.StatusNotFound {
		t.Errorf("missing file: status=%d", rec.Code)
	}
}

func TestSeekThumbnailCancelsExtractionWithRequest(t *testing.T) {
	extractor := &fakeSeekThumbnailExtractor{started: make(chan struct{})}
	handler, _ := seekThumbnailServer(t, extractor)
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/api/videos/1/seek-thumbnail?positionMs=1000", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(rec, req)
		close(done)
	}()

	<-extractor.started
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("request cancellation後もhandlerが終了しない")
	}
	if rec.Body.Len() != 0 {
		t.Errorf("cancel後にerror bodyを書いた: %s", rec.Body.String())
	}
}

func TestSeekThumbnailForMissingVideo(t *testing.T) {
	handler := newTestServer(t, Options{Videos: &fakeLibrary{}, SeekThumbnails: &fakeSeekThumbnailExtractor{}})
	rec := do(t, handler, http.MethodGet, "/api/videos/999/seek-thumbnail?positionMs=0")
	if rec.Code != http.StatusNotFound || decode[gen.Error](t, rec).Code != codeNotFound {
		t.Errorf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func removeFixture(path string) error {
	return os.Remove(path)
}
