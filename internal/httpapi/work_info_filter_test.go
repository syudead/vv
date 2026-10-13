package httpapi

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// 一覧の performer・maker（specs/047-video-work-info/contracts/screen-api.md「List parameters
// `performer` and `maker`」）を、本物の認証・保存層・経路をつないで確かめる。

// newWorkInfoFilterFixture は newLibraryFixture の show の ep2・ep10 に出演者を、ep2 と solo に
// 照合形の同じメーカーを付ける。
func newWorkInfoFilterFixture(t *testing.T) *libraryFixture {
	t.Helper()
	f := newLibraryFixture(t)
	for name, info := range map[string]domain.WorkInfo{
		"ep2":  {Maker: "Ａｃｍｅ", Performers: []string{"はなこ", "Alice"}},
		"ep10": {Maker: "other", Performers: []string{"ハナコ"}},
		"solo": {Maker: "acme"},
	} {
		if _, err := f.env.db.WorkInfo().Set(context.Background(), f.ids[name], info); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func listLibraryQuery(params url.Values) string {
	return "/api/library?" + params.Encode()
}

// 出演者・メーカーは幅とかなの違いを越えて一致し、両方は AND、一部だけ当たったグループは
// 当たったメンバーを動画の項目にする。total は絞り込み後の数で、一致が無ければ空の 200。
func TestListLibraryFiltersByPerformerAndMaker(t *testing.T) {
	f := newWorkInfoFilterFixture(t)
	cases := []struct {
		params url.Values
		want   []string
	}{
		{url.Values{"performer": {"ﾊﾅｺ"}}, []string{"ep10", "ep2"}},
		{url.Values{"maker": {"ACME"}}, []string{"ep2", "solo"}},
		{url.Values{"performer": {" はなこ "}, "maker": {"acme"}}, []string{"ep2"}},
		{url.Values{"performer": {"nobody"}}, []string{}},
	}
	for _, tc := range cases {
		rec := f.env.get(listLibraryQuery(tc.params), f.owner)
		if rec.Code != http.StatusOK {
			t.Fatalf("%v: status = %d: %s", tc.params, rec.Code, rec.Body)
		}
		page := decode[gen.LibraryPage](t, rec)
		if names := libraryItemNames(page); !slices.Equal(names, tc.want) || page.Total != len(tc.want) {
			t.Errorf("%v = %v・total %d, want %v", tc.params, names, page.Total, tc.want)
		}
		if page.MissingTagIds != nil {
			t.Errorf("%v: missingTagIds = %v, want なし", tc.params, *page.MissingTagIds)
		}
	}

	// GET /api/library/ids は同じ動画の id を返す。
	ids := decode[gen.VideoIdsResponse](t, f.env.get("/api/library/ids?performer="+url.QueryEscape("ﾊﾅｺ"), f.owner))
	slices.Sort(ids.Ids)
	want := []int64{f.ids["ep2"], f.ids["ep10"]}
	slices.Sort(want)
	if !slices.Equal(ids.Ids, want) {
		t.Errorf("ids = %v, want %v", ids.Ids, want)
	}

	// GET /api/videos も同じ条件で絞る。
	videos := decode[gen.VideoPage](t, f.env.get("/api/videos?maker=acme", f.owner))
	if videos.Total != 2 || len(videos.Items) != 2 {
		t.Errorf("/api/videos?maker=acme = %d 件・total %d, want 2", len(videos.Items), videos.Total)
	}
}

// 前後の空白を除いて空の値と 200 符号位置を超える値は 400、ゲストの指定は
// guest_filter_not_allowed の 400 である。
func TestListFiltersRejectInvalidPerformerAndMaker(t *testing.T) {
	f := newWorkInfoFilterFixture(t)
	tooLong := strings.Repeat("あ", domain.WorkInfoFilterMaxLength+1)
	for _, path := range []string{"/api/library", "/api/library/ids", "/api/videos"} {
		for _, params := range []url.Values{
			{"performer": {tooLong}},
			{"maker": {tooLong}},
			{"performer": {" 　"}},
			{"maker": {""}},
		} {
			rec := f.env.get(path+"?"+params.Encode(), f.owner)
			assertErrorBody(t, path+" "+params.Encode(), rec.Code, rec.Body.Bytes(),
				wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest})
		}
	}
	exact := strings.Repeat("あ", domain.WorkInfoFilterMaxLength)
	if rec := f.env.get(listLibraryQuery(url.Values{"maker": {exact}}), f.owner); rec.Code != http.StatusOK {
		t.Errorf("200 符号位置の maker: status = %d, want 200", rec.Code)
	}

	for _, path := range []string{"/api/library", "/api/videos"} {
		for _, query := range []string{"performer=x", "maker=x"} {
			rec := f.env.get(path + "?" + query)
			assertErrorBody(t, "ゲストの "+path+"?"+query, rec.Code, rec.Body.Bytes(),
				wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonGuestFilterNotAllowed})
		}
	}
}
