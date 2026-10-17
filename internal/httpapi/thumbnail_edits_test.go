package httpapi

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/store"
)

// サムネイルの編集のセッションの経路は、
// 本物の認証・保存層と app.ThumbnailEdits で確かめる。提供元・コマの取り出し・適用だけが偽である。

// editFakes は app.ThumbnailEdits の偽の部品である。
type editFakes struct {
	edits *app.ThumbnailEdits

	mu sync.Mutex
	// positions は FrameAt に渡された位置である。
	positions []int64
	starts    int
	startErr  error
	state     domain.ThumbnailEditState
	cancels   int
	noFrame   bool
	applyErr  error
	frameSize int
}

func newEditFakes(db *store.DB) *editFakes {
	f := &editFakes{state: domain.ThumbnailEditQueued, frameSize: 4}
	f.edits = app.NewThumbnailEdits(app.ThumbnailEditsOptions{
		Settings: db.Settings(), Editor: editEditor{f}, Frames: editFrames{f}, Images: db.Overrides(),
		Applier: editApplier{f, db},
	})
	return f
}

func (f *editFakes) set(apply func(*editFakes)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	apply(f)
}

func (f *editFakes) counts() (starts, cancels int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.starts, f.cancels
}

type editEditor struct{ f *editFakes }

func (e editEditor) Start(_ context.Context, conn domain.ComfyUISettings, _ domain.ImageEditInput) (domain.ImageEditJob, error) {
	e.f.mu.Lock()
	defer e.f.mu.Unlock()
	e.f.starts++
	if e.f.startErr != nil {
		return domain.ImageEditJob{}, e.f.startErr
	}
	e.f.state = domain.ThumbnailEditQueued
	return domain.ImageEditJob{Connection: conn, ID: "job"}, nil
}

func (e editEditor) State(context.Context, domain.ImageEditJob, domain.ThumbnailEditState) (domain.ThumbnailEditState, *domain.ImageEditError, error) {
	e.f.mu.Lock()
	defer e.f.mu.Unlock()
	return e.f.state, nil, nil
}

func (e editEditor) Cancel(context.Context, domain.ImageEditJob, domain.ThumbnailEditState) error {
	e.f.mu.Lock()
	defer e.f.mu.Unlock()
	e.f.cancels++
	return nil
}

func (e editEditor) Progress(domain.ImageEditJob) *domain.ImageEditProgress {
	return nil
}

func (e editEditor) Output(context.Context, domain.ImageEditJob) ([]byte, error) {
	return editPNG(4, 4), nil
}

type editFrames struct{ f *editFakes }

func (e editFrames) FrameAt(_ context.Context, _ string, positionMs int64) (domain.ThumbnailFrame, error) {
	e.f.mu.Lock()
	defer e.f.mu.Unlock()
	e.f.positions = append(e.f.positions, positionMs)
	if e.f.noFrame {
		return domain.ThumbnailFrame{}, errors.New("ffmpeg: no frame")
	}
	size := e.f.frameSize
	return domain.ThumbnailFrame{PNG: editPNG(size, size), Width: size, Height: size}, nil
}

// AutoThumbnailPositions は自動の位置を 6 秒、次に先頭とする。
func (e editFrames) AutoThumbnailPositions(int64) []int64 {
	return []int64{6_000, 0}
}

// editApplier は画像を本物の保存層へ記録する。applyErr があればそれを返して何も書かない。
type editApplier struct {
	f  *editFakes
	db *store.DB
}

func (e editApplier) ApplyThumbnailImage(
	ctx context.Context, videoID int64, contentKey string, image []byte, origin domain.ThumbnailImageOrigin,
) (domain.Video, error) {
	e.f.mu.Lock()
	err := e.f.applyErr
	e.f.mu.Unlock()
	if err != nil {
		return domain.Video{}, err
	}
	return e.db.Ingest().ApplyThumbnailImage(ctx, videoID, contentKey,
		domain.AppliedThumbnailImage{Image: image, Source: image, Origin: origin})
}

