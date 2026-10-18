package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/store"
)

// fakeTranscoder は media.LiveTranscoder の代わりである。保存済みの解析情報が
// 渡されなければ「その場で解析した」とみなして probes を数え、probed を返す。
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
	probes          int
	waits           int
	stops           int
	// beforeReturn は Start が戻る直前に呼ばれる（期限の間際に戻る変換を模す）。
	beforeReturn func(domain.LiveTranscodeRequest)
	// actualStartMs は Start が返す実際の開始位置である。
	actualStartMs int64
}

func (f *fakeTranscoder) Start(_ context.Context, request domain.LiveTranscodeRequest) (domain.LiveTranscode, error) {
	f.starts++
	f.path, f.startMs, f.normalize = request.Path, request.StartMs, request.Normalize
	f.quality = request.Quality
	f.source, f.probe = request.Source, request.Probe
	var probed *domain.TranscodeProbe
	if request.Probe == nil {
		f.probes++
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
	// シークだけではエンコードを強いない。
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
				// 画面が送らない値の誤りには reason を付けない。
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

// Start が期限の間際に最初のデータを持って戻っても、経路は期限をかけ直さずに配信する。
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

// transcodeProbeEnv は本物の保存層に動画を 3 本入れる。"ingested" は取り込みの解析で
// ライブ変換用の解析情報を保存済み、"bare" は保存が無い。"hd" は映像をコピーできる
// 1280×720 の動画で、解析情報を保存済みである。
type transcodeProbeEnv struct {
	db      *store.DB
	handler http.Handler
	fake    *fakeTranscoder
	ids     map[string]int64
	paths   map[string]string
}

func newTranscodeProbeEnv(t *testing.T) *transcodeProbeEnv {
	t.Helper()
	ctx := context.Background()
	db, err := store.OpenMigratedForTest(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	mediaDir := t.TempDir()
	if _, err := db.Settings().AddMediaFolder(ctx, mediaDir); err != nil {
		t.Fatal(err)
	}
	env := &transcodeProbeEnv{
		db:    db,
		fake:  &fakeTranscoder{body: "fragmented-mp4", probed: transcodeProbeFixture("fresh")},
		ids:   map[string]int64{},
		paths: map[string]string{},
	}
	for _, name := range []string{"ingested", "bare", "hd"} {
		path := filepath.Join(mediaDir, name+".mkv")
		if err := os.WriteFile(path, []byte(strings.Repeat(name, 1024)), 0o600); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		result, err := db.ScanIndex().UpsertVideo(ctx, domain.VideoFile{
			Path: path, Title: name, ContentKey: "key-" + name, SizeBytes: info.Size(),
			MTime: info.ModTime(), AddedAt: info.ModTime(), Container: "mkv",
		})
		if err != nil {
			t.Fatal(err)
		}
		probe := domain.Probe{DurationMs: 60_000, Width: 640, Height: 360, VideoCodec: "h264"}
		switch name {
		case "ingested":
			stored := transcodeProbeFixture("ingest")
			probe.Transcode, probe.Source = &stored, domain.FileStampOf(info)
		case "hd":
			stored := transcodeProbeFixture("ingest")
			stored.Video.Width, stored.Video.Height = 1280, 720
			stored.Video.Profile, stored.Video.Level = "High", 41
			stored.Video.PixelFormat, stored.Video.BitsPerRawSample = "yuv420p", 8
			probe.Width, probe.Height = 1280, 720
			probe.Transcode, probe.Source = &stored, domain.FileStampOf(info)
		}
		if err := db.Ingest().ApplyProbe(ctx, result.ID, probe, domain.Playability{}); err != nil {
			t.Fatal(err)
		}
		env.ids[name], env.paths[name] = result.ID, path
	}
	env.handler = newTestServer(t, Options{
		Videos:          db.Library(),
		Transcoder:      env.fake,
		TranscodeProbes: db.Ingest(),
	})
	return env
}

// transcodeProbeFixture は由来を FormatName に記した解析情報である。
func transcodeProbeFixture(origin string) domain.TranscodeProbe {
	return domain.TranscodeProbe{FormatName: origin, Video: domain.TranscodeVideo{
		CodecName: "h264", Width: 640, Height: 360, FPS: 30, RealFPS: 30,
	}}
}

// transcode は動画 1 本を変換させ、その要求で解析したか（probes の増分）と、
// 変換に渡された解析情報を返す。
func (e *transcodeProbeEnv) transcode(t *testing.T, name, query string) (bool, *domain.TranscodeProbe) {
	t.Helper()
	before := e.fake.probes
	rec := do(t, e.handler, http.MethodGet, fmt.Sprintf("/api/videos/%d/transcode.mp4%s", e.ids[name], query))
	if rec.Code != http.StatusOK {
		t.Fatalf("%s%s: status = %d: %s", name, query, rec.Code, rec.Body.String())
	}
	return e.fake.probes != before, e.fake.probe
}

func (e *transcodeProbeEnv) stored(t *testing.T, name string) *domain.StoredTranscodeProbe {
	t.Helper()
	stored, err := e.db.Library().TranscodeProbe(context.Background(), e.ids[name])
	if err != nil {
		t.Fatal(err)
	}
	return stored
}

// 取り込み済みの動画は、1 回目もシーク後も解析しない（親 Issue #371 受け入れ条件 7）。
func TestTranscodeUsesIngestedProbe(t *testing.T) {
	env := newTranscodeProbeEnv(t)
	for _, query := range []string{"", "?startMs=30000"} {
		probed, probe := env.transcode(t, "ingested", query)
		if probed || probe == nil || probe.FormatName != "ingest" {
			t.Errorf("%q: probed=%v probe=%+v", query, probed, probe)
		}
	}
	info, err := os.Stat(env.paths["ingested"])
	if err != nil {
		t.Fatal(err)
	}
	if env.fake.source != domain.FileStampOf(info) {
		t.Errorf("source = %+v", env.fake.source)
	}
}

// 解析情報が無い動画は 1 回目だけ解析し、結果を保存して 2 回目は解析しない（受け入れ条件 8）。
func TestTranscodeSavesFreshProbe(t *testing.T) {
	env := newTranscodeProbeEnv(t)
	if probed, _ := env.transcode(t, "bare", ""); !probed {
		t.Fatal("解析情報が無いのに解析しなかった")
	}
	info, err := os.Stat(env.paths["bare"])
	if err != nil {
		t.Fatal(err)
	}
	stored := env.stored(t, "bare")
	if probe, ok := domain.TranscodeProbeUsable(stored, domain.FileStampOf(info)); !ok || probe.FormatName != "fresh" {
		t.Fatalf("保存された解析情報 = %+v", stored)
	}
	if probed, probe := env.transcode(t, "bare", "?startMs=1000"); probed || probe == nil || probe.FormatName != "fresh" {
		t.Errorf("2 回目: probed=%v probe=%+v", probed, probe)
	}
}

// 大きさか更新時刻が違うファイルでは解析して保存を置き換え、次は解析しない（受け入れ条件 9）。
func TestTranscodeReprobesChangedFile(t *testing.T) {
	for _, change := range []string{"mtime", "size"} {
		t.Run(change, func(t *testing.T) {
			env := newTranscodeProbeEnv(t)
			path := env.paths["ingested"]
			if change == "mtime" {
				later := time.Now().Add(time.Hour)
				if err := os.Chtimes(path, later, later); err != nil {
					t.Fatal(err)
				}
			} else {
				info, err := os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("changed"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
					t.Fatal(err)
				}
			}
			if probed, _ := env.transcode(t, "ingested", ""); !probed {
				t.Fatal("変わったファイルで保存値を使った")
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			stored := env.stored(t, "ingested")
			if stored == nil || stored.Source != domain.FileStampOf(info) {
				t.Fatalf("保存が置き換わっていない: %+v", stored)
			}
			if probed, probe := env.transcode(t, "ingested", ""); probed || probe == nil || probe.FormatName != "fresh" {
				t.Errorf("次の変換: probed=%v probe=%+v", probed, probe)
			}
		})
	}
}

// 変換が始められなかったとき（打ち切られた解析を含む）は何も保存しない。
func TestTranscodeDoesNotSaveProbeWhenStartFails(t *testing.T) {
	env := newTranscodeProbeEnv(t)
	env.fake.err = context.Canceled
	rec := do(t, env.handler, http.MethodGet, fmt.Sprintf("/api/videos/%d/transcode.mp4", env.ids["bare"]))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
	if stored := env.stored(t, "bare"); stored != nil {
		t.Errorf("失敗した変換の解析情報を保存した: %+v", stored)
	}
}

// firstChunkReader は最初の Read で初期データを返し、そのことを read で知らせる。
type firstChunkReader struct {
	read chan struct{}
	done bool
}

func (r *firstChunkReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}
	r.done = true
	close(r.read)
	return copy(p, "init"), nil
}

func (r *firstChunkReader) Close() error { return nil }

// slowProbeWriter は初期データが読まれるまで保存を終えない。保存が初期データの
// 読み出しより先に同期で走ると、期限まで待って失敗を記録する。
type slowProbeWriter struct {
	firstRead   chan struct{}
	blocked     bool
	hasDeadline bool
	saves       int
}

func (w *slowProbeWriter) SaveTranscodeProbe(ctx context.Context, _ int64, _ domain.FileStamp, _ domain.TranscodeProbe) error {
	w.saves++
	_, w.hasDeadline = ctx.Deadline()
	select {
	case <-w.firstRead:
		return nil
	case <-time.After(2 * time.Second):
		w.blocked = true
		return errors.New("初期データより先に保存を待たされた")
	}
}

// その場の解析結果の保存は初期データの読み出しを待たせず、独立した期限で行う。
func TestTranscodeSavesFreshProbeWithoutDelayingInitialData(t *testing.T) {
	mediaDir, video, _ := streamFixture(t, "a.mkv", 128)
	stream := &firstChunkReader{read: make(chan struct{})}
	writer := &slowProbeWriter{firstRead: stream.read}
	handler := newTestServer(t, Options{
		Videos:          &fakeLibrary{videos: map[int64]domain.Video{video.ID: video}, roots: []string{mediaDir}},
		Transcoder:      &fakeTranscoder{stream: stream, probed: transcodeProbeFixture("fresh")},
		TranscodeProbes: writer,
	})
	rec := do(t, handler, http.MethodGet, "/api/videos/1/transcode.mp4")
	if rec.Code != http.StatusOK || rec.Body.String() != "init" {
		t.Fatalf("response = %d %q", rec.Code, rec.Body.String())
	}
	if writer.saves != 1 || writer.blocked || !writer.hasDeadline {
		t.Errorf("saves=%d blocked=%v hasDeadline=%v", writer.saves, writer.blocked, writer.hasDeadline)
	}
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

// 設定が無い経路は software でエンコードする。
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

// 画質は LiveTranscodeRequest.Quality に載り、開始のログに添えられる。
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

// 列挙に無い値、動画の短辺以上の画質、寸法の無い動画は 400 で、変換を始めない。
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
