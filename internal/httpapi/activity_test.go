package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/store"
)

// アクティビティの記録の画面 API。

// publishToEvents は保存層の発行を画面への知らせに渡す（cmd/mdm の購読の代わり）。
type publishToEvents struct{ events *Events }

func (p publishToEvents) Publish(events ...domain.Event) {
	for _, event := range events {
		p.events.Handle(event)
	}
}

// activityEnv は本物の保存層と app.Activity で組んだ経路である。やり直しは cmd/mdm と同じく、import を
// 保存層へ、scan を app.Scans へ振り分ける。走査は登録フォルダを読まず、すぐに終わる（finishingScanner）。
type activityEnv struct {
	db      *store.DB
	events  *Events
	scans   *app.Scans
	handler http.Handler
}

// finishingScanner は何も読まずに成功で終わる走査である。
type finishingScanner struct{}

func (finishingScanner) Scan(context.Context, domain.ActivityDecider) (domain.ScanResult, error) {
	return domain.ScanResult{}, nil
}

func (finishingScanner) ScanScoped(context.Context, []domain.DirtyDirectory) (domain.ScanResult, error) {
	return domain.ScanResult{}, nil
}

func newActivityEnv(t *testing.T) *activityEnv {
	t.Helper()
	db, err := store.OpenMigratedForTest(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	events := NewEvents()
	db.PublishTo(publishToEvents{events: events})
	ctx, cancel := context.WithCancel(context.Background())
	scans := app.NewScans(app.ScansOptions{
		Store: db.Scans(), Jobs: db.Ingest(), Context: ctx,
		NewScanner: func(app.ScanReporter) app.Scanner { return finishingScanner{} },
	})
	t.Cleanup(func() {
		cancel()
		scans.Wait()
	})
	activity := app.NewActivity(app.ActivityOptions{
		Store: db.Activity(),
		Retriers: map[domain.ActivityKind]app.ActivityRetrier{
			domain.ActivityImport: app.NewImportRetrier(db.Ingest()),
			domain.ActivityScan:   scans,
		},
	})
	handler := newTestServer(t, Options{Activity: activity, Events: events, Scans: &lockedScans{current: domain.Scan{ID: 1}}})
	return &activityEnv{db: db, events: events, scans: scans, handler: handler}
}

func (e *activityEnv) record(t *testing.T, entry domain.ActivityEntry) int64 {
	t.Helper()
	id, err := store.RecordActivityForTest(context.Background(), e.db, entry, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (e *activityEnv) serve(method, target, body string) *httptest.ResponseRecorder {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	return rec
}

func failedImport() domain.ActivityEntry {
	return domain.ActivityEntry{
		Kind:    domain.ActivityImport,
		Subject: domain.ActivitySubject{Kind: domain.ActivitySubjectVideo, Key: "key-a", Label: "a.mp4"},
		Status:  domain.ActivityAttention, Reason: domain.ActivityFailed, Stage: domain.JobProbe,
		Detail: "cannot read the file", Decider: domain.ActivityByRule,
	}
}

// 一覧は契約の形で、件数と機能ごとの数を添える。対象の動画がライブラリに無ければ videoId は入らない。
func TestListActivityReturnsThePage(t *testing.T) {
	e := newActivityEnv(t)
	probability := 0.9
	failed := e.record(t, failedImport())
	pair := e.record(t, domain.ActivityEntry{
		Kind: domain.ActivityVersionJudgement,
		Subject: domain.ActivitySubject{
			Kind: domain.ActivitySubjectPair, Key: "key-a", KeyB: "key-b", Label: "a.mp4", LabelB: "b.mp4",
		},
		Status: domain.ActivityDone, Outcome: domain.ActivitySame, Decider: domain.ActivityByAI, Probability: &probability,
	})

	rec := e.serve(http.MethodGet, "/api/activity?limit=1", "")
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != cacheNoStore {
		t.Fatalf("status = %d, Cache-Control = %q: %s", rec.Code, rec.Header().Get("Cache-Control"), rec.Body)
	}
	var page gen.ActivityPage
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Id != pair || page.NextCursor == nil {
		t.Fatalf("1 ページ目 = %+v", page)
	}
	if page.Counts != (gen.ActivityCounts{Attention: 1, Done: 1}) || page.Features.Import != 1 || page.Features.DuplicateCheck != 1 {
		t.Errorf("counts = %+v, features = %+v", page.Counts, page.Features)
	}
	judgement := page.Items[0]
	if judgement.Subject.Kind != gen.ActivitySubjectKindPair || len(judgement.Subject.Items) != 2 ||
		judgement.Subject.Items[1].Label != "b.mp4" || judgement.Subject.Items[0].VideoId != nil ||
		!judgement.Undoable || judgement.RetryRun || judgement.Probability == nil || *judgement.Probability != 0.9 {
		t.Errorf("組の記録 = %+v", judgement)
	}

	rec = e.serve(http.MethodGet, "/api/activity?cursor="+*page.NextCursor+"&status=attention&feature=import&feature=scan", "")
	page = gen.ActivityPage{}
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("2 ページ目 = %d: %s", rec.Code, rec.Body)
	}
	if len(page.Items) != 1 || page.Items[0].Id != failed || page.NextCursor != nil {
		t.Fatalf("2 ページ目 = %+v", page)
	}
	item := page.Items[0]
	if item.Stage == nil || *item.Stage != gen.ActivityStageProbe || item.Detail == nil || !item.RetryRun || item.Undoable ||
		item.Reason == nil || *item.Reason != gen.ActivityReasonFailed {
		t.Errorf("失敗した import = %+v", item)
	}
}

// 知らない値、読めない cursor と since、範囲の外の q と limit、10 個以上の feature は 400 invalid_request。
func TestListActivityRejectsInvalidFilters(t *testing.T) {
	e := newActivityEnv(t)
	for _, query := range []string{
		"status=failed", "feature=undo", "decider=model", "since=yesterday", "limit=0", "limit=101", "q=",
		"q=" + strings.Repeat("a", 201), strings.Repeat("feature=import&", 9) + "feature=scan",
	} {
		rec := e.serve(http.MethodGet, "/api/activity?"+query, "")
		assertErrorBody(t, query, rec.Code, rec.Body.Bytes(), wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest})
	}
	rec := e.serve(http.MethodGet, "/api/activity?cursor=broken", "")
	assertErrorBody(t, "cursor", rec.Code, rec.Body.Bytes(), wantError{
		status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: gen.ErrorReasonInvalidCursor,
	})
}

// 対象の動画がもう無い import のやり直しと、振り分け先の無い取り消しは 409 を返す。無い記録は 404。
func TestRetryAndUndoAnswerConflictForEveryKind(t *testing.T) {
	e := newActivityEnv(t)
	failed := e.record(t, failedImport())
	path := "/api/activity/" + itoa(failed)

	rec := e.serve(http.MethodPost, path+"/retry", "")
	assertErrorBody(t, "retry", rec.Code, rec.Body.Bytes(), wantError{status: http.StatusConflict, code: gen.ErrorCodeActivityNotRetryable})
	rec = e.serve(http.MethodPost, path+"/undo", "{}")
	assertErrorBody(t, "undo", rec.Code, rec.Body.Bytes(), wantError{status: http.StatusConflict, code: gen.ErrorCodeActivityNotUndoable})
	rec = e.serve(http.MethodPost, path+"/undo", `{"tagId":3}`)
	assertErrorBody(t, "undo with a tag", rec.Code, rec.Body.Bytes(), wantError{status: http.StatusConflict, code: gen.ErrorCodeActivityNotUndoable})
	rec = e.serve(http.MethodPost, path+"/undo", "")
	assertErrorBody(t, "undo without a body", rec.Code, rec.Body.Bytes(), wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest})
	rec = e.serve(http.MethodPost, path+"/undo", `{"tag":3}`)
	assertErrorBody(t, "undo with an unknown field", rec.Code, rec.Body.Bytes(), wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest})

	missing := "/api/activity/" + itoa(failed+10)
	rec = e.serve(http.MethodPost, missing+"/retry", "")
	assertErrorBody(t, "retry missing", rec.Code, rec.Body.Bytes(), wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound})
	rec = e.serve(http.MethodPost, missing+"/undo", "{}")
	assertErrorBody(t, "undo missing", rec.Code, rec.Body.Bytes(), wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound})
}

