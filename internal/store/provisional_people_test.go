package store

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// Face analysis creates people this way: a people row without any people_names row.
func createProvisionalPerson(t *testing.T, db *DB) int64 {
	t.Helper()
	res, err := db.sql.Exec(`insert into people (created_at) values (1)`)
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func creditPageIDs(page domain.CreditPage) []int64 {
	ids := make([]int64, 0, len(page.Items))
	for _, item := range page.Items {
		ids = append(ids, item.ID)
	}
	return ids
}

func TestProvisionalPeopleListAfterNamedPeople(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	provisional := createProvisionalPerson(t, db)
	b := createPerson(t, db, "B")
	a := createPerson(t, db, "A")

	// Page by one so each cursor crosses the boundary between named and provisional people.
	var names []int64
	query := domain.CreditQuery{Limit: 1}
	for {
		page, err := db.Credits().List(ctx, domain.CreditPerson, query)
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, creditPageIDs(page)...)
		if page.NextCursor == "" {
			break
		}
		query.Cursor = page.NextCursor
	}
	if want := []int64{a.ID, b.ID, provisional}; !slices.Equal(names, want) {
		t.Errorf("名前順 = %v, want %v", names, want)
	}
	// Equal video counts fall back to name order, which also puts the provisional person last.
	tie, err := db.Credits().List(ctx, domain.CreditPerson, domain.CreditQuery{Sort: domain.TagSortCountDesc})
	if err != nil {
		t.Fatal(err)
	}
	if want := []int64{a.ID, b.ID, provisional}; !slices.Equal(creditPageIDs(tie), want) {
		t.Errorf("本数が同じときの順 = %v, want %v", creditPageIDs(tie), want)
	}

	yes, no := true, false
	only, err := db.Credits().List(ctx, domain.CreditPerson, domain.CreditQuery{Provisional: &yes})
	if err != nil {
		t.Fatal(err)
	}
	if len(only.Items) != 1 || only.Total != 1 || only.TotalAll != 3 {
		t.Fatalf("provisional=true = %+v", only)
	}
	if got := only.Items[0]; got.ID != provisional || !got.Provisional || got.Name != "" {
		t.Errorf("仮の人物 = %+v", got)
	}
	named, err := db.Credits().List(ctx, domain.CreditPerson, domain.CreditQuery{Provisional: &no})
	if err != nil {
		t.Fatal(err)
	}
	if want := []int64{a.ID, b.ID}; !slices.Equal(creditPageIDs(named), want) || named.Total != 2 || named.Items[0].Provisional {
		t.Errorf("provisional=false = %+v", named)
	}
	if _, err := db.Credits().List(ctx, domain.CreditOrganization, domain.CreditQuery{Provisional: &no}); err == nil {
		t.Error("団体の provisional 絞り込みが通った")
	}
}

func TestNamingAProvisionalPersonMakesItOrdinary(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	id := createProvisionalPerson(t, db)

	kept, err := db.Credits().Update(ctx, domain.CreditPerson, id, domain.CreditPatch{Profile: &domain.PersonProfilePatch{
		Fields: []domain.PersonProfileField{domain.ProfileHeightCm}, Values: domain.PersonProfile{HeightCm: intPtr(160)},
	}})
	if err != nil {
		t.Fatalf("名前なしのプロフィール変更: %v", err)
	}
	if !kept.Provisional || kept.Name != "" || kept.Profile.HeightCm == nil || *kept.Profile.HeightCm != 160 {
		t.Errorf("名前なしのプロフィール変更のあと = %+v", kept)
	}
	aliases := []domain.Alias{{Name: "Alias"}}
	_, err = db.Credits().Update(ctx, domain.CreditPerson, id, domain.CreditPatch{Aliases: &aliases})
	var invalid *domain.InvalidWorkInfoError
	if !errors.As(err, &invalid) || invalid.Field != "name" || invalid.Problem != domain.WorkInfoEmpty {
		t.Errorf("名前なしの別名: err = %v", err)
	}

	name := "Alice"
	named, err := db.Credits().Update(ctx, domain.CreditPerson, id, domain.CreditPatch{Name: &name})
	if err != nil {
		t.Fatal(err)
	}
	if named.Provisional || named.Name != "Alice" || named.Profile.HeightCm == nil {
		t.Errorf("名前を付けたあと = %+v", named)
	}
}

func TestMergeIntoAProvisionalPersonTakesTheFirstNamedSourcesName(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	target := createProvisionalPerson(t, db)
	unnamed := createProvisionalPerson(t, db)
	alice := createPerson(t, db, "Alice", "Ally")
	bob := createPerson(t, db, "Bob")

	outcome, err := db.Credits().Merge(ctx, domain.CreditPerson, target, []int64{unnamed, alice.ID, bob.ID})
	if err != nil {
		t.Fatal(err)
	}
	got := outcome.Credit
	if got.Provisional || got.Name != "Alice" {
		t.Errorf("まとめたあとの名前 = %q (provisional %v), want Alice", got.Name, got.Provisional)
	}
	if want := []string{"Ally", "Bob"}; !slices.Equal(aliasNames(got), want) {
		t.Errorf("別名 = %v, want %v", aliasNames(got), want)
	}
}

func TestWorkInfoMarksAProvisionalPerformer(t *testing.T) {
	db := migratedDB(t)
	named := createPerson(t, db, "Alice")
	provisional := createProvisionalPerson(t, db)
	linkPeople(t, db, "k0", "performer", named.ID, provisional)

	infos, err := workInfoByUserKeys(context.Background(), db.sql, []string{"k0"})
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.CreditRef{{ID: named.ID, Name: "Alice"}, {ID: provisional, Provisional: true}}
	people := infos["k0"].People
	if len(people) != 2 || people[0].Person != want[0] || people[1].Person != want[1] {
		t.Errorf("出演者 = %+v, want %+v", people, want)
	}
}
