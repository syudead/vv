package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// attachTag は video_tags に直接1行足す。動画へのタグの付け外しは #265 の
// 範囲なので、この feature のテストは SQL で直接付ける。
func attachTag(t *testing.T, db *DB, contentKey string, tagID int64) {
	t.Helper()
	if _, err := db.sql.Exec(
		`insert or ignore into video_tags (content_key, tag_id, created_at) values (?, ?, ?)`,
		contentKey, tagID, time.Now().Unix(),
	); err != nil {
		t.Fatalf("video_tags へ付けられない: %v", err)
	}
}

// tagNameRow はタグ名1行の search_key・search_version を読む。
func tagNameRow(t *testing.T, db *DB, name string) (searchKey string, searchVersion int64) {
	t.Helper()
	if err := db.sql.QueryRow(`select search_key, search_version from tag_names where name = ?`, name).
		Scan(&searchKey, &searchVersion); err != nil {
		t.Fatalf("タグ名の行を読めない (%s): %v", name, err)
	}
	return searchKey, searchVersion
}

func TestCreateTagNormalizesAndRejectsInvalidNames(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	tag, err := db.Tags().CreateTag(ctx, "  旅行  ")
	if err != nil {
		t.Fatalf("CreateTag() error = %v", err)
	}
	if tag.Name != "旅行" {
		t.Errorf("Name = %q, want %q (前後の空白は取れる)", tag.Name, "旅行")
	}
	if tag.ID == 0 {
		t.Error("ID が入っていない")
	}
	if len(tag.Synonyms) != 0 {
		t.Errorf("Synonyms = %v, want 空", tag.Synonyms)
	}
	if tag.VideoCount != 0 {
		t.Errorf("VideoCount = %d, want 0", tag.VideoCount)
	}

	for _, name := range []string{"", "   ", "旅行\n", "\t旅行", "旅行\r"} {
		if _, err := db.Tags().CreateTag(ctx, name); !errors.Is(err, domain.ErrInvalidTagName) {
			t.Errorf("CreateTag(%q) error = %v, want ErrInvalidTagName", name, err)
		}
	}

	long := ""
	for range 101 {
		long += "あ"
	}
	if _, err := db.Tags().CreateTag(ctx, long); !errors.Is(err, domain.ErrInvalidTagName) {
		t.Errorf("CreateTag(101 符号位置) error = %v, want ErrInvalidTagName", err)
	}
}

func TestCreateTagKeepsCaseDistinctNames(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	upper, err := db.Tags().CreateTag(ctx, "Anime")
	if err != nil {
		t.Fatalf("CreateTag(Anime) error = %v", err)
	}
	lower, err := db.Tags().CreateTag(ctx, "anime")
	if err != nil {
		t.Fatalf("CreateTag(anime) error = %v, want 別のタグとして作れる", err)
	}
	if upper.ID == lower.ID {
		t.Error("Anime と anime が同じタグになっている")
	}
}

func TestCreateTagRejectsNameAlreadyTaken(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	tag, err := db.Tags().CreateTag(ctx, "旅行")
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Tags().CreateTag(ctx, "旅行")
	var conflict *domain.TagNameConflict
	if !errors.As(err, &conflict) {
		t.Fatalf("CreateTag() error = %v, want *domain.TagNameConflict", err)
	}
	if conflict.Tag.ID != tag.ID {
		t.Errorf("conflict.Tag.ID = %d, want %d", conflict.Tag.ID, tag.ID)
	}
}

func TestRenameTagToSameNameIsNoop(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	tag, err := db.Tags().CreateTag(ctx, "旅行")
	if err != nil {
		t.Fatal(err)
	}
	renamed, err := db.Tags().RenameTag(ctx, tag.ID, "旅行")
	if err != nil {
		t.Fatalf("RenameTag() error = %v", err)
	}
	if renamed.Name != "旅行" {
		t.Errorf("Name = %q, want %q", renamed.Name, "旅行")
	}
}