func editPNG(width, height int) []byte {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, width, height))); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// saveConnection は ComfyUI の接続先を保存する。
func (f *guestFixture) saveConnection(t *testing.T) {
	t.Helper()
	settings := domain.ComfyUISettings{Provider: domain.ComfyUISelfHosted, URL: "http://gpu:8188"}
	if err := f.env.db.Settings().SaveComfyUISettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
}

func (f *guestFixture) startEdit(name, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	return f.env.serve(authRequest{method: http.MethodPost, target: f.videoPath(name, "/thumbnail-edits"), body: body, cookies: cookies})
}

func (f *guestFixture) startedEdit(t *testing.T, name string) gen.ThumbnailEdit {
	t.Helper()
	rec := f.startEdit(name, `{"source":"frame","positionMs":1000}`, f.owner)
	if rec.Code != http.StatusCreated {
		t.Fatalf("start: %d %s", rec.Code, rec.Body)
	}
	return decode[gen.ThumbnailEdit](t, rec)
}

func (f *guestFixture) editRequest(method, id, suffix string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	return f.env.serve(authRequest{method: method, target: "/api/thumbnail-edits/" + id + suffix, cookies: cookies})
}

// runEdit は fields を multipart/form-data で送る。値は prompt なら文、mask と reference ならファイルである。
func (f *guestFixture) runEdit(t *testing.T, id string, fields map[string][]byte, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for name, value := range fields {
		var part interface{ Write([]byte) (int, error) }
		var err error
		if name == "prompt" || name == "extra" {
			part, err = writer.CreateFormField(name)
		} else {
			part, err = writer.CreateFormFile(name, name+".png")
		}
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(value); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/api/thumbnail-edits/"+id+"/runs", &buf)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	for _, cookie := range cookies {
		r.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	f.env.handler.ServeHTTP(rec, r)
	return rec
}

// ゲストはどの経路でも 401 で、セッションに触れない。
func TestThumbnailEditRoutesAreOwnerOnly(t *testing.T) {
	f := newGuestFixture(t, true)
	f.saveConnection(t)
	id := f.startedEdit(t, "a").Id

	assertUnauthenticated(t, "start", f.startEdit("a", `{"source":"frame","positionMs":1000}`))
	assertUnauthenticated(t, "start from the thumbnail", f.startEdit("a", `{"source":"thumbnail"}`))
	assertUnauthenticated(t, "runs", f.runEdit(t, id, map[string][]byte{"prompt": []byte("x")}))
	for _, route := range []struct{ method, suffix string }{
		{http.MethodGet, ""}, {http.MethodGet, "/frame"}, {http.MethodGet, "/result"},
		{http.MethodPost, "/cancel"}, {http.MethodPost, "/apply"}, {http.MethodDelete, ""},
	} {
		assertUnauthenticated(t, route.method+" "+route.suffix, f.editRequest(route.method, id, route.suffix))
	}
	assertUnauthenticated(t, "readiness", f.env.get("/api/thumbnail-edits/readiness"))
	if starts, _ := f.edits.counts(); starts != 0 {
		t.Fatalf("a guest started a run")
	}
	if rec := f.editRequest(http.MethodGet, id, "", f.owner); rec.Code != http.StatusOK {
		t.Fatalf("the guest requests ended the session: %d %s", rec.Code, rec.Body)
	}
}

// 始めるときの契約の状態。
func TestStartThumbnailEditStatuses(t *testing.T) {
	f := newGuestFixture(t, true)
	check := func(label string, rec *httptest.ResponseRecorder, want wantError) {
		t.Helper()
		assertErrorBody(t, label, rec.Code, rec.Body.Bytes(), want)
	}
	check("no connection", f.startEdit("a", `{"source":"frame","positionMs":1000}`, f.owner),
		wantError{status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: reasonThumbnailEditNotReady})
	f.saveConnection(t)

	check("no source", f.startEdit("a", `{"positionMs":1000}`, f.owner),
		wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest})
	check("unknown source", f.startEdit("a", `{"source":"poster"}`, f.owner),
		wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest})
	check("frame without a position", f.startEdit("a", `{"source":"frame"}`, f.owner),
		wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonThumbnailPositionOutOfRange, limit: 60_000})
	check("past the end", f.startEdit("a", `{"source":"frame","positionMs":60000}`, f.owner),
		wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonThumbnailPositionOutOfRange, limit: 60_000})
	check("no duration", f.startEdit("d", `{"source":"frame","positionMs":1000}`, f.owner),
		wantError{status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: reasonDurationUnknown})
	missing := f.env.serve(authRequest{method: http.MethodPost, target: "/api/videos/9999/thumbnail-edits",
		body: `{"source":"frame","positionMs":1000}`, cookies: []*http.Cookie{f.owner}})
	check("no video", missing, wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: reasonVideoNotFound})

	f.edits.set(func(e *editFakes) { e.noFrame = true })
	check("no frame", f.startEdit("a", `{"source":"frame","positionMs":1000}`, f.owner),
		wantError{status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: reasonThumbnailFrameUnavailable})
	f.edits.set(func(e *editFakes) { e.noFrame = false })

	video := decode[gen.Video](t, f.env.get(f.videoPath("b", ""), f.owner))
	if err := os.Remove(video.Location.Path); err != nil {
		t.Fatal(err)
	}
	check("no file", f.startEdit("b", `{"source":"frame","positionMs":1000}`, f.owner),
		wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: reasonFileUnavailable})

	rec := f.startEdit("a", `{"source":"frame","positionMs":1000}`, f.owner)
	edit := decode[gen.ThumbnailEdit](t, rec)
	if rec.Code != http.StatusCreated || rec.Header().Get("Cache-Control") != cacheNoStore ||
		edit.State != gen.ThumbnailEditStateReady || edit.VideoId != f.ids["a"] || edit.ElapsedMs != nil ||
		edit.FrameUrl != "/api/thumbnail-edits/"+edit.Id+"/frame" || edit.FrameWidth != 4 || edit.FrameHeight != 4 ||
		edit.ResultUrl != nil || edit.Source != gen.ThumbnailEditSourceFrame ||
		edit.PositionMs == nil || *edit.PositionMs != 1000 {
		t.Fatalf("start = %d %s", rec.Code, rec.Body)
	}
	frame := f.editRequest(http.MethodGet, edit.Id, "/frame", f.owner)
	if frame.Code != http.StatusOK || frame.Header().Get("Content-Type") != "image/png" ||
		!bytes.Equal(frame.Body.Bytes(), editPNG(4, 4)) || frame.Header().Get("Cache-Control") != cacheNoStore {
		t.Fatalf("frame = %d %v", frame.Code, frame.Header())
	}

	// 同じ動画の 2 つ目の編集は 1 つ目を終える。
	second := f.startedEdit(t, "a")
	rec = f.editRequest(http.MethodGet, edit.Id, "", f.owner)
	check("replaced", rec, wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: reasonThumbnailEditNotFound})
	if rec := f.editRequest(http.MethodGet, second.Id, "", f.owner); rec.Code != http.StatusOK {
		t.Fatalf("second = %d", rec.Code)
	}
}

