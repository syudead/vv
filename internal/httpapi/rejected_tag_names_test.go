package httpapi

import (
	"net/http"
	"net/url"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/httpapi/gen"
)

// 036 #727: GET /api/tags/rejected-names はページで返し、total と続きがあるときだけ nextCursor を
// 載せる。limit が範囲外・cursor が解釈できないときは 400 invalid_request
// （specs/036-tag-admin-scale/contracts/screen-api.md §6）。
func TestListRejectedTagNamesPages(t *testing.T) {
	f := newGuestFixture(t, true)
	for _, name := range []string{"name 10", "name 2", "name 1"} {
		id := f.addTentativeTag(t, "b", name)
		if rec := f.ownerRequest(http.MethodPost, tagPath(id, "/reject"), ""); rec.Code != http.StatusNoContent {
			t.Fatalf("却下 %s: status = %d: %s", name, rec.Code, rec.Body)
		}
	}

	first := decode[gen.RejectedTagNameList](t, f.ownerRequest(http.MethodGet, "/api/tags/rejected-names?limit=2", ""))
	if !slices.Equal(first.Items, []string{"name 1", "name 2"}) || first.Total != 3 || first.NextCursor == nil {
		t.Fatalf("1 ページ目 = %+v", first)
	}
	second := decode[gen.RejectedTagNameList](t, f.ownerRequest(http.MethodGet,
		"/api/tags/rejected-names?limit=2&cursor="+url.QueryEscape(*first.NextCursor), ""))
	if !slices.Equal(second.Items, []string{"name 10"}) || second.Total != 3 || second.NextCursor != nil {
		t.Errorf("2 ページ目 = %+v", second)
	}

	all := decode[gen.RejectedTagNameList](t, f.ownerRequest(http.MethodGet, "/api/tags/rejected-names", ""))
	if !slices.Equal(all.Items, []string{"name 1", "name 2", "name 10"}) || all.Total != 3 || all.NextCursor != nil {
		t.Errorf("パラメータ無し = %+v", all)
	}

	for _, query := range []string{"limit=0", "limit=201", "cursor=%21%21", "cursor=bmFtZQ"} {
		rec := f.ownerRequest(http.MethodGet, "/api/tags/rejected-names?"+query, "")
		if rec.Code != http.StatusBadRequest || decodeError(t, rec) != "invalid_request" {
			t.Errorf("%s: status = %d: %s", query, rec.Code, rec.Body)
		}
	}
}
