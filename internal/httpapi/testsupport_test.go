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

// fakeArtifacts maps content keys to files; nil sprite means fs.ErrNotExist, and returned
// values/arguments are controllable.
type fakeArtifacts struct {
	thumbnails map[string]string
	posters    map[string]string
	previews   map[string]string

	sprite     *domain.SeekSprite
	spriteErr  error
	image      []byte
	err        error
	contentKey string
	sheet      int
}

func (f *fakeArtifacts) ThumbnailFile(contentKey string) (*os.File, error) {
	return openFake(f.thumbnails, contentKey)
}

func (f *fakeArtifacts) PosterFile(contentKey string) (*os.File, error) {
	return openFake(f.posters, contentKey)
}

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

func (f *fakeArtifacts) SeekSprite(contentKey string) (domain.SeekSprite, error) {
	f.contentKey = contentKey
	switch {
	case f.spriteErr != nil:
		return domain.SeekSprite{}, f.spriteErr
	case f.sprite == nil:
		return domain.SeekSprite{}, fs.ErrNotExist
	}
	return *f.sprite, nil
}

func (f *fakeArtifacts) SeekSpriteSheet(contentKey string, sheet int) ([]byte, error) {
	f.contentKey, f.sheet = contentKey, sheet
	return f.image, f.err
}

// testSprite represents the older two-sheet layout.
func testSprite() *domain.SeekSprite {
	return &domain.SeekSprite{
		SeekSpriteLayout: domain.SeekSpriteLayout{IntervalMs: 5000, FrameCount: 150, Columns: 10, Rows: 10, SheetCount: 2},
		FrameWidth:       320,
		FrameHeight:      180,
	}
}

func openFake(paths map[string]string, contentKey string) (*os.File, error) {
	path, ok := paths[contentKey]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return os.Open(path)
}

// Use fixed videos to isolate response shape/status/headers without SQLite.
type fakeLibrary struct {
	videos map[int64]domain.Video
	roots  []string
	page   domain.VideoPage

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

// TranscodeProbe reports no stored metadata. Tests of metadata forwarding
// override it through probeLibrary.
func (f *fakeLibrary) TranscodeProbe(context.Context, int64) (*domain.StoredTranscodeProbe, error) {
	return nil, nil
}

func (f *fakeLibrary) ListMediaFolders(context.Context) ([]domain.MediaFolder, error) {
	folders := make([]domain.MediaFolder, 0, len(f.roots))
	for i, root := range f.roots {
		folders = append(folders, domain.MediaFolder{ID: int64(i + 1), Path: root, Version: 1})
	}
	return folders, nil
}

// Fake-backed route tests must request owner reads; guest behavior is tested with real storage in
// guest_test.go.
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

type fakeScans struct {
	current    domain.Scan
	hasScan    bool
	started    int
	startErr   error
	currentErr error
	rescanErr  error
	rescans    []domain.RescanScope

	issues []domain.ScanIssue
}

func (f *fakeScans) StartRescan(_ context.Context, scope domain.RescanScope) (domain.Scan, error) {
	f.rescans = append(f.rescans, scope)
	if f.rescanErr != nil {
		return domain.Scan{}, f.rescanErr
	}
	f.current = domain.Scan{ID: f.current.ID + 1, State: domain.ScanRunning, Origin: domain.ScanOriginManual, Rescan: &scope}
	f.hasScan = true
	return f.current, nil
}

func (f *fakeScans) ListScanIssues(_ context.Context, cursor string, limit int) (domain.ScanIssuePage, error) {
	if f.currentErr != nil {
		return domain.ScanIssuePage{}, f.currentErr
	}
	if !f.hasScan {
		return domain.ScanIssuePage{}, domain.ErrNotFound
	}
	items, next, err := domain.PageScanIssues(f.issues, cursor, limit)
	if err != nil {
		return domain.ScanIssuePage{}, err
	}
	return domain.ScanIssuePage{ScanID: f.current.ID, Items: items, NextCursor: next}, nil
}

func (f *fakeScans) StartScan(context.Context) (domain.Scan, bool, error) {
	if f.startErr != nil {
		return domain.Scan{}, false, f.startErr
	}
	f.started++
	if f.hasScan && f.current.State == domain.ScanRunning {
		return f.current, false, nil
	}
	f.current = domain.Scan{ID: f.current.ID + 1, State: domain.ScanRunning, StartedAt: time.Now()}
	f.hasScan = true
	return f.current, true, nil
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
		UserKey:        "abcdef0123456789abcdef:1024",
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

// Use ownerAuth to isolate routes; auth_test.go verifies the real boundary.
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

func (ownerAuth) Username(context.Context) (string, error) { return testUsername, nil }

func (ownerAuth) ChangeCredentials(context.Context, CredentialChange) (IssuedSession, error) {
	return IssuedSession{}, domain.ErrInvalidCredentials
}

type emptyAssets struct{}

func (emptyAssets) Open(string) (fs.File, error) { return nil, errors.New("assets はありません") }

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

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()

	var out T
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("応答を JSON として読めない (%d): %s", rec.Code, rec.Body.String())
	}
	return out
}

