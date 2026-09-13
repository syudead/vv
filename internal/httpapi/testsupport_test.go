package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// fakeLibrary は保存層の代わりに、決め打ちの動画を返す。ハンドラの振る舞い
// （形・状態コード・ヘッダ）だけを検証したいので SQLite には触れない。
type fakeLibrary struct {
	videos map[int64]domain.Video
	page   domain.VideoPage
	// lastQuery は最後に渡された問い合わせ条件。丸めの検証に使う。
	lastQuery domain.VideoQuery
	listErr   error
}

func (f *fakeLibrary) ListVideos(_ context.Context, q domain.VideoQuery) (domain.VideoPage, error) {
	f.lastQuery = q
	if f.listErr != nil {
		return domain.VideoPage{}, f.listErr
	}

	page := f.page
	page.Limit = q.Limit
	if page.Items == nil {
		page.Items = []domain.Video{}
	}
	return page, nil
}

func (f *fakeLibrary) GetVideo(_ context.Context, id int64) (domain.Video, error) {
	video, ok := f.videos[id]
	if !ok {
		return domain.Video{}, domain.ErrNotFound
	}
	return video, nil
}

// fakeScans は走査の制御を差し替える。
type fakeScans struct {
	current    domain.Scan
	hasScan    bool
	started    int
	startErr   error
	currentErr error
}

func (f *fakeScans) StartScan(context.Context) (domain.Scan, error) {
	if f.startErr != nil {
		return domain.Scan{}, f.startErr
	}
	f.started++
	if !f.hasScan {
		f.current = domain.Scan{ID: 1, State: domain.ScanRunning, StartedAt: time.Now()}
		f.hasScan = true
	}
	return f.current, nil
}

func (f *fakeScans) CurrentScan(context.Context) (domain.Scan, error) {
	if f.currentErr != nil {
		return domain.Scan{}, f.currentErr
	}
	if !f.hasScan {
		return domain.Scan{}, domain.ErrNotFound
	}
	return f.current, nil
}

// sampleVideo は解析済みの動画を1件返す。
func sampleVideo(id int64, title string) domain.Video {
	duration := int64(8533)
	width, height := 1280, 720
	return domain.Video{
		ID:             id,
		Path:           "/media/" + title + ".mp4",
		Title:          title,
		SizeBytes:      1024,
		AddedAt:        time.Unix(1_757_000_000, 0),
		ContentKey:     "abcdef0123456789abcdef:1024",
		DurationMs:     &duration,
		Width:          &width,
		Height:         &height,
		Container:      "mp4",
		VideoCodec:     "h264",
		AudioCodec:     "aac",
		Playable:       true,
		ProbeState:     domain.ProbeStateDone,
		ThumbnailState: domain.ThumbnailStateDone,
	}
}

// newTestServer は検証用の経路を組み立てる。
func newTestServer(t *testing.T, opts Options) http.Handler {
	t.Helper()

	if opts.Assets == nil {
		opts.Assets = emptyAssets{}
	}
	return NewRouter(opts)
}

// emptyAssets は SPA を持たないファイルシステムである。API の検証では
// index.html を要らない。
type emptyAssets struct{}

func (emptyAssets) Open(string) (fs.File, error) { return nil, errors.New("assets はありません") }

// do は要求を1つ投げて応答を返す。
func do(t *testing.T, handler http.Handler, method, target string) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}

// decode は JSON の応答を読み取る。
func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()

	var out T
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("応答を JSON として読めない (%d): %s", rec.Code, rec.Body.String())
	}
	return out
}
