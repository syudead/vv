package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

type fakeTranscoder struct {
	body      string
	err       error
	waitErr   error
	path      string
	startMs   int64
	normalize bool
	starts    int
	waits     int
	stops     int
}

func (f *fakeTranscoder) Start(_ context.Context, path string, startMs int64, normalize bool) (io.ReadCloser, func() error, func(), error) {
	f.starts++
	f.path, f.startMs, f.normalize = path, startMs, normalize
	if f.err != nil {
		return nil, nil, nil, f.err
	}
	return io.NopCloser(strings.NewReader(f.body)), func() error { f.waits++; return f.waitErr }, func() { f.stops++ }, nil
}

func transcodeServer(t *testing.T, playable bool, transcoder Transcoder) http.Handler {
	t.Helper()
	mediaDir, video, _ := streamFixture(t, "a.mkv", 128)
	video.Playable = playable
	return newTestServer(t, Options{
		Videos:     &fakeLibrary{videos: map[int64]domain.Video{video.ID: video}, roots: []string{mediaDir}},
		Transcoder: transcoder,
	})
}

func TestTranscodeStreamsMP4WithoutRangeHeaders(t *testing.T) {
	fake := &fakeTranscoder{body: "fragmented-mp4"}
	handler := transcodeServer(t, false, fake)
	req, err := http.NewRequest(http.MethodGet, "/api/videos/1/transcode.mp4?startMs=1000", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=20-")
	rec := doRequest(handler, req)

	if rec.Code != http.StatusOK || rec.Body.String() != fake.body {
		t.Fatalf("response = %d %q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "video/mp4" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != cacheNoStore {
		t.Errorf("Cache-Control = %q", got)
	}
	if got := rec.Header().Get("Accept-Ranges"); got != "" {
		t.Errorf("Accept-Ranges = %q", got)
	}
	if got := rec.Header().Get("Content-Length"); got != "" {
		t.Errorf("Content-Length = %q", got)
	}
	if fake.startMs != 1000 || !fake.normalize || fake.waits != 1 {
		t.Errorf("transcoder = %+v", fake)
	}
}

func TestTranscodeNormalizesPlayableDirectFallback(t *testing.T) {
	fake := &fakeTranscoder{body: "ok"}
	rec := do(t, transcodeServer(t, true, fake), http.MethodGet, "/api/videos/1/transcode.mp4")
	if rec.Code != http.StatusOK || !fake.normalize {
		t.Errorf("status=%d normalize=%v", rec.Code, fake.normalize)
	}
}

func TestTranscodeErrorsBeforeBody(t *testing.T) {
	tests := []struct {
		name       string
		target     string
		transcoder *fakeTranscoder
		want       int
	}{
		{"invalid start", "/api/videos/1/transcode.mp4?startMs=999999", &fakeTranscoder{}, http.StatusBadRequest},
		{"invalid query", "/api/videos/1/transcode.mp4?startMs=nope", &fakeTranscoder{}, http.StatusBadRequest},
		{"probe failure", "/api/videos/1/transcode.mp4", &fakeTranscoder{err: domain.ErrUnprocessableMedia}, http.StatusConflict},
		{"start failure", "/api/videos/1/transcode.mp4", &fakeTranscoder{err: errors.New("start failed")}, http.StatusInternalServerError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(t, transcodeServer(t, false, tc.transcoder), http.MethodGet, tc.target)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.want, rec.Body.String())
			}
			if got := rec.Header().Get("Cache-Control"); got != cacheNoStore {
				t.Errorf("Cache-Control = %q", got)
			}
		})
	}
}

func TestTranscodeRejectsMissingProbeAndFile(t *testing.T) {
	mediaDir, video, _ := streamFixture(t, "a.mkv", 128)
	video.ProbeState = domain.ProbeStateFailed
	handler := newTestServer(t, Options{Videos: &fakeLibrary{videos: map[int64]domain.Video{1: video}, roots: []string{mediaDir}}, Transcoder: &fakeTranscoder{}})
	if rec := do(t, handler, http.MethodGet, "/api/videos/1/transcode.mp4"); rec.Code != http.StatusConflict {
		t.Errorf("probe failure status = %d", rec.Code)
	}

	video.ProbeState = domain.ProbeStateDone
	video.Path = mediaDir + "/missing.mkv"
	handler = newTestServer(t, Options{Videos: &fakeLibrary{videos: map[int64]domain.Video{1: video}, roots: []string{mediaDir}}, Transcoder: &fakeTranscoder{}})
	if rec := do(t, handler, http.MethodGet, "/api/videos/1/transcode.mp4"); rec.Code != http.StatusNotFound {
		t.Errorf("missing file status = %d", rec.Code)
	}
}

func TestTranscodeLogsProcessFailureAfterBodyStarts(t *testing.T) {
	fake := &fakeTranscoder{body: "partial", waitErr: errors.New("exit 1")}
	rec := do(t, transcodeServer(t, false, fake), http.MethodGet, "/api/videos/1/transcode.mp4")
	if rec.Code != http.StatusOK || rec.Body.String() != "partial" || fake.waits != 1 {
		t.Errorf("response=%d %q waits=%d", rec.Code, rec.Body.String(), fake.waits)
	}
}

func TestTranscodeReturnsErrorWhenProcessProducesNoBody(t *testing.T) {
	fake := &fakeTranscoder{waitErr: errors.New("exit 1")}
	rec := do(t, transcodeServer(t, false, fake), http.MethodGet, "/api/videos/1/transcode.mp4")
	if rec.Code != http.StatusInternalServerError || fake.waits != 1 || fake.stops != 1 {
		t.Errorf("response=%d waits=%d stops=%d", rec.Code, fake.waits, fake.stops)
	}
}

func doRequest(handler http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

var _ gen.ServerInterface = (*server)(nil)
