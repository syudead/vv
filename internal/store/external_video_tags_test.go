package store

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// 外部連携 API の名前でのタグの一括操作（specs/026-external-api/research.md R-7）。

// tagNames は動画のタグを「名前:出所」の並びにする。出所は m（手）と f（フォルダ）。
func tagNames(tags []domain.VideoTag) []string {
	out := []string{}
	for _, tag := range tags {
		label := tag.Name + ":"
		if tag.Manual {
			label += "m"
		}
		if tag.FromFolder {
			label += "f"
		}
		out = append(out, label)
	}
	return out
}

// applyVideoTags は tentative を偽にした一括操作で、各動画の結果だけを返す。
func applyVideoTags(ctx context.Context, db *DB, videos []domain.VideoRef, action domain.VideoTagsAction, names []string) ([]domain.VideoTagsResult, error) {
	outcome, err := db.Tags().ApplyVideoTags(ctx, videos, action, names, false)
	return outcome.Items, err
}

func countTags(t *testing.T, db *DB) int {
	t.Helper()
	var count int
	if err := db.sql.QueryRow(`select count(*) from tags`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

// 受け入れ条件 4・5: パスで指定した動画に、無い名前とシノニムで add すると、タグが作られ、
// シノニムは元のタグとして付く。同じ要求の繰り返しは状態を変えない。
func TestApplyVideoTagsAddCreatesAndResolvesSynonyms(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	cat, err := db.Tags().CreateTag(ctx, "猫")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Tags().AddSynonym(ctx, cat.ID, "ねこ", nil); err != nil {
		t.Fatal(err)
	}
	ids := upsertAll(t, db,
		listingFile(fixturePath("/media/a.mp4"), "a", "key-a", 0),
		listingFile(fixturePath("/media/b.mp4"), "b", "key-b", 1),
	)
	refs := []domain.VideoRef{
		{Path: fixturePath("/media/a.mp4")},
		{ContentKey: "key-b"},
		{ID: ids[fixturePath("/media/a.mp4")]},
	}

	for round := range 2 {
		got, err := applyVideoTags(ctx, db, refs, domain.VideoTagsAdd, []string{" 犬 ", "ねこ", "猫"})
		if err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
		if len(got) != 3 {
			t.Fatalf("round %d: %d 件, want videos の順に 3 件", round, len(got))
		}
		wantIDs := []int64{ids[fixturePath("/media/a.mp4")], ids[fixturePath("/media/b.mp4")], ids[fixturePath("/media/a.mp4")]}
		for i, item := range got {
			if item.VideoID != wantIDs[i] {
				t.Errorf("round %d item %d: id = %d, want %d", round, i, item.VideoID, wantIDs[i])
			}
			if names := tagNames(item.Tags); !reflect.DeepEqual(names, []string{"犬:m", "猫:m"}) {
				t.Errorf("round %d item %d: tags = %v", round, i, names)
			}
		}
		if got[1].ContentKey != "key-b" {
			t.Errorf("contentKey = %q", got[1].ContentKey)
		}
		if n := countTags(t, db); n != 2 {
			t.Errorf("round %d: タグ %d 個, want 2（犬を 1 度だけ作る）", round, n)
		}
	}
	// 画面の API の絞り込みにも出る。
	if got := listIDs(t, db, domain.AudienceOwner, domain.VideoQuery{TagIDs: []int64{cat.ID}}); len(got) != 2 {
		t.Errorf("猫で絞り込み = %v", got)
	}
}

func TestApplyVideoTagsRemoveIgnoresUnknownNames(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	upsertAll(t, db, listingFile(fixturePath("/media/a.mp4"), "a", "key-a", 0))
	ref := []domain.VideoRef{{ContentKey: "key-a"}}
	if _, err := applyVideoTags(ctx, db, ref, domain.VideoTagsAdd, []string{"猫", "犬"}); err != nil {
		t.Fatal(err)
	}
	for round := range 2 {
		got, err := applyVideoTags(ctx, db, ref, domain.VideoTagsRemove, []string{"猫", "無い名前"})
		if err != nil {
			t.Fatal(err)
		}
		if names := tagNames(got[0].Tags); !reflect.DeepEqual(names, []string{"犬:m"}) {
			t.Errorf("round %d: tags = %v", round, names)
		}
	}
	if n := countTags(t, db); n != 2 {
		t.Errorf("remove がタグを作った: %d 個", n)
	}
}

// replace は手で付けたタグだけをちょうど指定の集合にし、フォルダ由来のタグは残す。
func TestApplyVideoTagsReplaceKeepsFolderTags(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	if _, err := db.Tags().CreateTag(ctx, "Anime"); err != nil {
		t.Fatal(err)
	}
	upsertFolderVideo(t, db, fixturePath("/media/Anime/1.mp4"), "k1")
	rebuildIndexForTest(t, db)
	ref := []domain.VideoRef{{ContentKey: "k1"}}
	if _, err := applyVideoTags(ctx, db, ref, domain.VideoTagsAdd, []string{"猫", "犬", "Anime"}); err != nil {
		t.Fatal(err)
	}

	got, err := applyVideoTags(ctx, db, ref, domain.VideoTagsReplace, []string{"犬", "鳥"})
	if err != nil {
		t.Fatal(err)
	}
	if names := tagNames(got[0].Tags); !reflect.DeepEqual(names, []string{"Anime:f", "犬:m", "鳥:m"}) {
		t.Errorf("replace の後 = %v", names)
	}

	got, err = applyVideoTags(ctx, db, ref, domain.VideoTagsReplace, []string{})
	if err != nil {
		t.Fatal(err)
	}
	if names := tagNames(got[0].Tags); !reflect.DeepEqual(names, []string{"Anime:f"}) {
		t.Errorf("空の replace の後 = %v", names)
	}
}

// 動画とタグの組をまとめて書き換える文は、要求に含む動画だけに効き、含まない動画の手で
// 付けたタグは変えない。
func TestApplyVideoTagsChangesOnlyRequestedVideos(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	upsertAll(t, db,
		listingFile(fixturePath("/media/a.mp4"), "a", "key-a", 0),
		listingFile(fixturePath("/media/b.mp4"), "b", "key-b", 1),
		listingFile(fixturePath("/media/c.mp4"), "c", "key-c", 2),
	)
	all := []domain.VideoRef{{ContentKey: "key-a"}, {ContentKey: "key-b"}, {ContentKey: "key-c"}}
	if _, err := applyVideoTags(ctx, db, all, domain.VideoTagsAdd, []string{"猫", "犬"}); err != nil {
		t.Fatal(err)
	}
	if _, err := applyVideoTags(ctx, db, all[:2], domain.VideoTagsReplace, []string{"鳥", "猫"}); err != nil {
		t.Fatal(err)
	}
	if _, err := applyVideoTags(ctx, db, all[:1], domain.VideoTagsRemove, []string{"鳥"}); err != nil {
		t.Fatal(err)
	}

	got, err := applyVideoTags(ctx, db, all, domain.VideoTagsAdd, []string{"猫"})
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"猫:m"}, {"猫:m", "鳥:m"}, {"犬:m", "猫:m"}}
	for i, item := range got {
		if names := tagNames(item.Tags); !reflect.DeepEqual(names, want[i]) {
			t.Errorf("%s: tags = %v, want %v", item.ContentKey, names, want[i])
		}
	}
}

