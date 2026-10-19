package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// fakeActivity answers the activity reads from fixed values and records the requested detail and undo targets.
type fakeActivity struct {
	page      domain.ActivityPage
	details   map[int64]domain.ActivityDetail
	requested []int64
	undone    []domain.ActivityUndoTarget
}

func (f *fakeActivity) ListActivity(
	context.Context, domain.Audience, domain.ActivityQuery, string, int,
) (domain.ActivityPage, error) {
	return f.page, nil
}

func (f *fakeActivity) ActivityDetail(_ context.Context, id int64) (domain.ActivityDetail, error) {
	f.requested = append(f.requested, id)
	detail, ok := f.details[id]
	if !ok {
		return domain.ActivityDetail{}, domain.ErrNotFound
	}
	return detail, nil
}

func (f *fakeActivity) RetryActivity(context.Context, int64) error {
	return domain.ErrActivityNotRetryable
}

func (f *fakeActivity) UndoActivity(_ context.Context, _ int64, target domain.ActivityUndoTarget) (domain.ActivityEntry, error) {
	f.undone = append(f.undone, target)
	return domain.ActivityEntry{}, domain.ErrActivityNotUndoable
}

var activityTime = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

func videoEntry(id int64, kind domain.ActivityKind, status domain.ActivityStatus) domain.ActivityEntry {
	return domain.ActivityEntry{
		ID: id, Kind: kind, Feature: domain.ActivityFeatureImport, Status: status, Decider: domain.ActivityByRule,
		Subject:  domain.ActivitySubject{Kind: domain.ActivitySubjectVideo, Key: "key-a", Label: "a.mp4"},
		VideoIDs: [2]int64{7}, CreatedAt: activityTime, UpdatedAt: activityTime,
	}
}

