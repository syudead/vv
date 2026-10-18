package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// Return on-demand probe results only when stored probe metadata was not supplied.
type fakeTranscoder struct {
	body            string
	stream          io.ReadCloser
	err             error
	waitErr         error
	hardwareFailure error
	path            string
	startMs         int64
	normalize       bool
	quality         domain.TranscodeQuality
	source          domain.FileStamp
	probe           *domain.TranscodeProbe
	probed          domain.TranscodeProbe
	starts          int
	waits           int
	stops           int
	// beforeReturn simulates Start completing near its deadline.
	beforeReturn func(domain.LiveTranscodeRequest)

	actualStartMs int64
}

func (f *fakeTranscoder) Start(_ context.Context, request domain.LiveTranscodeRequest) (domain.LiveTranscode, error) {
	f.starts++
	f.path, f.startMs, f.normalize = request.Path, request.StartMs, request.Normalize
	f.quality = request.Quality
	f.source, f.probe = request.Source, request.Probe
	var probed *domain.TranscodeProbe
	if request.Probe == nil {
		result := f.probed
		probed = &result
	}
	if f.err != nil {
		return domain.LiveTranscode{}, f.err
	}
	if f.beforeReturn != nil {
		f.beforeReturn(request)
	}
	stream := f.stream
	if stream == nil {
		stream = io.NopCloser(strings.NewReader(f.body))
	}
	return domain.LiveTranscode{
		Stream:          stream,
		Wait:            func() error { f.waits++; return f.waitErr },
		Stop:            func() { f.stops++ },
		Probed:          probed,
		StartMs:         f.actualStartMs,
		HardwareFailure: f.hardwareFailure,
	}, nil
}

type blockingReadCloser struct {
	started chan struct{}
	closed  chan struct{}
}

func newBlockingReadCloser() *blockingReadCloser {
	return &blockingReadCloser{started: make(chan struct{}), closed: make(chan struct{})}
}

func (r *blockingReadCloser) Read(_ []byte) (int, error) {
	close(r.started)
	<-r.closed
	return 0, io.EOF
}

func (r *blockingReadCloser) Close() error {
	select {
	case <-r.closed:
	default:
		close(r.closed)
	}
	return nil
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

	if fake.startMs != 1000 || fake.normalize || fake.waits != 1 {
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
		{"invalid attempt", "/api/videos/1/transcode.mp4?attempt=%21", &fakeTranscoder{}, http.StatusBadRequest},
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
			switch tc.name {
			case "probe failure":
				assertErrorBody(t, tc.name, rec.Code, rec.Body.Bytes(),
					wantError{status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: reasonTranscodeUnavailable})
			case "invalid start", "invalid attempt":
				// Errors for values the UI never sends have no reason.
				assertErrorBody(t, tc.name, rec.Code, rec.Body.Bytes(),
					wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest})
			}
		})
	}
}

func TestTranscodeRejectsMissingProbeAndFile(t *testing.T) {
	mediaDir, video, _ := streamFixture(t, "a.mkv", 128)
	video.ProbeState = domain.ProbeStateFailed
	handler := newTestServer(t, Options{Videos: &fakeLibrary{videos: map[int64]domain.Video{1: video}, roots: []string{mediaDir}}, Transcoder: &fakeTranscoder{}})
	rec := do(t, handler, http.MethodGet, "/api/videos/1/transcode.mp4")
	assertErrorBody(t, "解析情報が無い", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: reasonProbeInfoMissing})

	video.ProbeState = domain.ProbeStateDone
	video.Path = mediaDir + "/missing.mkv"
	handler = newTestServer(t, Options{Videos: &fakeLibrary{videos: map[int64]domain.Video{1: video}, roots: []string{mediaDir}}, Transcoder: &fakeTranscoder{}})
	rec = do(t, handler, http.MethodGet, "/api/videos/1/transcode.mp4")
	assertErrorBody(t, "実体が無い", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: reasonFileUnavailable})
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

func TestTranscodeSilentlyStopsWhenRequestIsCanceledBeforeInitialData(t *testing.T) {
	stream := newBlockingReadCloser()
	fake := &fakeTranscoder{stream: stream, waitErr: context.Canceled}
	handler := transcodeServer(t, false, fake)
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/api/videos/1/transcode.mp4", nil)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		done <- rec
	}()
	<-stream.started
	cancel()

	select {
	case rec := <-done:
		if rec.Body.Len() != 0 || fake.stops != 1 || fake.waits != 1 {
			t.Errorf("body=%q stops=%d waits=%d", rec.Body.String(), fake.stops, fake.waits)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled request did not stop")
	}
}

