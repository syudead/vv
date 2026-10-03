package store

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// まとめての確定・却下・削除と確認の数（specs/036-tag-admin-scale/data-model.md §2）。

// batchFixture は key-a に仮のタグ t1・t2・t3 を付け、確定したタグ c1・c2 を作る。
// 返す map は名前から id。
func batchFixture(t *testing.T) (*DB, map[string]int64) {
	t.Helper()
	db, _ := tentativeFixture(t)
	applyTentative(t, db, []domain.VideoRef{{ContentKey: "key-a"}}, domain.VideoTagsAdd, "t1", "t2", "t3")
	ids := map[string]int64{}
	for _, name := range []string{"t1", "t2", "t3"} {
		ids[name] = tagIDByName(t, db, name)
		if !tagTentative(t, db, ids[name]) {
			t.Fatalf("%s が仮のタグでない", name)
		}
	}
	for _, name := range []string{"c1", "c2"} {
		tag, err := db.Tags().CreateTag(context.Background(), name)
		if err != nil {
			t.Fatal(err)
		}
		attachTag(t, db, "key-b", tag.ID)
		ids[name] = tag.ID
	}
	return db, ids
}

// batchIDs は仮・確定・無い id を混ぜ、重複も含めた ids を返す。
func batchIDs(ids map[string]int64, missing int64) []int64 {
	return []int64{ids["t1"], ids["c1"], missing, ids["t2"], ids["c2"], ids["t3"], ids["t1"]}
}

// assertTagInvariantsNow は 031 の不変条件（と各タグがちょうど 1 つの元の名前を持つこと）を、
// テストの終わりを待たずにその場で確かめる。
func assertTagInvariantsNow(t *testing.T, db *DB) {
	t.Helper()
	assertTagCanonicalNameInvariant(t, db)
	assertTentativeTagInvariant(t, db)
}

func tagExists(t *testing.T, db *DB, id int64) bool {
	t.Helper()
	var count int
	if err := db.sql.QueryRow(`select count(*) from tags where id = ?`, id).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count == 1
}

func TestBatchTagsConfirmConfirmsOnlyTentativeTags(t *testing.T) {
	db, ids := batchFixture(t)
	const missing = 999999

	outcome, err := db.Tags().BatchTags(context.Background(), domain.TagBatchConfirm, batchIDs(ids, missing))
	if err != nil {
		t.Fatal(err)
	}
	want := domain.TagBatchOutcome{
		AppliedIDs:       []int64{ids["t1"], ids["t2"], ids["t3"]},
		NotFoundIDs:      []int64{missing},
		NotApplicableIDs: []int64{ids["c1"], ids["c2"]},
	}
	if !reflect.DeepEqual(outcome, want) {
		t.Errorf("outcome = %+v, want %+v", outcome, want)
	}
	for _, name := range []string{"t1", "t2", "t3"} {
		if tagTentative(t, db, ids[name]) {
			t.Errorf("%s がまだ仮のタグ", name)
		}
	}
	// 付与は変わらない。
	if got := videoTagsOf(t, db, "key-a"); len(got) != 3 {
		t.Errorf("key-a のタグ = %+v, want 3 件", got)
	}
	assertTagInvariantsNow(t, db)
}

func TestBatchTagsRejectRemembersNamesAndKeepsConfirmedTags(t *testing.T) {
	db, ids := batchFixture(t)
	const missing = 999999

	outcome, err := db.Tags().BatchTags(context.Background(), domain.TagBatchReject, batchIDs(ids, missing))
	if err != nil {
		t.Fatal(err)
	}
	want := domain.TagBatchOutcome{
		AppliedIDs:       []int64{ids["t1"], ids["t2"], ids["t3"]},
		NotFoundIDs:      []int64{missing},
		NotApplicableIDs: []int64{ids["c1"], ids["c2"]},
	}
	if !reflect.DeepEqual(outcome, want) {
		t.Errorf("outcome = %+v, want %+v", outcome, want)
	}
	for _, name := range []string{"t1", "t2", "t3"} {
		if tagExists(t, db, ids[name]) {
			t.Errorf("却下した %s が残っている", name)
		}
	}
	for _, name := range []string{"c1", "c2"} {
		if !tagExists(t, db, ids[name]) {
			t.Errorf("確定した %s が消えた", name)
		}
	}
	if got := rejectedNames(t, db); !reflect.DeepEqual(got, []string{"t1", "t2", "t3"}) {
		t.Errorf("却下した名前 = %v, want [t1 t2 t3]", got)
	}
	if got := videoTagsOf(t, db, "key-a"); len(got) != 0 {
		t.Errorf("key-a のタグ = %+v, want なし", got)
	}
	assertTagInvariantsNow(t, db)
}

