package store

import (
	"context"
	"reflect"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// フォルダ由来のタグ（specs/017-folder-groups/data-model.md §4）の読み出しを確かめる。

// rebuildIndexForTest はスキャンを閉じる前と同じ作り直しを行う。
func rebuildIndexForTest(t *testing.T, db *DB) {
	t.Helper()
	if err := db.ScanIndex().RebuildFolderIndex(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// listIDs はタグの絞り込みと検索式で一覧を引き、動画の id を小さい順に返す。
func listIDs(t *testing.T, db *DB, audience domain.Audience, q domain.VideoQuery) []int64 {
	t.Helper()
	page, err := db.Library().ListVideos(context.Background(), audience, q)
	if err != nil {
		t.Fatalf("ListVideos(%+v) error = %v", q, err)
	}
	ids := make([]int64, 0, len(page.Items))
	for _, video := range page.Items {
		ids = append(ids, video.ID)
	}
	slices.Sort(ids)
	return ids
}

// videoTagsOf は content_key の動画のタグを出所つきで返す。
func videoTagsOf(t *testing.T, db *DB, key string) []domain.VideoTag {
	t.Helper()
	got, err := db.Tags().TagsByContentKeys(context.Background(), []string{key})
	if err != nil {
		t.Fatal(err)
	}
	return got[key]
}

func tagVideoCount(t *testing.T, db *DB, id int64) int {
	t.Helper()
	tags, err := db.Tags().ListTags(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, tag := range tags {
		if tag.ID == id {
			return tag.VideoCount
		}
	}
	t.Fatalf("タグ %d が一覧に無い", id)
	return 0
}

// 受け入れ条件 6・7: 既存のタグと同じ名前のフォルダの下の動画にそのタグが付き、
// そのタグで絞り込むと出る。一致しない名前のフォルダからタグは作られない。
func TestFolderNameMatchingExistingTagTagsVideos(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	tag, err := db.Tags().CreateTag(ctx, "Anime")
	if err != nil {
		t.Fatal(err)
	}
	inside := upsertFolderVideo(t, db, "/media/Anime/Show/1.mp4", "k1")
	outside := upsertFolderVideo(t, db, "/media/Drama/1.mp4", "k2")
	rebuildIndexForTest(t, db)

	want := []domain.VideoTag{{TagRef: domain.TagRef{ID: tag.ID, Name: "Anime"}, FromFolder: true}}
	if got := videoTagsOf(t, db, "k1"); !reflect.DeepEqual(got, want) {
		t.Errorf("k1 tags = %+v, want %+v", got, want)
	}
	if got := videoTagsOf(t, db, "k2"); len(got) != 0 {
		t.Errorf("k2 tags = %+v, want なし", got)
	}
	if got := listIDs(t, db, domain.AudienceOwner, domain.VideoQuery{TagIDs: []int64{tag.ID}}); !slices.Equal(got, []int64{inside}) {
		t.Errorf("絞り込み = %v, want [%d] (outside=%d)", got, inside, outside)
	}
	if got := tagVideoCount(t, db, tag.ID); got != 1 {
		t.Errorf("videoCount = %d, want 1", got)
	}

	// 一致しないフォルダ名（Show・Drama）からタグは作られない。
	tags, err := db.Tags().ListTags(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 1 {
		t.Errorf("tags = %+v, want Anime だけ", tags)
	}
}

// 受け入れ条件 8 と Edge Cases: シノニムの登録・タグの削除・統合は再スキャン
// （索引の作り直し）なしで次の読み出しから効く。
func TestFolderTagsFollowTagChangesWithoutRescan(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	video := upsertFolderVideo(t, db, "/media/アニメ/1.mp4", "k1")
	rebuildIndexForTest(t, db)

	anime, err := db.Tags().CreateTag(ctx, "Anime")
	if err != nil {
		t.Fatal(err)
	}
	if got := videoTagsOf(t, db, "k1"); len(got) != 0 {
		t.Fatalf("シノニムの前 = %+v, want なし", got)
	}
	if _, err := db.Tags().AddSynonym(ctx, anime.ID, "アニメ", nil); err != nil {
		t.Fatal(err)
	}
	want := []domain.VideoTag{{TagRef: domain.TagRef{ID: anime.ID, Name: "Anime"}, FromFolder: true}}
	if got := videoTagsOf(t, db, "k1"); !reflect.DeepEqual(got, want) {
		t.Errorf("シノニムの後 = %+v, want %+v", got, want)
	}
	if got := listIDs(t, db, domain.AudienceOwner, domain.VideoQuery{TagIDs: []int64{anime.ID}}); !slices.Equal(got, []int64{video}) {
		t.Errorf("絞り込み = %v, want [%d]", got, video)
	}

	// 統合すると、統合先のタグとしてすぐ付く。
	target, err := db.Tags().CreateTag(ctx, "Cartoon")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Tags().MergeTag(ctx, target.ID, anime.ID); err != nil {
		t.Fatal(err)
	}
	want = []domain.VideoTag{{TagRef: domain.TagRef{ID: target.ID, Name: "Cartoon"}, FromFolder: true}}
	if got := videoTagsOf(t, db, "k1"); !reflect.DeepEqual(got, want) {
		t.Errorf("統合の後 = %+v, want %+v", got, want)
	}

	// タグを消すと、すぐ付かなくなる。
	if err := db.Tags().DeleteTag(ctx, target.ID); err != nil {
		t.Fatal(err)
	}
	if got := videoTagsOf(t, db, "k1"); len(got) != 0 {
		t.Errorf("削除の後 = %+v, want なし", got)
	}
}

// Edge Cases: 同名のフォルダが別の場所にあればどちらの下の動画にも付き、同じ
// 内容の複数の所在の祖先を両方使う。
func TestFolderTagsUseEveryAncestorOfEveryLocation(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	travel, err := db.Tags().CreateTag(ctx, "Travel")
	if err != nil {
		t.Fatal(err)
	}
	fav, err := db.Tags().CreateTag(ctx, "Fav")
	if err != nil {
		t.Fatal(err)
	}
	first := upsertFolderVideo(t, db, "/media/A/Travel/1.mp4", "k1")
	second := upsertFolderVideo(t, db, "/media/B/Travel/2.mp4", "k2")
	// 同じ内容（k2）の別の所在。
	if again := upsertFolderVideo(t, db, "/media/Fav/2.mp4", "k2"); again != second {
		t.Fatalf("同じ内容が別の動画になった: %d != %d", again, second)
	}
	rebuildIndexForTest(t, db)

	if got := listIDs(t, db, domain.AudienceOwner, domain.VideoQuery{TagIDs: []int64{travel.ID}}); !slices.Equal(got, []int64{first, second}) {
		t.Errorf("Travel = %v, want [%d %d]", got, first, second)
	}
	if got := listIDs(t, db, domain.AudienceOwner, domain.VideoQuery{TagIDs: []int64{travel.ID, fav.ID}}); !slices.Equal(got, []int64{second}) {
		t.Errorf("Travel AND Fav = %v, want [%d]", got, second)
	}
	if got := tagVideoCount(t, db, travel.ID); got != 2 {
		t.Errorf("Travel videoCount = %d, want 2", got)
	}
}

// 手で付けた分とフォルダ名の分は1件にまとまって出所を両方持ち、本数は重ねて
// 数えない。取り外しで外れるのは手で付けた分だけ（受け入れ条件 9 のサーバー側）。
func TestManualAndFolderTagsMergeAndDetachRemovesOnlyManual(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	tag, err := db.Tags().CreateTag(ctx, "Anime")
	if err != nil {
		t.Fatal(err)
	}
	both := upsertFolderVideo(t, db, "/media/Anime/1.mp4", "k1")
	manual := upsertFolderVideo(t, db, "/media/Other/2.mp4", "k2")
	rebuildIndexForTest(t, db)
	if _, _, err := db.Tags().AttachTagByID(ctx, []int64{both, manual}, tag.ID); err != nil {
		t.Fatal(err)
	}

	ref := domain.TagRef{ID: tag.ID, Name: "Anime"}
	if got, want := videoTagsOf(t, db, "k1"), []domain.VideoTag{{TagRef: ref, Manual: true, FromFolder: true}}; !reflect.DeepEqual(got, want) {
		t.Errorf("k1 = %+v, want %+v", got, want)
	}
	if got := tagVideoCount(t, db, tag.ID); got != 2 {
		t.Errorf("videoCount = %d, want 2", got)
	}
	summary, err := db.Tags().Summary(ctx, []int64{both, manual})
	if err != nil {
		t.Fatal(err)
	}
	if want := []domain.TagSummaryItem{{Tag: ref, Count: 2, ManualCount: 2}}; !reflect.DeepEqual(summary.Items, want) {
		t.Errorf("summary = %+v, want %+v", summary.Items, want)
	}

	_, applied, err := db.Tags().DetachTag(ctx, []int64{both, manual}, tag.ID)
	if err != nil {
		t.Fatal(err)
	}
	if applied != 2 {
		t.Errorf("applied = %d, want 2", applied)
	}
	if got, want := videoTagsOf(t, db, "k1"), []domain.VideoTag{{TagRef: ref, FromFolder: true}}; !reflect.DeepEqual(got, want) {
		t.Errorf("取り外しの後の k1 = %+v, want %+v (フォルダ名の分は残る)", got, want)
	}
	if got := videoTagsOf(t, db, "k2"); len(got) != 0 {
		t.Errorf("取り外しの後の k2 = %+v, want なし", got)
	}
	summary, err = db.Tags().Summary(ctx, []int64{both, manual})
	if err != nil {
		t.Fatal(err)
	}
	if want := []domain.TagSummaryItem{{Tag: ref, Count: 1, ManualCount: 0}}; !reflect.DeepEqual(summary.Items, want) {
		t.Errorf("取り外しの後の summary = %+v, want %+v", summary.Items, want)
	}
	if got := tagVideoCount(t, db, tag.ID); got != 1 {
		t.Errorf("取り外しの後の videoCount = %d, want 1", got)
	}
}

// 検索欄の語はフォルダ由来のタグの名前とシノニムにも当たる。ゲストの検索は
// タグ名に照合しない（data-model.md §7）。
func TestSearchMatchesFolderTagNamesForOwnerOnly(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	tag, err := db.Tags().CreateTag(ctx, "Kyoto")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Tags().AddSynonym(ctx, tag.ID, "Trip", nil); err != nil {
		t.Fatal(err)
	}
	video := upsertFolderVideo(t, db, "/media/Trip/a.mp4", "k1")
	other := upsertFolderVideo(t, db, "/media/Else/b.mp4", "k2")
	rebuildIndexForTest(t, db)
	setPublic(t, db, true, video, other)

	// 元の名前で検索すると、シノニムと同じ名前のフォルダの下の動画に当たる。
	if got := listIDs(t, db, domain.AudienceOwner, domain.VideoQuery{Query: "kyoto"}); !slices.Equal(got, []int64{video}) {
		t.Errorf("所有者の検索 = %v, want [%d]", got, video)
	}
	if got := listIDs(t, db, domain.AudienceGuest, domain.VideoQuery{Query: "kyoto"}); len(got) != 0 {
		t.Errorf("ゲストの検索 = %v, want なし", got)
	}
}
