package store

import (
	"context"
	"errors"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// The pair a/b of judgementFixture shares this fingerprint; refingerprinting a with it keeps the pair matched.
var fingerprintA = testFingerprint(1, 0)

func forceFingerprint(t *testing.T, db *DB, videoID int64) {
	t.Helper()
	ctx := context.Background()
	forceJobs(t, db, videoID, domain.JobFingerprint)
	job := claimKind(t, db, domain.JobFingerprint)
	if !job.Force || job.VideoID != videoID {
		t.Fatalf("claimed fingerprint job = video %d force %v, want video %d forced", job.VideoID, job.Force, videoID)
	}
	if applied, err := db.Ingest().ApplyFingerprintForJob(ctx, job, fingerprintA); err != nil || !applied {
		t.Fatalf("ApplyFingerprintForJob = %v, %v", applied, err)
	}
	if err := db.Ingest().CompleteClaimedJob(ctx, job); err != nil {
		t.Fatal(err)
	}
}

// rejudgeOf returns the rejudge_of of the a/b candidate, or 0 when the pair is not listed for judging again.
func rejudgeOf(t *testing.T, db *DB) int64 {
	t.Helper()
	var id int64
	if err := db.sql.QueryRow(`select coalesce(max(rejudge_of), 0) from video_version_candidates
		where key_a = 'key-a' and key_b = 'key-b'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

type pairState struct{ bundled, dismissed bool }

func abState(t *testing.T, db *DB) pairState {
	t.Helper()
	var s pairState
	if err := db.sql.QueryRow(`select
		exists (select 1 from video_bundle_members ma join video_bundle_members mb on mb.bundle_id = ma.bundle_id
		         where ma.content_key = 'key-a' and mb.content_key = 'key-b'),
		exists (select 1 from video_version_dismissals where key_a = 'key-a' and key_b = 'key-b')`).
		Scan(&s.bundled, &s.dismissed); err != nil {
		t.Fatal(err)
	}
	return s
}

func judgedPrior(t *testing.T, db *DB, outcome domain.ActivityOutcome) domain.ActivityEntry {
	t.Helper()
	verdict := domain.VersionSame
	if outcome == domain.ActivityDifferent {
		verdict = domain.VersionDifferent
	}
	return judgePair(t, db, domain.VersionJudgeResult{Verdict: verdict, Decider: domain.ActivityByAI})
}

func TestForcedFingerprintListsOnlyAutomaticDecisionsAgain(t *testing.T) {
	for _, tc := range []struct {
		name   string
		decide func(t *testing.T, db *DB, ids map[string]int64) int64 // returns the prior entry to list, or 0
		force  bool
	}{
		{"judged bundle", func(t *testing.T, db *DB, _ map[string]int64) int64 {
			t.Helper()
			return judgedPrior(t, db, domain.ActivitySame).ID
		}, true},
		{"judged dismissal", func(t *testing.T, db *DB, _ map[string]int64) int64 {
			t.Helper()
			return judgedPrior(t, db, domain.ActivityDifferent).ID
		}, true},
		{"unforced fingerprint", func(t *testing.T, db *DB, _ map[string]int64) int64 {
			t.Helper()
			judgedPrior(t, db, domain.ActivitySame)
			return 0
		}, false},
		{"owner bundle", func(t *testing.T, db *DB, ids map[string]int64) int64 {
			t.Helper()
			bundle(t, db, ids["a"], ids["a"], ids["b"])
			return 0
		}, true},
		{"owner dismissal", func(t *testing.T, db *DB, ids map[string]int64) int64 {
			t.Helper()
			if err := db.Versions().Dismiss(context.Background(), [2]int64{ids["a"], ids["b"]}); err != nil {
				t.Fatal(err)
			}
			return 0
		}, true},
		{"owner unbundled after the judged bundle", func(t *testing.T, db *DB, ids map[string]int64) int64 {
			t.Helper()
			judgedPrior(t, db, domain.ActivitySame)
			if _, err := db.Versions().Unbundle(context.Background(), ids["b"]); err != nil {
				t.Fatal(err)
			}
			return 0
		}, true},
		{"owner dismissed after the judged dismissal", func(t *testing.T, db *DB, ids map[string]int64) int64 {
			t.Helper()
			judgedPrior(t, db, domain.ActivityDifferent)
			if err := db.Versions().Dismiss(context.Background(), [2]int64{ids["a"], ids["b"]}); err != nil {
				t.Fatal(err)
			}
			return 0
		}, true},
		{"judging off", func(t *testing.T, db *DB, _ map[string]int64) int64 {
			t.Helper()
			judgedPrior(t, db, domain.ActivitySame)
			enableJudging(t, db, false)
			return 0
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, ids := judgementFixture(t)
			enableJudging(t, db, true)
			want := tc.decide(t, db, ids)

			if tc.force {
				forceFingerprint(t, db, ids["a"])
			} else {
				applyFingerprint(t, db, ids["a"], fingerprintA)
			}

			if got := rejudgeOf(t, db); got != want {
				t.Fatalf("rejudge_of = %d, want %d", got, want)
			}
			if want == 0 {
				expectNoClaim(t, db, "a pair with no automatic decision in force")
				return
			}
			claimJudgement(t, db)
			if entry := pairEntries(t, db)[0]; entry.ID == want || entry.Status != domain.ActivityRunning {
				t.Errorf("claimed entry = %d %s, want a new running entry beside prior %d", entry.ID, entry.Status, want)
			}
		})
	}
}

func TestRejudgeVerdictReplacesOrKeepsThePriorDecision(t *testing.T) {
	bundled, dismissed := pairState{bundled: true}, pairState{dismissed: true}
	for _, tc := range []struct {
		name    string
		prior   domain.ActivityOutcome
		verdict domain.VersionVerdict
		want    pairState
		entry   domain.ActivityState
	}{
		{"same on a bundle", domain.ActivitySame, domain.VersionSame, bundled,
			domain.ActivityState{Status: domain.ActivityDone, Outcome: domain.ActivitySame}},
		{"different on a bundle", domain.ActivitySame, domain.VersionDifferent, dismissed,
			domain.ActivityState{Status: domain.ActivityDone, Outcome: domain.ActivityDifferent}},
		{"same on a dismissal", domain.ActivityDifferent, domain.VersionSame, bundled,
			domain.ActivityState{Status: domain.ActivityDone, Outcome: domain.ActivitySame}},
		{"different on a dismissal", domain.ActivityDifferent, domain.VersionDifferent, dismissed,
			domain.ActivityState{Status: domain.ActivityDone, Outcome: domain.ActivityDifferent}},
		{"undecided on a bundle", domain.ActivitySame, domain.VersionUndecided, bundled,
			domain.VersionJudgementActivityState(domain.VersionJudgementUndecided)},
		{"failed on a bundle", domain.ActivitySame, domain.VersionFailed, bundled,
			domain.VersionJudgementActivityState(domain.VersionJudgementFailed)},
		{"no connection on a bundle", domain.ActivitySame, domain.VersionNoConnection, bundled,
			domain.VersionJudgementActivityState(domain.VersionJudgementNoConnection)},
		{"undecided on a dismissal", domain.ActivityDifferent, domain.VersionUndecided, dismissed,
			domain.VersionJudgementActivityState(domain.VersionJudgementUndecided)},
		{"failed on a dismissal", domain.ActivityDifferent, domain.VersionFailed, dismissed,
			domain.VersionJudgementActivityState(domain.VersionJudgementFailed)},
		{"no connection on a dismissal", domain.ActivityDifferent, domain.VersionNoConnection, dismissed,
			domain.VersionJudgementActivityState(domain.VersionJudgementNoConnection)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, ids := judgementFixture(t)
			enableJudging(t, db, true)
			prior := judgedPrior(t, db, tc.prior)
			forceFingerprint(t, db, ids["a"])
			job := claimJudgement(t, db)

			applyJudgement(t, db, job.Keys, domain.VersionJudgeResult{
				Verdict: tc.verdict, Decider: domain.ActivityByAI, Error: "model failed",
			})

			if got := abState(t, db); got != tc.want {
				t.Errorf("pair = %+v, want %+v", got, tc.want)
			}
			if rows := candidateRows(t, db); rows != 0 {
				t.Errorf("candidate rows = %d, want 0", rows)
			}
			if pairs := unbundledPairs(t, db); len(pairs) != 0 {
				t.Errorf("video_version_unbundles = %v, want none", pairs)
			}
			entry := pairEntries(t, db)[0]
			if entry.ID == prior.ID || entry.Status != tc.entry.Status || entry.Reason != tc.entry.Reason ||
				entry.Outcome != tc.entry.Outcome {
				t.Errorf("new entry = %d %s/%s%s, want a new %s/%s%s", entry.ID, entry.Status, entry.Reason,
					entry.Outcome, tc.entry.Status, tc.entry.Reason, tc.entry.Outcome)
			}
		})
	}
}

// Without an unbundles row the split pair stays judgeable, so the next force rescan judges it again.
func TestRejudgedSplitIsListedByTheNextForcedFingerprint(t *testing.T) {
	db, ids := judgementFixture(t)
	enableJudging(t, db, true)
	judgedPrior(t, db, domain.ActivitySame)
	forceFingerprint(t, db, ids["a"])
	job := claimJudgement(t, db)
	applyJudgement(t, db, job.Keys, domain.VersionJudgeResult{Verdict: domain.VersionDifferent, Decider: domain.ActivityByAI})
	split := pairEntries(t, db)[0]

	forceFingerprint(t, db, ids["a"])

	if got := rejudgeOf(t, db); got != split.ID {
		t.Errorf("rejudge_of = %d, want the split's entry %d", got, split.ID)
	}
}

// Confirming one re-judged bundle pair must not close the activity of another pair of the bundle still waiting to be
// judged again, or the forced pipeline finishes before that pair's verdict.
func TestConfirmedRejudgeKeepsTheBundlesOtherRejudgeOpen(t *testing.T) {
	ctx := context.Background()
	db, ids := judgementFixture(t)
	enableJudging(t, db, true)
	judgedPrior(t, db, domain.ActivitySame)
	applyFingerprint(t, db, ids["c"], fingerprintA)
	job, err := claimVersionJudgement(ctx, db)
	if err != nil {
		t.Fatalf("ClaimJob(version_judge): %v", err)
	}
	applyJudgement(t, db, job.Keys, domain.VersionJudgeResult{Verdict: domain.VersionSame, Decider: domain.ActivityByAI})
	// The pair judged second shares one key with a/b; forcing that key lists both automatic bundles again.
	shared := map[string]int64{"key-a": ids["a"], "key-b": ids["b"]}[job.Keys[0]]
	forceFingerprint(t, db, shared)
	if rows := candidateRows(t, db); rows != 2 {
		t.Fatalf("candidate rows = %d, want both bundle pairs listed again", rows)
	}

	first, err := claimVersionJudgement(ctx, db)
	if err != nil {
		t.Fatalf("ClaimJob(version_judge): %v", err)
	}
	applyJudgement(t, db, first.Keys, domain.VersionJudgeResult{Verdict: domain.VersionSame, Decider: domain.ActivityByAI})

	var status string
	if err := db.sql.QueryRow(`select e.status from video_version_candidates c
		join activity_entries e on e.id = c.activity_id`).Scan(&status); err != nil {
		t.Fatalf("the other pair's candidate and activity: %v", err)
	}
	if status == string(domain.ActivityDone) {
		t.Errorf("the other pair's activity = %s, want it open until its own verdict", status)
	}
}

// Splitting a re-judged bundle pair rebuilds the leaving key's candidates; another pair of that key listed for
// judging again must keep its listing, or its automatic decision is never judged again.
func TestSplitRejudgeKeepsTheLeavingKeysOtherListing(t *testing.T) {
	ctx := context.Background()
	db, ids := judgementFixture(t)
	enableJudging(t, db, true)
	judgedPrior(t, db, domain.ActivitySame)
	applyFingerprint(t, db, ids["c"], fingerprintA)
	for range 2 {
		job, err := claimVersionJudgement(ctx, db)
		if err != nil {
			t.Fatalf("ClaimJob(version_judge): %v", err)
		}
		applyJudgement(t, db, job.Keys, domain.VersionJudgeResult{Verdict: domain.VersionDifferent, Decider: domain.ActivityByAI})
	}
	forceFingerprint(t, db, ids["a"])
	if rows := candidateRows(t, db); rows != 2 {
		t.Fatalf("candidate rows = %d, want a/b and a/c listed again", rows)
	}

	// a joined b's bundle, so the different verdict on a/b moves key-a out and rebuilds its candidates.
	job := claimJudgement(t, db)
	applyJudgement(t, db, job.Keys, domain.VersionJudgeResult{Verdict: domain.VersionDifferent, Decider: domain.ActivityByAI})

	var rejudgeOf int64
	if err := db.sql.QueryRow(`select coalesce(max(rejudge_of), 0) from video_version_candidates
		where key_a = 'key-a' and key_b = 'key-c'`).Scan(&rejudgeOf); err != nil {
		t.Fatal(err)
	}
	if rejudgeOf == 0 {
		t.Fatal("a/c is no longer listed for judging again after the a/b split")
	}
	next, err := claimVersionJudgement(ctx, db)
	if err != nil || next.Keys != [2]string{"key-a", "key-c"} {
		t.Errorf("ClaimJob(version_judge) = %v, %v, want the listed a/c pair", next.Keys, err)
	}
}

// A verdict that keeps the prior decision replaces it: only the new entry offers the undo, so undoing the prior entry
// cannot revert the decision the force rescan confirmed.
func TestConfirmedRejudgeMovesTheUndoToTheNewEntry(t *testing.T) {
	for _, tc := range []struct {
		name  string
		prior domain.ActivityOutcome
		want  pairState
	}{
		{"same on a bundle", domain.ActivitySame, pairState{bundled: true}},
		{"different on a dismissal", domain.ActivityDifferent, pairState{dismissed: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			db, ids := judgementFixture(t)
			enableJudging(t, db, true)
			prior := judgedPrior(t, db, tc.prior)
			forceFingerprint(t, db, ids["a"])
			job := claimJudgement(t, db)
			verdict := domain.VersionSame
			if tc.prior == domain.ActivityDifferent {
				verdict = domain.VersionDifferent
			}
			applyJudgement(t, db, job.Keys, domain.VersionJudgeResult{Verdict: verdict, Decider: domain.ActivityByAI})

			replaced, err := db.Activity().ActivityEntry(ctx, prior.ID)
			if err != nil {
				t.Fatal(err)
			}
			if replaced.UndoOffered() {
				t.Error("the replaced prior entry still offers the undo")
			}
			if _, err := db.Activity().UndoVersionJudgement(ctx, prior.ID); !errors.Is(err, domain.ErrActivityNotUndoable) {
				t.Errorf("UndoVersionJudgement(prior) = %v, want ErrActivityNotUndoable", err)
			}
			if got := abState(t, db); got != tc.want {
				t.Errorf("pair = %+v, want %+v", got, tc.want)
			}
			if latest := pairEntries(t, db)[0]; latest.ID == prior.ID || !latest.UndoOffered() {
				t.Errorf("new entry %d undo offered = %v, want the new entry to offer it", latest.ID, latest.UndoOffered())
			}
		})
	}
}
