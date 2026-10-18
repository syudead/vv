package store

import (
	"context"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

func favoritePeople(t *testing.T, db *DB, favorite bool, ids ...int64) {
	t.Helper()
	setFavorites(t, db, domain.FavoriteChange{PersonIDs: ids, Favorite: favorite})
}

func personFavorite(t *testing.T, db *DB, id int64) bool {
	t.Helper()
	person, err := db.Credits().Get(context.Background(), domain.CreditPerson, id)
	if err != nil {
		t.Fatal(err)
	}
	return person.Favorite
}

func TestSetPersonFavoritesKeepsTheFirstTimeAndIgnoresMissingPeople(t *testing.T) {
	db := migratedDB(t)
	person := createPerson(t, db, "P")
	freezeFavoriteClock(t, time.UnixMilli(1_000))
	favoritePeople(t, db, true, person.ID, 999_999)
	freezeFavoriteClock(t, time.UnixMilli(5_000))
	favoritePeople(t, db, true, person.ID)

	if got := favoritedAt(t, db, "person_favorites", "person_id", person.ID); got != 1_000 {
		t.Errorf("付け直した favorited_at = %d, want 1000", got)
	}
	if got := favoriteRowCount(t, db, "person_favorites"); got != 1 {
		t.Errorf("person_favorites の行 = %d, want 1 (無い人物は無視)", got)
	}
	if !personFavorite(t, db, person.ID) {
		t.Error("Get の Favorite = false, want true")
	}
	favoritePeople(t, db, false, person.ID)
	if personFavorite(t, db, person.ID) {
		t.Error("外したあとの Favorite = true")
	}
}

func TestListPeopleFavoriteOnlyCountsFavorites(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	liked := createPerson(t, db, "Liked")
	createPerson(t, db, "Other")
	favoritePeople(t, db, true, liked.ID)

	page, err := db.Credits().List(ctx, domain.CreditPerson, domain.CreditQuery{FavoriteOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != liked.ID || !page.Items[0].Favorite || page.Total != 1 || page.TotalAll != 2 {
		t.Errorf("お気に入りだけの一覧 = %+v", page)
	}
	if _, err := db.Credits().List(ctx, domain.CreditOrganization, domain.CreditQuery{FavoriteOnly: true}); err == nil {
		t.Error("団体のお気に入り絞り込みが通った")
	}
}

func TestMergePeopleKeepsASourceFavorite(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	target := createPerson(t, db, "T")
	liked := createPerson(t, db, "S1")
	plain := createPerson(t, db, "S2")
	freezeFavoriteClock(t, time.UnixMilli(2_000))
	favoritePeople(t, db, true, liked.ID)

	outcome, err := db.Credits().Merge(ctx, domain.CreditPerson, target.ID, []int64{plain.ID, liked.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.Credit.Favorite {
		t.Error("お気に入りの統合元を取り込んだ統合先の Favorite = false")
	}
	if got := favoritedAt(t, db, "person_favorites", "person_id", target.ID); got != 2_000 {
		t.Errorf("統合先の favorited_at = %d, want 2000", got)
	}
	if got := favoriteRowCount(t, db, "person_favorites"); got != 1 {
		t.Errorf("person_favorites の行 = %d, want 1 (統合元の行は消える)", got)
	}

	// No source is a favorite, so the target stays unmarked.
	other := createPerson(t, db, "O")
	outcome, err = db.Credits().Merge(ctx, domain.CreditPerson, other.ID, []int64{createPerson(t, db, "S3").ID})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Credit.Favorite {
		t.Error("お気に入りでない統合元だけの統合先の Favorite = true")
	}
}

func TestDeletePersonRemovesTheFavorite(t *testing.T) {
	db := migratedDB(t)
	person := createPerson(t, db, "P")
	favoritePeople(t, db, true, person.ID)
	if err := db.Credits().Delete(context.Background(), domain.CreditPerson, person.ID); err != nil {
		t.Fatal(err)
	}
	if got := favoriteRowCount(t, db, "person_favorites"); got != 0 {
		t.Errorf("削除のあとの person_favorites の行 = %d, want 0", got)
	}
}
