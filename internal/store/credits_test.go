package store

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func createCredit(t *testing.T, db *DB, kind domain.CreditKind, input domain.CreditInput) domain.Credit {
	t.Helper()
	credit, err := db.Credits().Create(context.Background(), kind, input)
	if err != nil {
		t.Fatalf("Create(%s, %+v) error = %v", kind, input, err)
	}
	return credit
}

// namesOf is the list of name followed by others; an empty name leaves it out, for a provisional person.
func namesOf(name string, others ...domain.CreditName) []domain.CreditName {
	names := []domain.CreditName{}
	if name != "" {
		names = append(names, domain.CreditName{Name: name})
	}
	return append(names, others...)
}

func createPerson(t *testing.T, db *DB, name string, others ...string) domain.Credit {
	t.Helper()
	input := domain.CreditInput{Names: namesOf(name)}
	for _, other := range others {
		input.Names = append(input.Names, domain.CreditName{Name: other})
	}
	return createCredit(t, db, domain.CreditPerson, input)
}

func createOrganization(t *testing.T, db *DB, name string, parent *domain.Credit) domain.Credit {
	t.Helper()
	input := domain.CreditInput{Names: namesOf(name)}
	if parent != nil {
		input.ParentID = &parent.ID
	}
	return createCredit(t, db, domain.CreditOrganization, input)
}

func linkPeople(t *testing.T, db *DB, key, role string, people ...int64) {
	t.Helper()
	for position, id := range people {
		if _, err := db.sql.Exec(`insert into video_work_people (content_key, role, position, person_id) values (?, ?, ?, ?)`,
			key, role, position, id); err != nil {
			t.Fatal(err)
		}
	}
}