// Application tests cover rebuild/state decisions; fakeCatalog isolates how handlers present them.
type fakeCatalog struct {
	mu sync.Mutex

	previews map[string]bool
	// requeue changes missing done previews to pending.
	requeue  bool
	requeued []int64

	seekStates map[int64]domain.SeekThumbnailState
	seekErr    error

	related      domain.RelatedVideos
	relatedErr   error
	relatedAsked []int64

	relatedPages []domain.RelatedPage

	group    domain.VideoGroup
	grouped  bool
	groupErr error

	groupWindows []domain.GroupWindow

	groupContexts []int64

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

func (f *fakeCatalog) RelatedVideos(_ context.Context, audience domain.Audience, video domain.Video, page domain.RelatedPage) (domain.RelatedVideos, error) {
	if err := requireOwner(audience); err != nil {
		return domain.RelatedVideos{}, err
	}
	f.relatedAsked = append(f.relatedAsked, video.ID)
	f.relatedPages = append(f.relatedPages, page)
	return f.related, f.relatedErr
}

// The related list over both nodes is tested with the real catalog (peer_related_test.go).
var errFakeCatalogMerged = errors.New("the fake catalog has no related list over both nodes")

func (f *fakeCatalog) MergedRelatedVideos(context.Context, domain.Audience, domain.Video, domain.MergedRelatedPage, domain.RelatedPeer) (domain.MergedRelated, error) {
	return domain.MergedRelated{}, errFakeCatalogMerged
}

func (f *fakeCatalog) RemoteRelatedVideos(context.Context, domain.Audience, domain.RelatedClues, []int64, domain.MergedRelatedPage, domain.RelatedPeer) (domain.MergedRelated, error) {
	return domain.MergedRelated{}, errFakeCatalogMerged
}

func (f *fakeCatalog) PeerRelated(context.Context, domain.Audience, domain.RelatedClues, []int64, *domain.RelatedCursor, int) (domain.ScoredRelated, error) {
	return domain.ScoredRelated{}, errFakeCatalogMerged
}

func (f *fakeCatalog) RelatedClues(context.Context, domain.Audience, int64) (domain.RelatedClues, bool, error) {
	return domain.RelatedClues{}, false, errFakeCatalogMerged
}

func (f *fakeCatalog) VideoGroup(_ context.Context, _ domain.Audience, _ domain.Video, opened domain.VideoContext, window domain.GroupWindow) (domain.VideoGroup, bool, error) {
	f.groupWindows = append(f.groupWindows, window)
	f.groupContexts = append(f.groupContexts, opened.GroupID)
	return f.group, f.grouped, f.groupErr
}

func (f *fakeCatalog) RetryProbe(_ context.Context, video domain.Video) error {
	if f.retry == nil {
		return nil
	}
	return f.retry(video)
}

func (f *fakeLibrary) NameFilters(_ context.Context, q domain.VideoQuery) (domain.VideoQuery, bool, error) {
	return q, true, nil
}

func (f *fakeLibrary) KeyframeSpacing(context.Context, int64) (domain.KeyframeProbe, error) {
	return domain.KeyframeProbe{}, nil
}
