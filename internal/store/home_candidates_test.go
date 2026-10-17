package store

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// 規則の整理役の元の数え方（docs/design-docs/home.md）。

// candidatesFixture は 10 本の動画を入れ、Five を 0〜4、Four を 5〜8、仮のタグ Guess を 0〜5、種類付きの
// Kinded を 5〜9 に付け、花子を 0〜4 の出演者に、メーカー A を 5〜9 に結ぶ。
func candidatesFixture(t *testing.T) (*DB, map[string]int64) {
	t.Helper()
	db := migratedDB(t)
	ctx := context.Background()
	keys := make([]string, 10)
	ids := make([]int64, 10)
	for i := range keys {
		keys[i] = fmt.Sprintf("key-%d", i)
		path := fixturePath(fmt.Sprintf("/media/%d.mp4", i))
		ids[i] = upsertAll(t, db, listingFile(path, fmt.Sprintf("v%d", i), keys[i], i))[path]
	}
	tags := map[string]int64{}
	for name, span := range map[string][2]int{"Five": {0, 5}, "Four": {5, 9}, "Guess": {0, 6}, "Kinded": {5, 10}} {
		tag, err := db.Tags().CreateTag(ctx, name)
		if err != nil {
			t.Fatal(err)
		}
		tags[name] = tag.ID
		for i := span[0]; i < span[1]; i++ {
			attachTag(t, db, keys[i], tag.ID)
		}
	}
	if _, err := db.sql.Exec(`update tags set tentative = 1 where id = ?`, tags["Guess"]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.sql.Exec(`update tags set kind_id = (select min(id) from tag_kinds) where id = ?`, tags["Kinded"]); err != nil {
		t.Fatal(err)
	}
	hanako := createPerson(t, db, "山田花子")
	maker := createOrganization(t, db, "メーカーA", nil)
	tags["person"], tags["organization"] = hanako.ID, maker.ID
	for i := range ids {
		info := domain.WorkInfo{People: []domain.WorkPerson{performer(hanako.ID)}}
		if i >= 5 {
			info = domain.WorkInfo{Organizations: []domain.WorkOrganization{
				{Role: domain.OrganizationMaker, Organization: domain.CreditRef{ID: maker.ID}},
			}}
		}
		setWorkInfo(t, db, ids[i], info)
	}
	return db, tags
}

// candidateNames は候補を "kind:name:videos" にして並べ替えて返す。
func candidateNames(found domain.ShelfCandidates) []string {
	out := []string{}
	for _, c := range found.Candidates {
		out = append(out, fmt.Sprintf("%s:%s:%d", c.Kind, c.Name, c.Videos))
	}
	slices.Sort(out)
	return out
}

// 見せる動画 5 本以上のタグ・人物・団体が候補になり、4 本のタグと仮のタグはならない。種類付きのタグは
// 種類の id を持つ。
func TestShelfCandidatesCountsTagsPeopleAndOrganizations(t *testing.T) {
	db, tags := candidatesFixture(t)
	found, err := db.Home().ShelfCandidates(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if found.Videos != 10 {
		t.Errorf("Videos = %d, want 10", found.Videos)
	}
	want := []string{"organization:メーカーA:5", "person:山田花子:5", "tag:Five:5", "tag:Kinded:5"}
	if got := candidateNames(found); !slices.Equal(got, want) {
		t.Errorf("candidates = %v, want %v", got, want)
	}
	for _, c := range found.Candidates {
		if c.Kind == domain.ShelfSourceTag && c.ID == tags["Kinded"] && c.TagKindID == 0 {
			t.Errorf("Kinded has no kind id")
		}
		if c.Kind == domain.ShelfSourceTag && c.ID == tags["Five"] && c.TagKindID != 0 {
			t.Errorf("Five has kind id %d", c.TagKindID)
		}
	}
}

// unshelvedOnly は棚に一致する動画を数えない。Five の棚があれば 0〜4 は数えず、Five と花子は候補に
// ならない。
func TestShelfCandidatesUnshelvedOnly(t *testing.T) {
	db, tags := candidatesFixture(t)
	five := newSmartFolder(t, db, "Five", domain.SmartRule{Tags: []int64{tags["Five"]}}, domain.SortAddedDesc, 0)
	setHomeShelves(t, db, five)
	found, err := db.Home().ShelfCandidates(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if found.Videos != 5 {
		t.Errorf("Videos = %d, want 5", found.Videos)
	}
	want := []string{"organization:メーカーA:5", "tag:Kinded:5"}
	if got := candidateNames(found); !slices.Equal(got, want) {
		t.Errorf("candidates = %v, want %v", got, want)
	}
}
