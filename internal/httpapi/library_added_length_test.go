package httpapi

import (
	"net/http"
	"testing"

	"github.com/syudead/vv/internal/httpapi/gen"
)

// All fixture videos were added on 2026-01-01 and last sixty seconds.

var addedLengthPaths = []string{"/api/videos", "/api/library"}

// Check boundary filtering counts here; storage tests cover sort details.
func TestListVideosFiltersByAddedWithinAndLength(t *testing.T) {
	f := newLibraryFixture(t)
	cases := []struct {
		query string
		want  int
	}{
		{"minDuration=60&maxDuration=60", 6},
		{"minDuration=61", 0},
		{"maxDuration=59", 0},
		{"addedWithin=36500", 6},
		{"addedWithin=1", 0},
	}
	for _, tc := range cases {
		rec := f.env.get("/api/videos?"+tc.query, f.owner)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status = %d: %s", tc.query, rec.Code, rec.Body)
			continue
		}
		if page := decode[gen.VideoPage](t, rec); page.Total != tc.want {
			t.Errorf("%s: total = %d, want %d", tc.query, page.Total, tc.want)
		}
	}
}

func TestAddedWithinAndLengthRejectOutOfRangeValues(t *testing.T) {
	f := newLibraryFixture(t)
	invalid := wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest}
	queries := []string{
		"addedWithin=0", "addedWithin=36501", "addedWithin=abc",
		"minDuration=-1", "maxDuration=360001", "minDuration=601&maxDuration=600",
	}
	for _, path := range addedLengthPaths {
		for _, query := range queries {
			rec := f.env.get(path+"?"+query, f.owner)
			assertErrorBody(t, path+"?"+query, rec.Code, rec.Body.Bytes(), invalid)
		}
	}
}

func TestGuestMayFilterByAddedWithinAndLength(t *testing.T) {
	f := newLibraryFixture(t)
	for _, path := range []string{"/api/videos", "/api/library"} {
		query := "addedWithin=7&minDuration=60&maxDuration=600"
		if rec := f.env.get(path + "?" + query); rec.Code != http.StatusOK {
			t.Errorf("ゲストの %s?%s: status = %d: %s", path, query, rec.Code, rec.Body)
		}
	}
}