func TestBatchTagsDeleteDeletesOnlyConfirmedTags(t *testing.T) {
	db, ids := batchFixture(t)
	const missing = 999999

	outcome, err := db.Tags().BatchTags(context.Background(), domain.TagBatchDelete, batchIDs(ids, missing))
	if err != nil {
		t.Fatal(err)
	}
	want := domain.TagBatchOutcome{
		AppliedIDs:       []int64{ids["c1"], ids["c2"]},
		NotFoundIDs:      []int64{missing},
		NotApplicableIDs: []int64{ids["t1"], ids["t2"], ids["t3"]},
	}
	if !reflect.DeepEqual(outcome, want) {
		t.Errorf("outcome = %+v, want %+v", outcome, want)
	}
	for _, name := range []string{"c1", "c2"} {
		if tagExists(t, db, ids[name]) {
			t.Errorf("削除した %s が残っている", name)
		}
	}
	for _, name := range []string{"t1", "t2", "t3"} {
		if !tagExists(t, db, ids[name]) || !tagTentative(t, db, ids[name]) {
			t.Errorf("仮の %s が残っていない", name)
		}
	}
	if got := rejectedNames(t, db); len(got) != 0 {
		t.Errorf("却下した名前 = %v, want なし", got)
	}
	if got := videoTagsOf(t, db, "key-b"); len(got) != 0 {
		t.Errorf("key-b のタグ = %+v, want なし", got)
	}
	assertTagInvariantsNow(t, db)
}

// 働くタグが 1 つも無ければ何も変えず、どの配列も nil でなく空になる。
func TestBatchTagsWithNothingApplicableReturnsEmptyArrays(t *testing.T) {
	db, ids := batchFixture(t)

	outcome, err := db.Tags().BatchTags(context.Background(), domain.TagBatchDelete, []int64{ids["t1"]})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.AppliedIDs == nil || outcome.NotFoundIDs == nil || outcome.NotApplicableIDs == nil {
		t.Errorf("outcome = %#v, want どの配列も nil でない", outcome)
	}
	if !slices.Equal(outcome.NotApplicableIDs, []int64{ids["t1"]}) || len(outcome.AppliedIDs) != 0 {
		t.Errorf("outcome = %+v", outcome)
	}
	assertTagInvariantsNow(t, db)
}

// 受け入れ条件 11: 同じ動画に付いた 2 つのタグは 1 本と数え、フォルダ名からだけ付いている
// 動画も数え、ライブラリに無い動画は数えない。
func TestTagImpactCountsDistinctRegisteredVideos(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	anime, err := db.Tags().CreateTag(ctx, "Anime")
	if err != nil {
		t.Fatal(err)
	}
	x, err := db.Tags().CreateTag(ctx, "X")
	if err != nil {
		t.Fatal(err)
	}
	upsertFolderVideo(t, db, fixturePath("/media/Anime/1.mp4"), "k1") // フォルダ名からだけ Anime
	upsertFolderVideo(t, db, fixturePath("/media/Other/2.mp4"), "k2")
	rebuildIndexForTest(t, db)
	attachTag(t, db, "k2", anime.ID)
	attachTag(t, db, "k2", x.ID)
	attachTag(t, db, "key-gone", x.ID) // いまライブラリに無い動画

	for _, action := range []domain.TagImpactAction{domain.TagImpactDelete, domain.TagImpactMerge} {
		got, err := db.Tags().TagImpact(ctx, action, []int64{anime.ID, x.ID, x.ID, 999999})
		if err != nil {
			t.Fatal(err)
		}
		if want := (domain.TagImpact{TagCount: 2, VideoCount: 2}); got != want {
			t.Errorf("%s: TagImpact = %+v, want %+v", action, got, want)
		}
	}
	// 却下は仮のタグだけを数えるので、確定したタグだけなら 0。
	got, err := db.Tags().TagImpact(ctx, domain.TagImpactReject, []int64{anime.ID, x.ID})
	if err != nil {
		t.Fatal(err)
	}
	if got != (domain.TagImpact{}) {
		t.Errorf("reject: TagImpact = %+v, want 0 件", got)
	}
}

// 100 本に付いた仮のタグと、別の 1 本に付いた確定したタグを渡すと、操作ごとに働くタグと
// その動画だけを数える（research.md R-6）。
func TestTagImpactCountsOnlyTagsTheActionAppliesTo(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	files := make([]domain.VideoFile, 0, 101)
	refs := make([]domain.VideoRef, 0, 100)
	for i := range 101 {
		key := fmt.Sprintf("key-%03d", i)
		files = append(files, listingFile(fixturePath(fmt.Sprintf("/media/%03d.mp4", i)), key, key, i))
		if i < 100 {
			refs = append(refs, domain.VideoRef{ContentKey: key})
		}
	}
	upsertAll(t, db, files...)
	applyTentative(t, db, refs, domain.VideoTagsAdd, "仮")
	tentative := tagIDByName(t, db, "仮")
	confirmed, err := db.Tags().CreateTag(ctx, "確定")
	if err != nil {
		t.Fatal(err)
	}
	attachTag(t, db, "key-100", confirmed.ID)

	cases := []struct {
		action domain.TagImpactAction
		want   domain.TagImpact
	}{
		{domain.TagImpactDelete, domain.TagImpact{TagCount: 1, VideoCount: 1}},
		{domain.TagImpactReject, domain.TagImpact{TagCount: 1, VideoCount: 100}},
		{domain.TagImpactMerge, domain.TagImpact{TagCount: 2, VideoCount: 101}},
	}
	for _, tc := range cases {
		got, err := db.Tags().TagImpact(ctx, tc.action, []int64{tentative, confirmed.ID})
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Errorf("%s: TagImpact = %+v, want %+v", tc.action, got, tc.want)
		}
	}
}