// 実行の入力の誤りは何も上げる前に返し、接続先の消去と提供元の失敗は契約の状態で返す。
func TestRunThumbnailEditStatuses(t *testing.T) {
	f := newGuestFixture(t, true)
	f.saveConnection(t)
	id := f.startedEdit(t, "a").Id
	check := func(label string, rec *httptest.ResponseRecorder, want wantError) {
		t.Helper()
		assertErrorBody(t, label, rec.Code, rec.Body.Bytes(), want)
	}
	invalid := wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest}
	maskInvalid := wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonThumbnailEditMaskInvalid}

	check("mask of another size", f.runEdit(t, id, map[string][]byte{"mask": editPNG(5, 4)}, f.owner), maskInvalid)
	check("empty mask", f.runEdit(t, id, map[string][]byte{"mask": {}}, f.owner), maskInvalid)
	check("mask over the limit", f.runEdit(t, id, map[string][]byte{"mask": make([]byte, thumbnailEditMaxMaskBytes+1)}, f.owner), maskInvalid)
	check("reference", f.runEdit(t, id, map[string][]byte{"reference": []byte("text")}, f.owner),
		wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonThumbnailEditReferenceInvalid})
	check("long prompt", f.runEdit(t, id, map[string][]byte{"prompt": []byte(strings.Repeat("字", 4001))}, f.owner), invalid)
	check("unknown part", f.runEdit(t, id, map[string][]byte{"extra": []byte("x")}, f.owner), invalid)
	check("JSON body", f.env.serve(authRequest{method: http.MethodPost, target: "/api/thumbnail-edits/" + id + "/runs",
		body: `{"prompt":"x"}`, cookies: []*http.Cookie{f.owner}}), invalid)
	if starts, _ := f.edits.counts(); starts != 0 {
		t.Fatalf("an invalid run was sent to the provider %d times", starts)
	}

	if err := f.env.db.Settings().SaveComfyUISettings(context.Background(), domain.ComfyUISettings{}); err != nil {
		t.Fatal(err)
	}
	check("no connection", f.runEdit(t, id, nil, f.owner),
		wantError{status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: reasonThumbnailEditNotReady})
	f.saveConnection(t)

	f.edits.set(func(e *editFakes) {
		e.startErr = &domain.ImageEditError{Reason: domain.ImageEditUnreachable, Message: "dial tcp"}
	})
	rec := f.runEdit(t, id, map[string][]byte{"prompt": []byte("remove"), "mask": editPNG(4, 4)}, f.owner)
	check("provider", rec, wantError{status: http.StatusBadGateway, code: gen.ErrorCodeProviderFailed, reason: gen.ErrorReasonUnreachable})
	if strings.Contains(rec.Body.String(), "dial tcp") {
		t.Errorf("the provider's text is in the error message: %s", rec.Body)
	}
	failed := decode[gen.ThumbnailEdit](t, f.editRequest(http.MethodGet, id, "", f.owner))
	if failed.State != gen.ThumbnailEditStateFailed || failed.Error == nil || failed.Error.Reason != gen.ThumbnailEditFailureUnreachable ||
		failed.Error.Message == nil || *failed.Error.Message != "dial tcp" {
		t.Fatalf("failed = %+v", failed)
	}
	f.edits.set(func(e *editFakes) { e.startErr = nil })

	rec = f.runEdit(t, id, map[string][]byte{"prompt": []byte("remove"), "mask": editPNG(4, 4), "reference": editPNG(2, 2)}, f.owner)
	queued := decode[gen.ThumbnailEdit](t, rec)
	if rec.Code != http.StatusAccepted || queued.State != gen.ThumbnailEditStateQueued || queued.ElapsedMs == nil || queued.Error != nil {
		t.Fatalf("run = %d %s", rec.Code, rec.Body)
	}
	check("no result yet", f.editRequest(http.MethodGet, id, "/result", f.owner),
		wantError{status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: reasonThumbnailEditNoResult})
	check("apply without a result", f.editRequest(http.MethodPost, id, "/apply", f.owner),
		wantError{status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: reasonThumbnailEditNoResult})

	f.edits.set(func(e *editFakes) { e.state = domain.ThumbnailEditRunning })
	if got := decode[gen.ThumbnailEdit](t, f.editRequest(http.MethodGet, id, "", f.owner)); got.State != gen.ThumbnailEditStateRunning {
		t.Fatalf("state = %s", got.State)
	}
	rec = f.editRequest(http.MethodPost, id, "/cancel", f.owner)
	if got := decode[gen.ThumbnailEdit](t, rec); rec.Code != http.StatusOK || got.State != gen.ThumbnailEditStateCancelled {
		t.Fatalf("cancel = %d %s", rec.Code, rec.Body)
	}
	if _, cancels := f.edits.counts(); cancels != 1 {
		t.Fatalf("cancels = %d", cancels)
	}
	rec = f.editRequest(http.MethodPost, id, "/cancel", f.owner)
	if got := decode[gen.ThumbnailEdit](t, rec); rec.Code != http.StatusOK || got.State != gen.ThumbnailEditStateCancelled {
		t.Fatalf("second cancel = %d %s", rec.Code, rec.Body)
	}
}

