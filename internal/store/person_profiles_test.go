package store

import (
	"context"
	"reflect"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func intPtr(v int) *int { return &v }

func TestUpdatePersonProfileReplacesAndClearsSingleFields(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	person := createCredit(t, db, domain.CreditPerson, domain.CreditInput{Name: "山田花子", Profile: domain.PersonProfile{
		Gender: "female", BirthDate: "1990-05", HeightCm: intPtr(160), Links: []string{"https://a.example", "https://b.example"},
	}})
	if got, err := db.Credits().Get(ctx, domain.CreditPerson, person.ID); err != nil || !reflect.DeepEqual(got.Profile, person.Profile) ||
		!reflect.DeepEqual(got.Profile.Links, []string{"https://a.example", "https://b.example"}) {
		t.Fatalf("作成した人物の profile = %+v, %v", got.Profile, err)
	}

	patched, err := db.Credits().Update(ctx, domain.CreditPerson, person.ID, domain.CreditPatch{Profile: &domain.PersonProfilePatch{
		Fields: []domain.PersonProfileField{domain.ProfileHeightCm, domain.ProfileBirthDate, domain.ProfileLinks},
		Values: domain.PersonProfile{HeightCm: intPtr(161), Links: []string{"https://b.example"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	want := domain.PersonProfile{Gender: "female", HeightCm: intPtr(161), Links: []string{"https://b.example"}}
	if !reflect.DeepEqual(patched.Profile, want) {
		t.Errorf("profile の patch = %+v, want %+v", patched.Profile, want)
	}
	if patched.Name != "山田花子" {
		t.Errorf("名前が変わった: %q", patched.Name)
	}
	if got, err := db.Credits().Get(ctx, domain.CreditPerson, person.ID); err != nil || !reflect.DeepEqual(got.Profile, want) {
		t.Errorf("読み直した profile = %+v, %v", got.Profile, err)
	}
}

func TestMergePeopleFillsOnlyEmptyProfileFieldsFromTheFirstSource(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	target := createCredit(t, db, domain.CreditPerson, domain.CreditInput{Name: "T", Profile: domain.PersonProfile{
		HeightCm: intPtr(160),
	}})
	first := createCredit(t, db, domain.CreditPerson, domain.CreditInput{Name: "S1", Profile: domain.PersonProfile{
		HeightCm: intPtr(170), Cup: "C",
	}})
	second := createCredit(t, db, domain.CreditPerson, domain.CreditInput{Name: "S2", Profile: domain.PersonProfile{
		Cup: "D", BirthDate: "1990", Links: []string{"https://s2.example/1", "https://s2.example/2"},
	}})

	// second comes first in sourceIds, so it wins every field both sources have.
	outcome, err := db.Credits().Merge(ctx, domain.CreditPerson, target.ID, []int64{second.ID, first.ID})
	if err != nil {
		t.Fatal(err)
	}
	want := domain.PersonProfile{
		HeightCm: intPtr(160), Cup: "D", BirthDate: "1990", Links: []string{"https://s2.example/1", "https://s2.example/2"},
	}
	if !reflect.DeepEqual(outcome.Credit.Profile, want) {
		t.Errorf("統合先の profile = %+v, want %+v", outcome.Credit.Profile, want)
	}

	// A target with links keeps them instead of taking a source's.
	keeper := createCredit(t, db, domain.CreditPerson, domain.CreditInput{Name: "K", Profile: domain.PersonProfile{
		Links: []string{"https://k.example"},
	}})
	outcome, err = db.Credits().Merge(ctx, domain.CreditPerson, keeper.ID, []int64{target.ID})
	if err != nil {
		t.Fatal(err)
	}
	if got := outcome.Credit.Profile.Links; !reflect.DeepEqual(got, []string{"https://k.example"}) {
		t.Errorf("リンクを持つ統合先のリンク = %v", got)
	}
}
