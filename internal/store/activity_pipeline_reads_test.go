package store

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// writePipelineMember writes an entry that opens its own pipeline when joins is nil, or joins the pipelines of the
// entries in joins, as the recording roles do.
func writePipelineMember(t *testing.T, db *DB, entry domain.ActivityEntry, joins []int64, now time.Time) int64 {
	t.Helper()
	ctx := context.Background()
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	c := &changes{}
	if joins == nil {
		if entry.ID, err = recordActivity(ctx, tx, c, entry, now); err == nil {
			err = openImportPipeline(ctx, tx, c, entry, 0, now)
		}
	} else {
		entry.ID, err = recordContinuedActivity(ctx, tx, c, entry, joins, now)
	}
	if err == nil {
		err = db.commit(ctx, tx, c)
	}
	if err != nil {
		t.Fatalf("write the pipeline member: %v", err)
	}
	return entry.ID
}

type pipelineLog struct {
	importA, tagA, importB, pair, standalone int64
	pipelineA, pipelineB                     int64
}

// seedPipelineLog writes two import pipelines that share a duplicate judgement, a failed auto-tag run in the first,
// and a standalone owner entry.
func seedPipelineLog(t *testing.T, db *DB) pipelineLog {
	t.Helper()
	clock := newActivityClock()
	var log pipelineLog
	log.importA = writePipelineMember(t, db,
		videoActivity(domain.ActivityImport, "key-a", "Holiday.mp4", activityDone, domain.ActivityByRule), nil, clock.next())
	log.tagA = writePipelineMember(t, db,
		videoActivity(domain.ActivityAutoTag, "key-a", "Holiday.mp4", activityFailed, domain.ActivityByAI),
		[]int64{log.importA}, clock.next())
	log.importB = writePipelineMember(t, db,
		videoActivity(domain.ActivityImport, "key-b", "Sunset.mp4", activityDone, domain.ActivityByRule), nil, clock.next())
	same := 0.9
	log.pair = writePipelineMember(t, db, domain.ActivityEntry{
		Kind: domain.ActivityVersionJudgement,
		Subject: domain.ActivitySubject{
			Kind: domain.ActivitySubjectPair, Key: "key-a", KeyB: "key-b", Label: "Holiday.mp4", LabelB: "Sunset.mp4",
		},
		Status: domain.ActivityDone, Outcome: domain.ActivitySame, Decider: domain.ActivityByAI, Probability: &same,
	}, []int64{log.importA, log.importB}, clock.next())
	log.standalone = writeActivity(t, db,
		videoActivity(domain.ActivityTagAttach, "key-c", "Garden.mp4", activityDone, domain.ActivityByOwner), clock.next())
	log.pipelineA = onlyPipeline(t, db, log.importA).ID
	log.pipelineB = onlyPipeline(t, db, log.importB).ID
	return log
}

func pipelineMemberIDs(entry domain.ActivityEntry) []int64 {
	ids := make([]int64, 0, len(entry.Members))
	for _, member := range entry.Members {
		ids = append(ids, member.ID)
	}
	return ids
}

// matchedMembers returns the matched member ids of every listed pipeline.
func matchedMembers(page domain.ActivityPage) map[int64][]int64 {
	out := map[int64][]int64{}
	for _, entry := range page.Items {
		for _, member := range entry.Members {
			if member.Matched != nil && *member.Matched {
				out[entry.ID] = append(out[entry.ID], member.ID)
			}
		}
	}
	return out
}

func TestListActivityListsTopLevelEntriesWithTheirMembers(t *testing.T) {
	db, _ := jobsFixture(t)
	log := seedPipelineLog(t, db)

	page := listActivity(t, db, domain.ActivityQuery{}, "", 50)
	if want := []int64{log.standalone, log.pipelineB, log.pipelineA}; !slices.Equal(activityIDs(page), want) {
		t.Fatalf("items = %v, want %v: members are never listed on their own", activityIDs(page), want)
	}
	if got, want := pipelineMemberIDs(page.Items[1]), []int64{log.importB, log.pair}; !slices.Equal(got, want) {
		t.Errorf("members of pipeline B = %v, want %v", got, want)
	}
	if got, want := pipelineMemberIDs(page.Items[2]), []int64{log.importA, log.tagA, log.pair}; !slices.Equal(got, want) {
		t.Errorf("members of pipeline A = %v, want %v: the pair is under both pipelines", got, want)
	}
	if marked := matchedMembers(page); len(marked) != 0 {
		t.Errorf("matched without a filter = %v, want no mark", marked)
	}
	tag := page.Items[2].Members[1]
	if tag.Kind != domain.ActivityAutoTag || !tag.RetryOffered() || tag.VideoIDs[0] == 0 {
		t.Errorf("auto_tag member = %+v, want the failed run offering retry with its video", tag)
	}
	if page.Counts != (domain.ActivityCounts{Attention: 1, Done: 2}) {
		t.Errorf("counts = %+v, want one per listed entry", page.Counts)
	}
}

