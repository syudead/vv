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

// Forward storage events as cmd/mdm subscriptions do.
type publishToEvents struct{ events *Events }

func (p publishToEvents) Publish(events ...domain.Event) {
	for _, event := range events {
		p.events.Handle(event)
	}
}

type activityEnv struct {
	db      *store.DB
	events  *Events
	scans   *app.Scans
	handler http.Handler
}

type finishingScanner struct{}

func (finishingScanner) Scan(context.Context, domain.ActivityDecider) (domain.ScanResult, error) {
	return domain.ScanResult{}, nil
}

func (finishingScanner) ScanScoped(context.Context, []domain.DirtyDirectory) (domain.ScanResult, error) {
	return domain.ScanResult{}, nil
}

func (finishingScanner) Rescan(context.Context, int64, domain.RescanScope, domain.ActivityDecider) (domain.ScanResult, error) {
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

func TestListActivityFiltersFaceRecognitionAndNamesThePerson(t *testing.T) {
	e := newActivityEnv(t)
	e.record(t, failedImport())
	merge := e.record(t, domain.ActivityEntry{
		Kind:    domain.ActivityFaceMerge,
		Subject: domain.ActivitySubject{Kind: domain.ActivitySubjectPerson, Key: "42", Label: "Alice"},
		Status:  domain.ActivityDone, Outcome: domain.ActivityCompleted, Decider: domain.ActivityByRule,
	})

	rec := e.serve(http.MethodGet, "/api/activity?feature=face_recognition", "")
	var page gen.ActivityPage
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if len(page.Items) != 1 || page.Items[0].Id != merge || page.Features.FaceRecognition != 1 || page.Features.Import != 1 {
		t.Fatalf("page = %+v", page)
	}
	subject := page.Items[0].Subject
	if subject.Kind != gen.ActivitySubjectKindPerson || len(subject.Items) != 1 || subject.Items[0].Label != "Alice" ||
		subject.Items[0].PersonId == nil || *subject.Items[0].PersonId != 42 || subject.Items[0].VideoId != nil {
		t.Errorf("subject = %+v", subject)
	}
}

func TestListActivityRejectsInvalidFilters(t *testing.T) {
	e := newActivityEnv(t)
	for _, query := range []string{
		"status=failed", "feature=undo", "decider=model", "since=yesterday", "limit=0", "limit=101", "q=",
		"q=" + strings.Repeat("a", 201), strings.Repeat("feature=import&", 10) + "feature=scan",
	} {
		rec := e.serve(http.MethodGet, "/api/activity?"+query, "")
		assertErrorBody(t, query, rec.Code, rec.Body.Bytes(), wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest})
	}
	rec := e.serve(http.MethodGet, "/api/activity?cursor=broken", "")
	assertErrorBody(t, "cursor", rec.Code, rec.Body.Bytes(), wantError{
		status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: gen.ErrorReasonInvalidCursor,
	})
}

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

	pipeline := failedImport()
	pipeline.Kind, pipeline.Stage, pipeline.Detail = domain.ActivityPipeline, "", ""
	pipelinePath := "/api/activity/" + itoa(e.record(t, pipeline))
	rec = e.serve(http.MethodPost, pipelinePath+"/retry", "")
	assertErrorBody(t, "retry a pipeline", rec.Code, rec.Body.Bytes(), wantError{status: http.StatusConflict, code: gen.ErrorCodeActivityNotRetryable})
	rec = e.serve(http.MethodPost, pipelinePath+"/undo", "{}")
	assertErrorBody(t, "undo a pipeline", rec.Code, rec.Body.Bytes(), wantError{status: http.StatusConflict, code: gen.ErrorCodeActivityNotUndoable})

	missing := "/api/activity/" + itoa(failed+10)
	rec = e.serve(http.MethodPost, missing+"/retry", "")
	assertErrorBody(t, "retry missing", rec.Code, rec.Body.Bytes(), wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound})
	rec = e.serve(http.MethodPost, missing+"/undo", "{}")
	assertErrorBody(t, "undo missing", rec.Code, rec.Body.Bytes(), wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound})
}

