package store

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func pairEntries(t *testing.T, db *DB) []domain.ActivityEntry {
	t.Helper()
	return entriesOfKind(t, db, domain.ActivityVersionJudgement)
}

func onlyPairEntry(t *testing.T, db *DB) domain.ActivityEntry {
	t.Helper()
	entries := pairEntries(t, db)
	if len(entries) != 1 {
		t.Fatalf("version_judgement の記録 = %d 件, want 1: %+v", len(entries), entries)
	}
	return entries[0]
}

func expectEntryState(t *testing.T, entry domain.ActivityEntry, state domain.ActivityState, decider domain.ActivityDecider) {
	t.Helper()
	if entry.Status != state.Status || entry.Reason != state.Reason || entry.Outcome != state.Outcome ||
		entry.Decider != decider {
		t.Errorf("記録 = %s/%s%s（%s）, want %s/%s%s（%s）", entry.Status, entry.Reason, entry.Outcome, entry.Decider,
			state.Status, state.Reason, state.Outcome, decider)
	}
}

func probabilityOf(value float64) *float64 { return &value }

func judgePair(t *testing.T, db *DB, result domain.VersionJudgeResult) domain.ActivityEntry {
	t.Helper()
	job := claimJudgement(t, db)
	if entry := onlyPairEntry(t, db); entry.Status != domain.ActivityRunning || entry.Reason != domain.ActivityProcessing {
		t.Errorf("取り出した組の記録 = %s/%s, want running/processing", entry.Status, entry.Reason)
	}
	applyJudgement(t, db, job.Keys, result)
	return onlyPairEntry(t, db)
}

func undoEntry(t *testing.T, db *DB, id int64) domain.ActivityEntry {
	t.Helper()
	undo, err := db.Activity().UndoVersionJudgement(context.Background(), id)
	if err != nil {
		t.Fatalf("UndoVersionJudgement(%d): %v", id, err)
	}
	if undo.Kind != domain.ActivityUndo || undo.UndoesID != id || undo.Decider != domain.ActivityByOwner ||
		undo.Feature != domain.ActivityFeatureDuplicateCheck {
		t.Errorf("undo の記録 = %+v, want 記録 %d を取り消した owner の duplicate_check の undo", undo, id)
	}
	return undo
}