func TestListActivityFiltersMatchPipelinesThroughTheirMembers(t *testing.T) {
	db, _ := jobsFixture(t)
	log := seedPipelineLog(t, db)
	features := func(values ...domain.ActivityFeature) []domain.ActivityFeature { return values }

	for _, tc := range []struct {
		name    string
		query   domain.ActivityQuery
		items   []int64
		matched map[int64][]int64
	}{
		{
			name: "feature", query: domain.ActivityQuery{Features: features(domain.ActivityFeatureAutoTagging)},
			items: []int64{log.pipelineA}, matched: map[int64][]int64{log.pipelineA: {log.tagA}},
		},
		{
			name: "decider", query: domain.ActivityQuery{Decider: domain.ActivityByAI},
			items:   []int64{log.pipelineB, log.pipelineA},
			matched: map[int64][]int64{log.pipelineB: {log.pair}, log.pipelineA: {log.tagA, log.pair}},
		},
		{
			name: "search", query: domain.ActivityQuery{Search: "SUNSET"},
			items:   []int64{log.pipelineB, log.pipelineA},
			matched: map[int64][]int64{log.pipelineB: {log.importB, log.pair}, log.pipelineA: {log.pair}},
		},
		{
			name: "status only", query: domain.ActivityQuery{Status: domain.ActivityAttention},
			items: []int64{log.pipelineA}, matched: map[int64][]int64{log.pipelineA: {log.tagA}},
		},
		{
			// Status selects the pipeline by its roll-up but is not applied to the members it marks.
			name: "status and feature",
			query: domain.ActivityQuery{
				Status: domain.ActivityAttention, Features: features(domain.ActivityFeatureDuplicateCheck),
			},
			items: []int64{log.pipelineA}, matched: map[int64][]int64{log.pipelineA: {log.pair}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page := listActivity(t, db, tc.query, "", 50)
			if !slices.Equal(activityIDs(page), tc.items) {
				t.Fatalf("items = %v, want %v", activityIDs(page), tc.items)
			}
			if got := matchedMembers(page); !mapsOfSlicesEqual(got, tc.matched) {
				t.Errorf("matched = %v, want %v", got, tc.matched)
			}
		})
	}
}

func mapsOfSlicesEqual(a, b map[int64][]int64) bool {
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		if !slices.Equal(value, b[key]) {
			return false
		}
	}
	return true
}

func TestListActivityCountsTheListedEntries(t *testing.T) {
	db, _ := jobsFixture(t)
	seedPipelineLog(t, db)

	for _, query := range []domain.ActivityQuery{{}, {Decider: domain.ActivityByAI}, {Search: "sunset"}} {
		page := listActivity(t, db, query, "", 50)
		if total := page.Counts.Running + page.Counts.Attention + page.Counts.Done; total != len(page.Items) {
			t.Errorf("%+v: counts %+v sum to %d, want %d listed entries", query, page.Counts, total, len(page.Items))
		}
		for _, feature := range domain.ActivityFeatures {
			only := query
			only.Features = []domain.ActivityFeature{feature}
			if listed := len(listActivity(t, db, only, "", 50).Items); page.Features[feature] != listed {
				t.Errorf("%+v: features[%s] = %d, want %d listed with that feature", query, feature,
					page.Features[feature], listed)
			}
		}
	}
}

func TestListActivityCursorIsStableWhileMembersJoin(t *testing.T) {
	db, _ := jobsFixture(t)
	log := seedPipelineLog(t, db)

	first := listActivity(t, db, domain.ActivityQuery{}, "", 2)
	if !slices.Equal(activityIDs(first), []int64{log.standalone, log.pipelineB}) || first.NextCursor == "" {
		t.Fatalf("page 1 = %v (cursor %q)", activityIDs(first), first.NextCursor)
	}
	joined := writePipelineMember(t, db,
		videoActivity(domain.ActivityAutoTag, "key-a", "Holiday.mp4", activityWaiting, domain.ActivityByAI),
		[]int64{log.importA}, time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC))
	second := listActivity(t, db, domain.ActivityQuery{}, first.NextCursor, 2)
	if !slices.Equal(activityIDs(second), []int64{log.pipelineA}) || second.NextCursor != "" {
		t.Fatalf("page 2 = %v (cursor %q), want pipeline A only", activityIDs(second), second.NextCursor)
	}
	if got := pipelineMemberIDs(second.Items[0]); !slices.Contains(got, joined) {
		t.Errorf("members of pipeline A = %v, want the joined entry %d", got, joined)
	}
}

