package httpapi

import (
	"net/http"
	"testing"

	"github.com/syudead/vv/internal/httpapi/gen"
)

// Every fixture video is 60 seconds long, so the bounds either keep all six or none.
func TestListsPassDurationConditionsToTheStore(t *testing.T) {
	f := newLibraryFixture(t)
	for _, tc := range []struct {
		query string
		total int
	}{
		{"minDurationMs=60000&maxDurationMs=60000", 6},
		{"minDurationMs=60001", 0},
	} {
		rec := f.env.get("/api/videos?"+tc.query, f.owner)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d: %s", tc.query, rec.Code, rec.Body)
		}
		if page := decode[gen.VideoPage](t, rec); page.Total != tc.total {
			t.Errorf("/api/videos?%s total = %d, want %d", tc.query, page.Total, tc.total)
		}
	}
}

func TestConditionFiltersRejectInvalidValuesAndGuestWatchState(t *testing.T) {
	f := newLibraryFixture(t)
	invalid := wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest}
	for _, path := range []string{"/api/videos", "/api/library"} {
		for _, query := range []string{
			"minDurationMs=-1", "minDurationMs=2000&maxDurationMs=1000", "addedFrom=2025-13-01",
			"addedFrom=2025-03-02&addedTo=2025-03-01", "watchState=seen",
		} {
			rec := f.env.get(path+"?"+query, f.owner)
			assertErrorBody(t, path+"?"+query, rec.Code, rec.Body.Bytes(), invalid)
		}
		rec := f.env.get(path + "?watchState=watched")
		guest := wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonGuestFilterNotAllowed}
		assertErrorBody(t, "guest "+path+"?watchState=watched", rec.Code, rec.Body.Bytes(), guest)
	}
}