// Both features start off, so retrying their failed entry answers 409 with the feature's reason.
func TestRetryWhileTheFeatureIsOffAnswersItsReason(t *testing.T) {
	for _, tc := range []struct {
		kind    domain.ActivityKind
		retrier func(db *store.DB) app.ActivityRetrier
		reason  gen.ErrorReason
	}{
		{domain.ActivityAutoTag, func(db *store.DB) app.ActivityRetrier {
			return app.NewAutoTagger(app.AutoTaggerOptions{Queue: db.AutoTags(), Settings: db.Settings(), Undo: db.Activity()})
		}, gen.ErrorReasonAutoTaggingOff},
		{domain.ActivityFaceAnalysis, func(db *store.DB) app.ActivityRetrier {
			return app.NewFaceRecognizer(app.FaceRecognizerOptions{Queue: db.Faces(), Settings: db.Settings()})
		}, gen.ErrorReasonFaceRecognitionOff},
	} {
		t.Run(string(tc.kind), func(t *testing.T) {
			e := newActivityEnv(t)
			activity := app.NewActivity(app.ActivityOptions{
				Store:    e.db.Activity(),
				Retriers: map[domain.ActivityKind]app.ActivityRetrier{tc.kind: tc.retrier(e.db)},
			})
			handler := newTestServer(t, Options{Activity: activity, Events: e.events, Scans: &lockedScans{current: domain.Scan{ID: 1}}})
			failed := failedImport()
			failed.Kind, failed.Stage = tc.kind, ""
			id := e.record(t, failed)

			req := httptest.NewRequest(http.MethodPost, "/api/activity/"+itoa(id)+"/retry", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			assertErrorBody(t, "retry while off", rec.Code, rec.Body.Bytes(), wantError{
				status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: tc.reason,
			})
		})
	}
}

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

func TestActivityRoutesRefuseGuests(t *testing.T) {
	env := newAuthEnvWith(t, t.TempDir(), func(db *store.DB) Options {
		return Options{Activity: app.NewActivity(app.ActivityOptions{Store: db.Activity()})}
	})
	env.setup()
	for _, req := range []authRequest{
		{method: http.MethodGet, target: "/api/activity"},
		{method: http.MethodGet, target: "/api/activity/1"},
		{method: http.MethodPost, target: "/api/activity/1/retry"},
		{method: http.MethodPost, target: "/api/activity/1/undo", body: `{"tagId":1}`},
	} {
		assertUnauthenticated(t, req.method+" "+req.target, env.serve(req))
	}
}

func TestMemberSummaryKeepsFaceLinks(t *testing.T) {
	member := domain.ActivityMember{ActivityEntry: domain.ActivityEntry{
		ID: 3, Kind: domain.ActivityFaceAnalysis, Feature: domain.ActivityFeatureFaceRecognition,
		Subject: domain.ActivitySubject{Kind: domain.ActivitySubjectVideo, Key: "key-a", Label: "a.mp4"},
		Status:  domain.ActivityDone, Outcome: domain.ActivityCompleted, Decider: domain.ActivityByRule,
		FaceLinks: []domain.ActivityFaceLink{
			{AppearanceID: 7, Person: &domain.CreditRef{ID: 1, Provisional: true}, CreatedPerson: true},
		},
	}}

	summary, err := toAPIActivityMemberSummary(member)
	if err != nil {
		t.Fatal(err)
	}
	if summary.FaceLinks == nil || len(*summary.FaceLinks) != 1 {
		t.Fatalf("faceLinks = %+v", summary.FaceLinks)
	}
	link := (*summary.FaceLinks)[0]
	if link.AppearanceId != 7 || link.Person == nil || *link.Person != (gen.CreditRef{Id: 1, Provisional: true}) ||
		!link.CreatedPerson || link.Undone || !summary.Undoable {
		t.Errorf("link = %+v, undoable = %v", link, summary.Undoable)
	}
}

func itoa(id int64) string {
	data, _ := json.Marshal(id)
	return string(data)
}
