package store

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// Set kinds directly so fixture setup is independent of the batch operation under test.
func setTagKindForTest(t *testing.T, db *DB, tagID, kindID int64) {
	t.Helper()
	var value any
	if kindID != 0 {
		value = kindID
	}
	if _, err := db.sql.Exec(`update tags set kind_id = ? where id = ?`, value, tagID); err != nil {
		t.Fatalf("tags.kind_id を書けない: %v", err)
	}
}

func tagKindNames(t *testing.T, db *DB) []string {
	t.Helper()
	kinds, err := db.Tags().ListTagKinds(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		names = append(names, kind.Name)
	}
	return names
}

func kindOf(t *testing.T, db *DB, tagID int64) *domain.TagKindRef {
	t.Helper()
	tag, err := tagByID(context.Background(), db.sql, tagID)
	if err != nil {
		t.Fatal(err)
	}
	return tag.Kind
}

func createTagForTest(t *testing.T, db *DB, name string) int64 {
	t.Helper()
	tag, err := db.Tags().CreateTag(context.Background(), name)
	if err != nil {
		t.Fatal(err)
	}
	return tag.ID
}

func TestTagKindsStartWithPresets(t *testing.T) {
	db := migratedDB(t)
	kinds, err := db.Tags().ListTagKinds(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Genre", "Work", "Place", "Character"}
	if got := tagKindNames(t, db); !slices.Equal(got, want) {
		t.Fatalf("ListTagKinds = %v, want %v", got, want)
	}
	for i, kind := range kinds {
		if kind.ID != int64(i+1) || kind.TagCount != 0 || kind.CreatedAt.IsZero() {
			t.Errorf("kind[%d] = %+v", i, kind)
		}
	}
}

func TestCreateTagKindRejectsDuplicateName(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	createTagForTest(t, db, "Era")

	created, err := db.Tags().CreateTagKind(ctx, "  Era ")
	if err != nil {
		t.Fatalf("タグと同じ名前の種類を作れない: %v", err)
	}
	if created.Name != "Era" || created.ID != 5 || created.TagCount != 0 {
		t.Fatalf("CreateTagKind = %+v", created)
	}

	_, err = db.Tags().CreateTagKind(ctx, "Era")
	var conflict *domain.TagKindNameConflict
	if !errors.As(err, &conflict) || !errors.Is(err, domain.ErrTagKindNameTaken) || conflict.Kind.ID != created.ID {
		t.Fatalf("CreateTagKind(重複) error = %v, want TagKindNameConflict", err)
	}
	if got := tagKindNames(t, db); !slices.Equal(got, []string{"Genre", "Work", "Place", "Character", "Era"}) {
		t.Fatalf("重複のあとの種類 = %v", got)
	}

	for _, name := range []string{"", "  ", "a\nb"} {
		if _, err := db.Tags().CreateTagKind(ctx, name); !errors.Is(err, domain.ErrInvalidTagName) {
			t.Errorf("CreateTagKind(%q) error = %v, want ErrInvalidTagName", name, err)
		}
	}
	if _, err := db.Tags().CreateTagKind(ctx, "genre"); err != nil {
		t.Errorf("CreateTagKind(genre) error = %v", err)
	}
}

func TestRenameTagKind(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	tags := db.Tags()

	same, err := tags.RenameTagKind(ctx, 1, " Genre ")
	if err != nil || same.Name != "Genre" {
		t.Fatalf("同じ名前への変更 = %+v, %v", same, err)
	}
	_, err = tags.RenameTagKind(ctx, 1, "Work")
	var conflict *domain.TagKindNameConflict
	if !errors.As(err, &conflict) || conflict.Kind.ID != 2 {
		t.Fatalf("別の種類の名前への変更 error = %v", err)
	}
	if _, err := tags.RenameTagKind(ctx, 999, "Era"); !errors.Is(err, domain.ErrTagKindNotFound) {
		t.Fatalf("無い id の変更 error = %v", err)
	}
	if _, err := tags.RenameTagKind(ctx, 1, ""); !errors.Is(err, domain.ErrInvalidTagName) {
		t.Fatalf("空の名前への変更 error = %v", err)
	}
	if got := tagKindNames(t, db); !slices.Equal(got, []string{"Genre", "Work", "Place", "Character"}) {
		t.Fatalf("断った変更のあとの種類 = %v", got)
	}
}

func TestTagKindLifecycleKeepsTags(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	tags := db.Tags()
	action := createTagForTest(t, db, "Action")
	drama := createTagForTest(t, db, "Drama")

	kind, err := tags.CreateTagKind(ctx, "Mood")
	if err != nil {
		t.Fatal(err)
	}
	setTagKindForTest(t, db, action, kind.ID)
	setTagKindForTest(t, db, drama, kind.ID)
	if got := kindOf(t, db, action); got == nil || *got != (domain.TagKindRef{ID: kind.ID, Name: "Mood"}) {
		t.Fatalf("付けた種類 = %+v", got)
	}

	renamed, err := tags.RenameTagKind(ctx, kind.ID, "Feeling")
	if err != nil || renamed.Name != "Feeling" || renamed.TagCount != 2 {
		t.Fatalf("RenameTagKind = %+v, %v", renamed, err)
	}
	if got := kindOf(t, db, drama); got == nil || got.Name != "Feeling" {
		t.Fatalf("名前の変更のあとの種類 = %+v", got)
	}

	if err := tags.DeleteTagKind(ctx, kind.ID); err != nil {
		t.Fatal(err)
	}
	if err := tags.DeleteTagKind(ctx, kind.ID); !errors.Is(err, domain.ErrTagKindNotFound) {
		t.Fatalf("消した種類をもう一度消す error = %v", err)
	}
	page, err := tags.ListTags(ctx, domain.TagListQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("種類を消したあとのタグ = %+v", page.Items)
	}
	for _, tag := range page.Items {
		if tag.Kind != nil {
			t.Errorf("%s の種類 = %+v, want nil", tag.Name, tag.Kind)
		}
	}
}

func TestTagReadsCarryKind(t *testing.T) {
	db, ids := tentativeFixture(t)
	ctx := context.Background()
	tags := db.Tags()
	dog := createTagForTest(t, db, "犬")
	setTagKindForTest(t, db, dog, 1)
	attachTag(t, db, "key-a", dog)
	videoIDs := []int64{ids[fixturePath("/media/a.mp4")]}
	genre := domain.TagKindRef{ID: 1, Name: "Genre"}

	page, err := tags.ListTags(ctx, domain.TagListQuery{Search: "犬", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Kind == nil || *page.Items[0].Kind != genre {
		t.Errorf("ListTags = %+v", page.Items)
	}
	if page.Exact == nil || page.Exact.Kind == nil || *page.Exact.Kind != genre {
		t.Errorf("ListTags.Exact = %+v", page.Exact)
	}
	byKey, err := tags.TagsByContentKeys(ctx, []string{"key-a"})
	if err != nil {
		t.Fatal(err)
	}
	if got := byKey["key-a"]; len(got) != 1 || got[0].Kind == nil || *got[0].Kind != genre {
		t.Errorf("TagsByContentKeys = %+v", got)
	}
	summary, err := tags.Summary(ctx, videoIDs)
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.Items) != 1 || summary.Items[0].Tag.Kind == nil || *summary.Items[0].Tag.Kind != genre {
		t.Errorf("Summary = %+v", summary)
	}
	if ref, _, err := tags.AttachTagByName(ctx, videoIDs, "犬"); err != nil || ref.Kind == nil || *ref.Kind != genre {
		t.Errorf("AttachTagByName = %+v, %v", ref, err)
	}
	if ref, _, err := tags.DetachTag(ctx, videoIDs, dog); err != nil || ref.Kind == nil || *ref.Kind != genre {
		t.Errorf("DetachTag = %+v, %v", ref, err)
	}
	kinds, err := tags.ListTagKinds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if kinds[0].TagCount != 1 {
		t.Errorf("Genre の TagCount = %d, want 1", kinds[0].TagCount)
	}
}

func TestMergeKeepsTargetKind(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	target := createTagForTest(t, db, "Target")
	source := createTagForTest(t, db, "Source")
	setTagKindForTest(t, db, target, 1)
	setTagKindForTest(t, db, source, 2)
	if _, err := db.Tags().MergeTags(ctx, target, []int64{source}); err != nil {
		t.Fatal(err)
	}
	if got := kindOf(t, db, target); got == nil || got.ID != 1 {
		t.Fatalf("統合先の種類 = %+v, want Genre", got)
	}

	plain := createTagForTest(t, db, "Plain")
	other := createTagForTest(t, db, "Other")
	setTagKindForTest(t, db, other, 3)
	if _, err := db.Tags().MergeTags(ctx, plain, []int64{other}); err != nil {
		t.Fatal(err)
	}
	if got := kindOf(t, db, plain); got != nil {
		t.Fatalf("種類なしの統合先の種類 = %+v, want nil", got)
	}
}

func TestConfirmTentativeTagKeepsKind(t *testing.T) {
	db, _ := tentativeFixture(t)
	ctx := context.Background()
	applyTentative(t, db, bothVideos, domain.VideoTagsAdd, "犬")
	dog := tagIDByName(t, db, "犬")
	setTagKindForTest(t, db, dog, 4)

	tag, err := db.Tags().ConfirmTag(ctx, dog)
	if err != nil {
		t.Fatal(err)
	}
	if tag.Tentative || tag.Kind == nil || tag.Kind.Name != "Character" {
		t.Fatalf("ConfirmTag = %+v", tag)
	}
}

func TestTagKindsMigrationDown(t *testing.T) {
	db := migratedDB(t)
	downTo(t, db, 47)
	if _, ok := tableColumns(t, db, "tags")["kind_id"]; ok {
		t.Fatal("Down のあとも tags.kind_id が残っている")
	}
	var tables int
	if err := db.sql.QueryRow(`select count(*) from sqlite_master where name in ('tag_kinds', 'tags_kind_id')`).
		Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if tables != 0 {
		t.Fatal("Down のあとも tag_kinds か tags_kind_id が残っている")
	}
	if _, err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if got := tagKindNames(t, db); len(got) != 4 {
		t.Fatalf("Up のあとの種類 = %v", got)
	}
}