func activityDetail(t *testing.T, db *DB, id int64) domain.ActivityDetail {
	t.Helper()
	detail, err := db.Activity().ActivityDetail(context.Background(), id)
	if err != nil {
		t.Fatalf("ActivityDetail(%d): %v", id, err)
	}
	return detail
}

func TestActivityDetailLinksPipelinesAndMembers(t *testing.T) {
	db, _ := jobsFixture(t)
	log := seedPipelineLog(t, db)

	pipeline := activityDetail(t, db, log.pipelineA)
	if got, want := pipelineMemberIDs(pipeline.ActivityEntry), []int64{log.importA, log.tagA, log.pair}; !slices.Equal(got, want) {
		t.Errorf("members = %v, want %v", got, want)
	}
	if len(pipeline.Pipelines) != 0 {
		t.Errorf("pipelines of a pipeline = %+v, want none", pipeline.Pipelines)
	}
	pair := activityDetail(t, db, log.pair)
	if got := activityIDs(domain.ActivityPage{Items: pair.Pipelines}); !slices.Equal(got, []int64{log.pipelineB, log.pipelineA}) {
		t.Errorf("pipelines of the pair = %v, want both, newest first", got)
	}
	if pair.Probability == nil || *pair.Probability != 0.9 {
		t.Errorf("pair = %+v, want its probability", pair.ActivityEntry)
	}
	if imported := activityDetail(t, db, log.importA); !imported.StagesRecorded() {
		t.Errorf("an import of a pipeline without stage rows = not recorded, want recorded with no stage run")
	}
	older := writeActivity(t, db,
		videoActivity(domain.ActivityImport, "key-c", "Garden.mp4", activityDone, domain.ActivityByRule), time.Now())
	if imported := activityDetail(t, db, older); imported.StagesRecorded() {
		t.Errorf("an import of no pipeline without stage rows = recorded, want not recorded")
	}
	if _, err := db.Activity().ActivityDetail(context.Background(), log.standalone+100); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("missing entry: err = %v, want ErrNotFound", err)
	}
}

func TestActivityDetailReadsStagesAndReplacedMembers(t *testing.T) {
	db := migratedDB(t)
	queueNewFile(t, db, "retry", domain.ActivityByRule)
	giveUpStage(t, db, domain.JobProbe, errors.New("ffprobe failed"))
	for _, kind := range []domain.JobKind{domain.JobThumbnail, domain.JobSeekThumbnail, domain.JobFingerprint} {
		runImportStage(t, db, kind)
	}
	failed := importEntries(t, db)[0].ID
	if err := db.Ingest().RetryImport(context.Background(), failed, "key-retry"); err != nil {
		t.Fatal(err)
	}
	retry := importEntries(t, db)[0].ID

	imported := activityDetail(t, db, failed)
	want := []domain.ActivityStage{
		{Stage: domain.JobProbe, State: domain.ActivityStageFailed, Detail: "ffprobe failed"},
		{Stage: domain.JobThumbnail, State: domain.ActivityStageDone},
		{Stage: domain.JobSeekThumbnail, State: domain.ActivityStageDone},
		{Stage: domain.JobFingerprint, State: domain.ActivityStageDone},
	}
	if !imported.StagesRecorded() || !slices.EqualFunc(imported.Stages, want, func(a, b domain.ActivityStage) bool {
		return a.Stage == b.Stage && a.State == b.State && a.Detail == b.Detail && !a.UpdatedAt.IsZero()
	}) {
		t.Errorf("stages = %+v, want %+v in ingest order", imported.Stages, want)
	}
	pipeline := activityDetail(t, db, onlyPipeline(t, db, failed).ID)
	if len(pipeline.Members) != 2 || pipeline.Members[0].ID != failed || !pipeline.Members[0].Replaced ||
		pipeline.Members[1].ID != retry || pipeline.Members[1].Replaced {
		t.Errorf("members = %+v, want the failed entry replaced by its retry", pipeline.Members)
	}
}