// 引けない動画を 1 つ含む要求は、その位置を持つ誤りで全体を失敗させ、何も反映しない
// （タグも作らない）。
func TestApplyVideoTagsFailsWholeRequestOnMissingVideo(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	upsertAll(t, db, listingFile(fixturePath("/media/a.mp4"), "a", "key-a", 0))
	// 登録フォルダの外の所在だけの動画も引けない。
	outside := upsertAll(t, db, listingFile(fixturePath("/elsewhere/b.mp4"), "b", "key-b", 1))

	for _, tc := range []struct {
		name  string
		refs  []domain.VideoRef
		index int
	}{
		{"無い内容キー", []domain.VideoRef{{ContentKey: "key-a"}, {ContentKey: "missing"}}, 1},
		{"無い id", []domain.VideoRef{{ID: 999999}, {ContentKey: "key-a"}}, 0},
		{"登録外の所在", []domain.VideoRef{{ContentKey: "key-a"}, {Path: fixturePath("/elsewhere/b.mp4")}}, 1},
		{"登録外の動画の id", []domain.VideoRef{{ContentKey: "key-a"}, {ID: outside[fixturePath("/elsewhere/b.mp4")]}}, 1},
		{"形の誤り", []domain.VideoRef{{ContentKey: "key-a"}, {ContentKey: "key-a", Path: fixturePath("/media/a.mp4")}}, 1},
	} {
		_, err := applyVideoTags(ctx, db, tc.refs, domain.VideoTagsAdd, []string{"猫"})
		var notFound *domain.VideoRefNotFoundError
		if !errors.As(err, &notFound) || notFound.Index != tc.index || !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("%s: err = %v, want index %d", tc.name, err, tc.index)
		}
	}
	if n := countTags(t, db); n != 0 {
		t.Errorf("失敗した要求がタグを %d 個作った", n)
	}
	if got := videoTagsOf(t, db, "key-a"); len(got) != 0 {
		t.Errorf("失敗した要求が付けた: %v", got)
	}
}

func TestApplyVideoTagsRejectsInvalidNamesWithIndex(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	upsertAll(t, db, listingFile(fixturePath("/media/a.mp4"), "a", "key-a", 0))
	long := make([]rune, domain.TagNameMaxLength+1)
	for i := range long {
		long[i] = 'a'
	}
	for _, tc := range []struct {
		names   []string
		index   int
		problem domain.TagNameProblem
	}{
		{[]string{"猫", "  "}, 1, domain.TagNameEmpty},
		{[]string{"a\tb"}, 0, domain.TagNameControlCharacters},
		{[]string{"猫", "犬", string(long)}, 2, domain.TagNameTooLong},
	} {
		_, err := applyVideoTags(ctx, db, []domain.VideoRef{{ContentKey: "key-a"}}, domain.VideoTagsReplace, tc.names)
		var at *domain.TagNameAtError
		var invalid *domain.InvalidTagNameError
		if !errors.As(err, &at) || at.Index != tc.index || !errors.As(err, &invalid) || invalid.Problem != tc.problem {
			t.Errorf("%q: err = %v", tc.names, err)
		}
	}
	if n := countTags(t, db); n != 0 {
		t.Errorf("失敗した要求がタグを %d 個作った", n)
	}
}