// 成功した実行は結果の URL を持ち、適用は Video を返してセッションを終える。適用の誤りは契約の状態で返す。
func TestApplyThumbnailEdit(t *testing.T) {
	f := newGuestFixture(t, true)
	f.saveConnection(t)
	id := f.succeededEdit(t, "a")
	check := func(label string, rec *httptest.ResponseRecorder, want wantError) {
		t.Helper()
		assertErrorBody(t, label, rec.Code, rec.Body.Bytes(), want)
	}

	result := f.editRequest(http.MethodGet, id, "/result", f.owner)
	if result.Code != http.StatusOK || result.Header().Get("Content-Type") != "image/png" || !bytes.Equal(result.Body.Bytes(), editPNG(4, 4)) {
		t.Fatalf("result = %d %v", result.Code, result.Header())
	}

	f.edits.set(func(e *editFakes) { e.applyErr = domain.ErrThumbnailImageInvalid })
	check("invalid image", f.editRequest(http.MethodPost, id, "/apply", f.owner),
		wantError{status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: reasonThumbnailImageInvalid})
	f.edits.set(func(e *editFakes) { e.applyErr = nil })

	rec := f.editRequest(http.MethodPost, id, "/apply", f.owner)
	video := decode[gen.Video](t, rec)
	if rec.Code != http.StatusOK || video.Id != f.ids["a"] || video.ThumbnailImage == nil || *video.ThumbnailImage != gen.Generated || video.Location == nil {
		t.Fatalf("apply = %d %s", rec.Code, rec.Body)
	}
	check("ended", f.editRequest(http.MethodGet, id, "", f.owner),
		wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: reasonThumbnailEditNotFound})
	check("ended frame", f.editRequest(http.MethodGet, id, "/frame", f.owner),
		wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: reasonThumbnailEditNotFound})

	id = f.succeededEdit(t, "a")
	f.edits.set(func(e *editFakes) { e.applyErr = domain.ErrContentChanged })
	check("content changed", f.editRequest(http.MethodPost, id, "/apply", f.owner),
		wantError{status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: reasonContentChanged})
	check("after content changed", f.editRequest(http.MethodGet, id, "/result", f.owner),
		wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: reasonThumbnailEditNotFound})
}

