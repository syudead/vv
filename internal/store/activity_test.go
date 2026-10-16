package store

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// アクティビティの記録の書き込み・一覧・回復（specs/055-activity/data-model.md、research.md R-2・R-4・R-9）。

// activityClock は記録の作成時刻を 1 分ずつ進める。
type activityClock struct{ now time.Time }

func (c *activityClock) next() time.Time {
	c.now = c.now.Add(time.Minute)
	return c.now
}

func newActivityClock() *activityClock {
	return &activityClock{now: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}
}

// writeActivity は記録 entry を 1 つの取引で書き、その id を返す。
func writeActivity(t *testing.T, db *DB, entry domain.ActivityEntry, now time.Time) int64 {
	t.Helper()
	id, err := RecordActivityForTest(context.Background(), db, entry, now)
	if err != nil {
		t.Fatalf("recordActivity: %v", err)
	}
	return id
}

// videoActivity は動画 key の記録である。
func videoActivity(kind domain.ActivityKind, key, label string, state domain.ActivityState, decider domain.ActivityDecider) domain.ActivityEntry {
	return domain.ActivityEntry{
		Kind: kind, Subject: domain.ActivitySubject{Kind: domain.ActivitySubjectVideo, Key: key, Label: label},
		Status: state.Status, Reason: state.Reason, Outcome: state.Outcome, Decider: decider,
	}
}

var (
	activityDone      = domain.ActivityState{Status: domain.ActivityDone, Outcome: domain.ActivityCompleted}
	activityFailed    = domain.ActivityState{Status: domain.ActivityAttention, Reason: domain.ActivityFailed}
	activityWaiting   = domain.ActivityState{Status: domain.ActivityRunning, Reason: domain.ActivityWaiting}
	activityClaimed   = domain.ActivityState{Status: domain.ActivityRunning, Reason: domain.ActivityProcessing}
	activityUndecided = domain.ActivityState{Status: domain.ActivityAttention, Reason: domain.ActivityUndecided}
)

func listActivity(t *testing.T, db *DB, query domain.ActivityQuery, cursor string, limit int) domain.ActivityPage {
	t.Helper()
	page, err := db.Activity().ListActivity(context.Background(), domain.AudienceOwner, query, cursor, limit)
	if err != nil {
		t.Fatalf("ListActivity(%+v): %v", query, err)
	}
	return page
}

func activityIDs(page domain.ActivityPage) []int64 {
	ids := make([]int64, 0, len(page.Items))
	for _, entry := range page.Items {
		ids = append(ids, entry.ID)
	}
	return ids
}

// activityLog は絞り込みを確かめる 5 件の記録の id である。
type activityLog struct {
	importDone, importFailed, tagByOwner, judgement, undo int64
	since                                                 time.Time
}

// seedActivityLog は種類・状態・判断したもの・時刻・表示名の違う 5 件を古い順に書く。
func seedActivityLog(t *testing.T, db *DB) activityLog {
	t.Helper()
	clock := newActivityClock()
	var log activityLog
	log.importDone = writeActivity(t, db,
		videoActivity(domain.ActivityImport, "key-a", "Holiday.mp4", activityDone, domain.ActivityByRule), clock.next())
	log.importFailed = writeActivity(t, db,
		videoActivity(domain.ActivityImport, "key-b", "broken.mkv", activityFailed, domain.ActivityByOwner), clock.next())
	log.since = clock.next()
	log.tagByOwner = writeActivity(t, db,
		videoActivity(domain.ActivityTagAttach, "key-a", "Holiday.mp4", activityDone, domain.ActivityByOwner), log.since)
	same := 0.9
	log.judgement = writeActivity(t, db, domain.ActivityEntry{
		Kind: domain.ActivityVersionJudgement,
		Subject: domain.ActivitySubject{
			Kind: domain.ActivitySubjectPair, Key: "key-a", KeyB: "key-c", Label: "Holiday.mp4", LabelB: "ÉTÉ Trip.mp4",
		},
		Status: domain.ActivityDone, Outcome: domain.ActivitySame, Decider: domain.ActivityByAI, Probability: &same,
	}, clock.next())
	log.undo = writeActivity(t, db, domain.ActivityEntry{
		Kind: domain.ActivityUndo, UndoesID: log.judgement,
		Subject: domain.ActivitySubject{
			Kind: domain.ActivitySubjectPair, Key: "key-a", KeyB: "key-c", Label: "Holiday.mp4", LabelB: "ÉTÉ Trip.mp4",
		},
		Status: domain.ActivityDone, Outcome: domain.ActivityCompleted, Decider: domain.ActivityByOwner,
	}, clock.next())
	return log
}

