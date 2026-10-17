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

// 人物と団体の保存。

func createCredit(t *testing.T, db *DB, kind domain.CreditKind, input domain.CreditInput) domain.Credit {
	t.Helper()
	credit, err := db.Credits().Create(context.Background(), kind, input)
	if err != nil {
		t.Fatalf("Create(%s, %+v) error = %v", kind, input, err)
	}
	return credit
}

func createPerson(t *testing.T, db *DB, name string, aliases ...string) domain.Credit {
	t.Helper()
	input := domain.CreditInput{Name: name}
	for _, alias := range aliases {
		input.Aliases = append(input.Aliases, domain.Alias{Name: alias})
	}
	return createCredit(t, db, domain.CreditPerson, input)
}

func createOrganization(t *testing.T, db *DB, name string, parent *domain.Credit) domain.Credit {
	t.Helper()
	input := domain.CreditInput{Name: name}
	if parent != nil {
		input.ParentID = &parent.ID
	}
	return createCredit(t, db, domain.CreditOrganization, input)
}

// linkPeople は利用者データの鍵 key の役割 role に、people の人物を位置の順に結ぶ。作品情報の書き込みは
// 次の単位なので、行を直接書く。
func linkPeople(t *testing.T, db *DB, key, role string, people ...int64) {
	t.Helper()
	for position, id := range people {
		if _, err := db.sql.Exec(`insert into video_work_people (content_key, role, position, person_id) values (?, ?, ?, ?)`,
			key, role, position, id); err != nil {
			t.Fatal(err)
		}
	}
}

// linkedPeople は鍵 key の役割 role の人物を位置の順に返す。
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

// creditRows は人物・団体の表すべての行を文字列にして返す。拒まれた操作が何も変えないことを見る。
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

// tableRows は表 table のすべての行を文字列にして返す。
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

func aliasNames(credit domain.Credit) []string {
	names := []string{}
	for _, alias := range credit.Aliases {
		names = append(names, alias.Name)
	}
	return names
}