func serveActivity(t *testing.T, activity *fakeActivity, target string) *httptest.ResponseRecorder {
	t.Helper()
	handler := newTestServer(t, Options{Activity: activity, Scans: &lockedScans{current: domain.Scan{ID: 1}}})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func TestGetActivityReturnsAPipelineWithItsMembers(t *testing.T) {
	failed := videoEntry(1, domain.ActivityImport, domain.ActivityDone)
	failed.Detail = "ffprobe failed"
	pipeline := videoEntry(3, domain.ActivityPipeline, domain.ActivityRunning)
	pipeline.Members = []domain.ActivityMember{
		{ActivityEntry: failed, Replaced: true},
		{ActivityEntry: videoEntry(2, domain.ActivityImport, domain.ActivityRunning)},
	}
	activity := &fakeActivity{details: map[int64]domain.ActivityDetail{3: {ActivityEntry: pipeline}}}

	rec := serveActivity(t, activity, "/api/activity/3")
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != cacheNoStore {
		t.Fatalf("GET = %d (%q): %s", rec.Code, rec.Header().Get("Cache-Control"), rec.Body)
	}
	var detail gen.ActivityDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(activity.requested, []int64{3}) || detail.Id != 3 || detail.Kind != gen.ActivityKindPipeline {
		t.Fatalf("requested %v, detail = %+v", activity.requested, detail)
	}
	if detail.Members == nil || len(*detail.Members) != 2 {
		t.Fatalf("members = %+v, want two", detail.Members)
	}
	first, second := (*detail.Members)[0], (*detail.Members)[1]
	if first.Id != 1 || first.Replaced == nil || !*first.Replaced || first.Detail == nil || *first.Detail != "ffprobe failed" {
		t.Errorf("first member = %+v, want replaced with its full detail", first)
	}
	if second.Id != 2 || second.Replaced == nil || *second.Replaced {
		t.Errorf("second member = %+v, want not replaced", second)
	}
	if detail.Pipelines != nil || detail.Stages != nil || detail.StagesRecorded != nil {
		t.Errorf("pipeline detail = %+v, want no pipelines or stages", detail)
	}
}

func TestGetActivityReturnsAMembersPipelinesAndStages(t *testing.T) {
	imported := videoEntry(1, domain.ActivityImport, domain.ActivityAttention)
	imported.Stages = []domain.ActivityStage{
		{Stage: domain.JobProbe, State: domain.ActivityStageFailed, Detail: "ffprobe failed", UpdatedAt: activityTime},
		{Stage: domain.JobThumbnail, State: domain.ActivityStageDone, UpdatedAt: activityTime},
	}
	older := videoEntry(2, domain.ActivityImport, domain.ActivityDone)
	activity := &fakeActivity{details: map[int64]domain.ActivityDetail{
		1: {ActivityEntry: imported, Pipelines: []domain.ActivityEntry{videoEntry(5, domain.ActivityPipeline, domain.ActivityAttention)}},
		2: {ActivityEntry: older},
	}}

	var detail gen.ActivityDetail
	if err := json.Unmarshal(serveActivity(t, activity, "/api/activity/1").Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	videoID := int64(7)
	wantPipelines := []gen.ActivityPipelineRef{{
		Id: 5, Status: gen.ActivityStatusAttention, CreatedAt: activityTime,
		Subject: gen.ActivitySubject{Kind: gen.ActivitySubjectKindVideo, Items: []gen.ActivitySubjectItem{{Label: "a.mp4", VideoId: &videoID}}},
	}}
	if detail.Pipelines == nil || !pipelineRefsEqual(*detail.Pipelines, wantPipelines) {
		t.Errorf("pipelines = %+v, want %+v", detail.Pipelines, wantPipelines)
	}
	if detail.StagesRecorded == nil || !*detail.StagesRecorded || detail.Stages == nil || len(*detail.Stages) != 2 {
		t.Fatalf("stages = %+v (recorded %v), want two", detail.Stages, detail.StagesRecorded)
	}
	probe := (*detail.Stages)[0]
	if probe.Stage != gen.ActivityStageProbe || probe.State != gen.ActivityStageStateFailed || probe.Detail == nil ||
		*probe.Detail != "ffprobe failed" || !probe.UpdatedAt.Equal(activityTime) || (*detail.Stages)[1].Detail != nil {
		t.Errorf("stages = %+v, want the failed probe with its message", *detail.Stages)
	}
	if detail.Members != nil {
		t.Errorf("members of an import = %+v, want none", detail.Members)
	}

	detail = gen.ActivityDetail{}
	if err := json.Unmarshal(serveActivity(t, activity, "/api/activity/2").Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.StagesRecorded == nil || *detail.StagesRecorded || detail.Stages != nil {
		t.Errorf("an import without stage rows = recorded %v, stages %+v; want not recorded", detail.StagesRecorded, detail.Stages)
	}
}

func pipelineRefsEqual(a, b []gen.ActivityPipelineRef) bool {
	encodedA, _ := json.Marshal(a)
	encodedB, _ := json.Marshal(b)
	return string(encodedA) == string(encodedB)
}

func TestGetActivityAnswersNotFoundForAnUnknownID(t *testing.T) {
	rec := serveActivity(t, &fakeActivity{}, "/api/activity/9")
	assertErrorBody(t, "unknown id", rec.Code, rec.Body.Bytes(), wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound})
}

func TestListActivitySummarizesPipelineMembers(t *testing.T) {
	matched := true
	tags := videoEntry(2, domain.ActivityAutoTag, domain.ActivityDone)
	tags.Decider, tags.Detail = domain.ActivityByAI, "ignored"
	tags.Tags = []domain.ActivityEntryTag{{TagID: 4, Name: "beach"}, {TagID: 5, Name: "sea"}}
	imported := videoEntry(1, domain.ActivityImport, domain.ActivityDone)
	imported.Stages = []domain.ActivityStage{{Stage: domain.JobProbe, State: domain.ActivityStageDone, Detail: "kept out"}}
	pipeline := videoEntry(3, domain.ActivityPipeline, domain.ActivityDone)
	pipeline.Members = []domain.ActivityMember{
		{ActivityEntry: imported, Replaced: true},
		{ActivityEntry: tags, Matched: &matched},
	}
	activity := &fakeActivity{page: domain.ActivityPage{Items: []domain.ActivityEntry{pipeline}}}

	rec := serveActivity(t, activity, "/api/activity")
	var raw struct {
		Items []struct {
			Members []map[string]json.RawMessage `json:"members"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil || len(raw.Items) != 1 || len(raw.Items[0].Members) != 2 {
		t.Fatalf("GET /api/activity = %d: %s (%v)", rec.Code, rec.Body, err)
	}
	for _, member := range raw.Items[0].Members {
		if _, ok := member["detail"]; ok {
			t.Errorf("member %s carries detail", member["id"])
		}
		if _, ok := member["tags"]; ok {
			t.Errorf("member %s carries tags", member["id"])
		}
	}
	var page gen.ActivityPage
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	members := *page.Items[0].Members
	if first := members[0]; !first.Replaced || first.Matched != nil || first.TagCount != nil || first.Stages == nil ||
		!slices.Equal(*first.Stages, []gen.ActivityStageSummary{{Stage: gen.ActivityStageProbe, State: gen.ActivityStageStateDone}}) {
		t.Errorf("import member = %+v, want replaced with its stages", first)
	}
	if second := members[1]; second.Replaced || second.Matched == nil || !*second.Matched || second.TagCount == nil ||
		*second.TagCount != 2 || !second.Undoable || second.Stages != nil {
		t.Errorf("auto_tag member = %+v, want matched, two tags and undoable", second)
	}
}

func TestFaceAnalysisEntryListsItsLinksAndUndoTakesAnAppearance(t *testing.T) {
	entry := videoEntry(4, domain.ActivityFaceAnalysis, domain.ActivityDone)
	entry.Feature = domain.ActivityFeatureFaceRecognition
	entry.FaceLinks = []domain.ActivityFaceLink{
		{AppearanceID: 11, Person: &domain.CreditRef{ID: 3, Provisional: true}, CreatedPerson: true},
		{AppearanceID: 12, UndoneBy: 9},
	}
	activity := &fakeActivity{details: map[int64]domain.ActivityDetail{4: {ActivityEntry: entry}}}

	var detail gen.ActivityDetail
	if err := json.Unmarshal(serveActivity(t, activity, "/api/activity/4").Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	// A link without a person is written as person id 0.
	type link struct {
		appearance, person   int64
		provisional, created bool
		undone               bool
	}
	var got []link
	for _, l := range derefOr(detail.FaceLinks) {
		row := link{appearance: l.AppearanceId, created: l.CreatedPerson, undone: l.Undone}
		if l.Person != nil {
			row.person, row.provisional = l.Person.Id, l.Person.Provisional
		}
		got = append(got, row)
	}
	if want := []link{{11, 3, true, true, false}, {12, 0, false, false, true}}; !slices.Equal(got, want) {
		t.Errorf("faceLinks = %+v, want %+v", got, want)
	}

	handler := newTestServer(t, Options{Activity: activity, Scans: &lockedScans{current: domain.Scan{ID: 1}}})
	for _, body := range []string{`{"appearanceId":11}`, `{}`} {
		req := httptest.NewRequest(http.MethodPost, "/api/activity/4/undo", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}
	if want := []domain.ActivityUndoTarget{{AppearanceID: 11}, {}}; !slices.Equal(activity.undone, want) {
		t.Errorf("undo targets = %+v, want %+v", activity.undone, want)
	}
}

func derefOr[T any](p *[]T) []T {
	if p == nil {
		return nil
	}
	return *p
}