func TestRenameTagRejectsExistingNameOrSynonym(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	other, err := db.Tags().CreateTag(ctx, "アニメ")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Tags().AddSynonym(ctx, other.ID, "anime-jp", nil); err != nil {
		t.Fatal(err)
	}
	tag, err := db.Tags().CreateTag(ctx, "旅行")
	if err != nil {
		t.Fatal(err)
	}

	// 既存の元の名前への改名。
	_, err = db.Tags().RenameTag(ctx, tag.ID, "アニメ")
	var conflict *domain.TagNameConflict
	if !errors.As(err, &conflict) {
		t.Fatalf("RenameTag(既存の元の名前) error = %v, want *domain.TagNameConflict", err)
	}
	if conflict.Tag.ID != other.ID {
		t.Errorf("conflict.Tag.ID = %d, want %d", conflict.Tag.ID, other.ID)
	}

	// 既存のシノニムへの改名。
	_, err = db.Tags().RenameTag(ctx, tag.ID, "anime-jp")
	if !errors.As(err, &conflict) {
		t.Fatalf("RenameTag(既存のシノニム) error = %v, want *domain.TagNameConflict", err)
	}
	if conflict.Tag.ID != other.ID {
		t.Errorf("conflict.Tag.ID = %d, want %d", conflict.Tag.ID, other.ID)
	}

	// 自分自身のシノニムへの改名も誤り。
	if _, err := db.Tags().AddSynonym(ctx, tag.ID, "旅", nil); err != nil {
		t.Fatal(err)
	}
	_, err = db.Tags().RenameTag(ctx, tag.ID, "旅")
	if !errors.As(err, &conflict) {
		t.Fatalf("RenameTag(自分のシノニム) error = %v, want *domain.TagNameConflict", err)
	}
	if conflict.Tag.ID != tag.ID {
		t.Errorf("conflict.Tag.ID = %d, want %d", conflict.Tag.ID, tag.ID)
	}
}

