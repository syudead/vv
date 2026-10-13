package store

import (
	"context"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// ライブラリの検索欄と作品情報（specs/047-video-work-info/data-model.md「Search」、research.md R-6・R-10、
// #1024）。

// workInfoSearchFixture は題名にも相対パスにも検索語を含まない 2 本の動画を作り、1 本目に作品情報を書く。
// 1 本目の作品情報は品番 AB-12、出演者の人物（別名はひらがな）、メーカーの団体、説明を持つ。2 本目は
// 作品情報を持たない。どちらも公開する。
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

// 所有者の検索語は品番（2〜5 文字の部分）、人物の名前、別のかなの形で書いた別名、団体の名前に当たり、
// 除外語にするとその動画が除かれる。説明にだけある語では当たらない。
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

// ゲストの検索は作品情報に照合しないので、作品情報だけで当たる公開動画は返らない（R-10）。
func TestGuestSearchDoesNotMatchWorkInfo(t *testing.T) {
	db, _, _, _ := workInfoSearchFixture(t)
	for _, query := range []string{"ab-12", "山田花子", "やまだ", "メーカーA"} {
		if got := listIDs(t, db, domain.AudienceGuest, domain.VideoQuery{Query: query}); len(got) != 0 {
			t.Errorf("ゲストの %q = %v, want なし", query, got)
		}
	}
}

// 改名した人物は新しい名前で見つかり、古い名前では見つからない。動画側の鍵は書き直さない（R-6）。
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

// 集まりの代表以外のバージョンから書いた作品情報も集まりの利用者データの鍵に載るので、ライブラリの
// 項目（ListLibrary）では品番・人物・団体の名前の語で代表の 1 件が出て、除外語にすると集まりごと
// 除かれる（data-model.md「Search」、specs/030-video-versions/data-model.md §3）。
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