// 自動タグ付けが切のあいだ、失敗した auto_tag の記録のやり直しは 409 auto_tagging_off を返す。
func TestRetryAutoTagWhileOffAnswersAutoTaggingOff(t *testing.T) {
	e := newActivityEnv(t)
	tagger := app.NewAutoTagger(app.AutoTaggerOptions{Queue: e.db.AutoTags(), Settings: e.db.Settings(), Undo: e.db.Activity()})
	activity := app.NewActivity(app.ActivityOptions{
		Store:    e.db.Activity(),
		Retriers: map[domain.ActivityKind]app.ActivityRetrier{domain.ActivityAutoTag: tagger},
		Undoers:  map[domain.ActivityKind]app.ActivityUndoer{domain.ActivityAutoTag: tagger},
	})
	handler := newTestServer(t, Options{Activity: activity, Events: e.events, Scans: &lockedScans{current: domain.Scan{ID: 1}}})
	failed := failedImport()
	failed.Kind, failed.Stage = domain.ActivityAutoTag, ""
	id := e.record(t, failed)

	req := httptest.NewRequest(http.MethodPost, "/api/activity/"+itoa(id)+"/retry", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assertErrorBody(t, "retry while off", rec.Code, rec.Body.Bytes(), wantError{
		status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: gen.ErrorReasonAutoTaggingOff,
	})
}

// 受け入れ: 記録を書いた後に、所有者の stream が activity を受ける。続けての書き込みは 1 つにまとまる。
func TestStreamEventsSendsActivityToOwners(t *testing.T) {
	e := newActivityEnv(t)
	stream, _ := openEvents(t, e.handler)
	if first := nextEvent(t, stream); first.name != "scan" {
		t.Fatalf("最初のイベント = %q, want scan", first.name)
	}

	e.record(t, failedImport())
	event := nextEvent(t, stream)
	if event.name != "activity" || event.data != `{}` {
		t.Fatalf("イベント = %+v, want activity {}", event)
	}
	expectNoEvent(t, stream)
}

// ゲストには activity を送らない。
func TestStreamEventsSendsNoActivityToGuests(t *testing.T) {
	events := NewEvents()
	handler := newTestServer(t, Options{
		Scans: &lockedScans{current: domain.Scan{ID: 1}}, Events: events, Auth: guestEventsAuth{},
	})
	stream, _ := openEvents(t, handler)
	expectNoEvent(t, stream)
	events.Handle(domain.ActivityChanged{})
	expectNoEvent(t, stream)
}

// 受け入れ: ゲストは 3 つの操作すべてで 401 unauthenticated を受ける（accessRoutes に無い）。
func TestActivityRoutesRefuseGuests(t *testing.T) {
	env := newAuthEnvWith(t, t.TempDir(), func(db *store.DB) Options {
		return Options{Activity: app.NewActivity(app.ActivityOptions{Store: db.Activity()})}
	})
	env.setup()
	for _, req := range []authRequest{
		{method: http.MethodGet, target: "/api/activity"},
		{method: http.MethodPost, target: "/api/activity/1/retry"},
		{method: http.MethodPost, target: "/api/activity/1/undo", body: `{"tagId":1}`},
	} {
		assertUnauthenticated(t, req.method+" "+req.target, env.serve(req))
	}
}

func itoa(id int64) string {
	data, _ := json.Marshal(id)
	return string(data)
}