func TestMergeTagMovesContentAndSynonyms(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	x, err := db.Tags().CreateTag(ctx, "X")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Tags().AddSynonym(ctx, x.ID, "Xの別名", nil); err != nil {
		t.Fatal(err)
	}
	y, err := db.Tags().CreateTag(ctx, "Y")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := db.ScanIndex().UpsertVideo(ctx, sampleFile("/media/x-only.mp4", "x-only", "key-x", 1, 0)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ScanIndex().UpsertVideo(ctx, sampleFile("/media/both.mp4", "both", "key-both", 1, 0)); err != nil {
		t.Fatal(err)
	}
	attachTag(t, db, "key-x", x.ID)
	attachTag(t, db, "key-both", x.ID)
	attachTag(t, db, "key-both", y.ID)

	merged, err := db.Tags().MergeTag(ctx, y.ID, x.ID)
	if err != nil {
		t.Fatalf("MergeTag() error = %v", err)
	}
	if merged.ID != y.ID || merged.Name != "Y" {
		t.Errorf("merged = %+v, want id=%d name=Y", merged, y.ID)
	}

	// X が付いていた中身すべてに Y が1つだけ付く。
	for _, key := range []string{"key-x", "key-both"} {
		var count int
		if err := db.sql.QueryRow(`select count(*) from video_tags where content_key = ? and tag_id = ?`, key, y.ID).
			Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Errorf("content_key=%s の Y の付与数 = %d, want 1", key, count)
		}
	}

	// X の元の名前とシノニムが Y のシノニムになる。
	tags, err := db.Tags().ListTags(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, tag := range tags {
		if tag.ID == x.ID {
			t.Fatalf("統合元 X が一覧に残っている: %+v", tag)
		}
		if tag.ID == y.ID {
			found = true
			want := []string{"X", "Xの別名"}
			if len(tag.Synonyms) != len(want) {
				t.Fatalf("Synonyms = %v, want %v", tag.Synonyms, want)
			}
			for i, name := range want {
				if tag.Synonyms[i] != name {
					t.Errorf("Synonyms[%d] = %q, want %q", i, tag.Synonyms[i], name)
				}
			}
		}
	}
	if !found {
		t.Fatal("統合先 Y が一覧に無い")
	}

	// X の名前で引くと Y になる。
	lookup, foundX, err := lookupTagName(ctx, db.sql, "X")
	if err != nil {
		t.Fatal(err)
	}
	if !foundX {
		t.Fatal("X の名前が引けない")
	}
	if lookup.tagID != y.ID {
		t.Errorf("lookupTagName(X).tagID = %d, want %d", lookup.tagID, y.ID)
	}
}

func TestAddSynonymMergeRequiredAndAccepted(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	upper, err := db.Tags().CreateTag(ctx, "Anime")
	if err != nil {
		t.Fatal(err)
	}
	lower, err := db.Tags().CreateTag(ctx, "anime")
	if err != nil {
		t.Fatal(err)
	}
	for i := range 10 {
		key := fmt.Sprintf("anime-%d", i)
		if _, err := db.ScanIndex().UpsertVideo(ctx, sampleFile(fmt.Sprintf("/media/anime-%d.mp4", i), key, key, 1, 0)); err != nil {
			t.Fatal(err)
		}
		attachTag(t, db, key, lower.ID)
	}

	// 承諾が無い。
	_, err = db.Tags().AddSynonym(ctx, upper.ID, "anime", nil)
	var mergeRequired *domain.TagMergeRequired
	if !errors.As(err, &mergeRequired) {
		t.Fatalf("AddSynonym(承諾無し) error = %v, want *domain.TagMergeRequired", err)
	}
	if mergeRequired.Tag.ID != lower.ID {
		t.Errorf("mergeRequired.Tag.ID = %d, want %d", mergeRequired.Tag.ID, lower.ID)
	}

	// 違う id の承諾。
	wrong := upper.ID
	_, err = db.Tags().AddSynonym(ctx, upper.ID, "anime", &wrong)
	if !errors.As(err, &mergeRequired) {
		t.Fatalf("AddSynonym(違う承諾) error = %v, want *domain.TagMergeRequired", err)
	}

	// ここまで何も変わっていない。
	tagsBefore, err := db.Tags().ListTags(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(tagsBefore) != 2 {
		t.Fatalf("len(tagsBefore) = %d, want 2 (何も変わっていない)", len(tagsBefore))
	}
	for _, tag := range tagsBefore {
		switch tag.ID {
		case upper.ID:
			if tag.VideoCount != 0 || len(tag.Synonyms) != 0 {
				t.Errorf("Anime = %+v, want 0 本・シノニム無し (何も変わっていない)", tag)
			}
		case lower.ID:
			if tag.VideoCount != 10 {
				t.Errorf("anime.VideoCount = %d, want 10 (何も変わっていない)", tag.VideoCount)
			}
		}
	}

	// 正しい承諾。
	accepted := lower.ID
	tag, err := db.Tags().AddSynonym(ctx, upper.ID, "anime", &accepted)
	if err != nil {
		t.Fatalf("AddSynonym(正しい承諾) error = %v", err)
	}
	if tag.VideoCount != 10 {
		t.Errorf("VideoCount = %d, want 10", tag.VideoCount)
	}
	if len(tag.Synonyms) != 1 || tag.Synonyms[0] != "anime" {
		t.Errorf("Synonyms = %v, want [anime]", tag.Synonyms)
	}

	tagsAfter, err := db.Tags().ListTags(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(tagsAfter) != 1 {
		t.Fatalf("len(tagsAfter) = %d, want 1 (統合された)", len(tagsAfter))
	}
}

func TestAddSynonymRejectsNameOwnedByAnotherTag(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	other, err := db.Tags().CreateTag(ctx, "アニメ")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Tags().AddSynonym(ctx, other.ID, "anime-jp", nil); err != nil {
		t.Fatal(err)
	}
	tag, err := db.Tags().CreateTag(ctx, "旅行")
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Tags().AddSynonym(ctx, tag.ID, "anime-jp", nil)
	var conflict *domain.TagNameConflict
	if !errors.As(err, &conflict) {
		t.Fatalf("AddSynonym(別のタグのシノニム) error = %v, want *domain.TagNameConflict", err)
	}
	if conflict.Tag.ID != other.ID {
		t.Errorf("conflict.Tag.ID = %d, want %d", conflict.Tag.ID, other.ID)
	}
}

func TestVideoCountExcludesUnregisteredVideosAndMergeReachesThem(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	x, err := db.Tags().CreateTag(ctx, "X")
	if err != nil {
		t.Fatal(err)
	}
	y, err := db.Tags().CreateTag(ctx, "Y")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ScanIndex().UpsertVideo(ctx, sampleFile("/media/registered.mp4", "registered", "key-registered", 1, 0)); err != nil {
		t.Fatal(err)
	}
	attachTag(t, db, "key-registered", x.ID)
	// いまライブラリに無い動画（video_locations を持たない content_key）への付与。
	attachTag(t, db, "key-gone", x.ID)

	tags, err := db.Tags().ListTags(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, tag := range tags {
		if tag.ID == x.ID && tag.VideoCount != 1 {
			t.Errorf("VideoCount = %d, want 1 (所在が消えた動画を数えない)", tag.VideoCount)
		}
	}

	if _, err := db.Tags().MergeTag(ctx, y.ID, x.ID); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"key-registered", "key-gone"} {
		var count int
		if err := db.sql.QueryRow(`select count(*) from video_tags where content_key = ? and tag_id = ?`, key, y.ID).
			Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Errorf("統合が %s の付与に及んでいない: count = %d", key, count)
		}
	}
}

func TestDeleteTagCascadesToUnregisteredVideoAssignments(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	tag, err := db.Tags().CreateTag(ctx, "旅行")
	if err != nil {
		t.Fatal(err)
	}
	attachTag(t, db, "key-gone", tag.ID)

	if err := db.Tags().DeleteTag(ctx, tag.ID); err != nil {
		t.Fatalf("DeleteTag() error = %v", err)
	}
	var count int
	if err := db.sql.QueryRow(`select count(*) from video_tags where tag_id = ?`, tag.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("video_tags が %d 行残っている、want 0", count)
	}
	if err := db.Tags().DeleteTag(ctx, tag.ID); !errors.Is(err, domain.ErrTagNotFound) {
		t.Errorf("DeleteTag(無いタグ) error = %v, want ErrTagNotFound", err)
	}
}

// トリガーで統合の最後の一手（統合元の削除）を失敗させ、何も残らないことを
// 確かめる（Edge Case「一括操作・統合の途中失敗」）。
func TestMergeTagRollsBackEverythingOnMidTransactionFailure(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	target, err := db.Tags().CreateTag(ctx, "Anime")
	if err != nil {
		t.Fatal(err)
	}
	source, err := db.Tags().CreateTag(ctx, "anime")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ScanIndex().UpsertVideo(ctx, sampleFile("/media/a.mp4", "a", "key-a", 1, 0)); err != nil {
		t.Fatal(err)
	}
	attachTag(t, db, "key-a", source.ID)

	trigger := fmt.Sprintf(
		`create trigger poison_tag_delete before delete on tags when old.id = %d
		   begin select raise(abort, 'poison'); end;`, source.ID)
	if _, err := db.sql.Exec(trigger); err != nil {
		t.Fatal(err)
	}

	if _, err := db.Tags().MergeTag(ctx, target.ID, source.ID); err == nil {
		t.Fatal("MergeTag() = nil error, want failure")
	}

	var targetAssignments int
	if err := db.sql.QueryRow(`select count(*) from video_tags where tag_id = ?`, target.ID).Scan(&targetAssignments); err != nil {
		t.Fatal(err)
	}
	if targetAssignments != 0 {
		t.Errorf("target の video_tags = %d, want 0 (ロールバックされていない)", targetAssignments)
	}

	sourceName, err := canonicalNameByTagID(ctx, db.sql, source.ID)
	if err != nil {
		t.Fatalf("統合元が消えている（ロールバックされていない）: %v", err)
	}
	if sourceName != "anime" {
		t.Errorf("統合元の名前 = %q, want anime", sourceName)
	}

	var sourceAssignments int
	if err := db.sql.QueryRow(`select count(*) from video_tags where tag_id = ? and content_key = ?`, source.ID, "key-a").
		Scan(&sourceAssignments); err != nil {
		t.Fatal(err)
	}
	if sourceAssignments != 1 {
		t.Errorf("統合元の付与 = %d, want 1 (ロールバックされていない)", sourceAssignments)
	}
}

func TestSearchKeysAreWrittenOnCreateRenameAndSynonymAndRefreshed(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	tag, err := db.Tags().CreateTag(ctx, "Anime")
	if err != nil {
		t.Fatal(err)
	}
	if key, version := tagNameRow(t, db, "Anime"); key != domain.FoldForMatch("Anime") || version != domain.SearchKeyVersion {
		t.Errorf("作成後の search_key/version = %q/%d, want %q/%d", key, version, domain.FoldForMatch("Anime"), domain.SearchKeyVersion)
	}

	if _, err := db.Tags().RenameTag(ctx, tag.ID, "アニメ"); err != nil {
		t.Fatal(err)
	}
	if key, version := tagNameRow(t, db, "アニメ"); key != domain.FoldForMatch("アニメ") || version != domain.SearchKeyVersion {
		t.Errorf("改名後の search_key/version = %q/%d, want %q/%d", key, version, domain.FoldForMatch("アニメ"), domain.SearchKeyVersion)
	}

	if _, err := db.Tags().AddSynonym(ctx, tag.ID, "anime-jp", nil); err != nil {
		t.Fatal(err)
	}
	if key, version := tagNameRow(t, db, "anime-jp"); key != domain.FoldForMatch("anime-jp") || version != domain.SearchKeyVersion {
		t.Errorf("シノニム登録後の search_key/version = %q/%d, want %q/%d", key, version, domain.FoldForMatch("anime-jp"), domain.SearchKeyVersion)
	}

	// 版を 0 に戻した行が、起動時の作り直しで埋まる。
	if _, err := db.sql.Exec(`update tag_names set search_key = '', search_version = 0 where name = ?`, "アニメ"); err != nil {
		t.Fatal(err)
	}
	refreshed, err := db.Tags().RefreshSearchKeys(ctx)
	if err != nil {
		t.Fatalf("RefreshSearchKeys() error = %v", err)
	}
	if refreshed != 1 {
		t.Errorf("refreshed = %d, want 1", refreshed)
	}
	if key, version := tagNameRow(t, db, "アニメ"); key != domain.FoldForMatch("アニメ") || version != domain.SearchKeyVersion {
		t.Errorf("作り直し後の search_key/version = %q/%d, want %q/%d", key, version, domain.FoldForMatch("アニメ"), domain.SearchKeyVersion)
	}
}

func TestListTagsIncludesZeroCountTagsInNaturalOrder(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	for _, name := range []string{"tag10", "tag2", "tag1"} {
		if _, err := db.Tags().CreateTag(ctx, name); err != nil {
			t.Fatal(err)
		}
	}

	tags, err := db.Tags().ListTags(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"tag1", "tag2", "tag10"}
	if len(tags) != len(want) {
		t.Fatalf("len(tags) = %d, want %d", len(tags), len(want))
	}
	for i, name := range want {
		if tags[i].Name != name {
			t.Errorf("tags[%d].Name = %q, want %q", i, tags[i].Name, name)
		}
		if tags[i].VideoCount != 0 {
			t.Errorf("tags[%d].VideoCount = %d, want 0", i, tags[i].VideoCount)
		}
	}
}

func TestRemoveSynonymIsNoopWhenNotASynonym(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	tag, err := db.Tags().CreateTag(ctx, "旅行")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Tags().RemoveSynonym(ctx, tag.ID, "無い名前"); err != nil {
		t.Fatalf("RemoveSynonym() error = %v, want nil (何も変えない)", err)
	}

	if _, err := db.Tags().AddSynonym(ctx, tag.ID, "旅", nil); err != nil {
		t.Fatal(err)
	}
	if err := db.Tags().RemoveSynonym(ctx, tag.ID, "旅"); err != nil {
		t.Fatal(err)
	}
	got, err := db.Tags().ListTags(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || len(got[0].Synonyms) != 0 {
		t.Errorf("got = %+v, want シノニムが消えている", got)
	}

	if err := db.Tags().RemoveSynonym(ctx, 9999, "旅"); !errors.Is(err, domain.ErrTagNotFound) {
		t.Errorf("RemoveSynonym(無いタグ) error = %v, want ErrTagNotFound", err)
	}
}
