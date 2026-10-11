package store

import (
	"context"
	"errors"
	"io/fs"
	"reflect"
	"slices"
	"testing"

	"github.com/pressly/goose/v3"

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

// 保存値が無ければ既定値を返し、保存した値はそのまま読み戻せる。機能の設定と接続先は別に持つ。
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
	want := domain.AutoTagSettings{Enabled: true, AfterImport: true, Threshold: 0.65, TentativeThreshold: 0.4}
	if err := db.Settings().SaveAutoTagSettings(ctx, want); err != nil {
		t.Fatal(err)
	}
	if got, err = db.Settings().AutoTagSettings(ctx); err != nil || got != want {
		t.Fatalf("読み戻し = %+v, %v; want %+v", got, err, want)
	}
	if ollama, err := db.Settings().OllamaSettings(ctx); err != nil || ollama != domain.DefaultOllamaSettings() {
		t.Fatalf("機能の設定が接続先を変えた: %+v, %v", ollama, err)
	}
}

// 接続先は保存した値を読み戻し、新しいキーが無ければ前の版のキーを読む。
func TestOllamaSettingsRoundTripAndLegacyKeys(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	if err := writeSetting(ctx, db.sql, legacyOllamaEndpointKey, "http://old:1"); err != nil {
		t.Fatal(err)
	}
	got, err := db.Settings().OllamaSettings(ctx)
	if err != nil || got != (domain.OllamaSettings{Endpoint: "http://old:1", Model: domain.DefaultOllamaModel}) {
		t.Fatalf("前の版のキー = %+v, %v", got, err)
	}
	want := domain.OllamaSettings{Endpoint: "http://gpu:11434", Model: "clef"}
	if err := db.Settings().SaveOllamaSettings(ctx, want); err != nil {
		t.Fatal(err)
	}
	if got, err = db.Settings().OllamaSettings(ctx); err != nil || got != want {
		t.Fatalf("読み戻し = %+v, %v", got, err)
	}
	if auto, err := db.Settings().AutoTagSettings(ctx); err != nil || auto != domain.DefaultAutoTagSettings() {
		t.Fatalf("接続先が機能の設定を変えた: %+v, %v", auto, err)
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
	if err := store.Finish(ctx, soloKey, domain.AutoTagSelection{}); err != nil {
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
	// 次の積みが同じミリ秒に入っても順が決まるよう、最初に積んだ行を前の時刻にする。
	if _, err := db.sql.Exec(`update auto_tag_queue set queued_at = queued_at - 1000`); err != nil {
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
	if err := store.Finish(ctx, soloKey, domain.AutoTagSelection{Confirmed: []int64{cat.ID, gone.ID}}); err != nil {
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
	if err := store.Finish(ctx, soloKey, domain.AutoTagSelection{Confirmed: []int64{cat.ID}}); err != nil {
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
	// 判定の手がかりは、代表の所在とそれを含む登録フォルダである。
	var root string
	if err := db.sql.QueryRow(`select path from media_folders order by length(path) desc limit 1`).Scan(&root); err != nil {
		t.Fatal(err)
	}
	found, err := store.Subject(ctx, soloKey)
	if err != nil || found.MediaFolder != root || found.Video.Path != fixturePath("/media/solo.mp4") {
		t.Fatalf("Subject = %+v, %v (root %s)", found, err, root)
	}
	if _, err := db.sql.Exec(`delete from media_folders`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Subject(ctx, soloKey); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("登録外の動画の Subject = %v", err)
	}
	if err := store.Finish(ctx, soloKey, domain.AutoTagSelection{}); err != nil {
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
	got, err := db.AutoTags().Candidates(ctx, "key"+fixturePath("/media/solo.mp4"))
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

// その動画に確定して付いているタグと却下されたタグは聞かない。仮に付いているタグは聞く。
// ほかの動画の付与と却下は関係しない。
func TestAutoTagCandidatesExcludeAttachedAndRejected(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	store := db.AutoTags()
	soloKey := "key" + fixturePath("/media/solo.mp4")
	attached := createTestTag(t, db, "attached")
	suggested := createTestTag(t, db, "suggested")
	rejected := createTestTag(t, db, "rejected")
	createTestTag(t, db, "fresh")
	solo := ids[fixturePath("/media/solo.mp4")]
	if _, err := store.QueueVideos(ctx, []int64{solo}, domain.AutoTagQueueIfNew); err != nil {
		t.Fatal(err)
	}
	claimAutoTag(t, store)
	if err := store.Finish(ctx, soloKey, domain.AutoTagSelection{
		Confirmed: []int64{attached.ID}, Tentative: []int64{suggested.ID, rejected.ID},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.RejectTentative(ctx, solo, rejected.ID); err != nil {
		t.Fatal(err)
	}
	got, err := store.Candidates(ctx, soloKey)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, c := range got {
		names = append(names, c.Name)
	}
	if !slices.Equal(names, []string{"fresh", "suggested"}) {
		t.Fatalf("候補 = %v", names)
	}

	// 別の動画には、どのタグも聞く。
	var otherKey string
	if err := db.sql.QueryRowContext(ctx, `select content_key from videos where id <> ? and content_key <> '' limit 1`,
		solo).Scan(&otherKey); err != nil {
		t.Fatal(err)
	}
	others, err := store.Candidates(ctx, otherKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(others) != 4 {
		t.Fatalf("別の動画の候補 = %+v", others)
	}
	if _, err := store.Candidates(ctx, "no such key"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("無い動画の候補 err = %v", err)
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
	if err := store.Finish(ctx, job.ContentKey, domain.AutoTagSelection{Confirmed: []int64{cat.ID}}); err != nil {
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

// tentativeOf は利用者データの鍵 key に付いたタグ tagID の仮の状態を返す。行が無ければ -1。
func tentativeOf(t *testing.T, db *DB, key string, tagID int64) int {
	t.Helper()
	var tentative int
	if err := db.sql.QueryRow(`select tentative from video_tags where content_key = ? and tag_id = ?`, key, tagID).
		Scan(&tentative); err != nil {
		return -1
	}
	return tentative
}

func finishSolo(t *testing.T, db *DB, ids map[string]int64, selection domain.AutoTagSelection) {
	t.Helper()
	ctx := context.Background()
	store := db.AutoTags()
	if _, err := store.QueueVideos(ctx, []int64{ids[fixturePath("/media/solo.mp4")]}, domain.AutoTagQueueAgain); err != nil {
		t.Fatal(err)
	}
	job := claimAutoTag(t, store)
	if err := store.Finish(ctx, job.ContentKey, selection); err != nil {
		t.Fatal(err)
	}
}

// 中間の帯のタグは仮に付き、動画のタグに Unconfirmed として出る。手で付けた分は仮にしない。
func TestAutoTagFinishAttachesTentativeTags(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	solo := ids[fixturePath("/media/solo.mp4")]
	soloKey := "key" + fixturePath("/media/solo.mp4")
	cat, dog, manual := createTestTag(t, db, "cat"), createTestTag(t, db, "dog"), createTestTag(t, db, "manual")
	if _, _, err := db.Tags().AttachTagByID(ctx, []int64{solo}, manual.ID); err != nil {
		t.Fatal(err)
	}
	finishSolo(t, db, ids, domain.AutoTagSelection{Confirmed: []int64{cat.ID}, Tentative: []int64{dog.ID, manual.ID}})

	if tentativeOf(t, db, soloKey, cat.ID) != 0 || tentativeOf(t, db, soloKey, dog.ID) != 1 || tentativeOf(t, db, soloKey, manual.ID) != 0 {
		t.Fatalf("tentative = cat %d, dog %d, manual %d",
			tentativeOf(t, db, soloKey, cat.ID), tentativeOf(t, db, soloKey, dog.ID), tentativeOf(t, db, soloKey, manual.ID))
	}
	tags, err := db.Tags().TagsByContentKeys(ctx, []string{soloKey})
	if err != nil {
		t.Fatal(err)
	}
	unconfirmed := map[string]bool{}
	for _, tag := range tags[soloKey] {
		unconfirmed[tag.Name] = tag.Unconfirmed
	}
	if !reflect.DeepEqual(unconfirmed, map[string]bool{"cat": false, "dog": true, "manual": false}) {
		t.Fatalf("unconfirmed = %v", unconfirmed)
	}

	// 次の判定で確率が上がれば確定し、手で付けても確定する。
	finishSolo(t, db, ids, domain.AutoTagSelection{Confirmed: []int64{dog.ID}})
	if got := tentativeOf(t, db, soloKey, dog.ID); got != 0 {
		t.Fatalf("確率が上がった仮の付与が確定しない: %d", got)
	}
}

// 確定すると普通の付与になり、却下すると外れて、次の判定でも付かない。
func TestAutoTagConfirmAndRejectTentative(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	recorder := &eventRecorder{}
	db.PublishTo(recorder)
	solo := ids[fixturePath("/media/solo.mp4")]
	soloKey := "key" + fixturePath("/media/solo.mp4")
	cat, dog := createTestTag(t, db, "cat"), createTestTag(t, db, "dog")
	finishSolo(t, db, ids, domain.AutoTagSelection{Tentative: []int64{cat.ID, dog.ID}})
	store := db.AutoTags()

	if err := store.ConfirmTentative(ctx, solo, cat.ID); err != nil {
		t.Fatal(err)
	}
	if got := tentativeOf(t, db, soloKey, cat.ID); got != 0 {
		t.Fatalf("確定後の tentative = %d", got)
	}
	if err := store.ConfirmTentative(ctx, solo, cat.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("確定済みを確定した err = %v", err)
	}
	if err := store.RejectTentative(ctx, solo, dog.ID); err != nil {
		t.Fatal(err)
	}
	if got := tentativeOf(t, db, soloKey, dog.ID); got != -1 {
		t.Fatalf("却下したタグが残った: %d", got)
	}
	if err := store.RejectTentative(ctx, solo, cat.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("確定したタグを却下できた err = %v", err)
	}
	finishSolo(t, db, ids, domain.AutoTagSelection{Confirmed: []int64{dog.ID}})
	if got := tentativeOf(t, db, soloKey, dog.ID); got != -1 {
		t.Fatalf("却下したタグを付け直した: %d", got)
	}
	// 手で付けるのは止めない。
	if _, _, err := db.Tags().AttachTagByID(ctx, []int64{solo}, dog.ID); err != nil {
		t.Fatal(err)
	}
	if got := tentativeOf(t, db, soloKey, dog.ID); got != 0 {
		t.Fatalf("手で付けたタグ = %d", got)
	}
	applied := 0
	for _, e := range recorder.events {
		if _, ok := e.(domain.AutoTagApplied); ok {
			applied++
		}
	}
	if applied < 4 {
		t.Fatalf("AutoTagApplied の数 = %d", applied)
	}
}

// 00036 まで適用したデータベース（video_tags.tentative を持たない）を最新まで上げても、付いていた
// タグがそのまま読める。00036 を書き換えて列を足していた版では、適用済みのデータベースで列が
// できず、タグの読み出しが失敗して画面からタグが消えた。
func TestAutoTagSuggestionsMigrationKeepsTags(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	fsy, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, db.sql, fsy)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := provider.UpTo(ctx, 36); err != nil {
		t.Fatal(err)
	}
	if _, err := db.sql.Exec(`insert into tags (id, created_at) values (1, 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.sql.Exec(`insert into tag_names (name, tag_id, canonical) values ('cat', 1, 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.sql.Exec(`insert into video_tags (content_key, tag_id, created_at) values ('key-a', 1, 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	if got := tentativeOf(t, db, "key-a", 1); got != 0 {
		t.Fatalf("移行のあとの tentative = %d, want 0", got)
	}
	var count int
	if err := db.sql.QueryRow(`select count(*) from auto_tag_rejections`).Scan(&count); err != nil {
		t.Fatalf("auto_tag_rejections が無い: %v", err)
	}
	if _, err := provider.DownTo(ctx, 36); err != nil {
		t.Fatal(err)
	}
}