// 捨てると実行を中止して 204 を返し、終わったセッションも 204 である。
func TestDiscardThumbnailEdit(t *testing.T) {
	f := newGuestFixture(t, true)
	f.saveConnection(t)
	id := f.startedEdit(t, "a").Id
	if rec := f.runEdit(t, id, nil, f.owner); rec.Code != http.StatusAccepted {
		t.Fatalf("run = %d %s", rec.Code, rec.Body)
	}
	for range 2 {
		rec := f.editRequest(http.MethodDelete, id, "", f.owner)
		if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
			t.Fatalf("discard = %d %s", rec.Code, rec.Body)
		}
	}
	if _, cancels := f.edits.counts(); cancels != 1 {
		t.Fatalf("cancels = %d, want 1", cancels)
	}
	if rec := f.editRequest(http.MethodGet, id, "", f.owner); rec.Code != http.StatusNotFound {
		t.Fatalf("after discard = %d", rec.Code)
	}
}

// succeededEdit は動画 name の編集を始めて実行し、成功させてその id を返す。
func (f *guestFixture) succeededEdit(t *testing.T, name string) string {
	t.Helper()
	id := f.startedEdit(t, name).Id
	if rec := f.runEdit(t, id, map[string][]byte{"prompt": []byte("remove")}, f.owner); rec.Code != http.StatusAccepted {
		t.Fatalf("run = %d %s", rec.Code, rec.Body)
	}
	f.edits.set(func(e *editFakes) { e.state = domain.ThumbnailEditSucceeded })
	edit := decode[gen.ThumbnailEdit](t, f.editRequest(http.MethodGet, id, "", f.owner))
	if edit.State != gen.ThumbnailEditStateSucceeded || edit.ResultUrl == nil || *edit.ResultUrl != "/api/thumbnail-edits/"+id+"/result" {
		t.Fatalf("succeeded = %+v", edit)
	}
	return id
}
