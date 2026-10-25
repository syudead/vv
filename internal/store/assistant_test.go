package store

import (
	"context"
	"encoding/json"
	"errors"
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

func createProposal(t *testing.T, db *DB, plan domain.AssistantPlan, at time.Time) domain.AssistantProposal {
	t.Helper()
	p, err := db.Assistant().CreateProposal(context.Background(), "タグを付けて", plan, at)
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
	p := createProposal(t, db, batchedPlan("k1"), assistantNow)

	approved, err := db.Assistant().ApproveProposal(ctx, p.ID, assistantNow)
	if err != nil {
		t.Fatal(err)
	}
	if approved.Status != domain.AssistantApproved || approved.ActivityID == 0 {
		t.Fatalf("approved = %+v", approved)
	}
	again, err := db.Assistant().ApproveProposal(ctx, p.ID, assistantNow)
	if !errors.Is(err, domain.ErrAssistantProposalNotPending) || again.ID != p.ID || again.Status != domain.AssistantApproved {
		t.Fatalf("second approval = %+v, %v; want the approved row with not pending", again, err)
	}
	if _, err := db.Assistant().ApproveProposal(ctx, p.ID+1, assistantNow); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("missing proposal: err = %v, want not found", err)
	}
	entry := assistantEntry(t, db, approved.ActivityID)
	if entry.Kind != domain.ActivityAssistantRun || entry.Feature != domain.ActivityFeatureAssistant ||
		entry.Subject.Kind != domain.ActivitySubjectProposal || entry.Subject.Label != "タグを付けて" ||
		entry.Decider != domain.ActivityByOwner {
		t.Errorf("entry = %+v", entry)
	}
}

func TestAssistantRunEntryFollowsTheRun(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	p := createProposal(t, db, batchedPlan("k1", "k2", "k3"), assistantNow)
	if _, err := db.Assistant().ApproveProposal(ctx, p.ID, assistantNow); err != nil {
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

	counts := domain.AssistantCounts{Total: 3, Applied: 2, Failed: 1}
	if err := db.Assistant().FinishProposal(ctx, p.ID, domain.AssistantRunResult{
		Status: domain.AssistantPartial, Counts: counts,
		Error: &domain.AssistantProposalError{Code: "not_found", Reason: "video_not_found"},
	}, assistantNow); err != nil {
		t.Fatal(err)
	}
	finished, err := db.Assistant().Proposal(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finished.Status != domain.AssistantPartial || finished.Counts != counts || finished.Error == nil ||
		finished.Error.Reason != "video_not_found" || finished.FinishedAt.IsZero() {
		t.Errorf("finished = %+v", finished)
	}
	entry := assistantEntry(t, db, finished.ActivityID)
	var detail domain.AssistantRunDetail
	if err := json.Unmarshal([]byte(entry.Detail), &detail); err != nil {
		t.Fatalf("detail %q: %v", entry.Detail, err)
	}
	if entry.Status != domain.ActivityDone || entry.Outcome != domain.ActivityPartial ||
		detail != (domain.AssistantRunDetail{Operation: "updateVideoTags.add", Total: 3, Applied: 2, Failed: 1}) {
		t.Errorf("entry = %s/%s %+v", entry.Status, entry.Outcome, detail)
	}
}

func TestExpireAssistantProposalsDeletesOldProposedOnly(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	cutoff := assistantNow.Add(-domain.AssistantProposalTTL)
	old := createProposal(t, db, batchedPlan("k1"), cutoff.Add(-time.Minute))
	oldApproved := createProposal(t, db, batchedPlan("k1"), cutoff.Add(-time.Minute))
	if _, err := db.Assistant().ApproveProposal(ctx, oldApproved.ID, assistantNow); err != nil {
		t.Fatal(err)
	}
	recent := createProposal(t, db, batchedPlan("k1"), cutoff.Add(time.Minute))

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

	p := createProposal(t, db, batchedPlan("k1"), assistantNow)
	if len(recorder.events) != 1 || recorder.events[0] != (domain.AssistantChanged{}) {
		t.Fatalf("events after storing = %v", recorder.events)
	}
	if _, err := db.Assistant().Proposal(ctx, p.ID); err != nil {
		t.Fatalf("the published proposal is not committed: %v", err)
	}

	recorder.events = nil
	if _, err := db.Assistant().ApproveProposal(ctx, p.ID, assistantNow); err != nil {
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
	if _, err := db.Assistant().ApproveProposal(ctx, p.ID, assistantNow); !errors.Is(err, domain.ErrAssistantProposalNotPending) {
		t.Fatal(err)
	}
	if len(recorder.events) != 0 {
		t.Errorf("events after a refused approval = %v", recorder.events)
	}
}

func TestRecoverAssistantProposals(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	proposed := createProposal(t, db, batchedPlan("k1"), assistantNow)
	running := createProposal(t, db, batchedPlan("k1"), assistantNow)
	waiting := createProposal(t, db, batchedPlan("k1"), assistantNow)
	for _, id := range []int64{running.ID, waiting.ID} {
		if _, err := db.Assistant().ApproveProposal(ctx, id, assistantNow); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Assistant().ClaimProposal(ctx, assistantNow); err != nil {
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
