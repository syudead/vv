package store

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// tagCount は content_key に tagID が付いているかを数える。
func tagCount(t *testing.T, db *DB, contentKey string, tagID int64) int {
	t.Helper()
	var count int
	if err := db.sql.QueryRow(`select count(*) from video_tags where content_key = ? and tag_id = ?`, contentKey, tagID).
		Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

// TestAttachTagByIDAppliesToAllRequestedVideos は、3本に付けると3本すべてに
// 付き、既に付いていた動画があっても誤りにならないことを確かめる
// （完了の条件）。
func TestAttachTagByIDAppliesToAllRequestedVideos(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	tag, err := db.Tags().CreateTag(ctx, "旅行")
	if err != nil {
		t.Fatal(err)
	}
	ids := upsertAll(t, db,
		listingFile("/media/a.mp4", "a", "key-a", 0),
		listingFile("/media/b.mp4", "b", "key-b", 1),
		listingFile("/media/c.mp4", "c", "key-c", 2),
	)
	videoIDs := []int64{ids["/media/a.mp4"], ids["/media/b.mp4"], ids["/media/c.mp4"]}

	// b には先に付けておく。
	if _, applied, err := db.Tags().AttachTagByID(ctx, []int64{ids["/media/b.mp4"]}, tag.ID); err != nil || applied != 1 {
		t.Fatalf("先に付ける: applied=%d err=%v", applied, err)
	}

	ref, applied, err := db.Tags().AttachTagByID(ctx, videoIDs, tag.ID)
	if err != nil {
		t.Fatalf("AttachTagByID() error = %v", err)
	}
	if applied != 3 {
		t.Errorf("applied = %d, want 3 (既に付いていた動画があっても誤りにならない)", applied)
	}
	if ref.ID != tag.ID || ref.Name != "旅行" {
		t.Errorf("ref = %+v, want id=%d name=旅行", ref, tag.ID)
	}
	for _, key := range []string{"key-a", "key-b", "key-c"} {
		if tagCount(t, db, key, tag.ID) != 1 {
			t.Errorf("%s に付いていない", key)
		}
	}
}

// TestAttachTagByIDSkipsUnresolvableVideoIDs は、いまライブラリに無い id
// （消えた動画）を飛ばし、反映した本数だけを数えることを確かめる。
func TestAttachTagByIDSkipsUnresolvableVideoIDs(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	tag, err := db.Tags().CreateTag(ctx, "旅行")
	if err != nil {
		t.Fatal(err)
	}
	ids := upsertAll(t, db, listingFile("/media/a.mp4", "a", "key-a", 0))

	_, applied, err := db.Tags().AttachTagByID(ctx, []int64{ids["/media/a.mp4"], 999999}, tag.ID)
	if err != nil {
		t.Fatalf("AttachTagByID() error = %v", err)
	}
	if applied != 1 {
		t.Errorf("applied = %d, want 1 (消えた動画の id は飛ばす)", applied)
	}
}

// TestAttachTagByIDRejectsMissingTag は、id のタグが無ければ
// domain.ErrTagNotFound を返すことを確かめる。
func TestAttachTagByIDRejectsMissingTag(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	ids := upsertAll(t, db, listingFile("/media/a.mp4", "a", "key-a", 0))

	if _, _, err := db.Tags().AttachTagByID(ctx, []int64{ids["/media/a.mp4"]}, 9999); !errors.Is(err, domain.ErrTagNotFound) {
		t.Errorf("AttachTagByID(無いタグ) error = %v, want ErrTagNotFound", err)
	}
}

// TestAttachTagByNameUsesSynonymToFindOriginalTag は、シノニムの名前で付けると
// 元のタグが付くことを確かめる（完了の条件）。
func TestAttachTagByNameUsesSynonymToFindOriginalTag(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	tag, err := db.Tags().CreateTag(ctx, "アニメ")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Tags().AddSynonym(ctx, tag.ID, "anime-jp", nil); err != nil {
		t.Fatal(err)
	}
	ids := upsertAll(t, db, listingFile("/media/a.mp4", "a", "key-a", 0))

	ref, applied, err := db.Tags().AttachTagByName(ctx, []int64{ids["/media/a.mp4"]}, "anime-jp")
	if err != nil {
		t.Fatalf("AttachTagByName(シノニム) error = %v", err)
	}
	if applied != 1 {
		t.Errorf("applied = %d, want 1", applied)
	}
	if ref.ID != tag.ID || ref.Name != "アニメ" {
		t.Errorf("ref = %+v, want id=%d name=アニメ (元のタグ)", ref, tag.ID)
	}
	if tagCount(t, db, "key-a", tag.ID) != 1 {
		t.Error("元のタグが付いていない")
	}
}

// TestAttachTagByNameCreatesTagWhenMissing は、名前が無ければ同じトランザクション
// で作ることを確かめる（要件 1）。
func TestAttachTagByNameCreatesTagWhenMissing(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	ids := upsertAll(t, db, listingFile("/media/a.mp4", "a", "key-a", 0))

	ref, applied, err := db.Tags().AttachTagByName(ctx, []int64{ids["/media/a.mp4"]}, "新規")
	if err != nil {
		t.Fatalf("AttachTagByName(新しい名前) error = %v", err)
	}
	if applied != 1 {
		t.Errorf("applied = %d, want 1", applied)
	}
	if ref.Name != "新規" {
		t.Errorf("ref.Name = %q, want 新規", ref.Name)
	}
	tags, err := db.Tags().ListTags(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 1 || tags[0].ID != ref.ID {
		t.Errorf("tags = %+v, want 新しく作られた1件", tags)
	}
}

// TestDetachTagIgnoresAlreadyUnassignedVideos は、付いていないタグを外しても
// 誤りにならないことを確かめる（完了の条件）。
func TestDetachTagIgnoresAlreadyUnassignedVideos(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	tag, err := db.Tags().CreateTag(ctx, "旅行")
	if err != nil {
		t.Fatal(err)
	}
	ids := upsertAll(t, db,
		listingFile("/media/a.mp4", "a", "key-a", 0),
		listingFile("/media/b.mp4", "b", "key-b", 1),
	)
	if _, _, err := db.Tags().AttachTagByID(ctx, []int64{ids["/media/a.mp4"]}, tag.ID); err != nil {
		t.Fatal(err)
	}

	// b には付いていないが、外しても誤りにならない。
	ref, applied, err := db.Tags().DetachTag(ctx, []int64{ids["/media/a.mp4"], ids["/media/b.mp4"]}, tag.ID)
	if err != nil {
		t.Fatalf("DetachTag() error = %v", err)
	}
	if applied != 2 {
		t.Errorf("applied = %d, want 2 (付いていない動画も数に入る)", applied)
	}
	if ref.ID != tag.ID {
		t.Errorf("ref.ID = %d, want %d", ref.ID, tag.ID)
	}
	if tagCount(t, db, "key-a", tag.ID) != 0 {
		t.Error("key-a にまだ付いている")
	}

	// もう一度外しても誤りにならない。
	if _, applied, err := db.Tags().DetachTag(ctx, []int64{ids["/media/a.mp4"]}, tag.ID); err != nil || applied != 1 {
		t.Errorf("再度の DetachTag: applied=%d err=%v, want 1 と nil", applied, err)
	}
}

// TestDetachTagRejectsMissingTag は、tagID が無ければ domain.ErrTagNotFound を
// 返すことを確かめる。
func TestDetachTagRejectsMissingTag(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	if _, _, err := db.Tags().DetachTag(ctx, []int64{1}, 9999); !errors.Is(err, domain.ErrTagNotFound) {
		t.Errorf("DetachTag(無いタグ) error = %v, want ErrTagNotFound", err)
	}
}

// TestAttachTagByIDRollsBackEverythingOnMidTransactionFailure は、途中で失敗
// させると1本にも付いていないことを確かめる（完了の条件、Edge Case
// 「一括操作・統合の途中失敗」）。
func TestAttachTagByIDRollsBackEverythingOnMidTransactionFailure(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	tag, err := db.Tags().CreateTag(ctx, "旅行")
	if err != nil {
		t.Fatal(err)
	}
	ids := upsertAll(t, db,
		listingFile("/media/a.mp4", "a", "key-a", 0),
		listingFile("/media/b.mp4", "b", "key-b", 1),
	)

	trigger := fmt.Sprintf(
		`create trigger poison_video_tags_insert before insert on video_tags when new.content_key = 'key-b' and new.tag_id = %d
		   begin select raise(abort, 'poison'); end;`, tag.ID)
	if _, err := db.sql.Exec(trigger); err != nil {
		t.Fatal(err)
	}

	if _, _, err := db.Tags().AttachTagByID(ctx, []int64{ids["/media/a.mp4"], ids["/media/b.mp4"]}, tag.ID); err == nil {
		t.Fatal("AttachTagByID() = nil error, want failure")
	}

	for _, key := range []string{"key-a", "key-b"} {
		if tagCount(t, db, key, tag.ID) != 0 {
			t.Errorf("%s に付いている（ロールバックされていない）", key)
		}
	}
}

// TestSummaryCountsOnlyRegisteredVideosAndPartiallyTaggedTags は、要約で
// 1本だけに付いたタグの数が1、全体が3になることを確かめる（完了の条件）。
func TestSummaryCountsOnlyRegisteredVideosAndPartiallyTaggedTags(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	all, err := db.Tags().CreateTag(ctx, "全部")
	if err != nil {
		t.Fatal(err)
	}
	one, err := db.Tags().CreateTag(ctx, "一部")
	if err != nil {
		t.Fatal(err)
	}
	ids := upsertAll(t, db,
		listingFile("/media/a.mp4", "a", "key-a", 0),
		listingFile("/media/b.mp4", "b", "key-b", 1),
		listingFile("/media/c.mp4", "c", "key-c", 2),
	)
	videoIDs := []int64{ids["/media/a.mp4"], ids["/media/b.mp4"], ids["/media/c.mp4"]}
	if _, _, err := db.Tags().AttachTagByID(ctx, videoIDs, all.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.Tags().AttachTagByID(ctx, []int64{ids["/media/a.mp4"]}, one.ID); err != nil {
		t.Fatal(err)
	}

	// 消えた動画への付与は数に入らない。
	attachTag(t, db, "key-gone", all.ID)

	summary, err := db.Tags().Summary(ctx, append(videoIDs, 999999))
	if err != nil {
		t.Fatalf("Summary() error = %v", err)
	}
	if summary.Total != 3 {
		t.Errorf("Total = %d, want 3", summary.Total)
	}
	if len(summary.Items) != 2 {
		t.Fatalf("len(Items) = %d, want 2: %+v", len(summary.Items), summary.Items)
	}
	// 名前の自然順（一部 → 全部）。
	if summary.Items[0].Tag.ID != one.ID || summary.Items[0].Count != 1 {
		t.Errorf("Items[0] = %+v, want id=%d count=1", summary.Items[0], one.ID)
	}
	if summary.Items[1].Tag.ID != all.ID || summary.Items[1].Count != 3 {
		t.Errorf("Items[1] = %+v, want id=%d count=3", summary.Items[1], all.ID)
	}
}

// TestSummaryOfEmptyVideoIDsIsEmpty は、videoIDs が空なら要約も空になることを
// 確かめる。
func TestSummaryOfEmptyVideoIDsIsEmpty(t *testing.T) {
	db := migratedDB(t)
	summary, err := db.Tags().Summary(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Total != 0 || len(summary.Items) != 0 {
		t.Errorf("summary = %+v, want 空", summary)
	}
}

// TestTagsByContentKeysReturnsOriginalNamesInNaturalOrder は、content_key の
// 集合からタグをまとめて引く操作が、元の名前を自然順で返すことを確かめる
// （PlaybackStore.ProgressByContentKeys と同じ形）。
func TestTagsByContentKeysReturnsOriginalNamesInNaturalOrder(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	tag10, err := db.Tags().CreateTag(ctx, "tag10")
	if err != nil {
		t.Fatal(err)
	}
	tag2, err := db.Tags().CreateTag(ctx, "tag2")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Tags().AddSynonym(ctx, tag2.ID, "second", nil); err != nil {
		t.Fatal(err)
	}
	attachTag(t, db, "key-a", tag10.ID)
	attachTag(t, db, "key-a", tag2.ID)
	attachTag(t, db, "key-b", tag2.ID)

	got, err := db.Tags().TagsByContentKeys(ctx, []string{"key-a", "key-b", "key-none"})
	if err != nil {
		t.Fatalf("TagsByContentKeys() error = %v", err)
	}
	if _, ok := got["key-none"]; ok {
		t.Error("記録の無い content_key が結果に現れている")
	}
	wantA := []domain.TagRef{{ID: tag2.ID, Name: "tag2"}, {ID: tag10.ID, Name: "tag10"}}
	if len(got["key-a"]) != 2 || got["key-a"][0] != wantA[0] || got["key-a"][1] != wantA[1] {
		t.Errorf("key-a = %+v, want %+v (自然順、シノニムでなく元の名前)", got["key-a"], wantA)
	}
	wantB := []domain.TagRef{{ID: tag2.ID, Name: "tag2"}}
	if len(got["key-b"]) != 1 || got["key-b"][0] != wantB[0] {
		t.Errorf("key-b = %+v, want %+v", got["key-b"], wantB)
	}
}

// TestTagsByContentKeysOfEmptySetIsEmpty は、content_key の集合が空なら空の
// map を返すことを確かめる。
func TestTagsByContentKeysOfEmptySetIsEmpty(t *testing.T) {
	db := migratedDB(t)
	got, err := db.Tags().TagsByContentKeys(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("got = %+v, want 空", got)
	}
}