func unbundledPairs(t *testing.T, db *DB) []string {
	t.Helper()
	rows, err := db.sql.Query(`select key_a || '|' || key_b from video_version_unbundles order by 1`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var pairs []string
	for rows.Next() {
		var pair string
		if err := rows.Scan(&pair); err != nil {
			t.Fatal(err)
		}
		pairs = append(pairs, pair)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return pairs
}

func TestJudgementEntryTurnsFromNoConnectionIntoResult(t *testing.T) {
	db, _ := judgementFixture(t)
	first := judgePair(t, db, domain.VersionJudgeResult{Verdict: domain.VersionNoConnection, Decider: domain.ActivityByRule})
	expectEntryState(t, first, domain.ActivityState{Status: domain.ActivityAttention, Reason: domain.ActivityNoConnection},
		domain.ActivityByRule)
	if first.Subject.Label != "a" || first.Subject.LabelB != "b" {
		t.Errorf("表示名 = %q, %q, want a, b", first.Subject.Label, first.Subject.LabelB)
	}

	if _, err := db.Versions().ResetNoConnectionJudgements(context.Background()); err != nil {
		t.Fatal(err)
	}
	waiting := onlyPairEntry(t, db)
	expectEntryState(t, waiting, activityWaiting, domain.ActivityByRule)

	done := judgePair(t, db, domain.VersionJudgeResult{
		Verdict: domain.VersionSame, Probability: probabilityOf(0.9), Decider: domain.ActivityByAI,
	})
	if done.ID != first.ID {
		t.Errorf("結果の記録 = %d, want 同じ記録 %d", done.ID, first.ID)
	}
	expectEntryState(t, done, domain.ActivityState{Status: domain.ActivityDone, Outcome: domain.ActivitySame}, domain.ActivityByAI)
	if done.Probability == nil || *done.Probability != 0.9 {
		t.Errorf("確率 = %v, want 0.9", done.Probability)
	}
	if !done.UndoOffered() {
		t.Error("自動で束ねた組が取り消しを出さない")
	}
}

func TestJudgementEntryOfFrameRuleBundleIsRule(t *testing.T) {
	db, _ := judgementFixture(t)
	entry := judgePair(t, db, domain.VersionJudgeResult{Verdict: domain.VersionSame, Decider: domain.ActivityByRule})
	expectEntryState(t, entry, domain.ActivityState{Status: domain.ActivityDone, Outcome: domain.ActivitySame},
		domain.ActivityByRule)
	if entry.Probability != nil {
		t.Errorf("確率 = %v, want なし", *entry.Probability)
	}
}

func TestJudgementEntryRetryReturnsSameEntryToRunning(t *testing.T) {
	db, _ := judgementFixture(t)
	failed := judgePair(t, db, domain.VersionJudgeResult{Verdict: domain.VersionFailed, Error: "timeout"})
	expectEntryState(t, failed, activityFailed, domain.ActivityByRule)
	if failed.Detail != "timeout" || failed.RetryAt.IsZero() || !failed.RetryOffered() {
		t.Errorf("失敗の記録 = detail %q, retryAt %v, retry %v", failed.Detail, failed.RetryAt, failed.RetryOffered())
	}
	expectNoClaim(t, db, "再試行の時刻の前")

	if err := db.Versions().RetryJudgement(context.Background(), failed.ID); err != nil {
		t.Fatalf("RetryJudgement: %v", err)
	}
	retried := onlyPairEntry(t, db)
	expectEntryState(t, retried, activityWaiting, domain.ActivityByRule)
	done := judgePair(t, db, domain.VersionJudgeResult{
		Verdict: domain.VersionDifferent, Probability: probabilityOf(0.1), Decider: domain.ActivityByAI,
	})
	if done.ID != failed.ID || done.Outcome != domain.ActivityDifferent {
		t.Errorf("やり直した結果 = 記録 %d %s, want 記録 %d different", done.ID, done.Outcome, failed.ID)
	}
	if err := db.Versions().RetryJudgement(context.Background(), done.ID); !errors.Is(err, domain.ErrActivityNotRetryable) {
		t.Errorf("done の記録のやり直し = %v, want ErrActivityNotRetryable", err)
	}
}

func TestUndoAutomaticBundleSplitsTheJoinedVideo(t *testing.T) {
	db, ids := judgementFixture(t)
	a, b := ids["a"], ids["b"]
	entry := judgePair(t, db, domain.VersionJudgeResult{Verdict: domain.VersionSame, Decider: domain.ActivityByRule})
	folder := createSmartFolder(t, db, "版", 0)
	recorder := &eventRecorder{}
	db.PublishTo(recorder)

	undo := undoEntry(t, db, entry.ID)
	if undo.Outcome != domain.ActivityCompleted || undo.Undoes == nil || undo.Undoes.Outcome != domain.ActivitySame {
		t.Errorf("undo の記録 = %s, undoes %+v, want completed で same を取り消す", undo.Outcome, undo.Undoes)
	}
	if got, want := smartFolderNotices(recorder), []domain.SmartFoldersChanged{{IDs: []int64{folder.ID}}}; !reflect.DeepEqual(got, want) {
		t.Errorf("スマートフォルダの知らせ = %+v, want %+v", got, want)
	}
	for _, id := range []int64{a, b} {
		if ownerVideo(t, db, id).Versions != nil {
			t.Errorf("動画 %d がまだ集まりに属する", id)
		}
	}
	if pairs := unbundledPairs(t, db); !slices.Equal(pairs, []string{"key-a|key-b"}) {
		t.Errorf("外した組 = %v, want [key-a|key-b]", pairs)
	}
	expectNoClaim(t, db, "取り消した束ねの組")
	undone := onlyPairEntry(t, db)
	if undone.UndoneBy != undo.ID || undone.UndoOffered() {
		t.Errorf("取り消した記録 = undoneBy %d, undoable %v, want %d で取り消しを出さない", undone.UndoneBy,
			undone.UndoOffered(), undo.ID)
	}
	if _, err := db.Activity().UndoVersionJudgement(context.Background(), entry.ID); !errors.Is(err, domain.ErrActivityNotUndoable) {
		t.Errorf("2 度目の取り消し = %v, want ErrActivityNotUndoable", err)
	}
}

func TestUndoAutomaticDismissalReturnsThePairToCandidates(t *testing.T) {
	db, ids := judgementFixture(t)
	a, b := ids["a"], ids["b"]
	entry := judgePair(t, db, domain.VersionJudgeResult{
		Verdict: domain.VersionDifferent, Probability: probabilityOf(0.05), Decider: domain.ActivityByAI,
	})
	if listed, _ := candidatePairs(t, db); len(listed) != 0 {
		t.Fatalf("却下したあとの候補 = %v, want なし", listed)
	}

	if undo := undoEntry(t, db, entry.ID); undo.Outcome != domain.ActivityCompleted {
		t.Errorf("undo の結果 = %s, want completed", undo.Outcome)
	}
	if listed, _ := candidatePairs(t, db); len(listed) != 1 || listed[0] != [2]int64{min(a, b), max(a, b)} {
		t.Errorf("取り消したあとの候補 = %v, want [[%d %d]]", listed, min(a, b), max(a, b))
	}
	expectNoClaim(t, db, "判定を保留した組")
	if entries := pairEntries(t, db); len(entries) != 1 {
		t.Errorf("候補に戻した組に記録を開いた: %+v", entries)
	}
}

// Automatic undo must preserve a later owner decision for the same pair.
func TestUndoAutomaticDismissalKeepsTheOwnersLaterDismissal(t *testing.T) {
	db, ids := judgementFixture(t)
	a, b := ids["a"], ids["b"]
	entry := judgePair(t, db, domain.VersionJudgeResult{
		Verdict: domain.VersionDifferent, Probability: probabilityOf(0.05), Decider: domain.ActivityByAI,
	})
	if err := db.Versions().Dismiss(context.Background(), [2]int64{a, b}); err != nil {
		t.Fatal(err)
	}

	if undo := undoEntry(t, db, entry.ID); undo.Outcome != domain.ActivityAlreadyReverted {
		t.Errorf("undo の結果 = %s, want already_reverted", undo.Outcome)
	}
	if listed, _ := candidatePairs(t, db); len(listed) != 0 {
		t.Errorf("取り消したあとの候補 = %v, want なし（所有者の「違う動画」が残る）", listed)
	}
	var dismissals int
	if err := db.sql.QueryRow(`select count(*) from video_version_dismissals`).Scan(&dismissals); err != nil {
		t.Fatal(err)
	}
	if dismissals != 1 {
		t.Errorf("却下 = %d 行, want 1", dismissals)
	}
}

func TestUndoAfterOwnerUnbundledIsAlreadyReverted(t *testing.T) {
	db, ids := judgementFixture(t)
	a, b := ids["a"], ids["b"]
	entry := judgePair(t, db, domain.VersionJudgeResult{Verdict: domain.VersionSame, Decider: domain.ActivityByRule})
	if _, err := db.Versions().Unbundle(context.Background(), b); err != nil {
		t.Fatal(err)
	}
	bundles := bundleCount(t, db)

	undo := undoEntry(t, db, entry.ID)
	if undo.Outcome != domain.ActivityAlreadyReverted {
		t.Errorf("undo の結果 = %s, want already_reverted", undo.Outcome)
	}
	if got := bundleCount(t, db); got != bundles || ownerVideo(t, db, a).Versions != nil {
		t.Errorf("取り消しがまとまりを変えた: 集まり %d → %d", bundles, got)
	}
}

func TestOwnerDecisionClosesTheOpenPairEntry(t *testing.T) {
	for _, dismiss := range []bool{false, true} {
		t.Run(map[bool]string{false: "bundle", true: "dismiss"}[dismiss], func(t *testing.T) {
			db, ids := judgementFixture(t)
			a, b := ids["a"], ids["b"]
			undecided := judgePair(t, db, domain.VersionJudgeResult{
				Verdict: domain.VersionUndecided, Probability: probabilityOf(0.5), Decider: domain.ActivityByAI,
			})
			expectEntryState(t, undecided, activityUndecided, domain.ActivityByAI)
			if dismiss {
				if err := db.Versions().Dismiss(context.Background(), [2]int64{a, b}); err != nil {
					t.Fatal(err)
				}
			} else {
				bundle(t, db, a, a, b)
			}
			closed := onlyPairEntry(t, db)
			expectEntryState(t, closed, domain.ActivityState{Status: domain.ActivityDone, Outcome: domain.ActivityDecidedByOwner},
				domain.ActivityByOwner)
			if closed.ID != undecided.ID || closed.UndoOffered() {
				t.Errorf("閉じた記録 = %d（取り消し %v）, want %d で取り消しを出さない", closed.ID, closed.UndoOffered(), undecided.ID)
			}
		})
	}
}

func TestRebuildingCandidatesKeepsThePairEntry(t *testing.T) {
	db, ids := judgementFixture(t)
	undecided := judgePair(t, db, domain.VersionJudgeResult{Verdict: domain.VersionUndecided, Decider: domain.ActivityByRule})

	applyFingerprint(t, db, ids["a"], testFingerprint(1, 0))
	rebuilt := onlyPairEntry(t, db)
	if rebuilt.ID != undecided.ID {
		t.Errorf("作り直したあとの記録 = %d, want 同じ記録 %d", rebuilt.ID, undecided.ID)
	}
	expectEntryState(t, rebuilt, activityWaiting, domain.ActivityByRule)
	var attached int64
	if err := db.sql.QueryRow(`select activity_id from video_version_candidates where key_a = 'key-a' and key_b = 'key-b'`).
		Scan(&attached); err != nil {
		t.Fatal(err)
	}
	if attached != undecided.ID {
		t.Errorf("候補の activity_id = %d, want %d", attached, undecided.ID)
	}
	if again := judgePair(t, db, domain.VersionJudgeResult{Verdict: domain.VersionSame}); again.ID != undecided.ID {
		t.Errorf("作り直したあとの判定の記録 = %d, want %d", again.ID, undecided.ID)
	}
}

func TestRebuildingCandidatesClosesTheDroppedPairEntry(t *testing.T) {
	db, ids := judgementFixture(t)
	undecided := judgePair(t, db, domain.VersionJudgeResult{
		Verdict: domain.VersionUndecided, Decider: domain.ActivityByAI, Probability: probabilityOf(0.5),
	})

	applyFingerprint(t, db, ids["a"], testFingerprint(3, 0))
	var candidates int
	if err := db.sql.QueryRow(`select count(*) from video_version_candidates where key_a = 'key-a' and key_b = 'key-b'`).
		Scan(&candidates); err != nil {
		t.Fatal(err)
	}
	if candidates != 0 {
		t.Fatalf("作り直したあとの (key-a, key-b) の候補 = %d 行, want 0", candidates)
	}
	closed := onlyPairEntry(t, db)
	if closed.ID != undecided.ID {
		t.Errorf("閉じた記録 = %d, want %d", closed.ID, undecided.ID)
	}
	expectEntryState(t, closed, domain.ActivityState{Status: domain.ActivityDone, Outcome: domain.ActivitySuperseded},
		domain.ActivityByAI)
}

func TestJudgingSettingOpensWaitingEntriesAndGoneClosesThem(t *testing.T) {
	db, ids := judgementFixture(t)
	ctx := context.Background()
	if entries := pairEntries(t, db); len(entries) != 0 {
		t.Fatalf("判定が切のあいだの記録 = %+v, want なし", entries)
	}
	if err := db.Settings().SaveVersionJudgingEnabled(ctx, true); err != nil {
		t.Fatal(err)
	}
	opened := onlyPairEntry(t, db)
	expectEntryState(t, opened, activityWaiting, domain.ActivityByRule)

	fingerprintVideo(t, db, ids["d"], 100_000, testFingerprint(1, 0))
	if entries := pairEntries(t, db); len(entries) != 3 {
		t.Errorf("新しい候補のあとの記録 = %d 件, want 3（a-b, a-d, b-d）", len(entries))
	}
	if err := db.Settings().SaveVersionJudgingEnabled(ctx, false); err != nil {
		t.Fatal(err)
	}
	for _, entry := range pairEntries(t, db) {
		expectEntryState(t, entry, activityWaiting, domain.ActivityByRule)
	}

	if err := db.ScanIndex().DeleteVideos(ctx, []int64{ids["d"]}); err != nil {
		t.Fatal(err)
	}
	for _, entry := range pairEntries(t, db) {
		gone := entry.Subject.Key == "key-d" || entry.Subject.KeyB == "key-d"
		want := activityWaiting
		if gone {
			want = domain.ActivityState{Status: domain.ActivityDone, Outcome: domain.ActivityGone}
		}
		expectEntryState(t, entry, want, domain.ActivityByRule)
	}
}

func TestInterruptedPairEntryReturnsToRunningOnClaim(t *testing.T) {
	db, _ := judgementFixture(t)
	ctx := context.Background()
	claimJudgement(t, db)
	interrupted := onlyPairEntry(t, db)
	if _, err := db.Activity().RecoverInterruptedActivity(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Versions().RecoverRunningJudgements(ctx); err != nil {
		t.Fatal(err)
	}
	expectEntryState(t, onlyPairEntry(t, db), domain.ActivityState{
		Status: domain.ActivityAttention, Reason: domain.ActivityInterrupted,
	}, domain.ActivityByRule)

	claimJudgement(t, db)
	claimed := onlyPairEntry(t, db)
	if claimed.ID != interrupted.ID {
		t.Errorf("取り出し直した記録 = %d, want %d", claimed.ID, interrupted.ID)
	}
	expectEntryState(t, claimed, activityClaimed, domain.ActivityByRule)
	if _, err := db.Versions().RecoverRunningJudgements(ctx); err != nil {
		t.Fatal(err)
	}
	expectEntryState(t, onlyPairEntry(t, db), activityWaiting, domain.ActivityByRule)
}

func TestSuccessionRekeysHoldsAndOpenPairEntries(t *testing.T) {
	db, _ := judgementFixture(t)
	ctx := context.Background()
	claimJudgement(t, db)
	if _, err := db.sql.Exec(`insert into video_version_judging_holds (key_a, key_b, created_at) values ('key-a', 'key-b', 1)`); err != nil {
		t.Fatal(err)
	}
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := moveKeyPairs(ctx, tx, "video_version_judging_holds", "key-b", "key-0"); err != nil {
		t.Fatal(err)
	}
	moved, err := movePairActivityKey(ctx, tx, "key-b", "key-0", newActivityClock().next())
	if err != nil || !moved {
		t.Fatalf("movePairActivityKey = %v, %v", moved, err)
	}
	var holdA, holdB, key, keyB, label, labelB string
	if err := tx.QueryRow(`select key_a, key_b from video_version_judging_holds`).Scan(&holdA, &holdB); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(`select subject_key, subject_key_b, subject_label, subject_label_b from activity_entries
		where kind = 'version_judgement'`).Scan(&key, &keyB, &label, &labelB); err != nil {
		t.Fatal(err)
	}
	if holdA != "key-0" || holdB != "key-a" || key != "key-0" || keyB != "key-a" || label != "b" || labelB != "a" {
		t.Errorf("付け替えた保留 = %s|%s, 記録 = %s|%s（%s, %s）, want key-0|key-a（b, a）", holdA, holdB, key, keyB, label, labelB)
	}
}

// Undo restores both original representatives and records only cross-side unbundled pairs.
func TestUndoAutomaticBundleOfTwoBundlesRestoresBoth(t *testing.T) {
	db, ids := judgementFixture(t)
	a, b, c, d := ids["a"], ids["b"], ids["c"], ids["d"]
	bundle(t, db, b, b, d)
	bundle(t, db, a, a, c)
	attachNamedTag(t, db, "kept", b)
	entry := judgePair(t, db, domain.VersionJudgeResult{Verdict: domain.VersionSame, Decider: domain.ActivityByRule})
	if versions, err := db.Versions().Versions(context.Background(), domain.AudienceOwner, a); err != nil || len(versions.Items) != 4 {
		t.Fatalf("自動で束ねたまとまり = %+v, %v, want 4 本", versions, err)
	}

	undoEntry(t, db, entry.ID)
	for _, want := range []struct {
		representative int64
		members        []int64
	}{{b, []int64{b, d}}, {a, []int64{a, c}}} {
		versions, err := db.Versions().Versions(context.Background(), domain.AudienceOwner, want.members[1])
		if err != nil {
			t.Fatal(err)
		}
		var got []int64
		for _, item := range versions.Items {
			got = append(got, item.ID)
		}
		slices.Sort(got)
		wantMembers := slices.Sorted(slices.Values(want.members))
		if versions.RepresentativeID != want.representative || !slices.Equal(got, wantMembers) {
			t.Errorf("取り消したあとのまとまり = 代表 %d %v, want 代表 %d %v", versions.RepresentativeID, got,
				want.representative, wantMembers)
		}
	}
	if pairs := unbundledPairs(t, db); !slices.Equal(pairs, []string{"key-a|key-b", "key-a|key-d", "key-b|key-c", "key-c|key-d"}) {
		t.Errorf("外した組 = %v, want 両側をまたぐ 4 組", pairs)
	}
	for _, id := range []int64{a, b} {
		if names := manualTagNames(t, db, ownerVideo(t, db, id).UserKey); !slices.Contains(names, "kept") {
			t.Errorf("動画 %d のまとまりのタグ = %v, want kept を含む", id, names)
		}
	}
}

func TestSuccessionKeepsTheOpenPairEntryUntilDecided(t *testing.T) {
	for name, c := range map[string]struct {
		durationMs int64
		carried    bool
	}{
		"尺が同じ":  {100_400, true},
		"尺が幅の外": {150_000, false},
	} {
		t.Run(name, func(t *testing.T) {
			db, _ := judgementFixture(t)
			failed := judgePair(t, db, domain.VersionJudgeResult{Verdict: domain.VersionFailed, Error: "timeout"})
			expectEntryState(t, failed, activityFailed, domain.ActivityByRule)

			scan := startTestScan(t, db)
			next := upsertOne(t, db, listingFile(fixturePath("/media/b.mp4"), "b", "key-e", 5))
			if waiting := onlyPairEntry(t, db); waiting.Status != domain.ActivityAttention {
				t.Errorf("引き継ぎを待つ記録 = %s/%s%s, want attention のまま", waiting.Status, waiting.Reason, waiting.Outcome)
			}
			finishTestScan(t, db, scan, domain.ScanDone)
			probeDuration(t, db, next, c.durationMs)

			entry := onlyPairEntry(t, db)
			if entry.ID != failed.ID {
				t.Errorf("記録 = %d, want 同じ記録 %d", entry.ID, failed.ID)
			}
			if c.carried {
				expectEntryState(t, entry, activityFailed, domain.ActivityByRule)
				if entry.Subject.Key != "key-a" || entry.Subject.KeyB != "key-e" {
					t.Errorf("引き継いだ記録の組 = %s|%s, want key-a|key-e", entry.Subject.Key, entry.Subject.KeyB)
				}
			} else {
				expectEntryState(t, entry, domain.ActivityState{Status: domain.ActivityDone, Outcome: domain.ActivityGone},
					domain.ActivityByRule)
			}
		})
	}
}

// Keep owner edits in the same second to prove undo compares activity IDs rather than timestamps.
func TestUndoAfterOwnerRebundledIsAlreadyReverted(t *testing.T) {
	db, ids := judgementFixture(t)
	a, b := ids["a"], ids["b"]
	entry := judgePair(t, db, domain.VersionJudgeResult{Verdict: domain.VersionSame, Decider: domain.ActivityByRule})
	if _, err := db.Versions().Unbundle(context.Background(), b); err != nil {
		t.Fatal(err)
	}
	bundle(t, db, a, a, b)

	if undo := undoEntry(t, db, entry.ID); undo.Outcome != domain.ActivityAlreadyReverted {
		t.Errorf("undo の結果 = %s, want already_reverted", undo.Outcome)
	}
	if versions := ownerVideo(t, db, b).Versions; versions == nil || versions.Count != 2 {
		t.Errorf("所有者が束ね直したまとまり = %+v, want a と b の 2 本のまま", versions)
	}
}

func TestOrganizeBundleUndoReopensThePairEntry(t *testing.T) {
	db, ids := judgementFixture(t)
	ctx := context.Background()
	undecided := judgePair(t, db, domain.VersionJudgeResult{
		Verdict: domain.VersionUndecided, Probability: probabilityOf(0.5), Decider: domain.ActivityByAI,
	})
	image, err := db.Organize().Bundle(ctx, []int64{ids["b"]}, ids["a"])
	if err != nil {
		t.Fatal(err)
	}
	expectEntryState(t, onlyPairEntry(t, db), domain.ActivityState{
		Status: domain.ActivityDone, Outcome: domain.ActivityDecidedByOwner,
	}, domain.ActivityByOwner)

	recorder := &eventRecorder{}
	db.PublishTo(recorder)
	if err := db.Organize().Undo(ctx, image); err != nil {
		t.Fatal(err)
	}
	reopened := onlyPairEntry(t, db)
	if reopened.ID != undecided.ID {
		t.Errorf("開き直した記録 = %d, want 同じ記録 %d", reopened.ID, undecided.ID)
	}
	expectEntryState(t, reopened, activityUndecided, domain.ActivityByAI)
	if reopened.Probability == nil || *reopened.Probability != 0.5 {
		t.Errorf("開き直した記録の確率 = %v, want 0.5", reopened.Probability)
	}
	if !slices.ContainsFunc(recorder.events, func(event domain.Event) bool {
		_, ok := event.(domain.ActivityChanged)
		return ok
	}) {
		t.Errorf("取り消しの知らせ = %v, want ActivityChanged を含む", recorder.events)
	}
}

// Owner edits during processing precede the decision mark, even though they follow the activity's creation ID.
func TestUndoAutomaticBundleAfterTheOwnerBundledWhileItWaited(t *testing.T) {
	db, ids := judgementFixture(t)
	a, b, c := ids["a"], ids["b"], ids["c"]
	job := claimJudgement(t, db)
	bundle(t, db, a, a, c)
	if _, err := db.Versions().Unbundle(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	applyJudgement(t, db, job.Keys, domain.VersionJudgeResult{Verdict: domain.VersionSame, Decider: domain.ActivityByRule})
	entry := onlyPairEntry(t, db)
	if entry.Outcome != domain.ActivitySame {
		t.Fatalf("判定の記録 = %+v, want done/same", entry)
	}

	undo := undoEntry(t, db, entry.ID)
	if undo.Outcome != domain.ActivityCompleted {
		t.Errorf("undo の結果 = %s, want completed", undo.Outcome)
	}
	for _, id := range []int64{a, b} {
		if ownerVideo(t, db, id).Versions != nil {
			t.Errorf("動画 %d がまだ集まりに属する", id)
		}
	}
}

func succeedA(t *testing.T, db *DB) int64 {
	t.Helper()
	scan := startTestScan(t, db)
	next := upsertOne(t, db, listingFile(fixturePath("/media/a.mp4"), "a", "key-e", 5))
	finishTestScan(t, db, scan, domain.ScanDone)
	probeDuration(t, db, next, 100_000)
	return next
}

func TestUndoAutoTagAfterTheSubjectWasRekeyedBySuccession(t *testing.T) {
	db, ids := judgementFixture(t)
	ctx := context.Background()
	cat := createTestTag(t, db, "cat")
	entry := runAutoTag(t, db, ids["a"], domain.ActivityByRule, domain.AutoTagSelection{
		Confirmed: []int64{cat.ID}, Probabilities: map[int64]float64{cat.ID: 0.91},
	})
	next := succeedA(t, db)
	if got := manualTagIDs(t, db, "key-e"); !slices.Equal(got, []int64{cat.ID}) {
		t.Fatalf("引き継いだ鍵のタグ = %v, want cat", got)
	}

	undo, err := db.Activity().UndoAutoTag(ctx, entry.ID, cat.ID)
	if err != nil {
		t.Fatal(err)
	}
	if undo.Outcome != domain.ActivityCompleted || undo.Subject.Key != "key-e" {
		t.Fatalf("undo の記録 = %s（対象 %s）, want completed（key-e）", undo.Outcome, undo.Subject.Key)
	}
	if got := manualTagIDs(t, db, "key-e"); len(got) != 0 {
		t.Errorf("引き継いだ鍵に取り消したタグが残った: %v", got)
	}
	if !rejectedFor(t, db, "key-e", cat.ID) {
		t.Error("引き継いだ鍵に却下が残っていない")
	}
	rekeyed, err := db.Activity().ActivityEntry(ctx, entry.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rekeyed.Subject.Key != "key-e" || len(rekeyed.VideoIDs) == 0 || rekeyed.VideoIDs[0] != next {
		t.Errorf("付け替えた記録 = 対象 %s, 動画 %v, want key-e の動画 %d", rekeyed.Subject.Key, rekeyed.VideoIDs, next)
	}
}

func TestUndoAutomaticBundleAfterTheJoinedKeyWasRekeyedBySuccession(t *testing.T) {
	db, _ := judgementFixture(t)
	entry := judgePair(t, db, domain.VersionJudgeResult{Verdict: domain.VersionSame, Decider: domain.ActivityByRule})
	joined, err := domain.DecodeVersionBundleUndoKeys(entry.UndoKeys)
	if err != nil || !slices.Equal(joined.Members, []string{"key-a"}) {
		t.Fatalf("加わった側 = %+v, %v, want key-a", joined, err)
	}
	next := succeedA(t, db)
	if ownerVideo(t, db, next).Versions == nil {
		t.Fatal("引き継いだ動画がまとまりにいない")
	}

	undo := undoEntry(t, db, entry.ID)
	if undo.Outcome != domain.ActivityCompleted || undo.Subject.Key != "key-b" || undo.Subject.KeyB != "key-e" {
		t.Errorf("undo の記録 = %s（%s|%s）, want completed（key-b|key-e）", undo.Outcome, undo.Subject.Key, undo.Subject.KeyB)
	}
	if ownerVideo(t, db, next).Versions != nil {
		t.Error("引き継いだ動画がまだまとまりに属する")
	}
	if pairs := unbundledPairs(t, db); !slices.Equal(pairs, []string{"key-b|key-e"}) {
		t.Errorf("外した組 = %v, want [key-b|key-e]", pairs)
	}
}

// Adding members does not change ownership of the representative's existing members, which remain eligible for automatic undo.
func TestUndoAutomaticBundleAfterTheOwnerAddedAMember(t *testing.T) {
	db, ids := judgementFixture(t)
	a, b, c := ids["a"], ids["b"], ids["c"]
	entry := judgePair(t, db, domain.VersionJudgeResult{Verdict: domain.VersionSame, Decider: domain.ActivityByRule})
	joined, err := domain.DecodeVersionBundleUndoKeys(entry.UndoKeys)
	if err != nil || !slices.Equal(joined.Members, []string{"key-a"}) {
		t.Fatalf("加わった側 = %+v, %v, want key-a", joined, err)
	}
	bundle(t, db, b, b, c)
	expectOwnerEntries(t, db, 0, ownerEntry{domain.ActivityBundle, "key-c", "", "c", ""})

	undo := undoEntry(t, db, entry.ID)
	if undo.Outcome != domain.ActivityCompleted {
		t.Errorf("undo の結果 = %s, want completed", undo.Outcome)
	}
	if ownerVideo(t, db, a).Versions != nil {
		t.Error("加わった側の a がまだ集まりに属する")
	}
	for _, id := range []int64{b, c} {
		if versions := ownerVideo(t, db, id).Versions; versions == nil || versions.Count != 2 {
			t.Errorf("所有者が加えたまとまり（動画 %d） = %+v, want b と c の 2 本", id, versions)
		}
	}
}