// 受け入れ: 記録は新しい順に返り、status・feature・decider・since・q のそれぞれが一覧を絞る。
func TestListActivityNewestFirstAndFilters(t *testing.T) {
	db, _ := jobsFixture(t)
	log := seedActivityLog(t, db)

	all := listActivity(t, db, domain.ActivityQuery{}, "", 50)
	if want := []int64{log.undo, log.judgement, log.tagByOwner, log.importFailed, log.importDone}; !slices.Equal(activityIDs(all), want) {
		t.Fatalf("一覧 = %v, want %v", activityIDs(all), want)
	}
	if all.NextCursor != "" {
		t.Errorf("続きの無い一覧の nextCursor = %q", all.NextCursor)
	}

	for _, tc := range []struct {
		name  string
		query domain.ActivityQuery
		want  []int64
	}{
		{"status", domain.ActivityQuery{Status: domain.ActivityAttention}, []int64{log.importFailed}},
		{"feature", domain.ActivityQuery{Features: []domain.ActivityFeature{domain.ActivityFeatureImport}},
			[]int64{log.importFailed, log.importDone}},
		{"features", domain.ActivityQuery{Features: []domain.ActivityFeature{domain.ActivityFeatureTags, domain.ActivityFeatureImport}},
			[]int64{log.tagByOwner, log.importFailed, log.importDone}},
		{"decider", domain.ActivityQuery{Decider: domain.ActivityByOwner}, []int64{log.undo, log.tagByOwner, log.importFailed}},
		{"since", domain.ActivityQuery{Since: log.since}, []int64{log.undo, log.judgement, log.tagByOwner}},
		// 大文字と小文字を区別せず、組の 2 本目の表示名にも当たる。
		{"q", domain.ActivityQuery{Search: "BROKEN"}, []int64{log.importFailed}},
		{"q on the second label", domain.ActivityQuery{Search: "été"}, []int64{log.undo, log.judgement}},
		{"combined", domain.ActivityQuery{Status: domain.ActivityDone, Decider: domain.ActivityByRule, Search: "holiday"},
			[]int64{log.importDone}},
	} {
		if got := activityIDs(listActivity(t, db, tc.query, "", 50)); !slices.Equal(got, tc.want) {
			t.Errorf("%s: 一覧 = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// 受け入れ: counts は status を、features は feature を無視し、どちらもほかの絞り込みには従う。
func TestListActivityCountsIgnoreTheirOwnFilter(t *testing.T) {
	db, _ := jobsFixture(t)
	log := seedActivityLog(t, db)

	page := listActivity(t, db, domain.ActivityQuery{
		Status: domain.ActivityAttention, Features: []domain.ActivityFeature{domain.ActivityFeatureImport},
	}, "", 50)
	if !slices.Equal(activityIDs(page), []int64{log.importFailed}) {
		t.Fatalf("一覧 = %v, want [%d]", activityIDs(page), log.importFailed)
	}
	// status を外し、feature（import）には従う。
	if want := (domain.ActivityCounts{Attention: 1, Done: 1}); page.Counts != want {
		t.Errorf("counts = %+v, want %+v", page.Counts, want)
	}
	// feature を外し、status（attention）には従う。どの機能も 0 から入る。
	if len(page.Features) != len(domain.ActivityFeatures) || page.Features[domain.ActivityFeatureImport] != 1 ||
		page.Features[domain.ActivityFeatureTags] != 0 {
		t.Errorf("features = %v, want import 1 and every other feature 0", page.Features)
	}

	// decider・since・q には両方が従う。
	owner := listActivity(t, db, domain.ActivityQuery{Decider: domain.ActivityByOwner, Since: log.since}, "", 50)
	if want := (domain.ActivityCounts{Done: 2}); owner.Counts != want {
		t.Errorf("owner since counts = %+v, want %+v", owner.Counts, want)
	}
	if owner.Features[domain.ActivityFeatureTags] != 1 || owner.Features[domain.ActivityFeatureDuplicateCheck] != 1 ||
		owner.Features[domain.ActivityFeatureImport] != 0 {
		t.Errorf("owner since features = %v, want tags 1 and duplicate_check 1", owner.Features)
	}
	none := listActivity(t, db, domain.ActivityQuery{Search: "nothing matches"}, "", 50)
	if none.Counts != (domain.ActivityCounts{}) || none.Features[domain.ActivityFeatureImport] != 0 || len(none.Items) != 0 {
		t.Errorf("一致しない検索 = %+v", none)
	}
}

// 受け入れ: undo の記録は、取り消した記録の feature の下に出る。取り消した記録の要約を添える。
func TestListActivityUndoIsUnderTheUndoneFeature(t *testing.T) {
	db, _ := jobsFixture(t)
	log := seedActivityLog(t, db)

	page := listActivity(t, db, domain.ActivityQuery{Features: []domain.ActivityFeature{domain.ActivityFeatureDuplicateCheck}}, "", 50)
	if want := []int64{log.undo, log.judgement}; !slices.Equal(activityIDs(page), want) {
		t.Fatalf("duplicate_check の一覧 = %v, want %v", activityIDs(page), want)
	}
	undo := page.Items[0]
	if undo.Feature != domain.ActivityFeatureDuplicateCheck || undo.Undoes == nil ||
		undo.Undoes.Kind != domain.ActivityVersionJudgement || undo.Undoes.Outcome != domain.ActivitySame {
		t.Errorf("undo = %+v (undoes %+v), want duplicate_check undoing a same judgement", undo, undo.Undoes)
	}
	judgement := page.Items[1]
	if judgement.Probability == nil || *judgement.Probability != 0.9 || judgement.Undoes != nil {
		t.Errorf("judgement = %+v", judgement)
	}
}

// 対象の鍵ごとに、所有者が開ける今の動画の id を添える。ライブラリに無い鍵は 0 で、記録は残る（R-10）。
func TestListActivityResolvesSubjectVideos(t *testing.T) {
	db, videoID := jobsFixture(t)
	log := seedActivityLog(t, db)

	page := listActivity(t, db, domain.ActivityQuery{}, "", 50)
	byID := map[int64]domain.ActivityEntry{}
	for _, entry := range page.Items {
		byID[entry.ID] = entry
	}
	if got := byID[log.importDone].VideoIDs; got != [2]int64{videoID, 0} {
		t.Errorf("key-a の import の動画 = %v, want [%d 0]", got, videoID)
	}
	if got := byID[log.importFailed].VideoIDs; got != [2]int64{0, 0} {
		t.Errorf("ライブラリに無い key-b の動画 = %v, want [0 0]", got)
	}
	if got := byID[log.judgement].VideoIDs; got != [2]int64{videoID, 0} {
		t.Errorf("組の動画 = %v, want [%d 0]", got, videoID)
	}
	if guest, err := db.Activity().ListActivity(context.Background(), domain.AudienceGuest, domain.ActivityQuery{}, "", 50); err != nil ||
		guest.Items[len(guest.Items)-1].VideoIDs != [2]int64{} {
		t.Errorf("ゲストから見た非公開の動画 = %+v, %v; want no video", guest.Items, err)
	}
}

// 受け入れ: 記録が足されている間も、cursor が重複なしに続きを返す。読めない cursor は ErrInvalidCursor。
func TestListActivityCursorContinuesWhileEntriesAreAdded(t *testing.T) {
	db, _ := jobsFixture(t)
	clock := newActivityClock()
	var written []int64
	for range 5 {
		written = append(written, writeActivity(t, db,
			videoActivity(domain.ActivityImport, "key-a", "a.mp4", activityDone, domain.ActivityByRule), clock.next()))
	}

	first := listActivity(t, db, domain.ActivityQuery{}, "", 2)
	if !slices.Equal(activityIDs(first), []int64{written[4], written[3]}) || first.NextCursor == "" {
		t.Fatalf("1 ページ目 = %v (cursor %q)", activityIDs(first), first.NextCursor)
	}
	// 新しい記録が先頭に足されても、続きはずれない。
	added := writeActivity(t, db,
		videoActivity(domain.ActivityImport, "key-a", "a.mp4", activityDone, domain.ActivityByRule), clock.next())
	second := listActivity(t, db, domain.ActivityQuery{}, first.NextCursor, 2)
	if !slices.Equal(activityIDs(second), []int64{written[2], written[1]}) || second.NextCursor == "" {
		t.Fatalf("2 ページ目 = %v (cursor %q)", activityIDs(second), second.NextCursor)
	}
	third := listActivity(t, db, domain.ActivityQuery{}, second.NextCursor, 2)
	if !slices.Equal(activityIDs(third), []int64{written[0]}) || third.NextCursor != "" {
		t.Fatalf("3 ページ目 = %v (cursor %q)", activityIDs(third), third.NextCursor)
	}
	// 足した記録は先頭から読み直すと出て、件数にも入る。
	if top := listActivity(t, db, domain.ActivityQuery{}, "", 1); top.Items[0].ID != added || top.Counts.Done != 6 {
		t.Errorf("読み直した先頭 = %v (counts %+v), want %d", activityIDs(top), top.Counts, added)
	}

	if _, err := db.Activity().ListActivity(context.Background(), domain.AudienceOwner, domain.ActivityQuery{}, "not a cursor", 2); !errors.Is(err, domain.ErrInvalidCursor) {
		t.Errorf("読めない cursor の誤り = %v, want ErrInvalidCursor", err)
	}
}

// 受け入れ: 新しい記録が、同じ kind と subject の古い attention の記録を閉じる（superseded）。別の対象・
// 別の種類・attention でない記録は閉じない。version_judgement は組ごとに 1 件を保つので閉じない。
func TestRecordActivitySupersedesOlderAttention(t *testing.T) {
	db, _ := jobsFixture(t)
	clock := newActivityClock()
	failed := writeActivity(t, db,
		videoActivity(domain.ActivityImport, "key-a", "a.mp4", activityFailed, domain.ActivityByRule), clock.next())
	otherVideo := writeActivity(t, db,
		videoActivity(domain.ActivityImport, "key-b", "b.mp4", activityFailed, domain.ActivityByRule), clock.next())
	otherKind := writeActivity(t, db,
		videoActivity(domain.ActivityAutoTag, "key-a", "a.mp4", activityFailed, domain.ActivityByRule), clock.next())
	pair := domain.ActivitySubject{Kind: domain.ActivitySubjectPair, Key: "key-a", KeyB: "key-b", Label: "a", LabelB: "b"}
	undecided := writeActivity(t, db, domain.ActivityEntry{
		Kind: domain.ActivityVersionJudgement, Subject: pair, Status: activityUndecided.Status,
		Reason: activityUndecided.Reason, Decider: domain.ActivityByRule,
	}, clock.next())

	recorder := &eventRecorder{}
	db.PublishTo(recorder)
	newer := writeActivity(t, db,
		videoActivity(domain.ActivityImport, "key-a", "a.mp4", activityWaiting, domain.ActivityByRule), clock.next())
	writeActivity(t, db, domain.ActivityEntry{
		Kind: domain.ActivityVersionJudgement, Subject: pair, Status: activityWaiting.Status,
		Reason: activityWaiting.Reason, Decider: domain.ActivityByRule,
	}, clock.next())
	if !hasEvent[domain.ActivityChanged](recorder.events) {
		t.Errorf("記録を書いた取引の確定後に ActivityChanged を発行しない: %v", recorder.events)
	}

	states := activityStates(t, db)
	if got := states[failed]; got != (domain.ActivityState{Status: domain.ActivityDone, Outcome: domain.ActivitySuperseded}) {
		t.Errorf("古い attention の import = %+v, want done/superseded", got)
	}
	for name, id := range map[string]int64{"別の動画": otherVideo, "別の種類": otherKind} {
		if got := states[id]; got != activityFailed {
			t.Errorf("%s の記録 = %+v, want attention/failed のまま", name, got)
		}
	}
	if got := states[undecided]; got != activityUndecided {
		t.Errorf("version_judgement = %+v, want attention/undecided のまま", got)
	}
	if got := states[newer]; got != activityWaiting {
		t.Errorf("新しい記録 = %+v, want running/waiting", got)
	}
}

// 受け入れ: reason processing の running の記録は、回復の後に attention/interrupted になる。waiting は残す。
func TestRecoverInterruptedActivity(t *testing.T) {
	db, _ := jobsFixture(t)
	clock := newActivityClock()
	claimed := writeActivity(t, db,
		videoActivity(domain.ActivityImport, "key-a", "a.mp4", activityClaimed, domain.ActivityByRule), clock.next())
	waiting := writeActivity(t, db,
		videoActivity(domain.ActivityAutoTag, "key-a", "a.mp4", activityWaiting, domain.ActivityByRule), clock.next())
	done := writeActivity(t, db,
		videoActivity(domain.ActivityTagAttach, "key-a", "a.mp4", activityDone, domain.ActivityByOwner), clock.next())

	recorder := &eventRecorder{}
	db.PublishTo(recorder)
	recovered, err := db.Activity().RecoverInterruptedActivity(context.Background())
	if err != nil || recovered != 1 {
		t.Fatalf("RecoverInterruptedActivity = %d, %v; want 1", recovered, err)
	}
	if !hasEvent[domain.ActivityChanged](recorder.events) {
		t.Errorf("回復した後に ActivityChanged を発行しない: %v", recorder.events)
	}
	states := activityStates(t, db)
	if got := states[claimed]; got != (domain.ActivityState{Status: domain.ActivityAttention, Reason: domain.ActivityInterrupted}) {
		t.Errorf("processing の記録 = %+v, want attention/interrupted", got)
	}
	if states[waiting] != activityWaiting || states[done] != activityDone {
		t.Errorf("waiting = %+v, done = %+v; want unchanged", states[waiting], states[done])
	}

	// 回復するものが無ければ知らせない。
	recorder.events = nil
	if recovered, err := db.Activity().RecoverInterruptedActivity(context.Background()); err != nil || recovered != 0 {
		t.Fatalf("2 回目の RecoverInterruptedActivity = %d, %v; want 0", recovered, err)
	}
	if len(recorder.events) != 0 {
		t.Errorf("回復の無い 2 回目の知らせ = %v, want none", recorder.events)
	}
}

// 1 件の読み出しは一覧と同じ形で、無い id は ErrNotFound。auto_tag の記録には付けたタグを添える。
func TestActivityEntryReadsOneEntryWithTags(t *testing.T) {
	db, _ := jobsFixture(t)
	clock := newActivityClock()
	id := writeActivity(t, db,
		videoActivity(domain.ActivityAutoTag, "key-a", "a.mp4", activityDone, domain.ActivityByAI), clock.next())
	if _, err := db.sql.Exec(`insert into activity_entry_tags (entry_id, tag_id, tag_name, suggested, user_key,
		attached_at, probability) values (?, 7, 'outdoor', 0, 'key-a', 1, 0.91), (?, 8, 'beach', 1, 'key-a', 1, 0.6)`, id, id); err != nil {
		t.Fatal(err)
	}

	entry, err := db.Activity().ActivityEntry(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.ActivityEntryTag{
		{TagID: 8, Name: "beach", Suggested: true, Probability: 0.6},
		{TagID: 7, Name: "outdoor", Probability: 0.91},
	}
	if !slices.Equal(entry.Tags, want) || entry.Feature != domain.ActivityFeatureAutoTagging || !entry.UndoOffered() {
		t.Errorf("auto_tag の記録 = %+v, want tags %+v and undo offered", entry, want)
	}
	if _, err := db.Activity().ActivityEntry(context.Background(), id+100); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("無い記録の誤り = %v, want ErrNotFound", err)
	}
}

// activityStates は記録ごとの状態と理由か結果である。
func activityStates(t *testing.T, db *DB) map[int64]domain.ActivityState {
	t.Helper()
	rows, err := db.sql.Query(`select id, status, coalesce(reason, ''), coalesce(outcome, '') from activity_entries`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	states := map[int64]domain.ActivityState{}
	for rows.Next() {
		var id int64
		var status, reason, outcome string
		if err := rows.Scan(&id, &status, &reason, &outcome); err != nil {
			t.Fatal(err)
		}
		states[id] = domain.ActivityState{
			Status: domain.ActivityStatus(status), Reason: domain.ActivityReason(reason), Outcome: domain.ActivityOutcome(outcome),
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return states
}

// 記録の状態は、変える内容の取引の中で setActivityState が書き換え、確定後に ActivityChanged を発行する。
// 確定しなかった取引の書き換えは残らず、知らせも出ない。
func TestSetActivityStateInTheChangeTransaction(t *testing.T) {
	db, _ := jobsFixture(t)
	clock := newActivityClock()
	id := writeActivity(t, db,
		videoActivity(domain.ActivityImport, "key-a", "a.mp4", activityWaiting, domain.ActivityByRule), clock.next())
	update := func(state domain.ActivityState, commit bool) {
		t.Helper()
		ctx := context.Background()
		tx, err := db.sql.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback() }()
		c := &changes{}
		if err := setActivityState(ctx, tx, c, id, state, clock.next()); err != nil {
			t.Fatal(err)
		}
		if commit {
			if err := db.commit(ctx, tx, c); err != nil {
				t.Fatal(err)
			}
		}
	}

	recorder := &eventRecorder{}
	db.PublishTo(recorder)
	update(activityDone, false)
	if states := activityStates(t, db); states[id] != activityWaiting || len(recorder.events) != 0 {
		t.Fatalf("ロールバックした書き換え = %+v, events %v; want unchanged and none", states[id], recorder.events)
	}
	update(activityClaimed, true)
	if states := activityStates(t, db); states[id] != activityClaimed || !hasEvent[domain.ActivityChanged](recorder.events) {
		t.Fatalf("確定した書き換え = %+v, events %v; want running/processing and ActivityChanged", states[id], recorder.events)
	}
}
