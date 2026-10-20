package store

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func TestEditCreditsCountsOnlyChangedVideos(t *testing.T) {
	db, ids := versionFixture(t)
	ctx := context.Background()
	hanako := createPerson(t, db, "山田花子")
	maker := createOrganization(t, db, "メーカー", nil)
	setWorkInfo(t, db, ids["a"], domain.WorkInfo{Code: "A-1", People: []domain.WorkPerson{performer(hanako.ID)}})

	applied, err := db.WorkInfo().EditCredits(ctx, []int64{ids["a"], ids["b"], 999_999}, []domain.CreditEdit{
		{Kind: domain.CreditPerson, Role: string(domain.PersonPerformer), Action: domain.CreditEditAdd, IDs: []int64{hanako.ID}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if applied != 1 {
		t.Errorf("applied = %d, want 1 (a already had the performer)", applied)
	}
	b, _ := workInfoOf(t, db, ids["b"])
	if len(b.People) != 1 || b.People[0].Person.ID != hanako.ID {
		t.Errorf("b people = %+v, want the added performer", b.People)
	}

	applied, err = db.WorkInfo().EditCredits(ctx, []int64{ids["a"]}, []domain.CreditEdit{
		{Kind: domain.CreditOrganization, Role: string(domain.OrganizationMaker), Action: domain.CreditEditReplace, IDs: []int64{maker.ID}},
	})
	if err != nil || applied != 1 {
		t.Fatalf("applied = %d, err = %v; want 1, nil", applied, err)
	}
	a, _ := workInfoOf(t, db, ids["a"])
	if a.Code != "A-1" || len(a.People) != 1 || len(a.Organizations) != 1 || a.Organizations[0].Organization.ID != maker.ID {
		t.Errorf("a = %+v, want code and performer kept with the maker added", a)
	}
}

func TestEditCreditsWritesNothingForAMissingCredit(t *testing.T) {
	db, ids := versionFixture(t)
	hanako := createPerson(t, db, "山田花子")
	before := workInfoRowCount(t, db)

	_, err := db.WorkInfo().EditCredits(context.Background(), []int64{ids["a"]}, []domain.CreditEdit{
		{Kind: domain.CreditPerson, Role: string(domain.PersonPerformer), Action: domain.CreditEditAdd, IDs: []int64{hanako.ID}},
		{Kind: domain.CreditPerson, Role: string(domain.PersonDirector), Action: domain.CreditEditAdd, IDs: []int64{hanako.ID, 999_999}},
	})
	var missing *domain.CreditNotFoundError
	if !errors.As(err, &missing) || !reflect.DeepEqual(*missing, domain.CreditNotFoundError{
		Kind: domain.CreditPerson, ID: 999_999, Field: "edits[1].ids[1]",
	}) {
		t.Fatalf("err = %v, want person 999999 missing at edits[1].ids[1]", err)
	}
	if after := workInfoRowCount(t, db); after != before {
		t.Errorf("work-info rows = %d, want %d", after, before)
	}
}
