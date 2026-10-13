package store

import (
	"context"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// 作品情報での検索（specs/047-video-work-info/data-model.md「Search」、research.md R-3・R-7）。

// workInfoSearchFixture は動画 a だけに作品情報を持たせる。題名と所在はどの語にも当たらない。
func workInfoSearchFixture(t *testing.T) (*DB, map[string]int64) {
	t.Helper()
	db, ids := versionFixture(t)
	setWorkInfo(t, db, ids["a"], domain.WorkInfo{
		Code:        "AB-12",
		ReleaseDate: "2024-05-01",
		Maker:       "スタジオ北",
		Performers:  []string{"あいだ", "甲乙"},
		Description: "限定特典",
		SourceURL:   "https://example.com/works",
	})
	return db, ids
}

func audienceSearchTitles(t *testing.T, db *DB, audience domain.Audience, query string) []string {
	t.Helper()
	page, err := db.Library().ListVideos(context.Background(), audience, domain.VideoQuery{Query: query, Limit: domain.MaxLimit})
	if err != nil {
		t.Fatalf("%q で検索できない: %v", query, err)
	}
	return sorted(titlesOf(page)...)
}

// 品番（2〜5 文字）、メーカー、別のかな表記の出演者名のどれでも動画が当たり、除外語では除かれる。
// 説明・発売日・出典にだけある語では当たらない。
func TestSearchMatchesWorkInfo(t *testing.T) {
	db, _ := workInfoSearchFixture(t)
	cases := []struct {
		query string
		want  []string
	}{
		{"ab", []string{"a"}},
		{"ab-12", []string{"a"}},
		{"ＡＢ-１２", []string{"a"}},
		{"スタジオ北", []string{"a"}},
		{"アイダ", []string{"a"}},
		{"ｱｲﾀﾞ", []string{"a"}},
		{"甲乙", []string{"a"}},
		{"-ab-12", []string{"b", "c", "d"}},
		{"-アイダ", []string{"b", "c", "d"}},
		{"限定特典", nil},
		{"2024-05-01", nil},
		{"example", nil},
	}
	for _, tc := range cases {
		if got := audienceSearchTitles(t, db, domain.AudienceOwner, tc.query); !slices.Equal(got, sorted(tc.want...)) {
			t.Errorf("%q = %v, want %v", tc.query, got, tc.want)
		}
	}
}

// 作品情報は値と値の境目をまたいで当たらない。
func TestSearchWorkInfoDoesNotMatchAcrossValues(t *testing.T) {
	db, _ := workInfoSearchFixture(t)
	if got := audienceSearchTitles(t, db, domain.AudienceOwner, `"12 スタジオ"`); len(got) != 0 {
		t.Errorf("品番とメーカーをまたぐ語 = %v, want 無し", got)
	}
}

// ゲストの検索は作品情報に照合しない。公開した動画でも作品情報でだけ当たる語では返らない。
func TestGuestSearchIgnoresWorkInfo(t *testing.T) {
	db, ids := workInfoSearchFixture(t)
	setPublic(t, db, true, ids["a"])
	if got := audienceSearchTitles(t, db, domain.AudienceGuest, ""); !slices.Equal(got, []string{"a"}) {
		t.Fatalf("ゲストの一覧 = %v, want [a]", got)
	}
	for _, query := range []string{"ab-12", "スタジオ北", "アイダ"} {
		if got := audienceSearchTitles(t, db, domain.AudienceGuest, query); len(got) != 0 {
			t.Errorf("ゲストで %q = %v, want 無し", query, got)
		}
		if got := audienceSearchTitles(t, db, domain.AudienceOwner, query); !slices.Equal(got, []string{"a"}) {
			t.Errorf("所有者で %q = %v, want [a]", query, got)
		}
	}
}

// search_version = 0 の行は起動時の作り直しで鍵が補われ、その後の検索で当たる。
func TestSearchMatchesWorkInfoAfterRefresh(t *testing.T) {
	db, _ := workInfoSearchFixture(t)
	if _, err := db.sql.Exec(`update video_work_info set search_key = '', search_version = 0`); err != nil {
		t.Fatal(err)
	}
	if got := audienceSearchTitles(t, db, domain.AudienceOwner, "ab-12"); len(got) != 0 {
		t.Fatalf("鍵を消した後の ab-12 = %v, want 無し", got)
	}
	if refreshed, err := db.WorkInfo().RefreshSearchKeys(context.Background()); err != nil || refreshed != 1 {
		t.Fatalf("RefreshSearchKeys = %d, %v; want 1", refreshed, err)
	}
	if got := audienceSearchTitles(t, db, domain.AudienceOwner, "ab-12"); !slices.Equal(got, []string{"a"}) {
		t.Errorf("作り直した後の ab-12 = %v, want [a]", got)
	}
}
