package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/store"
)

type fakeVersionReview struct {
	err      error
	rejudged [][2]int64
	frame    [2]int64
	image    []byte
}

func (r *fakeVersionReview) Rejudge(_ context.Context, videoIDs [2]int64) error {
	r.rejudged = append(r.rejudged, videoIDs)
	return r.err
}

func (r *fakeVersionReview) JudgementFrame(_ context.Context, entryID int64, index int) ([]byte, error) {
	r.frame = [2]int64{entryID, int64(index)}
	return r.image, r.err
}

func newVersionReviewEnv(t *testing.T, review *fakeVersionReview) (*authEnv, *http.Cookie) {
	t.Helper()
	env := newAuthEnvWith(t, t.TempDir(), func(*store.DB) Options { return Options{VersionReview: review} })
	return env, env.setup()
}

func TestRejudgeVersionCandidateAnswers(t *testing.T) {
	for _, c := range []struct {
		name string
		err  error
		want wantError
	}{
		{"判定がオフ", domain.ErrVersionJudgingOff,
			wantError{status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: gen.ErrorReasonVersionJudgingOff}},
		{"待ちか判定中", domain.ErrVersionNotRejudgeable,
			wantError{status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: gen.ErrorReasonVersionNotRejudgeable}},
		{"無い動画", fmt.Errorf("video: %w", domain.ErrNotFound),
			wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: reasonVideoNotFound}},
		{"候補でない組", domain.ErrNotVersionCandidate, wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound}},
	} {
		t.Run(c.name, func(t *testing.T) {
			env, owner := newVersionReviewEnv(t, &fakeVersionReview{err: c.err})
			rec := env.serve(authRequest{method: http.MethodPost, target: "/api/version-candidates/rejudge",
				body: `{"videoIds":[2,1]}`, cookies: []*http.Cookie{owner}})
			assertErrorBody(t, c.name, rec.Code, rec.Body.Bytes(), c.want)
		})
	}

	review := &fakeVersionReview{}
	env, owner := newVersionReviewEnv(t, review)
	rec := env.serve(authRequest{method: http.MethodPost, target: "/api/version-candidates/rejudge",
		body: `{"videoIds":[2,1]}`, cookies: []*http.Cookie{owner}})
	if rec.Code != http.StatusAccepted || !reflect.DeepEqual(review.rejudged, [][2]int64{{2, 1}}) {
		t.Errorf("status = %d、戻した組 = %v, want 202 と [2 1]: %s", rec.Code, review.rejudged, rec.Body)
	}
	rec = env.serve(authRequest{method: http.MethodPost, target: "/api/version-candidates/rejudge",
		body: `{"videoIds":[1,1]}`, cookies: []*http.Cookie{owner}})
	assertErrorBody(t, "同じ動画", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest})
}

func TestActivityJudgementFrame(t *testing.T) {
	review := &fakeVersionReview{image: []byte{0xff, 0xd8, 0xff, 0xd9}}
	env, owner := newVersionReviewEnv(t, review)
	rec := env.get("/api/activity/7/judgement-frames/2", owner)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/jpeg" ||
		rec.Header().Get("Cache-Control") != cacheNoStore || review.frame != [2]int64{7, 2} {
		t.Errorf("status = %d、%q、%q、頼んだコマ %v", rec.Code, rec.Header().Get("Content-Type"),
			rec.Header().Get("Cache-Control"), review.frame)
	}

	review.err = domain.ErrNotFound
	rec = env.get("/api/activity/7/judgement-frames/9", owner)
	assertErrorBody(t, "無いコマ", rec.Code, rec.Body.Bytes(), wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound})
}

func TestToAPIJudgeEvidence(t *testing.T) {
	got := toAPIJudgeEvidence(&domain.VersionJudgeEvidence{
		Rule: "frames_differ", Compared: 12, Differing: 3, LongestRun: 3, RunStartMs: 2000, RunEndMs: 5000,
		Conflict: true, Frames: []domain.VersionEvidenceFrame{{A: 3, B: 6, AMs: 3000, BMs: 3100, Distance: 30}},
	})
	run, start, end, conflict := 3, int64(2000), int64(5000), true
	want := &gen.VersionJudgeEvidence{
		Rule: gen.FramesDiffer, Compared: 12, Differing: 3, LongestRun: &run,
		RunStartMs: &start, RunEndMs: &end, Conflict: &conflict,
		Frames: &[]gen.VersionEvidenceFrame{{AMs: 3000, BMs: 3100, Distance: 30}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("toAPIJudgeEvidence = %+v, want %+v", got, want)
	}
	if toAPIJudgeEvidence(nil) != nil {
		t.Error("根拠なしが値になった")
	}
}
