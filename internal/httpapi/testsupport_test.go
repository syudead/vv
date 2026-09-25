package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/mediafs"
)

// fakeArtifacts は生成物の置き場の代わりである。thumbnails・previews は content
// key から配信するファイルのパスへの対応で、無いものは「無い」と答える。
// シーク用プレビューは image と err を決め打ちで返し、渡された値を控える。
type fakeArtifacts struct {
	thumbnails map[string]string
	previews   map[string]string

	image      []byte
	err        error
	contentKey string
	positionMs int64
}

func (f *fakeArtifacts) ThumbnailFile(contentKey string) (*os.File, error) {
	return openFake(f.thumbnails, contentKey)
}

// PreviewFile は本物の置き場と同じく、内容の SHA-256 を添えて返す。
func (f *fakeArtifacts) PreviewFile(contentKey string) (*os.File, string, error) {
	file, err := openFake(f.previews, contentKey)
	if err != nil {
		return nil, "", err
	}
	h := sha256.New()
	_, err = io.Copy(h, file)
	if err == nil {
		_, err = file.Seek(0, io.SeekStart)
	}
	if err != nil {
		_ = file.Close()
		return nil, "", err
	}
	return file, hex.EncodeToString(h.Sum(nil)), nil
}

func (f *fakeArtifacts) SeekThumbnail(contentKey string, positionMs int64) ([]byte, error) {
	f.contentKey, f.positionMs = contentKey, positionMs
	return f.image, f.err
}

func openFake(paths map[string]string, contentKey string) (*os.File, error) {
	path, ok := paths[contentKey]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return os.Open(path)
}

// fakeLibrary は保存層の代わりに、決め打ちの動画を返す。ハンドラの振る舞い
// （形・状態コード・ヘッダ）だけを検証したいので SQLite には触れない。
type fakeLibrary struct {
	videos map[int64]domain.Video
	roots  []string
	page   domain.VideoPage
	// lastQuery は最後に渡された問い合わせ条件。丸めの検証に使う。
	lastQuery domain.VideoQuery
	listErr   error
}

