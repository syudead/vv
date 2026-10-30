package store

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

var assistantNow = time.UnixMilli(1_800_000_000_000)

func batchedPlan(keys ...string) domain.AssistantPlan {
	return domain.AssistantPlan{
		ActionID: "updateVideoTags.add", Targets: domain.AssistantPlanTargets{Kind: domain.AssistantTargetVideos, VideoKeys: keys},
	}
}

func createProposal(t *testing.T, db *DB, at time.Time, plans ...domain.AssistantPlan) domain.AssistantProposal {
	t.Helper()
	p, err := db.Assistant().CreateProposal(context.Background(), 0, "タグを付けて", plans, at)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func assistantEntry(t *testing.T, db *DB, id int64) domain.ActivityEntry {
	t.Helper()
	entry, err := db.Activity().ActivityEntry(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return entry
}

func TestApproveAssistantProposalOnce(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	p := createProposal(t, db, assistantNow, batchedPlan("k1"))

	approved, err := db.Assistant().ApproveProposal(ctx, p.ID, []int{0}, assistantNow)
	if err != nil {
		t.Fatal(err)
	}
	if approved.Status != domain.AssistantApproved || approved.ActivityID == 0 {
		t.Fatalf("approved = %+v", approved)
	}
	again, err := db.Assistant().ApproveProposal(ctx, p.ID, []int{0}, assistantNow)
	if !errors.Is(err, domain.ErrAssistantProposalNotPending) || again.ID != p.ID || again.Status != domain.AssistantApproved {
		t.Fatalf("second approval = %+v, %v; want the approved row with not pending", again, err)
	}
	if _, err := db.Assistant().ApproveProposal(ctx, p.ID+1, []int{0}, assistantNow); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("missing proposal: err = %v, want not found", err)
	}
	entry := assistantEntry(t, db, approved.ActivityID)
	if entry.Kind != domain.ActivityAssistantRun || entry.Feature != domain.ActivityFeatureAssistant ||
		entry.Subject.Kind != domain.ActivitySubjectProposal || entry.Subject.Label != "タグを付けて" ||
		entry.Decider != domain.ActivityByOwner {
		t.Errorf("entry = %+v", entry)
	}
}

func TestApproveExpiredAssistantProposalIsNotFound(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	p := createProposal(t, db, assistantNow.Add(-domain.AssistantProposalTTL-time.Millisecond), batchedPlan("k1"))

	if _, err := db.Assistant().ApproveProposal(ctx, p.ID, []int{0}, assistantNow); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("approval after the TTL: err = %v, want not found", err)
	}
	stored, err := db.Assistant().Proposal(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != domain.AssistantProposed || stored.ActivityID != 0 {
		t.Errorf("stored = %+v, want it still proposed without an entry", stored)
	}
}

func TestAssistantRunEntryFollowsTheRun(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	p := createProposal(t, db, assistantNow, domain.AssistantPlan{ActionID: "createTag"}, batchedPlan("k1", "k2", "k3"))
	if _, err := db.Assistant().ApproveProposal(ctx, p.ID, []int{0, 1}, assistantNow); err != nil {
		t.Fatal(err)
	}

	claimed, err := db.Assistant().ClaimProposal(ctx, assistantNow)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.ID != p.ID || claimed.Status != domain.AssistantRunning {
		t.Fatalf("claimed = %+v", claimed)
	}
	if entry := assistantEntry(t, db, claimed.ActivityID); entry.Status != domain.ActivityRunning {
		t.Fatalf("entry after the claim = %s/%s", entry.Status, entry.Reason)
	}
	if _, err := db.Assistant().ClaimProposal(ctx, assistantNow); !errors.Is(err, domain.ErrNoJob) {
		t.Fatalf("second claim: err = %v, want no job", err)
	}

	steps := slices.Clone(claimed.Steps)
	failure := &domain.AssistantProposalError{Code: "not_found", Reason: "video_not_found"}
	steps[0].AssistantStepState = domain.AssistantStepState{Status: domain.AssistantStepDone, Total: 1, Applied: 1}
	steps[1].AssistantStepState = domain.AssistantStepState{
		Status: domain.AssistantStepPartial, Total: 3, Applied: 2, Failed: 1, Error: failure,
	}
	if err := db.Assistant().FinishProposal(ctx, p.ID, domain.NewAssistantRunResult(steps), assistantNow); err != nil {
		t.Fatal(err)
	}
	finished, err := db.Assistant().Proposal(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finished.Status != domain.AssistantPartial || finished.Counts != (domain.AssistantCounts{Total: 4, Applied: 3, Failed: 1}) ||
		finished.Error == nil || finished.Error.Reason != "video_not_found" || finished.FinishedAt.IsZero() ||
		len(finished.Steps) != 2 || finished.Steps[1].Status != domain.AssistantStepPartial {
		t.Errorf("finished = %+v", finished)
	}
	entry := assistantEntry(t, db, finished.ActivityID)
	var detail domain.AssistantRunDetail
	if err := json.Unmarshal([]byte(entry.Detail), &detail); err != nil {
		t.Fatalf("detail %q: %v", entry.Detail, err)
	}
	want := domain.AssistantRunDetail{Total: 4, Applied: 3, Failed: 1, Steps: []domain.AssistantRunStep{
		{Operation: "createTag", Status: domain.AssistantStepDone, Total: 1, Applied: 1},
		{Operation: "updateVideoTags.add", Status: domain.AssistantStepPartial, Total: 3, Applied: 2, Failed: 1},
	}}
	if entry.Status != domain.ActivityDone || entry.Outcome != domain.ActivityPartial || !reflect.DeepEqual(detail, want) {
		t.Errorf("entry = %s/%s %+v, want %+v", entry.Status, entry.Outcome, detail, want)
	}
}

func TestApproveAssistantProposalSkipsUntickedSteps(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	needsFirst := batchedPlan("k1")
	needsFirst.CreatedBy = []domain.AssistantCreatedBy{{Name: "Rewatch", Step: 0}}
	p := createProposal(t, db, assistantNow, domain.AssistantPlan{ActionID: "createTag"}, needsFirst, batchedPlan("k2"))

	var needs *domain.AssistantStepNeedsSkippedError
	if _, err := db.Assistant().ApproveProposal(ctx, p.ID, []int{1, 2}, assistantNow); !errors.As(err, &needs) || needs.Step != 1 {
		t.Fatalf("approval without the creating step: err = %v, want step 1 needs a skipped step", err)
	}
	approved, err := db.Assistant().ApproveProposal(ctx, p.ID, []int{0, 1}, assistantNow)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := db.Assistant().Proposal(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, got := range []domain.AssistantProposal{approved, stored} {
		if len(got.Steps) != 3 || got.Steps[0].Status != domain.AssistantStepPending ||
			got.Steps[2].Status != domain.AssistantStepSkipped || got.Counts.Total != 2 {
			t.Errorf("proposal = %+v, want the third step skipped and a total of 2", got)
		}
	}
}

// The step migration deletes the proposals the one-shot build stored, whose plan column holds a single plan.
func TestAssistantStepsMigrationDeletesOneShotProposals(t *testing.T) {
	db, provider := migrateTo(t, 88)
	execSQL(t, db, `insert into assistant_proposals (status, request, plan, created_at)
		values ('proposed', 'x', '{"actionId":"mergeTag"}', 1), ('done', 'y', '{"actionId":"deleteTag"}', 2)`)

	if _, err := provider.UpTo(context.Background(), 89); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.sql.QueryRow(`select count(*) from assistant_proposals`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("proposals after the migration = %d, want none", count)
	}
}

func TestExpireAssistantProposalsDeletesOldProposedOnly(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	cutoff := assistantNow.Add(-domain.AssistantProposalTTL)
	old := createProposal(t, db, cutoff.Add(-time.Minute), batchedPlan("k1"))
	oldApproved := createProposal(t, db, cutoff.Add(-time.Minute), batchedPlan("k1"))
	if _, err := db.Assistant().ApproveProposal(ctx, oldApproved.ID, []int{0}, oldApproved.CreatedAt); err != nil {
		t.Fatal(err)
	}
	recent := createProposal(t, db, cutoff.Add(time.Minute), batchedPlan("k1"))

	deleted, err := db.Assistant().ExpireProposals(ctx, cutoff)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1", deleted)
	}
	if _, err := db.Assistant().Proposal(ctx, old.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("old proposal: err = %v, want not found", err)
	}
	for _, id := range []int64{oldApproved.ID, recent.ID} {
		if _, err := db.Assistant().Proposal(ctx, id); err != nil {
			t.Errorf("proposal %d: %v", id, err)
		}
	}
}

func TestAssistantChangedIsPublishedAfterCommit(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	recorder := &eventRecorder{}
	db.PublishTo(recorder)

	p := createProposal(t, db, assistantNow, batchedPlan("k1"))
	if len(recorder.events) != 1 || recorder.events[0] != (domain.AssistantChanged{}) {
		t.Fatalf("events after storing = %v", recorder.events)
	}
	if _, err := db.Assistant().Proposal(ctx, p.ID); err != nil {
		t.Fatalf("the published proposal is not committed: %v", err)
	}

	recorder.events = nil
	if _, err := db.Assistant().ApproveProposal(ctx, p.ID, []int{0}, assistantNow); err != nil {
		t.Fatal(err)
	}
	want := map[domain.Event]bool{domain.AssistantChanged{}: false, domain.ActivityChanged{}: false}
	for _, event := range recorder.events {
		if _, ok := want[event]; ok {
			want[event] = true
		}
	}
	if !want[domain.AssistantChanged{}] || !want[domain.ActivityChanged{}] {
		t.Errorf("events after approval = %v", recorder.events)
	}

	// A refused approval changes nothing and publishes nothing.
	recorder.events = nil
	if _, err := db.Assistant().ApproveProposal(ctx, p.ID, []int{0}, assistantNow); !errors.Is(err, domain.ErrAssistantProposalNotPending) {
		t.Fatal(err)
	}
	if len(recorder.events) != 0 {
		t.Errorf("events after a refused approval = %v", recorder.events)
	}
}

func TestRecoverAssistantProposals(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	proposed := createProposal(t, db, assistantNow, batchedPlan("k1"))
	running := createProposal(t, db, assistantNow, batchedPlan("k1"), batchedPlan("k2"))
	waiting := createProposal(t, db, assistantNow, batchedPlan("k1"))
	for _, p := range []domain.AssistantProposal{running, waiting} {
		if _, err := db.Assistant().ApproveProposal(ctx, p.ID, []int{0, len(p.Steps) - 1}, assistantNow); err != nil {
			t.Fatal(err)
		}
	}
	claimed, err := db.Assistant().ClaimProposal(ctx, assistantNow)
	if err != nil {
		t.Fatal(err)
	}
	claimed.Steps[0].Status = domain.AssistantStepRunning
	if err := db.Assistant().RecordProgress(ctx, claimed.ID, claimed.Steps, assistantNow); err != nil {
		t.Fatal(err)
	}

	if _, err := db.Assistant().RecoverProposals(ctx, assistantNow); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Assistant().Proposal(ctx, proposed.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("proposed: err = %v, want deleted", err)
	}
	failed, err := db.Assistant().Proposal(ctx, running.ID)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Status != domain.AssistantFailed || failed.Error == nil || failed.Error.Code != domain.AssistantInterrupted {
		t.Errorf("running after recovery = %+v", failed)
	}
	if steps := failed.Steps; len(steps) != 2 || steps[0].Status != domain.AssistantStepFailed || steps[0].Error == nil ||
		steps[0].Error.Code != domain.AssistantInterrupted || steps[1].Status != domain.AssistantStepNotRun {
		t.Errorf("steps after recovery = %+v, want the running step interrupted and the next not run", steps)
	}
	if entry := assistantEntry(t, db, failed.ActivityID); entry.Status != domain.ActivityAttention ||
		entry.Reason != domain.ActivityInterrupted {
		t.Errorf("entry = %s/%s", entry.Status, entry.Reason)
	}
	if still, err := db.Assistant().Proposal(ctx, waiting.ID); err != nil || still.Status != domain.AssistantApproved {
		t.Errorf("approved after recovery = %+v, %v", still, err)
	}
}

func TestAssistantCurrentLabelsReadsNamesAndVideos(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	tag, err := db.Tags().CreateTag(ctx, "Anime")
	if err != nil {
		t.Fatal(err)
	}
	videoID := upsertFolderVideo(t, db, fixturePath("/media/a.mp4"), "k1")
	rebuildIndexForTest(t, db)
	plan := batchedPlan("k1", "gone")
	plan.Change = []domain.AssistantChange{{Argument: domain.AssistantArgTagNames, Labels: []domain.AssistantLabel{
		{Kind: domain.AssistantLabelTag, ID: tag.ID, Name: "Anime"},
	}}}

	current, err := db.Assistant().CurrentLabels(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	if got := current.Names[domain.AssistantLabelRef{Kind: domain.AssistantLabelTag, ID: tag.ID}]; got != "Anime" {
		t.Errorf("tag name = %q", got)
	}
	if id, ok := current.Videos["k1"]; !ok || id != videoID {
		t.Errorf("videos = %v, want k1 → %d", current.Videos, videoID)
	}
	if _, ok := current.Videos["gone"]; ok {
		t.Error("a key without a video maps to one")
	}
}

func TestAssistantCurrentLabelsResolvesAVersionByItsContentKey(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	first := upsertFolderVideo(t, db, fixturePath("/media/a.mp4"), "ka")
	second := upsertFolderVideo(t, db, fixturePath("/media/b.mp4"), "kb")
	rebuildIndexForTest(t, db)
	if _, err := db.Versions().Bundle(ctx, []int64{first, second}, first); err != nil {
		t.Fatal(err)
	}
	version := domain.AssistantPlan{
		ActionID: "makeRepresentativeVersion",
		Targets: domain.AssistantPlanTargets{Kind: domain.AssistantTargetVideo, Labels: []domain.AssistantLabel{
			{Kind: domain.AssistantLabelVideo, Key: "kb", Name: "b"},
		}},
	}

	current, err := db.Assistant().CurrentLabels(ctx, version)
	if err != nil {
		t.Fatal(err)
	}
	if id, ok := current.Videos["kb"]; !ok || id != second {
		t.Errorf("version videos = %v, want kb → %d, the version itself", current.Videos, second)
	}
	// Shared user data is written under the bundle's user key, so a member's content key no longer names it.
	shared, err := db.Assistant().CurrentLabels(ctx, batchedPlan("kb"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := shared.Videos["kb"]; ok {
		t.Errorf("shared videos = %v, want the bundled content key absent", shared.Videos)
	}
}

func TestAssistantMergeImpactCountsDistinctCreditedVideos(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	a := createPerson(t, db, "Alice")
	b := createPerson(t, db, "Bob")
	destination := createPerson(t, db, "Carol")
	upsertFolderVideo(t, db, fixturePath("/media/shared.mp4"), "shared")
	upsertFolderVideo(t, db, fixturePath("/media/other.mp4"), "other")
	upsertFolderVideo(t, db, fixturePath("/media/carol.mp4"), "carol")
	rebuildIndexForTest(t, db)
	linkPeople(t, db, "shared", "performer", a.ID, b.ID)
	linkPeople(t, db, "other", "performer", b.ID)
	linkPeople(t, db, "carol", "performer", destination.ID)

	got, err := db.Assistant().MergeImpact(ctx, domain.AssistantTargetPerson, []int64{a.ID, b.ID})
	if err != nil {
		t.Fatal(err)
	}
	if got != 2 {
		t.Errorf("impact = %d, want 2", got)
	}
}

func TestAssistantFindNamesFoldsAndReadsSynonymsAndAliases(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	tag, err := db.Tags().CreateTag(ctx, "ネコ")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Tags().AddSynonym(ctx, tag.ID, "Kitty", nil); err != nil {
		t.Fatal(err)
	}
	person := createPerson(t, db, "山田花子", "Hanako Yamada")
	tagMatch := domain.AssistantCandidate{Kind: domain.AssistantLabelTag, ID: tag.ID, Name: "ネコ"}
	cases := []struct {
		kind domain.AssistantLabelKind
		name string
		want domain.AssistantCandidate
	}{
		{domain.AssistantLabelTag, "ねこ", tagMatch},
		{domain.AssistantLabelTag, "ＫＩＴＴＹ", tagMatch},
		{domain.AssistantLabelPerson, "hanako yamada", domain.AssistantCandidate{
			Kind: domain.AssistantLabelPerson, ID: person.ID, Name: "山田花子",
		}},
	}
	for _, c := range cases {
		got, err := db.Assistant().FindNames(ctx, c.kind, c.name)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0] != c.want {
			t.Errorf("FindNames(%s, %q) = %+v, want %+v", c.kind, c.name, got, c.want)
		}
	}
	if got, err := db.Assistant().FindNames(ctx, domain.AssistantLabelTag, "ねこちゃん"); err != nil || len(got) != 0 {
		t.Errorf("an unrelated name matched %+v, %v", got, err)
	}
}

func TestAssistantFindSimilarNamesMatchesPartOfAName(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	var ids []int64
	for _, name := range []string{"コスプレ衣装", "コスプレ衣裳", "ネコ"} {
		tag, err := db.Tags().CreateTag(ctx, name)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, tag.ID)
	}
	if got, err := db.Assistant().FindNames(ctx, domain.AssistantLabelTag, "こすぷれ"); err != nil || len(got) != 0 {
		t.Fatalf("FindNames = %+v, %v; want no exact match", got, err)
	}
	got, err := db.Assistant().FindSimilarNames(ctx, domain.AssistantLabelTag, "こすぷれ")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != ids[0] || got[1].ID != ids[1] {
		t.Fatalf("FindSimilarNames = %+v, want the two cosplay tags", got)
	}
	kind, err := db.Tags().CreateTagKind(ctx, "ジャンル")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := db.Assistant().FindSimilarNames(ctx, domain.AssistantLabelTagKind, "ジャン"); err != nil ||
		len(got) != 1 || got[0].ID != kind.ID {
		t.Fatalf("FindSimilarNames(tagKind) = %+v, %v", got, err)
	}
}

func TestAssistantDiscardProposalOnlyWhileProposed(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	p := createProposal(t, db, assistantNow, batchedPlan("k1"))
	if err := db.Assistant().DiscardProposal(ctx, p.ID, assistantNow); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.Assistant().Proposal(ctx, p.ID); got.Status != domain.AssistantDiscarded {
		t.Fatalf("status = %q, want discarded", got.Status)
	}
	if err := db.Assistant().DiscardProposal(ctx, p.ID, assistantNow); !errors.Is(err, domain.ErrAssistantProposalNotPending) {
		t.Errorf("second discard: err = %v, want not pending", err)
	}
}
