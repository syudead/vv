package store

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// 重複の候補の判定の記録と、自動の束ねと「違う動画」の取り消し（specs/055-activity/data-model.md「Rules」、
// research.md R-4・R-5・R-7）。

// pairEntries は version_judgement の記録を新しい順に返す。
func pairEntries(t *testing.T, db *DB) []domain.ActivityEntry {
	t.Helper()
	page := listActivity(t, db, domain.ActivityQuery{Features: []domain.ActivityFeature{domain.ActivityFeatureDuplicateCheck}}, "", 100)
	var entries []domain.ActivityEntry
	for _, entry := range page.Items {
		if entry.Kind == domain.ActivityVersionJudgement {
			entries = append(entries, entry)
		}
	}
	return entries
}

// onlyPairEntry は version_judgement の記録がちょうど 1 件であることを確かめて返す。
func onlyPairEntry(t *testing.T, db *DB) domain.ActivityEntry {
	t.Helper()
	entries := pairEntries(t, db)
	if len(entries) != 1 {
		t.Fatalf("version_judgement の記録 = %d 件, want 1: %+v", len(entries), entries)
	}
	return entries[0]
}

// expectEntryState は記録 entry の状態・理由か結果・判断したものを確かめる。
func expectEntryState(t *testing.T, entry domain.ActivityEntry, state domain.ActivityState, decider domain.ActivityDecider) {
	t.Helper()
	if entry.Status != state.Status || entry.Reason != state.Reason || entry.Outcome != state.Outcome ||
		entry.Decider != decider {
		t.Errorf("記録 = %s/%s%s（%s）, want %s/%s%s（%s）", entry.Status, entry.Reason, entry.Outcome, entry.Decider,
			state.Status, state.Reason, state.Outcome, decider)
	}
}

func probabilityOf(value float64) *float64 { return &value }

// judgePair は (key-a, key-b) を取り出して result を反映し、その組の記録を返す。
func judgePair(t *testing.T, db *DB, result domain.VersionJudgeResult) domain.ActivityEntry {
	t.Helper()
	job := claimJudgement(t, db)
	if entry := onlyPairEntry(t, db); entry.Status != domain.ActivityRunning || entry.Reason != domain.ActivityProcessing {
		t.Errorf("取り出した組の記録 = %s/%s, want running/processing", entry.Status, entry.Reason)
	}
	applyJudgement(t, db, job.Keys, result)
	return onlyPairEntry(t, db)
}

// undoEntry は記録 id を取り消し、所有者の undo の記録を返す。
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

// unbundledPairs は video_version_unbundles の組を "key_a|key_b" の順で返す。
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

// モデルが要る組を接続なしで判定すると attention/no_connection になり、接続を保存して 0.9 の答えで束ねると、
// 同じ記録が done/same、ai、確率 0.9 になる（親 Issue の受け入れ条件 3）。
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

// コマの規則で束ねた組は rule で、確率を持たない（research.md R-5）。
func TestJudgementEntryOfFrameRuleBundleIsRule(t *testing.T) {
	db, _ := judgementFixture(t)
	entry := judgePair(t, db, domain.VersionJudgeResult{Verdict: domain.VersionSame, Decider: domain.ActivityByRule})
	expectEntryState(t, entry, domain.ActivityState{Status: domain.ActivityDone, Outcome: domain.ActivitySame},
		domain.ActivityByRule)
	if entry.Probability != nil {
		t.Errorf("確率 = %v, want なし", *entry.Probability)
	}
}

// 失敗した組は attention/failed で再試行の時刻を持ち、やり直すと同じ記録が待ちに戻ってすぐ取り出せる
// （contracts/screen-api.md「POST /api/activity/{id}/retry」）。
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

// 自動の束ねを取り消すと、加わった動画がまとまりから外れ、その組は二度と取り出されない。2 度目の取り消しは
// 出さない（research.md R-7）。
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
	// まとまりから外れるとスマートフォルダの一致が変わりうるので、すべてのフォルダを 1 度名指す。
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

// 自動の「違う動画」を取り消すと、組が候補の一覧に戻り、ワーカーはそれを取り出さない（research.md R-7）。
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

// 自動の「違う動画」の後に所有者が同じ組を「違う動画」と決め直していれば、取り消しは already_reverted で、
// 却下を消さず組を候補に戻さない（所有者の判断を自動の判断の取り消しで失わない）。
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

