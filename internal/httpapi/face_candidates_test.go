package httpapi

import (
	"context"
	"net/http"
	"reflect"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/store"
)

// faceCandidateCredits fakes the candidate operations and records their person ids; store tests cover the pairs.
type faceCandidateCredits struct {
	Credits
	pairs  []domain.FacePair
	merged domain.Credit
	err    error
	calls  [][2]int64
}

func (c *faceCandidateCredits) FaceCandidates(_ context.Context, personID int64) ([]domain.FacePair, error) {
	c.calls = append(c.calls, [2]int64{personID, 0})
	return c.pairs, c.err
}

func (c *faceCandidateCredits) ConfirmFaceCandidate(_ context.Context, personID, otherID int64) (domain.Credit, error) {
	c.calls = append(c.calls, [2]int64{personID, otherID})
	return c.merged, c.err
}

func (c *faceCandidateCredits) DismissFaceCandidate(_ context.Context, personID, otherID int64) error {
	c.calls = append(c.calls, [2]int64{personID, otherID})
	return c.err
}

func newFaceCandidateFixture(t *testing.T, credits *faceCandidateCredits) *creditFixture {
	t.Helper()
	env := newAuthEnvWith(t, t.TempDir(), func(db *store.DB) Options {
		credits.Credits = db.Credits()
		return Options{Credits: credits}
	})
	return &creditFixture{env: env, owner: env.setup()}
}

func TestListFaceCandidatesAnswersTheOtherPeople(t *testing.T) {
	credits := &faceCandidateCredits{pairs: []domain.FacePair{
		{Person: domain.CreditRef{ID: 9, Provisional: true}, ImageRevision: 3, Similarity: 0.75},
		{Person: domain.CreditRef{ID: 4, Name: "Hanako"}, Similarity: 0.5},
	}}
	f := newFaceCandidateFixture(t, credits)

	rec := f.owned(http.MethodGet, "/api/people/5/face-candidates", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	revision := gen.PersonImageRevision(3)
	want := gen.FaceCandidateList{Items: []gen.FaceCandidate{
		{Person: gen.FaceCandidatePerson{Id: 9, Name: "", Provisional: true, ImageRevision: &revision}, Similarity: 0.75},
		{Person: gen.FaceCandidatePerson{Id: 4, Name: "Hanako"}, Similarity: 0.5},
	}}
	if got := decode[gen.FaceCandidateList](t, rec); !reflect.DeepEqual(got, want) {
		t.Errorf("body = %+v, want %+v", got, want)
	}
	if !slices.Equal(credits.calls, [][2]int64{{5, 0}}) {
		t.Errorf("store calls = %v, want person 5", credits.calls)
	}

	credits.err = domain.ErrNotFound
	rec = f.owned(http.MethodGet, "/api/people/5/face-candidates", "")
	assertErrorBody(t, "missing person", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: gen.ErrorReasonPersonNotFound})
}

func TestConfirmAndDismissFaceCandidatePassThePathPeople(t *testing.T) {
	notFound := wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: gen.ErrorReasonFaceCandidateNotFound}
	for _, action := range []struct {
		name   string
		status int
	}{{"confirm", http.StatusOK}, {"dismiss", http.StatusNoContent}} {
		credits := &faceCandidateCredits{merged: domain.Credit{Kind: domain.CreditPerson, ID: 9, Name: "Hanako"}}
		f := newFaceCandidateFixture(t, credits)
		target := "/api/people/5/face-candidates/9/" + action.name

		rec := f.owned(http.MethodPost, target, "")
		if rec.Code != action.status {
			t.Fatalf("%s: status = %d, want %d: %s", action.name, rec.Code, action.status, rec.Body)
		}
		if action.name == "confirm" {
			if got := decode[gen.Person](t, rec); got.Id != 9 || got.Name != "Hanako" {
				t.Errorf("confirm: body = %+v, want the merged person 9", got)
			}
		}
		if !slices.Equal(credits.calls, [][2]int64{{5, 9}}) {
			t.Errorf("%s: store calls = %v, want people 5 and 9 in path order", action.name, credits.calls)
		}

		credits.err = domain.ErrFaceCandidateNotFound
		rec = f.owned(http.MethodPost, target, "")
		assertErrorBody(t, action.name+" without an open pair", rec.Code, rec.Body.Bytes(), notFound)
	}
}
