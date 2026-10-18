package store

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func tagCount(t *testing.T, db *DB, contentKey string, tagID int64) int {
	t.Helper()
	var count int
	if err := db.sql.QueryRow(`select count(*) from video_tags where content_key = ? and tag_id = ?`, contentKey, tagID).
		Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestAttachTagByIDAppliesToAllRequestedVideos(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	tag, err := db.Tags().CreateTag(ctx, "旅行")
	if err != nil {
		t.Fatal(err)
	}
	ids := upsertAll(t, db,
		listingFile(fixturePath("/media/a.mp4"), "a", "key-a", 0),
		listingFile(fixturePath("/media/b.mp4"), "b", "key-b", 1),
		listingFile(fixturePath("/media/c.mp4"), "c", "key-c", 2),
	)
	videoIDs := []int64{ids[fixturePath("/media/a.mp4")], ids[fixturePath("/media/b.mp4")], ids[fixturePath("/media/c.mp4")]}

	if _, applied, err := db.Tags().AttachTagByID(ctx, []int64{ids[fixturePath("/media/b.mp4")]}, tag.ID); err != nil || applied != 1 {
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

func TestAttachTagByIDSkipsUnresolvableVideoIDs(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	tag, err := db.Tags().CreateTag(ctx, "旅行")
	if err != nil {
		t.Fatal(err)
	}
	ids := upsertAll(t, db, listingFile(fixturePath("/media/a.mp4"), "a", "key-a", 0))

	_, applied, err := db.Tags().AttachTagByID(ctx, []int64{ids[fixturePath("/media/a.mp4")], 999999}, tag.ID)
	if err != nil {
		t.Fatalf("AttachTagByID() error = %v", err)
	}
	if applied != 1 {
		t.Errorf("applied = %d, want 1 (消えた動画の id は飛ばす)", applied)
	}
}

func TestAttachTagByIDCountsDuplicateVideoIDsOnce(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	tag, err := db.Tags().CreateTag(ctx, "旅行")
	if err != nil {
		t.Fatal(err)
	}
	ids := upsertAll(t, db, listingFile(fixturePath("/media/a.mp4"), "a", "key-a", 0))
	id := ids[fixturePath("/media/a.mp4")]

	_, applied, err := db.Tags().AttachTagByID(ctx, []int64{id, id, id}, tag.ID)
	if err != nil {
		t.Fatalf("AttachTagByID() error = %v", err)
	}
	if applied != 1 {
		t.Errorf("applied = %d, want 1 (重複した id は1つとして数える)", applied)
	}
	if tagCount(t, db, "key-a", tag.ID) != 1 {
		t.Error("key-a に付いていない")
	}
}

func TestAttachTagByIDRejectsMissingTag(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	ids := upsertAll(t, db, listingFile(fixturePath("/media/a.mp4"), "a", "key-a", 0))

	if _, _, err := db.Tags().AttachTagByID(ctx, []int64{ids[fixturePath("/media/a.mp4")]}, 9999); !errors.Is(err, domain.ErrTagNotFound) {
		t.Errorf("AttachTagByID(無いタグ) error = %v, want ErrTagNotFound", err)
	}
}

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
	ids := upsertAll(t, db, listingFile(fixturePath("/media/a.mp4"), "a", "key-a", 0))

	ref, applied, err := db.Tags().AttachTagByName(ctx, []int64{ids[fixturePath("/media/a.mp4")]}, "anime-jp")
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

func TestAttachTagByNameCreatesTagWhenMissing(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	ids := upsertAll(t, db, listingFile(fixturePath("/media/a.mp4"), "a", "key-a", 0))

	ref, applied, err := db.Tags().AttachTagByName(ctx, []int64{ids[fixturePath("/media/a.mp4")]}, "新規")
	if err != nil {
		t.Fatalf("AttachTagByName(新しい名前) error = %v", err)
	}
	if applied != 1 {
		t.Errorf("applied = %d, want 1", applied)
	}
	if ref.Name != "新規" {
		t.Errorf("ref.Name = %q, want 新規", ref.Name)
	}
	tagsPage, err := db.Tags().ListTags(ctx, domain.TagListQuery{})
	if err != nil {
		t.Fatal(err)
	}
	tags := tagsPage.Items
	if len(tags) != 1 || tags[0].ID != ref.ID {
		t.Errorf("tags = %+v, want 新しく作られた1件", tags)
	}
}

func TestDetachTagIgnoresAlreadyUnassignedVideos(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	tag, err := db.Tags().CreateTag(ctx, "旅行")
	if err != nil {
		t.Fatal(err)
	}
	ids := upsertAll(t, db,
		listingFile(fixturePath("/media/a.mp4"), "a", "key-a", 0),
		listingFile(fixturePath("/media/b.mp4"), "b", "key-b", 1),
	)
	if _, _, err := db.Tags().AttachTagByID(ctx, []int64{ids[fixturePath("/media/a.mp4")]}, tag.ID); err != nil {
		t.Fatal(err)
	}

	ref, applied, err := db.Tags().DetachTag(ctx, []int64{ids[fixturePath("/media/a.mp4")], ids[fixturePath("/media/b.mp4")]}, tag.ID)
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

	if _, applied, err := db.Tags().DetachTag(ctx, []int64{ids[fixturePath("/media/a.mp4")]}, tag.ID); err != nil || applied != 1 {
		t.Errorf("再度の DetachTag: applied=%d err=%v, want 1 と nil", applied, err)
	}
}

func TestDetachTagCountsDuplicateVideoIDsOnce(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	tag, err := db.Tags().CreateTag(ctx, "旅行")
	if err != nil {
		t.Fatal(err)
	}
	ids := upsertAll(t, db, listingFile(fixturePath("/media/a.mp4"), "a", "key-a", 0))
	id := ids[fixturePath("/media/a.mp4")]
	if _, _, err := db.Tags().AttachTagByID(ctx, []int64{id}, tag.ID); err != nil {
		t.Fatal(err)
	}

	_, applied, err := db.Tags().DetachTag(ctx, []int64{id, id}, tag.ID)
	if err != nil {
		t.Fatalf("DetachTag() error = %v", err)
	}
	if applied != 1 {
		t.Errorf("applied = %d, want 1 (重複した id は1つとして数える)", applied)
	}
	if tagCount(t, db, "key-a", tag.ID) != 0 {
		t.Error("key-a にまだ付いている")
	}
}

func TestDetachTagRejectsMissingTag(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	if _, _, err := db.Tags().DetachTag(ctx, []int64{1}, 9999); !errors.Is(err, domain.ErrTagNotFound) {
		t.Errorf("DetachTag(無いタグ) error = %v, want ErrTagNotFound", err)
	}
}

func TestAttachTagByIDRollsBackEverythingOnMidTransactionFailure(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	tag, err := db.Tags().CreateTag(ctx, "旅行")
	if err != nil {
		t.Fatal(err)
	}
	ids := upsertAll(t, db,
		listingFile(fixturePath("/media/a.mp4"), "a", "key-a", 0),
		listingFile(fixturePath("/media/b.mp4"), "b", "key-b", 1),
	)

	trigger := fmt.Sprintf(
		`create trigger poison_video_tags_insert before insert on video_tags when new.content_key = 'key-b' and new.tag_id = %d
		   begin select raise(abort, 'poison'); end;`, tag.ID)
	if _, err := db.sql.Exec(trigger); err != nil {
		t.Fatal(err)
	}

	if _, _, err := db.Tags().AttachTagByID(ctx, []int64{ids[fixturePath("/media/a.mp4")], ids[fixturePath("/media/b.mp4")]}, tag.ID); err == nil {
		t.Fatal("AttachTagByID() = nil error, want failure")
	}

	for _, key := range []string{"key-a", "key-b"} {
		if tagCount(t, db, key, tag.ID) != 0 {
			t.Errorf("%s に付いている（ロールバックされていない）", key)
		}
	}
}

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
		listingFile(fixturePath("/media/a.mp4"), "a", "key-a", 0),
		listingFile(fixturePath("/media/b.mp4"), "b", "key-b", 1),
		listingFile(fixturePath("/media/c.mp4"), "c", "key-c", 2),
	)
	videoIDs := []int64{ids[fixturePath("/media/a.mp4")], ids[fixturePath("/media/b.mp4")], ids[fixturePath("/media/c.mp4")]}
	if _, _, err := db.Tags().AttachTagByID(ctx, videoIDs, all.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.Tags().AttachTagByID(ctx, []int64{ids[fixturePath("/media/a.mp4")]}, one.ID); err != nil {
		t.Fatal(err)
	}

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
	if summary.Items[0].Tag.ID != one.ID || summary.Items[0].Count != 1 {
		t.Errorf("Items[0] = %+v, want id=%d count=1", summary.Items[0], one.ID)
	}
	if summary.Items[1].Tag.ID != all.ID || summary.Items[1].Count != 3 {
		t.Errorf("Items[1] = %+v, want id=%d count=3", summary.Items[1], all.ID)
	}
}

func TestSummaryCountsDuplicateVideoIDsOnce(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	tag, err := db.Tags().CreateTag(ctx, "旅行")
	if err != nil {
		t.Fatal(err)
	}
	ids := upsertAll(t, db, listingFile(fixturePath("/media/a.mp4"), "a", "key-a", 0))
	id := ids[fixturePath("/media/a.mp4")]
	if _, _, err := db.Tags().AttachTagByID(ctx, []int64{id}, tag.ID); err != nil {
		t.Fatal(err)
	}

	summary, err := db.Tags().Summary(ctx, []int64{id, id, id})
	if err != nil {
		t.Fatalf("Summary() error = %v", err)
	}
	if summary.Total != 1 {
		t.Errorf("Total = %d, want 1 (重複した id は1つとして数える)", summary.Total)
	}
	if len(summary.Items) != 1 || summary.Items[0].Count != 1 {
		t.Errorf("Items = %+v, want [{タグ=%d count=1}]", summary.Items, tag.ID)
	}
}

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
	wantA := []domain.VideoTag{
		{TagRef: domain.TagRef{ID: tag2.ID, Name: "tag2"}, Manual: true},
		{TagRef: domain.TagRef{ID: tag10.ID, Name: "tag10"}, Manual: true},
	}
	if len(got["key-a"]) != 2 || got["key-a"][0] != wantA[0] || got["key-a"][1] != wantA[1] {
		t.Errorf("key-a = %+v, want %+v (自然順、シノニムでなく元の名前)", got["key-a"], wantA)
	}
	wantB := []domain.VideoTag{{TagRef: domain.TagRef{ID: tag2.ID, Name: "tag2"}, Manual: true}}
	if len(got["key-b"]) != 1 || got["key-b"][0] != wantB[0] {
		t.Errorf("key-b = %+v, want %+v", got["key-b"], wantB)
	}
}

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