// 所有者が束ねを外した後の取り消しは already_reverted で、まとまりを変えない（親 Issue の Edge Cases）。
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

// 決まらなかった組を所有者が手で束ねると、その記録は decided_by_owner で閉じる。手の「違う動画」も同じ。
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

// 1 つの鍵の候補を作り直しても、組の記録は保たれ、候補が付き直して待ちに戻る（data-model.md「Rules」）。
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

// 指紋の作り直しで組が候補から外れると、両方の鍵がライブラリにあっても組の開いている記録は done/superseded で
// 閉じ、判断したものは変えない（data-model.md「Outcomes」）。
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

// 判定を入れると待っている判定できる候補に記録を開き、入のあいだに現れた候補にも開く。切っても待ちのまま残り、
// 鍵の動画がライブラリから外れると gone で閉じる（data-model.md「Kinds」「Rules」）。
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

	// 入のあいだに現れた候補（d が a と同じ指紋を得る）にも記録を開く。
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

// 起動時に判定中だった組の記録は attention/interrupted になり、判定中の行を戻しても残って、次の取り出しで同じ
// 記録が running に戻る（research.md R-4 の Deviation）。
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
	// 判定のあいだに切られて行を待ちに戻すと、記録も待ちに戻る。
	if _, err := db.Versions().RecoverRunningJudgements(ctx); err != nil {
		t.Fatal(err)
	}
	expectEntryState(t, onlyPairEntry(t, db), activityWaiting, domain.ActivityByRule)
}

// 同じパスの中身が変わって引き継ぐと、判定の保留と開いている記録の鍵も付け替える（data-model.md「Rules」）。
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

// 所有者のまとまり {B1, B2}（代表 B1）と {A, C} を自動で束ねた後に取り消すと、両方のまとまりが元の代表のまま
// 残り、video_version_unbundles は両側をまたぐ組だけに書かれる（research.md R-7）。
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
	// 外れた側も、合わさったまとまりのタグの写しを持つ。
	for _, id := range []int64{a, b} {
		if names := manualTagNames(t, db, ownerVideo(t, db, id).UserKey); !slices.Contains(names, "kept") {
			t.Errorf("動画 %d のまとまりのタグ = %v, want kept を含む", id, names)
		}
	}
}

// 同じパスの中身が変わっても、前の中身の組の開いている記録は引き継ぎを決めるまで閉じない。尺が合って引き継ぐと
// 同じ記録が新しい鍵の組になり、引き継がないと決まったときだけ gone で閉じる（data-model.md「Rules」）。
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

// 自動の束ねの後に所有者が加わった動画を外して手で束ね直したら、古い記録の取り消しは already_reverted で、
// 所有者のまとまりを変えない（research.md R-7）。所有者の操作は判定と同じ秒の中で行い、取り消しが時刻ではなく
// 所有者の記録の id で見分けることを確かめる。
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

// ドラッグで束ねて閉じた組の記録は、そのドラッグを取り消すと、戻った候補の判定の状態で同じ記録が開き直す
// （data-model.md「Rules」）。
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

// 判定中に所有者が加わる側の動画を別のまとまりに束ねて外しても、その記録は判定の結果より前なので、取り消しは判定が
// 束ねた側を外す。記録は取り出したときに開くので、所有者の記録の id は記録の id より大きいが、結果を書いた印
// （decided_mark）より小さい（research.md R-7）。
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

// succeedA は key-a の動画の同じパスの中身を key-e に替え、尺が合って引き継がれるところまで進め、新しい動画の id を
// 返す。
func succeedA(t *testing.T, db *DB) int64 {
	t.Helper()
	scan := startTestScan(t, db)
	next := upsertOne(t, db, listingFile(fixturePath("/media/a.mp4"), "a", "key-e", 5))
	finishTestScan(t, db, scan, domain.ScanDone)
	probeDuration(t, db, next, 100_000)
	return next
}

// 同じパスの中身が変わって引き継がれると、終わった記録の対象の鍵も新しい鍵になるので、その後の自動タグの取り消し
// は新しい鍵の行を見つけて消す（data-model.md「Rules」、research.md R-7）。
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

// 自動で束ねた後に加わった側の中身が同じパスで変わって引き継がれると、記録の組と加わった側の鍵も新しい鍵になり、
// 取り消しは新しい鍵のメンバーを外す。
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