func (f *fakeLibrary) VideoLocations(_ context.Context, videoID int64) ([]domain.VideoLocation, error) {
	video, ok := f.videos[videoID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return []domain.VideoLocation{{ID: 1, VideoID: videoID, Path: video.Path, Version: 1}}, nil
}

func (f *fakeLibrary) ListMediaFolders(context.Context) ([]domain.MediaFolder, error) {
	folders := make([]domain.MediaFolder, 0, len(f.roots))
	for i, root := range f.roots {
		folders = append(folders, domain.MediaFolder{ID: int64(i + 1), Path: root, Version: 1})
	}
	return folders, nil
}

// requireOwner は、偽物を使う経路のテストが所有者として読むことを偽物の側で確かめる。
// それらのテストは ownerAuth で境界を越えるので、ゲストとして読んだら取り違えである。
// ゲストとしての読み出しは guest_test.go が本物の保存層で確かめる。
func requireOwner(audience domain.Audience) error {
	if !audience.IsOwner() {
		return errors.New("所有者として読んでいません")
	}
	return nil
}

func (f *fakeLibrary) ListVideos(_ context.Context, audience domain.Audience, q domain.VideoQuery) (domain.VideoPage, error) {
	if err := requireOwner(audience); err != nil {
		return domain.VideoPage{}, err
	}
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

func (f *fakeLibrary) GetVideo(_ context.Context, audience domain.Audience, id int64) (domain.Video, error) {
	if err := requireOwner(audience); err != nil {
		return domain.Video{}, err
	}
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
		PreviewState:   domain.PreviewStatePending,
	}
}

// newTestServer は検証用の経路を組み立てる。
func newTestServer(t *testing.T, opts Options) http.Handler {
	t.Helper()

	if opts.Assets == nil {
		opts.Assets = emptyAssets{}
	}
	if opts.Files == nil {
		opts.Files = mediafs.New()
	}
	if opts.Auth == nil {
		opts.Auth = ownerAuth{}
	}
	return NewRouter(opts)
}

// ownerAuth はどの要求も所有者として通す認証の代わりである。認証の境界そのものは
// auth_test.go が本物の Auth で確かめるので、他の経路のテストはこれで境界を越える。
type ownerAuth struct{}

func (ownerAuth) Setup(context.Context, string, string) (IssuedSession, error) {
	return IssuedSession{}, domain.ErrAccountAlreadyConfigured
}

func (ownerAuth) Login(context.Context, LoginAttempt) (IssuedSession, error) {
	return IssuedSession{}, domain.ErrInvalidCredentials
}

func (ownerAuth) CheckSession(context.Context, string) (time.Time, bool, error) {
	return time.Now().Add(time.Hour), true, nil
}

func (ownerAuth) Logout(context.Context, string) error { return nil }

func (ownerAuth) State(context.Context, string) (AuthState, error) { return AuthStateOwner, nil }

// emptyAssets は SPA を持たないファイルシステムである。API の検証では
// index.html を要らない。
type emptyAssets struct{}

func (emptyAssets) Open(string) (fs.File, error) { return nil, errors.New("assets はありません") }

// do は要求を1つ投げて応答を返す。
func do(t *testing.T, handler http.Handler, method, target string) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	body := ""
	if method == http.MethodPost || method == http.MethodPut {
		body = "{}"
	}
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if method == http.MethodPost || method == http.MethodPut {
		req.Header.Set("Content-Type", "application/json")
	}
	handler.ServeHTTP(rec, req)
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

// fakeCatalog はアプリケーション層の代わりに、決め打ちの判断を返す。プレビューの
// 作り直しやシーク用プレビューの状態の導き方は internal/app で検証するので、
// ここでは返された判断が応答にどう写るかだけを見る。
type fakeCatalog struct {
	mu sync.Mutex
	// previews はファイルがある一覧用プレビューの内容の識別子。
	previews map[string]bool
	// requeue が真なら、ファイルの無い done のプレビューを pending と読み替える。
	requeue  bool
	requeued []int64

	seekStates map[int64]domain.SeekThumbnailState
	seekErr    error

	related      domain.RelatedVideos
	relatedErr   error
	relatedAsked []int64

	// group は VideoGroup が返すグループ。grouped が false ならグループに属さない。
	group    domain.VideoGroup
	grouped  bool
	groupErr error

	retry func(domain.Video) error
}

func (f *fakeCatalog) PresentVideos(_ context.Context, videos []domain.Video) []domain.VideoView {
	f.mu.Lock()
	defer f.mu.Unlock()
	views := make([]domain.VideoView, 0, len(videos))
	for _, video := range videos {
		available := f.previews[video.ContentKey]
		if video.PreviewState == domain.PreviewStateDone && !available && f.requeue {
			f.requeued = append(f.requeued, video.ID)
			video.PreviewState = domain.PreviewStatePending
		}
		views = append(views, domain.VideoView{Video: video, PreviewAvailable: available})
	}
	return views
}

func (f *fakeCatalog) SeekThumbnailState(_ context.Context, video domain.Video) (domain.SeekThumbnailState, error) {
	if f.seekErr != nil {
		return "", f.seekErr
	}
	if state, ok := f.seekStates[video.ID]; ok {
		return state, nil
	}
	return domain.SeekThumbnailPending, nil
}

func (f *fakeCatalog) RelatedVideos(_ context.Context, audience domain.Audience, video domain.Video) (domain.RelatedVideos, error) {
	if err := requireOwner(audience); err != nil {
		return domain.RelatedVideos{}, err
	}
	f.relatedAsked = append(f.relatedAsked, video.ID)
	return f.related, f.relatedErr
}

func (f *fakeCatalog) VideoGroup(_ context.Context, _ domain.Audience, _ domain.Video) (domain.VideoGroup, bool, error) {
	return f.group, f.grouped, f.groupErr
}

func (f *fakeCatalog) RetryProbe(_ context.Context, video domain.Video) error {
	if f.retry == nil {
		return nil
	}
	return f.retry(video)
}
