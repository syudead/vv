package store

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// 自動タグ付けの待ち行列と設定（docs/design-docs/auto-tagging.md）。

func autoTagState(t *testing.T, db *DB, contentKey string) string {
	t.Helper()
	var state string
	err := db.sql.QueryRow(`select state from auto_tag_queue where content_key = ?`, contentKey).Scan(&state)
	if err != nil {
		return ""
	}
	return state
}

func manualTagIDs(t *testing.T, db *DB, contentKey string) []int64 {
	t.Helper()
	rows, err := db.sql.Query(`select tag_id from video_tags where content_key = ? order by tag_id`, contentKey)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return ids
}

func createTestTag(t *testing.T, db *DB, name string) domain.Tag {
	t.Helper()
	tag, err := db.Tags().CreateTag(context.Background(), name)
	if err != nil {
		t.Fatalf("タグ %q を作れない: %v", name, err)
	}
	return tag
}

func claimAutoTag(t *testing.T, store *AutoTagStore) domain.AutoTagJob {
	t.Helper()
	job, err := store.Claim(context.Background())
	if err != nil {
		t.Fatalf("判定を取り出せない: %v", err)
	}
	return job
}

// 保存値が無ければ既定値を返し、保存した値はそのまま読み戻せる。
func TestAutoTagSettingsDefaultsAndRoundTrip(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	got, err := db.Settings().AutoTagSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got != domain.DefaultAutoTagSettings() {
		t.Fatalf("既定値 = %+v", got)
	}
	want := domain.AutoTagSettings{Enabled: true, Endpoint: "http://gpu:11434", Model: "clef", Threshold: 0.65}
	if err := db.Settings().SaveAutoTagSettings(ctx, want); err != nil {
		t.Fatal(err)
	}
	if got, err = db.Settings().AutoTagSettings(ctx); err != nil || got != want {
		t.Fatalf("読み戻し = %+v, %v; want %+v", got, err, want)
	}
}

// 積み方ごとに、既にある行の扱いが変わる。判定中の行はどの積み方でも動かさない。
func TestAutoTagQueueModes(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	store := db.AutoTags()
	solo := ids[fixturePath("/media/solo.mp4")]
	soloKey := "key" + fixturePath("/media/solo.mp4")

	if n, err := store.QueueVideos(ctx, []int64{solo}, domain.AutoTagQueueIfNew); err != nil || n != 1 {
		t.Fatalf("初回の積み = %d, %v", n, err)
	}
	if n, _ := store.QueueVideos(ctx, []int64{solo}, domain.AutoTagQueueIfNew); n != 0 {
		t.Fatalf("2 回目の IfNew が %d 件積んだ", n)
	}
	claimAutoTag(t, store)
	for _, mode := range []domain.AutoTagQueueMode{domain.AutoTagQueueIfNew, domain.AutoTagQueueUnlessDone, domain.AutoTagQueueAgain} {
		if n, _ := store.QueueVideos(ctx, []int64{solo}, mode); n != 0 {
			t.Fatalf("判定中の行を mode %d が積み直した", mode)
		}
	}
	if err := store.Fail(ctx, soloKey, "boom"); err != nil {
		t.Fatal(err)
	}
	if n, _ := store.QueueVideos(ctx, []int64{solo}, domain.AutoTagQueueIfNew); n != 0 {
		t.Fatalf("失敗した行を IfNew が積み直した")
	}
	if n, _ := store.QueueVideos(ctx, []int64{solo}, domain.AutoTagQueueUnlessDone); n != 1 {
		t.Fatalf("失敗した行を UnlessDone が積み直さなかった")
	}
	claimAutoTag(t, store)
	if err := store.Finish(ctx, soloKey, nil); err != nil {
		t.Fatal(err)
	}
	if n, _ := store.QueueVideos(ctx, []int64{solo}, domain.AutoTagQueueUnlessDone); n != 0 {
		t.Fatalf("判定を終えた行を UnlessDone が積み直した")
	}
	if n, _ := store.QueueVideos(ctx, []int64{solo}, domain.AutoTagQueueAgain); n != 1 {
		t.Fatalf("判定を終えた行を Again が積み直さなかった")
	}
	if state := autoTagState(t, db, soloKey); state != "queued" {
		t.Fatalf("state = %q", state)
	}
}