func TestCreditsWithTheSameNameAreDifferentPeople(t *testing.T) {
	db := migratedDB(t)
	a := createPerson(t, db, "山田花子")
	b := createPerson(t, db, "山田花子")
	if a.ID == b.ID {
		t.Fatalf("同じ名前の 2 人が同じ id %d を持つ", a.ID)
	}
	page, err := db.Credits().List(context.Background(), domain.CreditPerson, domain.CreditQuery{})
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
	for i := range 150 {
		createPerson(t, db, fmt.Sprintf("人物%03d", i), fmt.Sprintf("Hanako %d", i))
	}
	createPerson(t, db, "別の人", "Taro")

	query := domain.CreditQuery{Q: "ＨＡＮＡＫＯ", Limit: 100}
	first, err := db.Credits().List(context.Background(), domain.CreditPerson, query)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 100 || first.Total != 150 || first.TotalAll != 151 || first.NextCursor == "" {
		t.Fatalf("1 ページ目 = %d 件、total %d、totalAll %d、nextCursor %q", len(first.Items), first.Total, first.TotalAll, first.NextCursor)
	}
	if first.Items[0].Name != "人物000" || len(first.Items[0].Aliases) != 1 {
		t.Errorf("先頭 = %+v", first.Items[0])
	}
	query.Cursor = first.NextCursor
	second, err := db.Credits().List(context.Background(), domain.CreditPerson, query)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 50 || second.Total != 150 || second.NextCursor != "" || second.Items[0].Name != "人物100" {
		t.Fatalf("2 ページ目 = %d 件、total %d、nextCursor %q", len(second.Items), second.Total, second.NextCursor)
	}

	// 別の種類のカーソルは受け付けない。
	if _, err := db.Credits().List(context.Background(), domain.CreditOrganization,
		domain.CreditQuery{Limit: 100, Cursor: first.NextCursor}); !errors.Is(err, domain.ErrInvalidCursor) {
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

	page, err := db.Credits().List(context.Background(), domain.CreditPerson, domain.CreditQuery{Sort: domain.TagSortCountDesc, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || page.Items[0].ID != many.ID || page.Items[0].VideoCount != 2 ||
		page.Items[1].ID != few.ID || page.Items[1].VideoCount != 1 {
		t.Fatalf("本数の多い順 = %+v", page.Items)
	}
	rest, err := db.Credits().List(context.Background(), domain.CreditPerson,
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

func TestCreateCreditStoresEverythingAndGetReadsIt(t *testing.T) {
	db := migratedDB(t)
	root := createOrganization(t, db, "親会社", nil)
	org := createCredit(t, db, domain.CreditOrganization, domain.CreditInput{
		Name:        " メーカーB ",
		Aliases:     []domain.Alias{{Name: "Maker B", Language: "en"}, {Name: "めーかーB"}, {Name: "Maker 10"}, {Name: "Maker 9"}},
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
	if want := []string{"Maker 9", "Maker 10", "Maker B", "めーかーB"}; !reflect.DeepEqual(aliasNames(got), want) {
		t.Errorf("別名 = %v, want %v", aliasNames(got), want)
	}
	if got.Aliases[2].Language != "en" {
		t.Errorf("別名の言語 = %+v", got.Aliases)
	}
	if want := []domain.ExternalID{{Source: "a", Value: "2"}, {Source: "z", Value: "1"}}; !reflect.DeepEqual(got.ExternalIDs, want) {
		t.Errorf("外部 ID = %v", got.ExternalIDs)
	}
	if _, err := db.Credits().Get(context.Background(), domain.CreditOrganization, org.ID+100); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("無い団体: err = %v", err)
	}

	missing := int64(9999)
	_, err = db.Credits().Create(context.Background(), domain.CreditOrganization, domain.CreditInput{Name: "X", ParentID: &missing})
	var notFound *domain.CreditNotFoundError
	if !errors.As(err, &notFound) || notFound.Field != "parentId" || notFound.ID != missing {
		t.Errorf("無い親: err = %v", err)
	}
	// 人物は親を読まない。
	person := createCredit(t, db, domain.CreditPerson, domain.CreditInput{Name: "P", ParentID: &missing})
	if person.Parent != nil {
		t.Errorf("人物に親がある: %+v", person.Parent)
	}
}

func TestUpdateCreditReplacesOnlyTheGivenFields(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	person := createCredit(t, db, domain.CreditPerson, domain.CreditInput{
		Name: "山田花子", Aliases: []domain.Alias{{Name: "Hanako"}}, ExternalIDs: []domain.ExternalID{{Source: "ex", Value: "1"}},
	})

	name := "山田 花子"
	renamed, err := db.Credits().Update(ctx, domain.CreditPerson, person.ID, domain.CreditPatch{Name: &name})
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Name != name || !reflect.DeepEqual(renamed.Aliases, person.Aliases) || !reflect.DeepEqual(renamed.ExternalIDs, person.ExternalIDs) {
		t.Fatalf("名前だけの更新 = %+v", renamed)
	}

	aliases := []domain.Alias{{Name: "山田花子"}, {Name: "Yamada", Language: "en"}}
	replaced, err := db.Credits().Update(ctx, domain.CreditPerson, person.ID, domain.CreditPatch{Aliases: &aliases})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"Yamada", "山田花子"}; !reflect.DeepEqual(aliasNames(replaced), want) {
		t.Fatalf("別名の置き換え = %v", aliasNames(replaced))
	}

	// 何も変えない更新はそのまま返す。
	same, err := db.Credits().Update(ctx, domain.CreditPerson, person.ID, domain.CreditPatch{Name: &name, Aliases: &aliases})
	if err != nil || !reflect.DeepEqual(same, replaced) {
		t.Fatalf("変えない更新 = %+v, %v", same, err)
	}

	// 残す別名と同じ新しい名前は name の重なり。
	clash := "Yamada"
	_, err = db.Credits().Update(ctx, domain.CreditPerson, person.ID, domain.CreditPatch{Name: &clash})
	var invalid *domain.InvalidWorkInfoError
	if !errors.As(err, &invalid) || invalid.Field != "name" || invalid.Problem != domain.WorkInfoDuplicate {
		t.Errorf("別名と同じ名前: err = %v", err)
	}

	// null の一覧は空にする。
	cleared, err := db.Credits().Update(ctx, domain.CreditPerson, person.ID, domain.CreditPatch{
		Aliases: new([]domain.Alias), ExternalIDs: new([]domain.ExternalID),
	})
	if err != nil || len(cleared.Aliases) != 0 || len(cleared.ExternalIDs) != 0 {
		t.Fatalf("一覧を空にする更新 = %+v, %v", cleared, err)
	}
	if _, err := db.Credits().Update(ctx, domain.CreditPerson, person.ID+100, domain.CreditPatch{Name: &name}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("無い人物: err = %v", err)
	}
}

func TestExternalIDHeldByAnotherPersonIsRefused(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	holder := createCredit(t, db, domain.CreditPerson, domain.CreditInput{Name: "A", ExternalIDs: []domain.ExternalID{{Source: "ex", Value: "42"}}})
	other := createPerson(t, db, "B")
	// 同じ外部 ID を団体が持つのは構わない。
	createCredit(t, db, domain.CreditOrganization, domain.CreditInput{Name: "O", ExternalIDs: []domain.ExternalID{{Source: "ex", Value: "42"}}})

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
	_, err := db.Credits().Create(ctx, domain.CreditPerson, domain.CreditInput{Name: "C", ExternalIDs: taken})
	check("Create", err)
	_, err = db.Credits().Update(ctx, domain.CreditPerson, other.ID, domain.CreditPatch{ExternalIDs: &taken})
	check("Update", err)
	if after := creditRows(t, db); !reflect.DeepEqual(before, after) {
		t.Errorf("拒んだ書き込みが行を変えた")
	}
	// 自分が持つ外部 ID は書き直せる。
	keep := []domain.ExternalID{{Source: "ex", Value: "42"}, {Source: "ex", Value: "43"}}
	if _, err := db.Credits().Update(ctx, domain.CreditPerson, holder.ID, domain.CreditPatch{ExternalIDs: &keep}); err != nil {
		t.Errorf("自分の外部 ID: err = %v", err)
	}
}

func TestLookupCreditByExternalID(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	person := createCredit(t, db, domain.CreditPerson, domain.CreditInput{Name: "A", ExternalIDs: []domain.ExternalID{{Source: "example", Value: "42"}}})
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
	target := createCredit(t, db, domain.CreditPerson, domain.CreditInput{Name: "T", Aliases: []domain.Alias{{Name: "共通"}}})
	source := createCredit(t, db, domain.CreditPerson, domain.CreditInput{
		Name: "S", Aliases: []domain.Alias{{Name: "共通"}, {Name: "Sの別名", Language: "ja"}},
		ExternalIDs: []domain.ExternalID{{Source: "ex", Value: "1"}},
	})
	other := createPerson(t, db, "X")
	linkPeople(t, db, "k0", "performer", source.ID, other.ID)
	linkPeople(t, db, "k1", "performer", target.ID, other.ID, source.ID)
	linkPeople(t, db, "k1", "director", other.ID, source.ID)

	outcome, err := db.Credits().Merge(ctx, domain.CreditPerson, target.ID, []int64{source.ID, 9999, source.ID})
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
	if want := []string{"S", "Sの別名", "共通"}; !reflect.DeepEqual(aliasNames(merged), want) {
		t.Errorf("別名 = %v, want %v", aliasNames(merged), want)
	}
	if !reflect.DeepEqual(merged.ExternalIDs, []domain.ExternalID{{Source: "ex", Value: "1"}}) {
		t.Errorf("外部 ID = %v", merged.ExternalIDs)
	}
	// 統合先は統合元の場所を引き継ぐ。両方を持っていた一覧は統合元の行を捨てて詰め直す。
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
	targetAliases := make([]domain.Alias, 30)
	for i := range targetAliases {
		targetAliases[i] = domain.Alias{Name: fmt.Sprintf("T%d", i)}
	}
	if _, err := db.Credits().Update(ctx, domain.CreditPerson, target.ID, domain.CreditPatch{Aliases: &targetAliases}); err != nil {
		t.Fatal(err)
	}
	sourceAliases := make([]domain.Alias, 25)
	for i := range sourceAliases {
		sourceAliases[i] = domain.Alias{Name: fmt.Sprintf("S%d", i)}
	}
	source := createCredit(t, db, domain.CreditPerson, domain.CreditInput{Name: "S", Aliases: sourceAliases})
	linkPeople(t, db, "k0", "performer", source.ID, target.ID)

	before := creditRows(t, db)
	if _, err := db.Credits().Merge(ctx, domain.CreditOrganization, child.ID, []int64{grandparent.ID}); !errors.Is(err, domain.ErrCreditParentCycle) {
		t.Errorf("祖父母の統合: err = %v", err)
	}
	_, err := db.Credits().Merge(ctx, domain.CreditPerson, target.ID, []int64{source.ID})
	var invalid *domain.InvalidWorkInfoError
	if !errors.As(err, &invalid) || *invalid != (domain.InvalidWorkInfoError{Field: "aliases", Problem: domain.WorkInfoTooMany, Limit: 50}) {
		t.Errorf("別名が 50 を超える統合: err = %v", err)
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
	// 輪にならない付け替えと、親を外す更新は通る。
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
	page, err := db.Credits().List(ctx, domain.CreditPerson, domain.CreditQuery{Q: "hanako"})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != person.ID {
		t.Errorf("作り直した鍵で引く = %+v, %v", page, err)
	}
	if page, err := db.Credits().List(ctx, domain.CreditOrganization, domain.CreditQuery{Q: "めーかー"}); err != nil || len(page.Items) != 1 || page.Items[0].ID != org.ID {
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

// 正でない id はどの実体も指さない。一覧の絞り込みなしと取り違えて、表の最初の実体を返さない。
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
