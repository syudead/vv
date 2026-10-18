package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/syudead/vv/internal/domain"
)

type probeLibrary struct {
	*fakeLibrary
	stored        *domain.StoredTranscodeProbe
	keyframes     domain.KeyframeProbe
	err           error
	reads         []int64
	keyframeReads []int64
}

func (l *probeLibrary) TranscodeProbe(_ context.Context, id int64) (*domain.StoredTranscodeProbe, error) {
	l.reads = append(l.reads, id)
	return l.stored, l.err
}
func (l *probeLibrary) KeyframeSpacing(_ context.Context, id int64) (domain.KeyframeProbe, error) {
	l.keyframeReads = append(l.keyframeReads, id)
	return l.keyframes, nil
}

func transcodeMetadataFixture(t *testing.T) (*probeLibrary, domain.FileStamp) {
	t.Helper()
	root, video, _ := streamFixture(t, "a.mkv", 1)
	video.ID = 7
	info, err := os.Stat(video.Path)
	if err != nil {
		t.Fatal(err)
	}
	return &probeLibrary{fakeLibrary: &fakeLibrary{videos: map[int64]domain.Video{7: video}, roots: []string{root}}}, domain.FileStampOf(info)
}

func transcodeProbeFixture(origin string) domain.TranscodeProbe {
	return domain.TranscodeProbe{FormatName: origin, Video: domain.TranscodeVideo{CodecName: "h264", Width: 640, Height: 360, FPS: 30, RealFPS: 30}}
}

func TestTranscodeForwardsOnlyUsableProbe(t *testing.T) {
	for _, name := range []string{"matching", "stale", "missing", "read failure"} {
		t.Run(name, func(t *testing.T) {
			library, source := transcodeMetadataFixture(t)
			probe := transcodeProbeFixture("ingest")
			encoded, err := json.Marshal(probe)
			if err != nil {
				t.Fatal(err)
			}
			library.stored = &domain.StoredTranscodeProbe{Version: domain.TranscodeProbeVersion, Source: source, Probe: string(encoded)}
			switch name {
			case "stale":
				library.stored.Source.SizeBytes++
			case "missing":
				library.stored = nil
			case "read failure":
				library.err = errors.New("storage unavailable")
			}
			fake := &fakeTranscoder{body: "fragmented-mp4"}
			handler := newTestServer(t, Options{Videos: library, Transcoder: fake})
			rec := do(t, handler, http.MethodGet, "/api/videos/7/transcode.mp4?startMs=1000")
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d: %s", rec.Code, rec.Body)
			}
			if !slices.Equal(library.reads, []int64{7}) || fake.source != source {
				t.Fatalf("reads=%v source=%+v, want %+v", library.reads, fake.source, source)
			}
			if name == "matching" {
				if fake.probe == nil || !reflect.DeepEqual(*fake.probe, probe) {
					t.Fatalf("forwarded probe = %+v", fake.probe)
				}
			} else if fake.probe != nil {
				t.Fatalf("unusable probe forwarded: %+v", fake.probe)
			}
		})
	}
}

type savedTranscodeProbe struct {
	ctx     context.Context
	videoID int64
	source  domain.FileStamp
	probe   domain.TranscodeProbe
}

type recordingProbeWriter struct {
	calls   []savedTranscodeProbe
	started chan struct{}
	release chan struct{}
}

func (w *recordingProbeWriter) SaveTranscodeProbe(ctx context.Context, id int64, source domain.FileStamp, probe domain.TranscodeProbe) error {
	w.calls = append(w.calls, savedTranscodeProbe{ctx, id, source, probe})
	if w.started != nil {
		close(w.started)
		<-w.release
	}
	return nil
}

func TestTranscodeSavesOnlySuccessfullyStartedProbe(t *testing.T) {
	for _, startFails := range []bool{false, true} {
		library, source := transcodeMetadataFixture(t)
		fresh := transcodeProbeFixture("fresh")
		fake := &fakeTranscoder{body: "fragmented-mp4", probed: fresh}
		writer := &recordingProbeWriter{}
		if startFails {
			fake.err = errors.New("start failed")
		}
		handler := newTestServer(t, Options{Videos: library, Transcoder: fake, TranscodeProbes: writer})
		rec := do(t, handler, http.MethodGet, "/api/videos/7/transcode.mp4")
		if startFails {
			if rec.Code != http.StatusInternalServerError || len(writer.calls) != 0 {
				t.Fatalf("status=%d saves=%d", rec.Code, len(writer.calls))
			}
		} else {
			if rec.Code != http.StatusOK || len(writer.calls) != 1 {
				t.Fatalf("status=%d saves=%d: %s", rec.Code, len(writer.calls), rec.Body)
			}
			got := writer.calls[0]
			if got.videoID != 7 || got.source != source || !reflect.DeepEqual(got.probe, fresh) {
				t.Fatalf("saved probe = %+v", got)
			}
		}
	}
}

type firstWriteRecorder struct {
	*httptest.ResponseRecorder
	written chan struct{}
	once    sync.Once
}

func (w *firstWriteRecorder) Write(p []byte) (int, error) {
	n, err := w.ResponseRecorder.Write(p)
	w.once.Do(func() { close(w.written) })
	return n, err
}

func TestTranscodeSavesFreshProbeWithoutDelayingInitialData(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		library, _ := transcodeMetadataFixture(t)
		writer := &recordingProbeWriter{started: make(chan struct{}), release: make(chan struct{})}
		fake := &fakeTranscoder{body: "fragmented-mp4", probed: transcodeProbeFixture("fresh")}
		handler := newTestServer(t, Options{Videos: library, Transcoder: fake, TranscodeProbes: writer})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		req := httptest.NewRequest(http.MethodGet, "/api/videos/7/transcode.mp4", nil).WithContext(ctx)
		rec := &firstWriteRecorder{ResponseRecorder: httptest.NewRecorder(), written: make(chan struct{})}
		done := make(chan struct{})
		release := sync.OnceFunc(func() { close(writer.release) })
		go func() { handler.ServeHTTP(rec, req); close(done) }()
		defer func() { release(); <-done }()
		synctest.Wait()
		select {
		case <-writer.started:
		default:
			t.Fatal("probe save did not start")
		}
		select {
		case <-rec.written:
		default:
			t.Fatal("saving the probe blocked the response body")
		}
		if rec.Code != http.StatusOK || rec.Body.String() != fake.body {
			t.Fatalf("response=%d %q", rec.Code, rec.Body)
		}
		select {
		case <-done:
			t.Fatal("handler returned before probe save completed")
		default:
		}
		cancel()
		saveCtx := writer.calls[0].ctx
		if err := saveCtx.Err(); err != nil {
			t.Fatalf("request cancellation reached probe save: %v", err)
		}
		deadline, ok := saveCtx.Deadline()
		if !ok || !deadline.Equal(time.Now().Add(transcodeProbeSaveTimeout)) {
			t.Fatalf("save deadline=%v, present=%v", deadline, ok)
		}
		release()
		<-done
	})
}