// ライブラリ全体を積むと、登録フォルダの下の動画がすべて積まれ、古い順に取り出せる。
func TestAutoTagQueueLibraryAndClaimOrder(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	store := db.AutoTags()
	first := ids[fixturePath("/media/pair/p2.mp4")]
	if _, err := store.QueueVideos(ctx, []int64{first}, domain.AutoTagQueueIfNew); err != nil {
		t.Fatal(err)
	}
	n, err := store.QueueLibrary(ctx, domain.AutoTagQueueUnlessDone)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(ids)-1 {
		t.Fatalf("積んだ件数 = %d, want %d", n, len(ids)-1)
	}
	if job := claimAutoTag(t, store); job.ContentKey != "key"+fixturePath("/media/pair/p2.mp4") {
		t.Fatalf("最初に取り出したのは %q", job.ContentKey)
	}
	counts, err := store.Counts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Queued != len(ids)-1 || counts.Running != 1 {
		t.Fatalf("counts = %+v", counts)
	}
	for range len(ids) - 1 {
		claimAutoTag(t, store)
	}
	if _, err := store.Claim(ctx); !errors.Is(err, domain.ErrNoJob) {
		t.Fatalf("空の待ち行列の Claim = %v", err)
	}
	if requeued, err := store.RequeueRunning(ctx); err != nil || requeued != len(ids) {
		t.Fatalf("RequeueRunning = %d, %v", requeued, err)
	}
}

