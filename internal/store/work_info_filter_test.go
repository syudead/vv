package store

import (
	"context"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// 一覧の出演者・メーカーの絞り込み（specs/047-video-work-info/data-model.md「Lists」）。

// workInfoFilterFixture は itemsFixture に作品情報を付ける。show の ep2・ep10 は出演者で、
// pair の 2 本はメーカーで当たるようにし、solo は両方を持つ。
func workInfoFilterFixture(t *testing.T) (*DB, map[string]int64) {
	t.Helper()
	db, ids := itemsFixture(t)
	set := func(path, maker string, performers ...string) {
		setWorkInfo(t, db, ids[fixturePath(path)], domain.WorkInfo{Maker: maker, Performers: performers})
	}
	set("/media/show/ep2.mp4", "Ａｃｍｅ", "はなこ", "Alice")
	set("/media/show/ep10.mp4", "other", "ハナコ")
	set("/media/solo.mp4", "acme", "alice")
	set("/media/pair/p1.mp4", "PairCo")
	set("/media/pair/p2.mp4", "pairco", "はなこ様")
	return db, ids
}

func sortedItemNames(items []domain.LibraryItem) []string {
	names := itemNames(items)
	slices.Sort(names)
	return names
}

// 出演者とメーカーは照合形の一致で当たり、両方は AND になる。一部だけ当たったグループは
// 当たったメンバーを動画の項目にし、全メンバーが当たったグループはグループの項目になる。
// total は絞り込み後の項目の数（libraryPages が確かめる）。
func TestListLibraryFiltersByPerformerAndMaker(t *testing.T) {
	db, _ := workInfoFilterFixture(t)
	cases := []struct {
		name  string
		query domain.VideoQuery
		want  []string
	}{
		// 半角カナ・ひらがな・カタカナの違いを越えて当たり、名前の一部（はなこ様）には当たらない。
		{"performer kana", domain.VideoQuery{Performer: "ﾊﾅｺ"}, []string{"ep10", "ep2"}},
		{"performer case", domain.VideoQuery{Performer: "ALICE"}, []string{"ep2", "solo"}},
		// 全角・大文字小文字の違いを越えて当たる。
		{"maker width", domain.VideoQuery{Maker: "ACME"}, []string{"ep2", "solo"}},
		{"maker whole group", domain.VideoQuery{Maker: "pairco"}, []string{"group:pair"}},
		{"both", domain.VideoQuery{Performer: "はなこ", Maker: "acme"}, []string{"ep2"}},
		{"both across videos", domain.VideoQuery{Performer: "ハナコ", Maker: "pairco"}, []string{}},
		{"no match", domain.VideoQuery{Performer: "nobody"}, []string{}},
		{"with search", domain.VideoQuery{Query: "solo", Maker: "acme"}, []string{"solo"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sortedItemNames(libraryPages(t, db, domain.AudienceOwner, tc.query))
			if !slices.Equal(got, tc.want) {
				t.Errorf("%+v = %v, want %v", tc.query, got, tc.want)
			}
		})
	}
}

// 「すべて選択」の id と GET /api/videos の一覧も同じ絞り込みに従う。
func TestLibraryIDsAndListVideosFilterByPerformerAndMaker(t *testing.T) {
	db, ids := workInfoFilterFixture(t)
	ctx := context.Background()
	ep2, ep10 := ids[fixturePath("/media/show/ep2.mp4")], ids[fixturePath("/media/show/ep10.mp4")]
	p1, p2 := ids[fixturePath("/media/pair/p1.mp4")], ids[fixturePath("/media/pair/p2.mp4")]

	idCases := []struct {
		query domain.VideoQuery
		want  []int64
	}{
		{domain.VideoQuery{Performer: "ﾊﾅｺ"}, []int64{ep2, ep10}},
		{domain.VideoQuery{Maker: "PAIRCO"}, []int64{p1, p2}},
		{domain.VideoQuery{Performer: "はなこ", Maker: "ａｃｍｅ"}, []int64{ep2}},
		{domain.VideoQuery{Maker: "nobody"}, []int64{}},
	}
	for _, tc := range idCases {
		selection, _, err := db.Library().LibraryIDs(ctx, tc.query)
		if err != nil {
			t.Fatal(err)
		}
		got := selection.AllVideoIDs()
		slices.Sort(got)
		want := slices.Clone(tc.want)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("ids %+v = %v, want %v", tc.query, got, want)
		}
	}

	page, err := db.Library().ListVideos(ctx, domain.AudienceOwner, domain.VideoQuery{Performer: "はなこ", Limit: domain.MaxLimit})
	if err != nil {
		t.Fatal(err)
	}
	var got []int64
	for _, video := range page.Items {
		got = append(got, video.ID)
	}
	slices.Sort(got)
	want := []int64{ep2, ep10}
	slices.Sort(want)
	if !slices.Equal(got, want) || page.Total != 2 {
		t.Errorf("ListVideos performer = %v (total %d), want %v (total 2)", got, page.Total, want)
	}
}

// 集まりの代表は、集まりの鍵に結んだ作品情報で当たる。
func TestListVideosFiltersBundleByItsWorkInfo(t *testing.T) {
	db, ids := versionFixture(t)
	ctx := context.Background()
	bundle(t, db, ids["a"], ids["a"], ids["b"])
	setWorkInfo(t, db, ids["b"], domain.WorkInfo{Maker: "Bundled", Performers: []string{"代表"}})

	for _, q := range []domain.VideoQuery{{Maker: "bundled"}, {Performer: "代表"}} {
		q.Limit = domain.MaxLimit
		page, err := db.Library().ListVideos(ctx, domain.AudienceOwner, q)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Items) != 1 || page.Items[0].ID != ids["a"] || page.Total != 1 {
			t.Errorf("%+v = %+v (total %d), want 代表 %d だけ", q, page.Items, page.Total, ids["a"])
		}
	}
}
