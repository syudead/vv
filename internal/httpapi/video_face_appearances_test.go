package httpapi

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/store"
)

// videoFaceCredits fakes the per-video appearance read; the store test covers which appearances it returns.
type videoFaceCredits struct {
	Credits
	found  domain.VideoFaceAppearances
	err    error
	videos []int64
}

func (c *videoFaceCredits) VideoFaceAppearances(_ context.Context, videoID int64) (domain.VideoFaceAppearances, error) {
	c.videos = append(c.videos, videoID)
	return c.found, c.err
}

func TestListVideoFaceAppearancesAnswersTheVideosPeople(t *testing.T) {
	credits := &videoFaceCredits{found: domain.VideoFaceAppearances{Analysed: true, Items: []domain.VideoFaceAppearance{
		{ID: 7, Person: domain.CreditRef{ID: 3, Provisional: true}, Spans: []domain.FaceTimeSpan{{Start: 5, End: 8}}},
		{ID: 4, Person: domain.CreditRef{ID: 2, Name: "Hanako"}, ImageRevision: 6, HasCrop: true,
			Spans: []domain.FaceTimeSpan{{Start: 20, End: 40}}},
	}}}
	env := newAuthEnvWith(t, t.TempDir(), func(db *store.DB) Options {
		credits.Credits = db.Credits()
		return Options{Credits: credits}
	})
	f := &creditFixture{env: env, owner: env.setup()}
	const target = "/api/videos/9/face-appearances"

	if rec := f.request(http.MethodGet, target, ""); rec.Code != http.StatusUnauthorized || len(credits.videos) != 0 {
		t.Fatalf("without a session: status = %d, reads = %v; want 401 and no read", rec.Code, credits.videos)
	}
	rec := f.owned(http.MethodGet, target, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	revision := int64(6)
	want := gen.VideoFaceAppearanceList{Analysed: true, Items: []gen.VideoFaceAppearance{
		{AppearanceId: 7, Person: gen.CreditRef{Id: 3, Provisional: true}, Spans: []gen.FaceAppearanceSpan{{Start: 5, End: 8}}},
		{AppearanceId: 4, Person: gen.CreditRef{Id: 2, Name: "Hanako"}, ImageRevision: &revision, HasCrop: true,
			Spans: []gen.FaceAppearanceSpan{{Start: 20, End: 40}}},
	}}
	if got := decode[gen.VideoFaceAppearanceList](t, rec); !reflect.DeepEqual(got, want) || !reflect.DeepEqual(credits.videos, []int64{9}) {
		t.Errorf("body = %+v, reads = %v; want %+v from video 9", got, credits.videos, want)
	}

	credits.err = domain.ErrNotFound
	rec = f.owned(http.MethodGet, target, "")
	assertErrorBody(t, "missing video", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: gen.ErrorReasonVideoNotFound})
}
