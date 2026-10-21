package store

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func testEvidence() *domain.VersionJudgeEvidence {
	return &domain.VersionJudgeEvidence{
		Rule: "frames_differ", Compared: 12, Differing: 3, LongestRun: 3, RunStartMs: 2000, RunEndMs: 5000,
		Frames: []domain.VersionEvidenceFrame{{A: 3, B: 3, AMs: 3000, BMs: 3000, Distance: 30}},
	}
}

func TestJudgementEvidenceReachesCandidateAndActivity(t *testing.T) {
	db, ids := judgementFixture(t)
	job := claimJudgement(t, db)
	probability := 0.5
	applyJudgement(t, db, job.Keys, domain.VersionJudgeResult{
		Verdict: domain.VersionUndecided, Probability: &probability, Decider: domain.ActivityByAI, Evidence: testEvidence(),
	})

	_, page := candidatePairs(t, db)
	if len(page.Items) != 1 {
		t.Fatalf("候補 = %+v, want 1 組", page.Items)
	}
	judgement := page.Items[0].Judgement
	if judgement.State != domain.VersionJudgementUndecided || judgement.Probability == nil || *judgement.Probability != 0.5 ||
		judgement.LeftVideoID != ids["a"] || judgement.ActivityID == 0 || !reflect.DeepEqual(judgement.Evidence, testEvidence()) {
		t.Errorf("候補の判定 = %+v（根拠 %+v）, want undecided 0.5、左は a、根拠つき", judgement, judgement.Evidence)
	}
	entry, err := db.Activity().ActivityEntry(context.Background(), judgement.ActivityID)
	if err != nil || !reflect.DeepEqual(entry.Evidence, testEvidence()) {
		t.Errorf("記録の根拠 = %+v, %v, want %+v", entry.Evidence, err, testEvidence())
	}
	keys, evidence, err := db.Versions().JudgementEvidence(context.Background(), judgement.ActivityID)
	if err != nil || keys != job.Keys || !reflect.DeepEqual(evidence, testEvidence()) {
		t.Errorf("JudgementEvidence = %v, %+v, %v", keys, evidence, err)
	}
}

func TestJudgementSameConflictMarksEvidence(t *testing.T) {
	db, ids := judgementFixture(t)
	a, b, c := ids["a"], ids["b"], ids["c"]
	bundle(t, db, a, a, c)
	job := claimJudgement(t, db)
	if err := db.Versions().Dismiss(context.Background(), [2]int64{b, c}); err != nil {
		t.Fatal(err)
	}
	applyJudgement(t, db, job.Keys, domain.VersionJudgeResult{Verdict: domain.VersionSame, Evidence: testEvidence()})

	_, page := candidatePairs(t, db)
	if len(page.Items) != 1 || page.Items[0].Judgement.Evidence == nil || !page.Items[0].Judgement.Evidence.Conflict {
		t.Errorf("候補 = %+v, want 衝突の印がついた根拠", page.Items)
	}
}

func TestRejudgeCandidateReturnsPairToWaiting(t *testing.T) {
	db, ids := judgementFixture(t)
	a, b, c := ids["a"], ids["b"], ids["c"]
	job := claimJudgement(t, db)
	if err := db.Versions().RejudgeCandidate(context.Background(), [2]int64{b, a}); !errors.Is(err, domain.ErrVersionNotRejudgeable) {
		t.Errorf("判定中の組: RejudgeCandidate = %v, want ErrVersionNotRejudgeable", err)
	}
	applyJudgement(t, db, job.Keys, domain.VersionJudgeResult{Verdict: domain.VersionFailed, Error: "boom", Evidence: testEvidence()})
	_, page := candidatePairs(t, db)
	activityID := page.Items[0].Judgement.ActivityID

	if err := db.Versions().RejudgeCandidate(context.Background(), [2]int64{b, a}); err != nil {
		t.Fatalf("RejudgeCandidate: %v", err)
	}
	if judgement, attempts, retryAt, _ := candidateJudgement(t, db); judgement != nil || attempts != 0 || retryAt != nil {
		t.Errorf("候補 = %v（失敗 %d 回、再試行 %v）, want 待ち、回数と再試行時刻なし", judgement, attempts, retryAt)
	}
	entry, err := db.Activity().ActivityEntry(context.Background(), activityID)
	if err != nil || entry.Status != domain.ActivityRunning || entry.Reason != domain.ActivityWaiting || entry.Evidence != nil {
		t.Errorf("記録 = %s/%s（根拠 %+v）, %v, want running/waiting、根拠なし", entry.Status, entry.Reason, entry.Evidence, err)
	}
	claimJudgement(t, db)

	if err := db.Versions().RejudgeCandidate(context.Background(), [2]int64{a, c}); !errors.Is(err, domain.ErrNotVersionCandidate) {
		t.Errorf("候補でない組: RejudgeCandidate = %v, want ErrNotVersionCandidate", err)
	}
}

func TestCandidatesReportOwnerHeldPairsAsHeld(t *testing.T) {
	db, _ := judgementFixture(t)
	if _, err := db.sql.Exec(`insert into video_version_judging_holds (key_a, key_b, created_at)
		values ('key-a', 'key-b', 0)`); err != nil {
		t.Fatal(err)
	}
	_, page := candidatePairs(t, db)
	if len(page.Items) != 1 || page.Items[0].Judgement.State != domain.VersionJudgementHeld {
		t.Errorf("候補 = %+v, want held の 1 組", page.Items)
	}
	expectNoClaim(t, db, "持ち主が止めた組")
}
