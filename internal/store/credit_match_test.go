package store

import (
	"context"
	"reflect"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// 取得した人物・団体と既存の実体の照合（specs/052-plugin-work-info-fields/research.md R-3、#1157）。

func matchFetched(t *testing.T, db *DB, kind domain.CreditKind, entries ...domain.CreditMatchEntry) []domain.CreditMatch {
	t.Helper()
	matches, err := db.Credits().Match(context.Background(), kind, entries)
	if err != nil {
		t.Fatalf("Match(%s) error = %v", kind, err)
	}
	return matches
}

func existingMatch(credit domain.Credit) domain.CreditMatch {
	return domain.CreditMatch{State: domain.CreditMatchExisting, Credit: &domain.CreditRef{ID: credit.ID, Name: credit.Name}}
}

func TestCreditMatchPrefersExternalIDOverName(t *testing.T) {
	db := migratedDB(t)
	holder := createCredit(t, db, domain.CreditPerson, domain.CreditInput{
		Name: "Aoi", ExternalIDs: []domain.ExternalID{{Source: "fanza", Value: "a-1"}},
	})
	createPerson(t, db, "Mika")

	got := matchFetched(t, db, domain.CreditPerson, domain.CreditMatchEntry{
		Name: "Mika", ExternalIDs: []domain.ExternalID{{Source: "other", Value: "x"}, {Source: "fanza", Value: "a-1"}},
	})
	if want := []domain.CreditMatch{existingMatch(holder)}; !reflect.DeepEqual(got, want) {
		t.Errorf("Match = %+v, want %+v", got, want)
	}
}

func TestCreditMatchFoldsWidthAgainstAliases(t *testing.T) {
	db := migratedDB(t)
	person := createPerson(t, db, "佐藤みか", "Ｍｉｋａ　Ｓａｔｏ")

	got := matchFetched(t, db, domain.CreditPerson,
		domain.CreditMatchEntry{Name: "mika sato"},
		domain.CreditMatchEntry{Name: "佐藤ミカ"},
	)
	if want := []domain.CreditMatch{existingMatch(person), existingMatch(person)}; !reflect.DeepEqual(got, want) {
		t.Errorf("Match = %+v, want %+v", got, want)
	}
}

func TestCreditMatchAmbiguous(t *testing.T) {
	db := migratedDB(t)
	createPerson(t, db, "Rin")
	createPerson(t, db, "Rin Kato", "ＲＩＮ")
	createCredit(t, db, domain.CreditPerson, domain.CreditInput{
		Name: "Aoi", ExternalIDs: []domain.ExternalID{{Source: "fanza", Value: "a-1"}},
	})
	createCredit(t, db, domain.CreditPerson, domain.CreditInput{
		Name: "Yui", ExternalIDs: []domain.ExternalID{{Source: "mgs", Value: "y-1"}},
	})

	got := matchFetched(t, db, domain.CreditPerson,
		domain.CreditMatchEntry{Name: "rin"},
		domain.CreditMatchEntry{Name: "Aoi", ExternalIDs: []domain.ExternalID{{Source: "fanza", Value: "a-1"}, {Source: "mgs", Value: "y-1"}}},
	)
	ambiguous := domain.CreditMatch{State: domain.CreditMatchAmbiguous}
	if want := []domain.CreditMatch{ambiguous, ambiguous}; !reflect.DeepEqual(got, want) {
		t.Errorf("Match = %+v, want %+v", got, want)
	}
}

func TestCreditMatchNew(t *testing.T) {
	db := migratedDB(t)
	createCredit(t, db, domain.CreditPerson, domain.CreditInput{
		Name: "Aoi", ExternalIDs: []domain.ExternalID{{Source: "fanza", Value: "a-1"}},
	})

	got := matchFetched(t, db, domain.CreditPerson,
		domain.CreditMatchEntry{Name: "Nana"},
		domain.CreditMatchEntry{Name: "Nana", ExternalIDs: []domain.ExternalID{{Source: "fanza", Value: "a-2"}}},
	)
	created := domain.CreditMatch{State: domain.CreditMatchNew}
	if want := []domain.CreditMatch{created, created}; !reflect.DeepEqual(got, want) {
		t.Errorf("Match = %+v, want %+v", got, want)
	}
}

func TestCreditMatchKeepsKindsApart(t *testing.T) {
	db := migratedDB(t)
	createCredit(t, db, domain.CreditPerson, domain.CreditInput{
		Name: "Studio A", ExternalIDs: []domain.ExternalID{{Source: "fanza", Value: "s-1"}},
	})
	entry := domain.CreditMatchEntry{Name: "Studio A", ExternalIDs: []domain.ExternalID{{Source: "fanza", Value: "s-1"}}}

	if got, want := matchFetched(t, db, domain.CreditOrganization, entry), []domain.CreditMatch{{State: domain.CreditMatchNew}}; !reflect.DeepEqual(got, want) {
		t.Errorf("organization Match before = %+v, want %+v", got, want)
	}
	organization := createOrganization(t, db, "Studio A", nil)
	if got, want := matchFetched(t, db, domain.CreditOrganization, entry), []domain.CreditMatch{existingMatch(organization)}; !reflect.DeepEqual(got, want) {
		t.Errorf("organization Match after = %+v, want %+v", got, want)
	}
}

func TestCreditMatchIsStable(t *testing.T) {
	db := migratedDB(t)
	person := createPerson(t, db, "Mika")
	createPerson(t, db, "Rin")
	createPerson(t, db, "rin")
	entries := []domain.CreditMatchEntry{{Name: "MIKA"}, {Name: "Rin"}, {Name: "Nana"}}

	first := matchFetched(t, db, domain.CreditPerson, entries...)
	second := matchFetched(t, db, domain.CreditPerson, entries...)
	want := []domain.CreditMatch{
		existingMatch(person), {State: domain.CreditMatchAmbiguous}, {State: domain.CreditMatchNew},
	}
	if !reflect.DeepEqual(first, want) || !reflect.DeepEqual(second, want) {
		t.Errorf("Match = %+v then %+v, want %+v both times", first, second, want)
	}
}

func TestCreditMatchUnknownKind(t *testing.T) {
	db := migratedDB(t)
	if _, err := db.Credits().Match(context.Background(), domain.CreditKind("tag"), nil); err == nil {
		t.Error("Match(tag) error = nil, want an error")
	}
}
