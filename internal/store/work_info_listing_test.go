package store

import (
	"context"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func sortedLibraryNames(t *testing.T, db *DB, q domain.VideoQuery) []string {
	t.Helper()
	names := itemNames(libraryPages(t, db, domain.AudienceOwner, q))
	slices.Sort(names)
	return names
}

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

func TestListVideosFiltersByPerson(t *testing.T) {
	db, ids, hanako, _, _ := workInfoListingFixture(t)
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
}

func TestListVideosFiltersByPersonRole(t *testing.T) {
	db, ids := itemsFixture(t)
	hanako := createPerson(t, db, "山田花子")
	performerVideo, directorVideo := ids[fixturePath("/media/show/ep1.mp4")], ids[fixturePath("/media/solo.mp4")]
	setWorkInfo(t, db, performerVideo, domain.WorkInfo{People: []domain.WorkPerson{performer(hanako.ID)}})
	setWorkInfo(t, db, directorVideo, domain.WorkInfo{
		People: []domain.WorkPerson{{Role: domain.PersonDirector, Person: domain.CreditRef{ID: hanako.ID}}},
	})

	for role, want := range map[domain.PersonRole]int64{
		domain.PersonPerformer: performerVideo,
		domain.PersonDirector:  directorVideo,
	} {
		page, err := db.Library().ListVideos(context.Background(), domain.AudienceOwner,
			domain.VideoQuery{Person: hanako.ID, PersonRole: role, Limit: 50})
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Items) != 1 || page.Items[0].ID != want || page.Total != 1 {
			t.Errorf("%s: items = %v（total %d）, want [%d]（total 1）", role, page.Items, page.Total, want)
		}
	}
}
