package httpapi

import (
	"net/http"
	"testing"

	"github.com/syudead/vv/internal/httpapi/gen"
)

func TestListsFilterByMissingTagKindAreEmpty(t *testing.T) {
	f := newLibraryFixture(t)
	for _, path := range []string{"/api/videos", "/api/library", "/api/library/ids"} {
		rec := f.env.get(path+"?tagKind=999999", f.owner)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d: %s", path, rec.Code, rec.Body)
		}
		switch path {
		case "/api/videos":
			page := decode[gen.VideoPage](t, rec)
			if len(page.Items) != 0 || page.Total != 0 || page.MissingTagIds != nil {
				t.Errorf("%s = %d 件・total %d・missing %v, want 空", path, len(page.Items), page.Total, page.MissingTagIds)
			}
		case "/api/library":
			page := decode[gen.LibraryPage](t, rec)
			if len(page.Items) != 0 || page.Total != 0 || page.MissingTagIds != nil {
				t.Errorf("%s = %d 件・total %d・missing %v, want 空", path, len(page.Items), page.Total, page.MissingTagIds)
			}
		default:
			if ids := decode[gen.VideoIdsResponse](t, rec); len(ids.Ids) != 0 {
				t.Errorf("%s = %v, want 空", path, ids.Ids)
			}
		}
	}
}

func TestTagKindFilterRejectsInvalidValuesAndGuests(t *testing.T) {
	f := newLibraryFixture(t)
	invalid := wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest}
	for _, path := range []string{"/api/videos", "/api/library", "/api/library/ids"} {
		for _, query := range []string{"tagKind=0", "tagKind=-1", "tagKind=abc"} {
			rec := f.env.get(path+"?"+query, f.owner)
			assertErrorBody(t, path+"?"+query, rec.Code, rec.Body.Bytes(), invalid)
		}
	}
	guest := wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonGuestFilterNotAllowed}
	for _, path := range []string{"/api/videos", "/api/library"} {
		rec := f.env.get(path + "?tagKind=1")
		assertErrorBody(t, "ゲストの "+path+"?tagKind=1", rec.Code, rec.Body.Bytes(), guest)
	}
}