func linkedPeople(t *testing.T, db *DB, key, role string) []int64 {
	t.Helper()
	rows, err := db.sql.Query(`select position, person_id from video_work_people where content_key = ? and role = ? order by position`, key, role)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	ids := []int64{}
	for rows.Next() {
		var position int
		var id int64
		if err := rows.Scan(&position, &id); err != nil {
			t.Fatal(err)
		}
		if position != len(ids) {
			t.Errorf("%s %s の位置 %d が詰まっていない", key, role, position)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return ids
}

// Snapshot every credit table to detect partial writes after rejected operations.
func creditRows(t *testing.T, db *DB) []string {
	t.Helper()
	var all []string
	for _, table := range []string{
		"people", "organizations", "people_names", "organization_names", "people_external_ids",
		"organization_external_ids", "video_work_people", "video_work_organizations",
	} {
		all = append(all, tableRows(t, db, table)...)
	}
	slices.Sort(all)
	return all
}

func tableRows(t *testing.T, db *DB, table string) []string {
	t.Helper()
	rows, err := db.sql.Query(`select * from ` + table)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	columns, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	var all []string
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			t.Fatal(err)
		}
		all = append(all, fmt.Sprint(table, values))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return all
}

func TestCreditsWithTheSameNameAreDifferentPeople(t *testing.T) {
	db := migratedDB(t)
	a := createPerson(t, db, "山田花子")
	b := createPerson(t, db, "山田花子")
	if a.ID == b.ID {
		t.Fatalf("同じ名前の 2 人が同じ id %d を持つ", a.ID)
	}
	page, err := db.Credits().List(context.Background(), owner, domain.CreditPerson, domain.CreditQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || page.Total != 2 || page.TotalAll != 2 {
		t.Fatalf("List = %+v, want both people", page)
	}
	if page.Items[0].ID != a.ID || page.Items[1].ID != b.ID {
		t.Errorf("同じ名前は id の順: %d, %d", page.Items[0].ID, page.Items[1].ID)
	}
}

func TestListCreditsPagesUnderAnAliasQuery(t *testing.T) {
	db := migratedDB(t)
	for i := range 3 {
		createPerson(t, db, fmt.Sprintf("人物%03d", i), fmt.Sprintf("Hanako %d", i))
	}
	createPerson(t, db, "別の人", "Taro")

	query := domain.CreditQuery{Q: "ＨＡＮＡＫＯ", Limit: 2}
	first, err := db.Credits().List(context.Background(), owner, domain.CreditPerson, query)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 2 || first.Total != 3 || first.TotalAll != 4 || first.NextCursor == "" {
		t.Fatalf("1 ページ目 = %d 件、total %d、totalAll %d、nextCursor %q", len(first.Items), first.Total, first.TotalAll, first.NextCursor)
	}
	if first.Items[0].Name != "人物000" || len(first.Items[0].Names) != 2 {
		t.Errorf("先頭 = %+v", first.Items[0])
	}
	query.Cursor = first.NextCursor
	second, err := db.Credits().List(context.Background(), owner, domain.CreditPerson, query)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 1 || second.Total != 3 || second.NextCursor != "" || second.Items[0].Name != "人物002" {
		t.Fatalf("2 ページ目 = %d 件、total %d、nextCursor %q", len(second.Items), second.Total, second.NextCursor)
	}

	if _, err := db.Credits().List(context.Background(), owner, domain.CreditOrganization,
		domain.CreditQuery{Limit: 2, Cursor: first.NextCursor}); !errors.Is(err, domain.ErrInvalidCursor) {
		t.Errorf("団体の一覧に人物のカーソル: err = %v", err)
	}
}

func TestListCreditsCountsShownVideosAndSortsByCount(t *testing.T) {
	db := migratedDB(t)
	keys := addLibraryVideos(t, db, 3)
	few := createPerson(t, db, "A")
	many := createPerson(t, db, "B")
	none := createPerson(t, db, "C")
	linkPeople(t, db, keys[0], "performer", many.ID, few.ID)
	linkPeople(t, db, keys[1], "performer", many.ID)
	linkPeople(t, db, keys[1], "director", many.ID)
	linkPeople(t, db, "no-such-video", "performer", few.ID)

	page, err := db.Credits().List(context.Background(), owner, domain.CreditPerson, domain.CreditQuery{Sort: domain.TagSortCountDesc, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || page.Items[0].ID != many.ID || page.Items[0].VideoCount != 2 ||
		page.Items[1].ID != few.ID || page.Items[1].VideoCount != 1 {
		t.Fatalf("本数の多い順 = %+v", page.Items)
	}
	rest, err := db.Credits().List(context.Background(), owner, domain.CreditPerson,
		domain.CreditQuery{Sort: domain.TagSortCountDesc, Limit: 2, Cursor: page.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(rest.Items) != 1 || rest.Items[0].ID != none.ID || rest.Items[0].VideoCount != 0 {
		t.Fatalf("続き = %+v", rest.Items)
	}
	got, err := db.Credits().Get(context.Background(), domain.CreditPerson, many.ID)
	if err != nil || got.VideoCount != 2 {
		t.Fatalf("Get = %+v, %v", got, err)
	}
}

func TestGetCreditCountsABundleOnce(t *testing.T) {
	db, ids := versionFixture(t)
	person := createPerson(t, db, "A")
	bundle(t, db, ids["a"], ids["a"], ids["b"])
	setWorkInfo(t, db, ids["a"], domain.WorkInfo{People: []domain.WorkPerson{performer(person.ID)}})
	setWorkInfo(t, db, ids["c"], domain.WorkInfo{People: []domain.WorkPerson{performer(person.ID)}})

	got, err := db.Credits().Get(context.Background(), domain.CreditPerson, person.ID)
	if err != nil || got.VideoCount != 2 {
		t.Fatalf("Get = %+v, %v; want the bundle and c counted once each", got, err)
	}
}

func TestCreateCreditStoresEverythingAndGetReadsIt(t *testing.T) {
	db := migratedDB(t)
	root := createOrganization(t, db, "親会社", nil)
	org := createCredit(t, db, domain.CreditOrganization, domain.CreditInput{
		Names: []domain.CreditName{
			{Name: " メーカーB ", Language: "ja"}, {Name: "Maker B", Language: "en"}, {Name: "めーかーB"},
			{Name: "Maker 10"}, {Name: "Maker 9"},
		},
		ExternalIDs: []domain.ExternalID{{Source: "z", Value: "1"}, {Source: "a", Value: "2"}},
		ParentID:    &root.ID,
	})
	got, err := db.Credits().Get(context.Background(), domain.CreditOrganization, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, org) {
		t.Errorf("Get = %+v, Create = %+v", got, org)
	}
	if got.Name != "メーカーB" || got.Parent == nil || *got.Parent != (domain.CreditRef{ID: root.ID, Name: "親会社"}) {
		t.Errorf("名前と親 = %q, %+v", got.Name, got.Parent)
	}
	// The list reads back in the sent order, with the display name's language.
	want := []domain.CreditName{
		{Name: "メーカーB", Language: "ja"}, {Name: "Maker B", Language: "en"}, {Name: "めーかーB"},
		{Name: "Maker 10"}, {Name: "Maker 9"},
	}
	if !reflect.DeepEqual(got.Names, want) {
		t.Errorf("Names = %+v, want %+v", got.Names, want)
	}
	if want := []domain.ExternalID{{Source: "a", Value: "2"}, {Source: "z", Value: "1"}}; !reflect.DeepEqual(got.ExternalIDs, want) {
		t.Errorf("外部 ID = %v", got.ExternalIDs)
	}
	if _, err := db.Credits().Get(context.Background(), domain.CreditOrganization, org.ID+100); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("無い団体: err = %v", err)
	}

	missing := int64(9999)
	_, err = db.Credits().Create(context.Background(), domain.CreditOrganization, domain.CreditInput{Names: namesOf("X"), ParentID: &missing})
	var notFound *domain.CreditNotFoundError
	if !errors.As(err, &notFound) || notFound.Field != "parentId" || notFound.ID != missing {
		t.Errorf("無い親: err = %v", err)
	}
	person := createCredit(t, db, domain.CreditPerson, domain.CreditInput{Names: namesOf("P"), ParentID: &missing})
	if person.Parent != nil {
		t.Errorf("人物に親がある: %+v", person.Parent)
	}
}

func TestUpdateCreditReplacesOnlyTheGivenFields(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	person := createCredit(t, db, domain.CreditPerson, domain.CreditInput{
		Names:       []domain.CreditName{{Name: "山田花子", Language: "ja"}, {Name: "Hanako"}},
		ExternalIDs: []domain.ExternalID{{Source: "ex", Value: "1"}},
	})

	name := "山田 花子"
	renamed, err := db.Credits().Update(ctx, domain.CreditPerson, person.ID, domain.CreditPatch{Name: &name})
	if err != nil {
		t.Fatal(err)
	}
	if want := []domain.CreditName{{Name: name, Language: "ja"}, {Name: "Hanako"}}; renamed.Name != name ||
		!reflect.DeepEqual(renamed.Names, want) || !reflect.DeepEqual(renamed.ExternalIDs, person.ExternalIDs) {
		t.Fatalf("名前だけの更新 = %+v", renamed)
	}

	names := []domain.CreditName{{Name: "Yamada", Language: "en"}, {Name: "山田 花子", Language: "ja"}, {Name: "山田花子"}}
	replaced, err := db.Credits().Update(ctx, domain.CreditPerson, person.ID, domain.CreditPatch{Names: &names})
	if err != nil {
		t.Fatal(err)
	}
	if replaced.Name != "Yamada" || !reflect.DeepEqual(replaced.Names, names) {
		t.Fatalf("一覧の置き換え = %+v", replaced)
	}

	same, err := db.Credits().Update(ctx, domain.CreditPerson, person.ID, domain.CreditPatch{Names: &names})
	if err != nil || !reflect.DeepEqual(same, replaced) {
		t.Fatalf("変えない更新 = %+v, %v", same, err)
	}

	clash := "山田花子"
	_, err = db.Credits().Update(ctx, domain.CreditPerson, person.ID, domain.CreditPatch{Name: &clash})
	var invalid *domain.InvalidWorkInfoError
	if !errors.As(err, &invalid) || invalid.Field != "name" || invalid.Problem != domain.WorkInfoDuplicate {
		t.Errorf("一覧の別の行と同じ名前: err = %v", err)
	}

	cleared, err := db.Credits().Update(ctx, domain.CreditPerson, person.ID, domain.CreditPatch{
		Names: new([]domain.CreditName), ExternalIDs: new([]domain.ExternalID),
	})
	if err != nil || !cleared.Provisional || cleared.Name != "" || len(cleared.Names) != 0 || len(cleared.ExternalIDs) != 0 {
		t.Fatalf("一覧を空にする更新 = %+v, %v", cleared, err)
	}
	if _, err := db.Credits().Update(ctx, domain.CreditPerson, person.ID+100, domain.CreditPatch{Name: &name}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("無い人物: err = %v", err)
	}

	org := createOrganization(t, db, "O", nil)
	_, err = db.Credits().Update(ctx, domain.CreditOrganization, org.ID, domain.CreditPatch{Names: new([]domain.CreditName)})
	if !errors.As(err, &invalid) || *invalid != (domain.InvalidWorkInfoError{Field: "names", Problem: domain.WorkInfoEmpty}) {
		t.Errorf("団体の一覧を空にする: err = %v", err)
	}
}

// A list migrated with more than the limit survives patches that do not send names, and a names patch is counted.
func TestUpdateCreditCountsNamesOnlyWhenTheyAreSent(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	person := createPerson(t, db, "名前0")
	for i := 1; i <= domain.CreditNamesMaxCount; i++ {
		execAll(t, db, fmt.Sprintf(`insert into people_names (person_id, name, position) values (%d, '名前%d', %d)`,
			person.ID, i, i))
	}
	stored, err := db.Credits().Get(ctx, domain.CreditPerson, person.ID)
	if err != nil || len(stored.Names) != domain.CreditNamesMaxCount+1 {
		t.Fatalf("Get = %d names, %v", len(stored.Names), err)
	}
	profile := domain.PersonProfilePatch{Fields: []domain.PersonProfileField{domain.ProfileBirthplace}, Values: domain.PersonProfile{Birthplace: "Tokyo"}}
	if _, err := db.Credits().Update(ctx, domain.CreditPerson, person.ID, domain.CreditPatch{Profile: &profile}); err != nil {
		t.Errorf("プロフィールだけの更新: err = %v", err)
	}
	name := "新しい名前"
	renamed, err := db.Credits().Update(ctx, domain.CreditPerson, person.ID, domain.CreditPatch{Name: &name})
	if err != nil || renamed.Name != name || len(renamed.Names) != domain.CreditNamesMaxCount+1 {
		t.Errorf("name の更新 = %q (%d), %v", renamed.Name, len(renamed.Names), err)
	}
	names := renamed.Names
	_, err = db.Credits().Update(ctx, domain.CreditPerson, person.ID, domain.CreditPatch{Names: &names})
	var invalid *domain.InvalidWorkInfoError
	if !errors.As(err, &invalid) || *invalid != (domain.InvalidWorkInfoError{Field: "names", Problem: domain.WorkInfoTooMany, Limit: 50}) {
		t.Errorf("51 件の names: err = %v", err)
	}
}
func TestExternalIDHeldByAnotherPersonIsRefused(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	holder := createCredit(t, db, domain.CreditPerson, domain.CreditInput{Names: namesOf("A"), ExternalIDs: []domain.ExternalID{{Source: "ex", Value: "42"}}})
	other := createPerson(t, db, "B")
	// External-ID uniqueness is per kind, so an organization may share a person's external ID.
	createCredit(t, db, domain.CreditOrganization, domain.CreditInput{Names: namesOf("O"), ExternalIDs: []domain.ExternalID{{Source: "ex", Value: "42"}}})

	taken := []domain.ExternalID{{Source: "", Value: ""}, {Source: "ex", Value: "1"}, {Source: " ex ", Value: "42"}}
	check := func(what string, err error) {
		t.Helper()
		var takenErr *domain.CreditExternalIDTakenError
		if !errors.As(err, &takenErr) || takenErr.Field != "externalIds[2]" || takenErr.HolderID != holder.ID ||
			takenErr.Kind != domain.CreditPerson || !errors.Is(err, domain.ErrExternalIDTaken) {
			t.Errorf("%s: err = %v", what, err)
		}
	}
	before := creditRows(t, db)
	_, err := db.Credits().Create(ctx, domain.CreditPerson, domain.CreditInput{Names: namesOf("C"), ExternalIDs: taken})
	check("Create", err)
	_, err = db.Credits().Update(ctx, domain.CreditPerson, other.ID, domain.CreditPatch{ExternalIDs: &taken})
	check("Update", err)
	if after := creditRows(t, db); !reflect.DeepEqual(before, after) {
		t.Errorf("拒んだ書き込みが行を変えた")
	}
	keep := []domain.ExternalID{{Source: "ex", Value: "42"}, {Source: "ex", Value: "43"}}
	if _, err := db.Credits().Update(ctx, domain.CreditPerson, holder.ID, domain.CreditPatch{ExternalIDs: &keep}); err != nil {
		t.Errorf("自分の外部 ID: err = %v", err)
	}
}

func TestLookupCreditByExternalID(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	person := createCredit(t, db, domain.CreditPerson, domain.CreditInput{Names: namesOf("A"), ExternalIDs: []domain.ExternalID{{Source: "example", Value: "42"}}})
	got, err := db.Credits().Lookup(ctx, domain.CreditPerson, "example", "42")
	if err != nil || got.ID != person.ID {
		t.Fatalf("Lookup = %+v, %v", got, err)
	}
	if _, err := db.Credits().Lookup(ctx, domain.CreditPerson, "example", "43"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("無い外部 ID: err = %v", err)
	}
	if _, err := db.Credits().Lookup(ctx, domain.CreditOrganization, "example", "42"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("別の種類: err = %v", err)
	}
}

func TestMergeCreditsMovesLinksNamesAndExternalIDs(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	target := createCredit(t, db, domain.CreditPerson, domain.CreditInput{Names: namesOf("T", domain.CreditName{Name: "共通"})})
	source := createCredit(t, db, domain.CreditPerson, domain.CreditInput{
		Names:       namesOf("S", domain.CreditName{Name: "共通"}, domain.CreditName{Name: "Sの別名", Language: "ja"}),
		ExternalIDs: []domain.ExternalID{{Source: "ex", Value: "1"}},
	})
	second := createPerson(t, db, "U", "S", "Uの別名")
	other := createPerson(t, db, "X")
	linkPeople(t, db, "k0", "performer", source.ID, other.ID)
	linkPeople(t, db, "k1", "performer", target.ID, other.ID, source.ID)
	linkPeople(t, db, "k1", "director", other.ID, source.ID)

	outcome, err := db.Credits().Merge(ctx, domain.CreditPerson, target.ID, []int64{source.ID, 9999, second.ID, source.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(outcome.NotFoundIDs, []int64{9999}) {
		t.Errorf("NotFoundIDs = %v", outcome.NotFoundIDs)
	}
	merged := outcome.Credit
	if merged.ID != target.ID || merged.Name != "T" {
		t.Fatalf("統合先 = %+v", merged)
	}
	// The target's list stays first, then each source's in source order, each spelling once.
	want := []domain.CreditName{
		{Name: "T"}, {Name: "共通"}, {Name: "S"}, {Name: "Sの別名", Language: "ja"}, {Name: "U"}, {Name: "Uの別名"},
	}
	if !reflect.DeepEqual(merged.Names, want) {
		t.Errorf("Names = %+v, want %+v", merged.Names, want)
	}
	if !reflect.DeepEqual(merged.ExternalIDs, []domain.ExternalID{{Source: "ex", Value: "1"}}) {
		t.Errorf("外部 ID = %v", merged.ExternalIDs)
	}
	// The target inherits source positions; existing target links force duplicate removal and compaction.
	if got := linkedPeople(t, db, "k0", "performer"); !reflect.DeepEqual(got, []int64{target.ID, other.ID}) {
		t.Errorf("k0 performer = %v", got)
	}
	if got := linkedPeople(t, db, "k1", "performer"); !reflect.DeepEqual(got, []int64{target.ID, other.ID}) {
		t.Errorf("k1 performer = %v", got)
	}
	if got := linkedPeople(t, db, "k1", "director"); !reflect.DeepEqual(got, []int64{other.ID, target.ID}) {
		t.Errorf("k1 director = %v", got)
	}
	if _, err := db.Credits().Get(ctx, domain.CreditPerson, source.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("統合元が残る: err = %v", err)
	}
	if got, err := db.Credits().Lookup(ctx, domain.CreditPerson, "ex", "1"); err != nil || got.ID != target.ID {
		t.Errorf("外部 ID の持ち主 = %+v, %v", got, err)
	}

	if _, err := db.Credits().Merge(ctx, domain.CreditPerson, target.ID, []int64{target.ID}); !errors.Is(err, domain.ErrMergeSameCredit) {
		t.Errorf("自分への統合: err = %v", err)
	}
	if _, err := db.Credits().Merge(ctx, domain.CreditPerson, 9999, []int64{other.ID}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("無い統合先: err = %v", err)
	}
}

func TestMergeOrganizationsReparentsChildren(t *testing.T) {
	db := migratedDB(t)
	target := createOrganization(t, db, "T", nil)
	source := createOrganization(t, db, "S", nil)
	child := createOrganization(t, db, "C", &source)
	if _, err := db.Credits().Merge(context.Background(), domain.CreditOrganization, target.ID, []int64{source.ID}); err != nil {
		t.Fatal(err)
	}
	got, err := db.Credits().Get(context.Background(), domain.CreditOrganization, child.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Parent == nil || *got.Parent != (domain.CreditRef{ID: target.ID, Name: "T"}) {
		t.Errorf("子の親 = %+v", got.Parent)
	}
}

func TestMergeCreditsRefusalsChangeNothing(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	grandparent := createOrganization(t, db, "A", nil)
	parent := createOrganization(t, db, "B", &grandparent)
	child := createOrganization(t, db, "C", &parent)

	target := createPerson(t, db, "T")
	targetNames := namesOf("T")
	for i := range 29 {
		targetNames = append(targetNames, domain.CreditName{Name: fmt.Sprintf("T%d", i)})
	}
	if _, err := db.Credits().Update(ctx, domain.CreditPerson, target.ID, domain.CreditPatch{Names: &targetNames}); err != nil {
		t.Fatal(err)
	}
	sourceNames := make([]domain.CreditName, 20)
	for i := range sourceNames {
		sourceNames[i] = domain.CreditName{Name: fmt.Sprintf("S%d", i)}
	}
	source := createCredit(t, db, domain.CreditPerson, domain.CreditInput{Names: namesOf("S", sourceNames...)})
	linkPeople(t, db, "k0", "performer", source.ID, target.ID)

	before := creditRows(t, db)
	if _, err := db.Credits().Merge(ctx, domain.CreditOrganization, child.ID, []int64{grandparent.ID}); !errors.Is(err, domain.ErrCreditParentCycle) {
		t.Errorf("祖父母の統合: err = %v", err)
	}
	_, err := db.Credits().Merge(ctx, domain.CreditPerson, target.ID, []int64{source.ID})
	var invalid *domain.InvalidWorkInfoError
	if !errors.As(err, &invalid) || *invalid != (domain.InvalidWorkInfoError{Field: "names", Problem: domain.WorkInfoTooMany, Limit: 50}) {
		t.Errorf("名前が 51 件になる統合: err = %v", err)
	}
	if after := creditRows(t, db); !reflect.DeepEqual(before, after) {
		t.Errorf("拒んだ統合が行を変えた:\nbefore %v\nafter  %v", before, after)
	}
}

func TestOrganizationParentCycleIsRefused(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	root := createOrganization(t, db, "A", nil)
	child := createOrganization(t, db, "B", &root)
	grandchild := createOrganization(t, db, "C", &child)

	for _, parentID := range []int64{root.ID, grandchild.ID} {
		id := parentID
		if _, err := db.Credits().Update(ctx, domain.CreditOrganization, root.ID, domain.CreditPatch{ParentSet: true, ParentID: &id}); !errors.Is(err, domain.ErrCreditParentCycle) {
			t.Errorf("parentId %d: err = %v", id, err)
		}
	}
	moved, err := db.Credits().Update(ctx, domain.CreditOrganization, grandchild.ID, domain.CreditPatch{ParentSet: true, ParentID: &root.ID})
	if err != nil || moved.Parent == nil || moved.Parent.ID != root.ID {
		t.Fatalf("付け替え = %+v, %v", moved, err)
	}
	cleared, err := db.Credits().Update(ctx, domain.CreditOrganization, grandchild.ID, domain.CreditPatch{ParentSet: true})
	if err != nil || cleared.Parent != nil {
		t.Fatalf("親を外す = %+v, %v", cleared, err)
	}
}

func TestDeleteCreditRemovesLinksAndRenumbers(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	a, x, y := createPerson(t, db, "A"), createPerson(t, db, "X"), createPerson(t, db, "Y")
	linkPeople(t, db, "k0", "performer", a.ID, x.ID, y.ID)
	linkPeople(t, db, "k0", "director", a.ID)
	if err := db.Credits().Delete(ctx, domain.CreditPerson, a.ID); err != nil {
		t.Fatal(err)
	}
	if got := linkedPeople(t, db, "k0", "performer"); !reflect.DeepEqual(got, []int64{x.ID, y.ID}) {
		t.Errorf("performer = %v", got)
	}
	if got := linkedPeople(t, db, "k0", "director"); len(got) != 0 {
		t.Errorf("director = %v", got)
	}
	if err := db.Credits().Delete(ctx, domain.CreditPerson, a.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("2 度目の削除: err = %v", err)
	}

	parent := createOrganization(t, db, "P", nil)
	child := createOrganization(t, db, "C", &parent)
	if err := db.Credits().Delete(ctx, domain.CreditOrganization, parent.ID); err != nil {
		t.Fatal(err)
	}
	if got, err := db.Credits().Get(ctx, domain.CreditOrganization, child.ID); err != nil || got.Parent != nil {
		t.Errorf("親を消した子 = %+v, %v", got, err)
	}
}

func TestRefreshCreditSearchKeysFillsStaleRows(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	person := createPerson(t, db, "ハナコ", "Hanako")
	org := createOrganization(t, db, "メーカー", nil)
	if _, err := db.sql.Exec(`update people_names set search_key = '', sort_key = '', search_version = 0`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.sql.Exec(`update organization_names set search_key = '', search_version = 0`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.sql.Exec(`insert into video_work_info (content_key, code, code_key, search_version) values ('k0', 'AB-12', '', 0)`); err != nil {
		t.Fatal(err)
	}
	refreshed, err := db.Credits().RefreshSearchKeys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed != 4 {
		t.Errorf("作り直した行 = %d, want 4", refreshed)
	}
	page, err := db.Credits().List(ctx, owner, domain.CreditPerson, domain.CreditQuery{Q: "hanako"})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != person.ID {
		t.Errorf("作り直した鍵で引く = %+v, %v", page, err)
	}
	if page, err := db.Credits().List(ctx, owner, domain.CreditOrganization, domain.CreditQuery{Q: "めーかー"}); err != nil || len(page.Items) != 1 || page.Items[0].ID != org.ID {
		t.Errorf("団体 = %+v, %v", page, err)
	}
	var codeKey string
	if err := db.sql.QueryRow(`select code_key from video_work_info where content_key = 'k0'`).Scan(&codeKey); err != nil {
		t.Fatal(err)
	}
	if codeKey != domain.FoldForMatch("AB-12") {
		t.Errorf("code_key = %q", codeKey)
	}
	if again, err := db.Credits().RefreshSearchKeys(ctx); err != nil || again != 0 {
		t.Errorf("2 度目 = %d, %v", again, err)
	}
}

// Nonpositive detail IDs must not accidentally invoke the unfiltered list path.
func TestNonPositiveCreditIDIsNotFound(t *testing.T) {
	db := migratedDB(t)
	createPerson(t, db, "山田")
	createOrganization(t, db, "メーカー", nil)
	ctx := context.Background()
	for _, kind := range []domain.CreditKind{domain.CreditPerson, domain.CreditOrganization} {
		for _, id := range []int64{0, -1} {
			if got, err := db.Credits().Get(ctx, kind, id); !errors.Is(err, domain.ErrNotFound) {
				t.Errorf("Get(%s, %d) = %+v, %v; want ErrNotFound", kind, id, got, err)
			}
			if got, err := db.Credits().Update(ctx, kind, id, domain.CreditPatch{}); !errors.Is(err, domain.ErrNotFound) {
				t.Errorf("Update(%s, %d, {}) = %+v, %v; want ErrNotFound", kind, id, got, err)
			}
		}
	}
}