// Initial output arriving just before the deadline must stream without restarting the timeout.
func TestTranscodeServesStreamStartedAtDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fake := &fakeTranscoder{body: "fragmented-mp4", beforeReturn: func(request domain.LiveTranscodeRequest) {
			time.Sleep(time.Until(request.StartupDeadline) + 20*time.Millisecond)
		}}
		rec := do(t, transcodeServer(t, false, fake), http.MethodGet, "/api/videos/1/transcode.mp4")
		if rec.Code != http.StatusOK || rec.Body.String() != fake.body || fake.stops != 0 {
			t.Fatalf("response = %d %q stops=%d", rec.Code, rec.Body.String(), fake.stops)
		}
	})
}

func doRequest(handler http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

var _ gen.ServerInterface = (*server)(nil)

// The HTTP layer samples settings once per request and forwards that choice.
type selectedEncoderSettings struct {
	TranscodeSettings
	encoding domain.TranscodeEncoding
	reads    int
}

func (s *selectedEncoderSettings) Current() domain.TranscodeEncoding {
	s.reads++
	return s.encoding
}

func TestTranscodeUsesEncoderSelectedBeforeRequestStarts(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		settings := &selectedEncoderSettings{encoding: domain.TranscodeEncoding{Effective: domain.VideoEncoderSoftware}}
		entered, release := make(chan struct{}), make(chan struct{})
		releaseFirst := sync.OnceFunc(func() { close(release) })
		defer releaseFirst()
		var encoders []domain.VideoEncoder
		fake := &fakeTranscoder{body: "fragmented-mp4", beforeReturn: func(request domain.LiveTranscodeRequest) {
			encoders = append(encoders, request.VideoEncoder)
			if len(encoders) == 1 {
				close(entered)
				<-release
			}
		}}
		mediaDir, video, _ := streamFixture(t, "a.mkv", 1)
		handler := newTestServer(t, Options{
			Videos:     &fakeLibrary{videos: map[int64]domain.Video{video.ID: video}, roots: []string{mediaDir}},
			Transcoder: fake, TranscodeSettings: settings,
		})
		first := make(chan *httptest.ResponseRecorder, 1)
		go func() {
			first <- doRequest(handler, httptest.NewRequest(http.MethodGet, "/api/videos/1/transcode.mp4", nil))
		}()
		<-entered
		settings.encoding = domain.TranscodeEncoding{Choice: domain.EncoderChoiceNVENC, Effective: domain.VideoEncoderNVENC}
		releaseFirst()
		if rec := <-first; rec.Code != http.StatusOK || rec.Body.String() != fake.body {
			t.Fatalf("first = %d %q", rec.Code, rec.Body)
		}
		rec := do(t, handler, http.MethodGet, "/api/videos/1/transcode.mp4")
		if rec.Code != http.StatusOK || rec.Body.String() != fake.body {
			t.Fatalf("second = %d %q", rec.Code, rec.Body)
		}
		if want := []domain.VideoEncoder{domain.VideoEncoderSoftware, domain.VideoEncoderNVENC}; !slices.Equal(encoders, want) {
			t.Fatalf("encoders = %v, want %v", encoders, want)
		}
		if settings.reads != 2 || fake.starts != 2 {
			t.Fatalf("settings reads=%d starts=%d", settings.reads, fake.starts)
		}
	})
}