// 終えると今もあるタグだけを付け、行を done にし、確定の後に AutoTagApplied を発行する。
func TestAutoTagFinishAttachesExistingTags(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	recorder := &eventRecorder{}
	db.PublishTo(recorder)
	store := db.AutoTags()
	solo := ids[fixturePath("/media/solo.mp4")]
	soloKey := "key" + fixturePath("/media/solo.mp4")
	cat := createTestTag(t, db, "cat")
	gone := createTestTag(t, db, "gone")
	if err := db.Tags().DeleteTag(ctx, gone.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := store.QueueVideos(ctx, []int64{solo}, domain.AutoTagQueueIfNew); err != nil {
		t.Fatal(err)
	}
	claimAutoTag(t, store)
	if err := store.Finish(ctx, soloKey, []int64{cat.ID, gone.ID}); err != nil {
		t.Fatal(err)
	}
	if got := manualTagIDs(t, db, soloKey); !reflect.DeepEqual(got, []int64{cat.ID}) {
		t.Fatalf("付いたタグ = %v", got)
	}
	if state := autoTagState(t, db, soloKey); state != "done" {
		t.Fatalf("state = %q", state)
	}
	want := domain.AutoTagApplied{VideoIDs: []int64{solo}}
	if !slices.ContainsFunc(recorder.events, func(e domain.Event) bool { return reflect.DeepEqual(e, want) }) {
		t.Fatalf("発行した変化 = %#v", recorder.events)
	}
}

// 判定の最中に積み直された行は、古い判定の結果では終えない。
func TestAutoTagFinishIgnoresJobsNoLongerRunning(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	store := db.AutoTags()
	solo := ids[fixturePath("/media/solo.mp4")]
	soloKey := "key" + fixturePath("/media/solo.mp4")
	cat := createTestTag(t, db, "cat")
	if _, err := store.QueueVideos(ctx, []int64{solo}, domain.AutoTagQueueIfNew); err != nil {
		t.Fatal(err)
	}
	claimAutoTag(t, store)
	if _, err := store.RequeueRunning(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.Finish(ctx, soloKey, []int64{cat.ID}); err != nil {
		t.Fatal(err)
	}
	if got := manualTagIDs(t, db, soloKey); len(got) != 0 {
		t.Fatalf("積み直した行の古い結果でタグが付いた: %v", got)
	}
	if err := store.Fail(ctx, soloKey, "late"); err != nil {
		t.Fatal(err)
	}
	if state := autoTagState(t, db, soloKey); state != "queued" {
		t.Fatalf("state = %q", state)
	}
}

// 判定の間に動画が消えたら、行を消してタグを付けない。
func TestAutoTagFinishDropsRemovedVideos(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	store := db.AutoTags()
	solo := ids[fixturePath("/media/solo.mp4")]
	soloKey := "key" + fixturePath("/media/solo.mp4")
	if _, err := store.QueueVideos(ctx, []int64{solo}, domain.AutoTagQueueIfNew); err != nil {
		t.Fatal(err)
	}
	claimAutoTag(t, store)
	if _, err := db.sql.Exec(`delete from media_folders`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Subject(ctx, soloKey); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("登録外の動画の Subject = %v", err)
	}
	if err := store.Finish(ctx, soloKey, nil); err != nil {
		t.Fatal(err)
	}
	if state := autoTagState(t, db, soloKey); state != "" {
		t.Fatalf("行が残った: %q", state)
	}
}

// 候補は確定したタグだけで、シノニムを添え、名前の自然順に並ぶ。
func TestAutoTagCandidates(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	beach := createTestTag(t, db, "tag10 beach")
	cat := createTestTag(t, db, "tag2 cat")
	if _, err := db.Tags().AddSynonym(ctx, cat.ID, "kitty", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Tags().ApplyVideoTags(ctx, []domain.VideoRef{{ID: ids[fixturePath("/media/solo.mp4")]}},
		domain.VideoTagsAdd, []string{"tentative one"}, true); err != nil {
		t.Fatal(err)
	}
	got, err := db.AutoTags().Candidates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.AutoTagCandidate{
		{ID: cat.ID, Name: "tag2 cat", Synonyms: []string{"kitty"}},
		{ID: beach.ID, Name: "tag10 beach"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("候補 = %+v, want %+v", got, want)
	}
}

// 件数は状態ごとに数え、直近の失敗の理由を返す。
func TestAutoTagCountsLastError(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	store := db.AutoTags()
	if _, err := store.QueueVideos(ctx, []int64{ids[fixturePath("/media/solo.mp4")]}, domain.AutoTagQueueIfNew); err != nil {
		t.Fatal(err)
	}
	job := claimAutoTag(t, store)
	if err := store.Fail(ctx, job.ContentKey, "cannot reach Ollama"); err != nil {
		t.Fatal(err)
	}
	counts, err := store.Counts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if counts != (domain.AutoTagCounts{Failed: 1, LastError: "cannot reach Ollama"}) {
		t.Fatalf("counts = %+v", counts)
	}
}

// 集まりのメンバーを判定すると、タグは集まりの鍵に付き、すべてのメンバーに知らせる。
func TestAutoTagFinishNotifiesBundleMembers(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	p1, p2 := ids[fixturePath("/media/pair/p1.mp4")], ids[fixturePath("/media/pair/p2.mp4")]
	if _, err := db.Versions().Bundle(ctx, []int64{p1, p2}, p1); err != nil {
		t.Fatal(err)
	}
	recorder := &eventRecorder{}
	db.PublishTo(recorder)
	store := db.AutoTags()
	cat := createTestTag(t, db, "cat")
	if _, err := store.QueueVideos(ctx, []int64{p2}, domain.AutoTagQueueIfNew); err != nil {
		t.Fatal(err)
	}
	job := claimAutoTag(t, store)
	if err := store.Finish(ctx, job.ContentKey, []int64{cat.ID}); err != nil {
		t.Fatal(err)
	}
	if tags := ownerVideo(t, db, p1).UserKey; len(manualTagIDs(t, db, tags)) != 1 {
		t.Fatalf("代表の動画にタグが見えない")
	}
	want := []int64{min(p1, p2), max(p1, p2)}
	found := false
	for _, e := range recorder.events {
		if applied, ok := e.(domain.AutoTagApplied); ok && reflect.DeepEqual(applied.VideoIDs, want) {
			found = true
		}
	}
	if !found {
		t.Fatalf("発行した変化 = %#v, want AutoTagApplied%v", recorder.events, want)
	}
}
