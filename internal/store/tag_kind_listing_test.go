package store

import (
	"context"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// タグの種類による一覧の絞り込み（specs/051-tag-kinds/research.md R-5、data-model.md の Rules、
// contracts/screen-api.md の List parameter tagKind、#1151）。

type tagKindListing struct {
	db               *DB
	genre, work      domain.TagKind
	action           domain.Tag
	folder, manual   int64
	both             int64
	workVideo, plain int64
}

// tagKindListingFixture は、ジャンルの種類のタグ Anime をフォルダ名から（folder）、ジャンルの
// Action を手で（manual・both）、作品の種類のタグを手で（workVideo）、種類なしの
// タグを手で（plain）付けた動画を作る。both には種類なしのタグも付ける。
func tagKindListingFixture(t *testing.T) tagKindListing {
	t.Helper()
	db := migratedDB(t)
	ctx := context.Background()
	f := tagKindListing{db: db}
	var err error
	if f.genre, err = db.Tags().CreateTagKind(ctx, "ジャンル"); err != nil {
		t.Fatal(err)
	}
	if f.work, err = db.Tags().CreateTagKind(ctx, "作品"); err != nil {
		t.Fatal(err)
	}
	anime := createTestTag(t, db, "Anime")
	f.action = createTestTag(t, db, "Action")
	title := createTestTag(t, db, "Title")
	untyped := createTestTag(t, db, "Untyped")
	setTagKindForTest(t, db, anime.ID, f.genre.ID)
	setTagKindForTest(t, db, f.action.ID, f.genre.ID)
	setTagKindForTest(t, db, title.ID, f.work.ID)

	f.folder = upsertFolderVideo(t, db, fixturePath("/media/Anime/1.mp4"), "k-folder")
	f.manual = upsertFolderVideo(t, db, fixturePath("/media/manual.mp4"), "k-manual")
	f.both = upsertFolderVideo(t, db, fixturePath("/media/both.mp4"), "k-both")
	f.workVideo = upsertFolderVideo(t, db, fixturePath("/media/work.mp4"), "k-work")
	f.plain = upsertFolderVideo(t, db, fixturePath("/media/plain.mp4"), "k-plain")
	upsertFolderVideo(t, db, fixturePath("/media/none.mp4"), "k-none")
	rebuildIndexForTest(t, db)
	attachTag(t, db, "k-manual", f.action.ID)
	attachTag(t, db, "k-both", f.action.ID)
	attachTag(t, db, "k-both", untyped.ID)
	attachTag(t, db, "k-work", title.ID)
	attachTag(t, db, "k-plain", untyped.ID)
	return f
}

func sortedIDs(ids ...int64) []int64 {
	slices.Sort(ids)
	return ids
}

// 動画は、その種類のタグが手で付いていてもフォルダ名から付いていても当たり、別の種類か種類なしの
// タグだけの動画は当たらない。tag と AND で組み合わさり、無い種類は空の一覧になる。total も数える。
func TestListVideosFiltersByTagKind(t *testing.T) {
	f := tagKindListingFixture(t)
	cases := []struct {
		name string
		q    domain.VideoQuery
		want []int64
	}{
		{"手で付けた分とフォルダ名の分", domain.VideoQuery{TagKindID: f.genre.ID}, sortedIDs(f.folder, f.manual, f.both)},
		{"別の種類", domain.VideoQuery{TagKindID: f.work.ID}, []int64{f.workVideo}},
		{"tag との AND", domain.VideoQuery{TagKindID: f.genre.ID, TagIDs: []int64{f.action.ID}}, sortedIDs(f.manual, f.both)},
		{"tag との AND で当たらない", domain.VideoQuery{TagKindID: f.work.ID, TagIDs: []int64{f.action.ID}}, []int64{}},
		{"ほかの絞り込みとの AND", domain.VideoQuery{TagKindID: f.genre.ID, Query: "both"}, []int64{f.both}},
		{"無い種類", domain.VideoQuery{TagKindID: f.work.ID + 1000}, []int64{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.q.Limit = 50
			if got := listIDs(t, f.db, domain.AudienceOwner, tc.q); !slices.Equal(got, tc.want) {
				t.Errorf("ListVideos = %v, want %v", got, tc.want)
			}
			page, err := f.db.Library().ListLibrary(context.Background(), domain.AudienceOwner, tc.q)
			if err != nil {
				t.Fatal(err)
			}
			if page.Total != len(tc.want) {
				t.Errorf("ListLibrary の total = %d, want %d", page.Total, len(tc.want))
			}
			selection, _, err := f.db.Library().LibraryIDs(context.Background(), tc.q)
			if err != nil {
				t.Fatal(err)
			}
			got := selection.AllVideoIDs()
			slices.Sort(got)
			if !slices.Equal(got, tc.want) {
				t.Errorf("LibraryIDs = %v, want %v", got, tc.want)
			}
		})
	}
}

// 種類を消すとそのタグは種類なしになり、消した種類の id では何にも当たらない（data-model.md の Rules）。
func TestListVideosByDeletedTagKindIsEmpty(t *testing.T) {
	f := tagKindListingFixture(t)
	if err := f.db.Tags().DeleteTagKind(context.Background(), f.work.ID); err != nil {
		t.Fatal(err)
	}
	if got := listIDs(t, f.db, domain.AudienceOwner, domain.VideoQuery{TagKindID: f.work.ID, Limit: 50}); len(got) != 0 {
		t.Errorf("ListVideos = %v, want 空", got)
	}
}
