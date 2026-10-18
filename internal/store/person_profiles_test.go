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

// Each source holds individually valid dates and career years; a filled pair must still hold together.
func TestMergePeopleSkipsSourceValuesThatContradictTheTargetProfile(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	target := createCredit(t, db, domain.CreditPerson, domain.CreditInput{Name: "T", Profile: domain.PersonProfile{
		BirthDate: "2000", CareerEndYear: intPtr(2010),
	}})
	contradicting := createCredit(t, db, domain.CreditPerson, domain.CreditInput{Name: "S1", Profile: domain.PersonProfile{
		DeathDate: "1990", CareerStartYear: intPtr(2020), HeightCm: intPtr(150),
	}})
	fitting := createCredit(t, db, domain.CreditPerson, domain.CreditInput{Name: "S2", Profile: domain.PersonProfile{
		DeathDate: "2005", CareerStartYear: intPtr(2001),
	}})

	outcome, err := db.Credits().Merge(ctx, domain.CreditPerson, target.ID, []int64{contradicting.ID, fitting.ID})
	if err != nil {
		t.Fatal(err)
	}
	want := domain.PersonProfile{
		BirthDate: "2000", DeathDate: "2005", CareerStartYear: intPtr(2001), CareerEndYear: intPtr(2010), HeightCm: intPtr(150),
	}
	if !reflect.DeepEqual(outcome.Credit.Profile, want) {
		t.Errorf("統合先の profile = %+v, want %+v", outcome.Credit.Profile, want)
	}
}

func TestProfileOnlyUpdateDoesNotRefreshSmartFolders(t *testing.T) {
	db := migratedDB(t)
	person := createCredit(t, db, domain.CreditPerson, domain.CreditInput{Name: "山田花子"})
	newSmartFolder(t, db, "folder", domain.SmartRule{}, domain.SortAddedDesc, 0)
	execSQL(t, db, `create trigger reject_refresh before insert on smart_folder_values
		begin select raise(abort, 'refresh rejected'); end`)

	_, err := db.Credits().Update(context.Background(), domain.CreditPerson, person.ID, domain.CreditPatch{
		Profile: &domain.PersonProfilePatch{
			Fields: []domain.PersonProfileField{domain.ProfileBio}, Values: domain.PersonProfile{Bio: "紹介文"},
		},
	})
	if err != nil {
		t.Fatalf("profile だけの更新がスマートフォルダを再計算した: %v", err)
	}
}

func TestListPeopleByAgePagesUnknownBirthDatesLast(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	born := func(name, date string) domain.Credit {
		return createCredit(t, db, domain.CreditPerson, domain.CreditInput{Name: name, Profile: domain.PersonProfile{BirthDate: date}})
	}
	day := born("A", "2001-03-04")
	month := born("B", "1980")
	year := born("C", "1990")
	unknown := born("D", "")
	alsoUnknown := born("E", "")
	// An update must rewrite the sort key with the date: 1980 would otherwise keep B oldest.
	if _, err := db.Credits().Update(ctx, domain.CreditPerson, month.ID, domain.CreditPatch{Profile: &domain.PersonProfilePatch{
		Fields: []domain.PersonProfileField{domain.ProfileBirthDate}, Values: domain.PersonProfile{BirthDate: "2001-03"},
	}}); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		sort domain.TagSort
		want []int64
	}{
		// A partial date is its first possible day, so 2001-03 is older than 2001-03-04.
		{domain.CreditSortAgeAsc, []int64{day.ID, month.ID, year.ID, unknown.ID, alsoUnknown.ID}},
		{domain.CreditSortAgeDesc, []int64{year.ID, month.ID, day.ID, unknown.ID, alsoUnknown.ID}},
	} {
		var got []int64
		cursor := ""
		for pages := 0; pages == 0 || cursor != ""; pages++ {
			if pages == 3 {
				t.Fatalf("%s: more than 3 pages of 2 for 5 people", tc.sort)
			}
			page, err := db.Credits().List(ctx, domain.CreditPerson, domain.CreditQuery{Sort: tc.sort, Limit: 2, Cursor: cursor})
			if err != nil {
				t.Fatalf("%s: %v", tc.sort, err)
			}
			for _, item := range page.Items {
				got = append(got, item.ID)
			}
			cursor = page.NextCursor
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s の順 = %v, want %v", tc.sort, got, tc.want)
		}
	}

	if _, err := db.Credits().List(ctx, domain.CreditOrganization, domain.CreditQuery{Sort: domain.CreditSortAgeAsc}); err == nil {
		t.Error("団体の ageAsc がエラーにならない")
	}
}
