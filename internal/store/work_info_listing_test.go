package store

import (
	"context"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// 人物と団体による一覧の絞り込み（specs/047-video-work-info/data-model.md の Lists、
// contracts/screen-api.md の List parameters person and organization、research.md R-7、#1025）。
// fixture のグループ show のメンバーは ep1・ep2・ep10、pair のメンバーは p1・p2 である。

// sortedLibraryNames は所有者の ListLibrary を最後まで読み、項目の名前を並べて返す。
func sortedLibraryNames(t *testing.T, db *DB, q domain.VideoQuery) []string {
	t.Helper()
	names := itemNames(libraryPages(t, db, domain.AudienceOwner, q))
	slices.Sort(names)
	return names
}

// workInfoListingFixture は、花子を ep1 の出演者・solo の監督・p1 の出演者に、メーカー A を ep1 と p2 に、
// レーベル B を show の全メンバーに結ぶ。
func workInfoListingFixture(t *testing.T) (db *DB, ids map[string]int64, hanako, makerA, labelB domain.Credit) {
	t.Helper()
	db, ids = itemsFixture(t)
	hanako = createPerson(t, db, "山田花子")
	makerA = createOrganization(t, db, "メーカーA", nil)
	labelB = createOrganization(t, db, "レーベルB", nil)
	label := domain.WorkOrganization{Role: domain.OrganizationLabel, Organization: domain.CreditRef{ID: labelB.ID}}
	maker := domain.WorkOrganization{Role: domain.OrganizationMaker, Organization: domain.CreditRef{ID: makerA.ID}}
	setWorkInfo(t, db, ids[fixturePath("/media/show/ep1.mp4")], domain.WorkInfo{
		People: []domain.WorkPerson{performer(hanako.ID)}, Organizations: []domain.WorkOrganization{maker, label},
	})
	setWorkInfo(t, db, ids[fixturePath("/media/show/ep2.mp4")], domain.WorkInfo{Organizations: []domain.WorkOrganization{label}})
	setWorkInfo(t, db, ids[fixturePath("/media/show/ep10.mp4")], domain.WorkInfo{Organizations: []domain.WorkOrganization{label}})
	setWorkInfo(t, db, ids[fixturePath("/media/solo.mp4")], domain.WorkInfo{
		People: []domain.WorkPerson{{Role: domain.PersonDirector, Person: domain.CreditRef{ID: hanako.ID}}},
	})
	setWorkInfo(t, db, ids[fixturePath("/media/pair/p1.mp4")], domain.WorkInfo{People: []domain.WorkPerson{performer(hanako.ID)}})
	setWorkInfo(t, db, ids[fixturePath("/media/pair/p2.mp4")], domain.WorkInfo{Organizations: []domain.WorkOrganization{maker}})
	return db, ids, hanako, makerA, labelB
}

// person と organization はどの役割でも結び付いた動画だけにし、一部のメンバーだけが当たったグループは
// そのメンバーを動画の項目にする。total も絞り込みを掛けて数える。
func TestListLibraryFiltersByPersonAndOrganization(t *testing.T) {
	db, _, hanako, makerA, labelB := workInfoListingFixture(t)

	cases := []struct {
		name string
		q    domain.VideoQuery
		want []string
	}{
		{"人物（出演者と監督のどちらでも）", domain.VideoQuery{Person: hanako.ID}, []string{"ep1", "p1", "solo"}},
		{"団体", domain.VideoQuery{Organization: makerA.ID}, []string{"ep1", "p2"}},
		{"全メンバーが当たる団体はグループの項目", domain.VideoQuery{Organization: labelB.ID}, []string{"group:show"}},
		{"人物と団体の AND", domain.VideoQuery{Person: hanako.ID, Organization: makerA.ID}, []string{"ep1"}},
		{"ほかの絞り込みとの AND", domain.VideoQuery{Person: hanako.ID, Watch: domain.WatchWatched}, []string{"ep1", "solo"}},
		{"実体の無い id", domain.VideoQuery{Person: hanako.ID + 1000}, []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sortedLibraryNames(t, db, tc.q); !slices.Equal(got, tc.want) {
				t.Errorf("項目 = %v, want %v", got, tc.want)
			}
			page, err := db.Library().ListLibrary(context.Background(), domain.AudienceOwner, tc.q)
			if err != nil {
				t.Fatal(err)
			}
			if page.Total != len(tc.want) {
				t.Errorf("total = %d, want %d", page.Total, len(tc.want))
			}
		})
	}
}

// GET /api/videos の一覧と GET /api/library/ids も同じ条件で絞る。
func TestListVideosAndLibraryIDsFilterByPerson(t *testing.T) {
	db, ids, hanako, _, labelB := workInfoListingFixture(t)
	ctx := context.Background()
	want := []int64{ids[fixturePath("/media/show/ep1.mp4")], ids[fixturePath("/media/pair/p1.mp4")], ids[fixturePath("/media/solo.mp4")]}
	slices.Sort(want)

	page, err := db.Library().ListVideos(ctx, domain.AudienceOwner, domain.VideoQuery{Person: hanako.ID, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	var listed []int64
	for _, video := range page.Items {
		listed = append(listed, video.ID)
	}
	slices.Sort(listed)
	if !slices.Equal(listed, want) || page.Total != len(want) {
		t.Errorf("ListVideos = %v（total %d）, want %v", listed, page.Total, want)
	}

	selection, _, err := db.Library().LibraryIDs(ctx, domain.VideoQuery{Person: hanako.ID})
	if err != nil {
		t.Fatal(err)
	}
	got := selection.AllVideoIDs()
	slices.Sort(got)
	if !slices.Equal(got, want) || len(selection.Groups) != 0 {
		t.Errorf("LibraryIDs = %v・グループ %+v, want %v・グループ無し", got, selection.Groups, want)
	}

	selection, _, err = db.Library().LibraryIDs(ctx, domain.VideoQuery{Organization: labelB.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(selection.Groups) != 1 || selection.Groups[0].ID != groupIDOf(t, db, fixturePath("/media/show")) || len(selection.VideoIDs) != 0 {
		t.Errorf("LibraryIDs（レーベル B） = %+v, want グループ show だけ", selection)
	}
}
