package store

import (
	"context"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// Keep query terms out of titles and paths to isolate work-info matching.
func workInfoSearchFixture(t *testing.T) (*DB, int64, int64, domain.Credit) {
	t.Helper()
	db := migratedDB(t)
	paths := upsertAll(t, db,
		listingFile(fixturePath("/media/x1.mp4"), "x1", "key-x1", 1),
		listingFile(fixturePath("/media/x2.mp4"), "x2", "key-x2", 2),
	)
	tagged, other := paths[fixturePath("/media/x1.mp4")], paths[fixturePath("/media/x2.mp4")]
	person := createPerson(t, db, "山田花子", "やまだはなこ")
	maker := createOrganization(t, db, "メーカーA", nil)
	setWorkInfo(t, db, tagged, domain.WorkInfo{
		Code:          "AB-12",
		Description:   "説明にだけある語",
		People:        []domain.WorkPerson{performer(person.ID)},
		Organizations: []domain.WorkOrganization{{Role: domain.OrganizationMaker, Organization: domain.CreditRef{ID: maker.ID}}},
	})
	setPublic(t, db, true, tagged, other)
	return db, tagged, other, person
}

func TestSearchMatchesWorkInfoCodeAndCreditNames(t *testing.T) {
	db, tagged, other, _ := workInfoSearchFixture(t)
	for _, query := range []string{"ab", "AB-", "ab-1", "ab-12", "山田花子", "ヤマダハナコ", "ﾔﾏﾀﾞ", "メーカーA", "めーかーa"} {
		if got := listIDs(t, db, domain.AudienceOwner, domain.VideoQuery{Query: query}); !slices.Equal(got, []int64{tagged}) {
			t.Errorf("所有者の %q = %v, want [%d]", query, got, tagged)
		}
		if got := listIDs(t, db, domain.AudienceOwner, domain.VideoQuery{Query: "-" + query}); !slices.Equal(got, []int64{other}) {
			t.Errorf("所有者の -%q = %v, want [%d]", query, got, other)
		}
	}
	for _, query := range []string{"説明にだけある語", "説明"} {
		if got := listIDs(t, db, domain.AudienceOwner, domain.VideoQuery{Query: query}); len(got) != 0 {
			t.Errorf("説明の語 %q = %v, want なし", query, got)
		}
	}
}

func TestGuestSearchDoesNotMatchWorkInfo(t *testing.T) {
	db, _, _, _ := workInfoSearchFixture(t)
	for _, query := range []string{"ab-12", "山田花子", "やまだ", "メーカーA"} {
		if got := listIDs(t, db, domain.AudienceGuest, domain.VideoQuery{Query: query}); len(got) != 0 {
			t.Errorf("ゲストの %q = %v, want なし", query, got)
		}
	}
}

func TestSearchFollowsRenamedPerson(t *testing.T) {
	db, tagged, _, person := workInfoSearchFixture(t)
	name := "佐藤春子"
	if _, err := db.Credits().Update(context.Background(), domain.CreditPerson, person.ID, domain.CreditPatch{Name: &name}); err != nil {
		t.Fatal(err)
	}
	if got := listIDs(t, db, domain.AudienceOwner, domain.VideoQuery{Query: "佐藤春子"}); !slices.Equal(got, []int64{tagged}) {
		t.Errorf("新しい名前 = %v, want [%d]", got, tagged)
	}
	if got := listIDs(t, db, domain.AudienceOwner, domain.VideoQuery{Query: "山田花子"}); len(got) != 0 {
		t.Errorf("古い名前 = %v, want なし", got)
	}
}

func TestSearchFindsAMergedPersonByTheMergedName(t *testing.T) {
	db, tagged, other, person := workInfoSearchFixture(t)
	source := createPerson(t, db, "佐藤秋子")
	setWorkInfo(t, db, other, domain.WorkInfo{People: []domain.WorkPerson{performer(source.ID)}})
	if _, err := db.Credits().Merge(context.Background(), domain.CreditPerson, person.ID, []int64{source.ID}); err != nil {
		t.Fatal(err)
	}
	if got := slices.Sorted(slices.Values(listIDs(t, db, domain.AudienceOwner, domain.VideoQuery{Query: "佐藤秋子"}))); !slices.Equal(got, slices.Sorted(slices.Values([]int64{tagged, other}))) {
		t.Errorf("統合元の名前 = %v, want [%d %d]", got, tagged, other)
	}
}

func TestLibrarySearchMatchesWorkInfoOfTheBundle(t *testing.T) {
	db := migratedDB(t)
	paths := upsertAll(t, db,
		listingFile(fixturePath("/media/alpha.mp4"), "alpha", "key-a", 1),
		listingFile(fixturePath("/media/bravo.mp4"), "bravo", "key-b", 2),
		listingFile(fixturePath("/media/charlie.mp4"), "charlie", "key-c", 3),
	)
	a, b, c := paths[fixturePath("/media/alpha.mp4")], paths[fixturePath("/media/bravo.mp4")], paths[fixturePath("/media/charlie.mp4")]
	bundle(t, db, a, a, b)
	person := createPerson(t, db, "山田花子", "やまだはなこ")
	maker := createOrganization(t, db, "メーカーA", nil)
	setWorkInfo(t, db, b, domain.WorkInfo{
		Code:          "AB-12",
		People:        []domain.WorkPerson{performer(person.ID)},
		Organizations: []domain.WorkOrganization{{Role: domain.OrganizationMaker, Organization: domain.CreditRef{ID: maker.ID}}},
	})

	for _, query := range []string{"ab-12", "ヤマダ", "めーかーa"} {
		if got := libraryItemIDs(t, db, domain.AudienceOwner, domain.VideoQuery{Query: query}); !slices.Equal(got, []int64{a}) {
			t.Errorf("%q の ListLibrary = %v, want [%d]", query, got, a)
		}
		if got := libraryItemIDs(t, db, domain.AudienceOwner, domain.VideoQuery{Query: "-" + query}); !slices.Equal(got, []int64{c}) {
			t.Errorf("-%q の ListLibrary = %v, want [%d]", query, got, c)
		}
	}
}