// Fallback belongs to media. HTTP reports the returned warning while serving the
// successful stream, and does not change the selected encoder.
func TestTranscodeReportsHardwareFallback(t *testing.T) {
	settings := &selectedEncoderSettings{encoding: domain.TranscodeEncoding{Choice: domain.EncoderChoiceNVENC, Effective: domain.VideoEncoderNVENC}}
	fake := &fakeTranscoder{body: "fragmented-mp4", hardwareFailure: errors.New("OpenEncodeSessionEx failed")}
	logs := &syncBuffer{}
	mediaDir, video, _ := streamFixture(t, "a.mkv", 1)
	handler := newTestServer(t, Options{
		Videos:     &fakeLibrary{videos: map[int64]domain.Video{video.ID: video}, roots: []string{mediaDir}},
		Transcoder: fake, TranscodeSettings: settings,
		Logger: slog.New(slog.NewTextHandler(logs, nil)),
	})
	rec := do(t, handler, http.MethodGet, "/api/videos/1/transcode.mp4")
	if rec.Code != http.StatusOK || rec.Body.String() != fake.body || fake.starts != 1 {
		t.Fatalf("status=%d body=%q starts=%d", rec.Code, rec.Body, fake.starts)
	}
	for _, want := range []string{"level=WARN", "fell back to software", "encoder=nvenc", "video=1", "OpenEncodeSessionEx failed"} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("missing %q: %s", want, logs.String())
		}
	}
	if settings.encoding.Choice != domain.EncoderChoiceNVENC || settings.encoding.Effective != domain.VideoEncoderNVENC {
		t.Fatalf("settings changed: %+v", settings.encoding)
	}
}

func TestTranscodeWithoutSettingsRequestsSoftware(t *testing.T) {
	var got domain.VideoEncoder
	fake := &fakeTranscoder{body: "fragmented-mp4", beforeReturn: func(request domain.LiveTranscodeRequest) {
		got = request.VideoEncoder
	}}
	rec := do(t, transcodeServer(t, false, fake), http.MethodGet, "/api/videos/1/transcode.mp4")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if got != domain.VideoEncoderSoftware {
		t.Errorf("VideoEncoder = %q, want software", got)
	}
}

// Pass quality through LiveTranscodeRequest and include it in startup logs.
func TestTranscodePassesQualityToTranscoder(t *testing.T) {
	fake := &fakeTranscoder{body: "fragmented-mp4"}
	logs := &syncBuffer{}
	mediaDir, video, _ := streamFixture(t, "a.mkv", 128)
	video.Playable = false
	handler := newTestServer(t, Options{
		Videos:     &fakeLibrary{videos: map[int64]domain.Video{video.ID: video}, roots: []string{mediaDir}},
		Transcoder: fake,
		Logger:     slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
	})

	rec := do(t, handler, http.MethodGet, "/api/videos/1/transcode.mp4?quality=480p")
	if rec.Code != http.StatusOK || fake.quality != domain.TranscodeQuality480p {
		t.Fatalf("status = %d, Quality = %q: %s", rec.Code, fake.quality, rec.Body)
	}
	if !strings.Contains(logs.String(), "quality=480p") {
		t.Errorf("開始のログに画質が無い: %s", logs)
	}

	rec = do(t, handler, http.MethodGet, "/api/videos/1/transcode.mp4")
	if rec.Code != http.StatusOK || fake.quality != "" {
		t.Errorf("画質の無い要求: status = %d, Quality = %q", rec.Code, fake.quality)
	}
}

// Reject unsupported quality, values at/above the short side, and unknown dimensions before
// starting a transcode.
func TestTranscodeRejectsUnavailableQuality(t *testing.T) {
	width, height := 1280, 720
	tests := []struct {
		name          string
		width, height *int
		query         string
	}{
		{"列挙に無い値", &width, &height, "?quality=240p"},
		{"空の値", &width, &height, "?quality="},
		{"短辺と同じ画質", &width, &height, "?quality=720p"},
		{"短辺より大きい画質", &width, &height, "?quality=1080p"},
		{"寸法の無い動画", nil, nil, "?quality=360p"},
		{"幅だけの動画", &width, nil, "?quality=360p"},
		{"attempt 付き", &width, &height, "?quality=1080p&attempt=q-1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeTranscoder{body: "fragmented-mp4"}
			mediaDir, video, _ := streamFixture(t, "a.mkv", 128)
			video.Width, video.Height = tc.width, tc.height
			handler := newTestServer(t, Options{
				Videos:     &fakeLibrary{videos: map[int64]domain.Video{video.ID: video}, roots: []string{mediaDir}},
				Transcoder: fake,
			})
			rec := do(t, handler, http.MethodGet, "/api/videos/1/transcode.mp4"+tc.query)
			assertErrorBody(t, tc.name, rec.Code, rec.Body.Bytes(),
				wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest})
			if got := rec.Header().Get("Cache-Control"); got != cacheNoStore {
				t.Errorf("Cache-Control = %q", got)
			}
			if fake.starts != 0 {
				t.Errorf("変換を %d 回始めた", fake.starts)
			}
		})
	}
}
