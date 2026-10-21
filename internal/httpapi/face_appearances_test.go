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

type appearanceCall struct {
	personID    int64
	sort        domain.PersonAppearanceSort
	cursor      string
	limit       int
	appearances []int64
	target      int64
}

// faceAppearanceCredits fakes the appearance reads and records their arguments; store tests cover paging and drops.
type faceAppearanceCredits struct {
	Credits
	page    domain.PersonAppearancePage
	similar []domain.SimilarPerson
	crop    []byte
	person  domain.Credit
	err     error
	calls   []appearanceCall
}

func (c *faceAppearanceCredits) PersonAppearances(_ context.Context, personID int64, q domain.PersonAppearanceQuery) (domain.PersonAppearancePage, error) {
	c.calls = append(c.calls, appearanceCall{personID: personID, sort: q.Sort, cursor: q.Cursor, limit: q.Limit})
	return c.page, c.err
}

func (c *faceAppearanceCredits) FaceAppearanceCrop(_ context.Context, id int64) ([]byte, error) {
	c.calls = append(c.calls, appearanceCall{personID: id})
	return c.crop, c.err
}

func (c *faceAppearanceCredits) SplitFaceAppearances(_ context.Context, personID int64, appearanceIDs []int64) (domain.Credit, error) {
	c.calls = append(c.calls, appearanceCall{personID: personID, appearances: appearanceIDs})
	return c.person, c.err
}

func (c *faceAppearanceCredits) MoveFaceAppearances(_ context.Context, personID int64, appearanceIDs []int64, targetID int64) (domain.Credit, error) {
	c.calls = append(c.calls, appearanceCall{personID: personID, appearances: appearanceIDs, target: targetID})
	return c.person, c.err
}

func (c *faceAppearanceCredits) SimilarPeople(_ context.Context, personID int64, appearanceIDs []int64, limit int) ([]domain.SimilarPerson, error) {
	c.calls = append(c.calls, appearanceCall{personID: personID, appearances: appearanceIDs, limit: limit})
	return c.similar, c.err
}

func newFaceAppearanceFixture(t *testing.T, credits *faceAppearanceCredits) *creditFixture {
	t.Helper()
	env := newAuthEnvWith(t, t.TempDir(), func(db *store.DB) Options {
		credits.Credits = db.Credits()
		return Options{Credits: credits}
	})
	return &creditFixture{env: env, owner: env.setup()}
}

func TestListPersonAppearancesAnswersThePage(t *testing.T) {
	similarity := 0.25
	credits := &faceAppearanceCredits{page: domain.PersonAppearancePage{
		Items: []domain.PersonAppearance{
			{ID: 7, VideoID: 3, Title: "Beach", Spans: []domain.FaceTimeSpan{{Start: 12, End: 100}}, HasCrop: true,
				Similarity: &similarity, Unlike: true},
			{ID: 4, Title: "key-gone", Spans: []domain.FaceTimeSpan{{Start: 0, End: 10}}},
		},
		NextCursor: "next",
		Total:      12,
	}}
	f := newFaceAppearanceFixture(t, credits)

	rec := f.owned(http.MethodGet, "/api/people/5/appearances?sort=leastSimilar&limit=2&cursor=prev", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	videoID, next, unlike := int64(3), "next", true
	want := gen.FaceAppearanceList{Items: []gen.FaceAppearance{
		{AppearanceId: 7, VideoId: &videoID, Title: "Beach", Spans: []gen.FaceAppearanceSpan{{Start: 12, End: 100}}, HasCrop: true,
			Similarity: &similarity, Unlike: &unlike},
		{AppearanceId: 4, Title: "key-gone", Spans: []gen.FaceAppearanceSpan{{Start: 0, End: 10}}},
	}, NextCursor: &next, Total: 12}
	if got := decode[gen.FaceAppearanceList](t, rec); !reflect.DeepEqual(got, want) {
		t.Errorf("body = %+v, want %+v", got, want)
	}
	if want := []appearanceCall{{personID: 5, sort: domain.PersonAppearancesLeastSimilar, cursor: "prev", limit: 2}}; !reflect.DeepEqual(credits.calls, want) {
		t.Errorf("store calls = %+v, want %+v", credits.calls, want)
	}

	rec = f.owned(http.MethodGet, "/api/people/5/appearances?limit=201", "")
	assertErrorBody(t, "limit over 200", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest})
	rec = f.owned(http.MethodGet, "/api/people/5/appearances?sort=random", "")
	assertErrorBody(t, "unknown sort", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest})
	credits.calls = nil
	rec = f.owned(http.MethodGet, "/api/people/5/appearances", "")
	if want := []appearanceCall{{personID: 5, sort: domain.PersonAppearancesNewest}}; rec.Code != http.StatusOK ||
		!reflect.DeepEqual(credits.calls, want) {
		t.Errorf("without sort: status %d, calls %+v; want newest", rec.Code, credits.calls)
	}
	for _, tc := range []struct {
		err  error
		want wantError
	}{
		{domain.ErrInvalidCursor, wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonInvalidCursor}},
		{domain.ErrNotFound, wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: gen.ErrorReasonPersonNotFound}},
	} {
		credits.err = tc.err
		rec = f.owned(http.MethodGet, "/api/people/5/appearances?limit=2&cursor=x", "")
		assertErrorBody(t, tc.err.Error(), rec.Code, rec.Body.Bytes(), tc.want)
	}
}

func TestSplitFaceAppearanceAnswersTheNewPerson(t *testing.T) {
	credits := &faceAppearanceCredits{person: domain.Credit{Kind: domain.CreditPerson, ID: 9, Provisional: true}}
	f := newFaceAppearanceFixture(t, credits)
	target := "/api/people/5/appearances/7/split"

	if rec := f.request(http.MethodPost, target, ""); rec.Code != http.StatusUnauthorized || len(credits.calls) != 0 {
		t.Fatalf("without a session: status = %d, calls = %+v; want 401 and no split", rec.Code, credits.calls)
	}
	rec := f.owned(http.MethodPost, target, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if got := decode[gen.Person](t, rec); got.Id != 9 || !got.Provisional {
		t.Errorf("body = %+v, want the new provisional person 9", got)
	}
	if want := []appearanceCall{{personID: 5, appearances: []int64{7}}}; !reflect.DeepEqual(credits.calls, want) {
		t.Errorf("store calls = %+v, want person 5 and appearance 7", credits.calls)
	}

	for _, tc := range []struct {
		err  error
		want wantError
	}{
		{domain.ErrFaceAppearanceNotFound, wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: gen.ErrorReasonFaceAppearanceNotFound}},
		{domain.ErrNotFound, wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: gen.ErrorReasonPersonNotFound}},
	} {
		credits.err = tc.err
		rec = f.owned(http.MethodPost, target, "")
		assertErrorBody(t, tc.err.Error(), rec.Code, rec.Body.Bytes(), tc.want)
	}
}

func TestGetFaceAppearanceCropAnswersTheJPEG(t *testing.T) {
	credits := &faceAppearanceCredits{crop: []byte("jpeg")}
	f := newFaceAppearanceFixture(t, credits)

	rec := f.owned(http.MethodGet, "/api/face-appearances/7/crop", "")
	if rec.Code != http.StatusOK || rec.Body.String() != "jpeg" {
		t.Fatalf("status = %d, body = %q; want 200 with the crop", rec.Code, rec.Body)
	}
	for header, want := range map[string]string{"Content-Type": "image/jpeg", "Cache-Control": "private, max-age=86400"} {
		if got := rec.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
	if want := []appearanceCall{{personID: 7}}; !reflect.DeepEqual(credits.calls, want) {
		t.Errorf("store calls = %+v, want appearance 7", credits.calls)
	}

	credits.err = domain.ErrNotFound
	rec = f.owned(http.MethodGet, "/api/face-appearances/7/crop", "")
	assertErrorBody(t, "crop of a deleted person's appearance", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound})
}

func TestSplitFaceAppearancesSplitsTheListedOnesTogether(t *testing.T) {
	credits := &faceAppearanceCredits{person: domain.Credit{Kind: domain.CreditPerson, ID: 9, Provisional: true}}
	f := newFaceAppearanceFixture(t, credits)
	target := "/api/people/5/appearances/split"

	rec := f.owned(http.MethodPost, target, `{"appearanceIds":[7,3]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if got := decode[gen.Person](t, rec); got.Id != 9 {
		t.Errorf("body = %+v, want the new person 9", got)
	}
	if want := []appearanceCall{{personID: 5, appearances: []int64{7, 3}}}; !reflect.DeepEqual(credits.calls, want) {
		t.Errorf("store calls = %+v, want %+v", credits.calls, want)
	}
	for _, body := range []string{`{"appearanceIds":[]}`, `{"appearanceIds":[7,7]}`, `{"appearanceIds":[0]}`} {
		rec = f.owned(http.MethodPost, target, body)
		assertErrorBody(t, body, rec.Code, rec.Body.Bytes(),
			wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest})
	}
	if len(credits.calls) != 1 {
		t.Errorf("store calls after invalid bodies = %+v, want only the first", credits.calls)
	}
}

func TestMoveFaceAppearancesPassesTheTarget(t *testing.T) {
	credits := &faceAppearanceCredits{person: domain.Credit{Kind: domain.CreditPerson, ID: 9, Name: "Mio"}}
	f := newFaceAppearanceFixture(t, credits)
	target := "/api/people/5/appearances/move"

	rec := f.owned(http.MethodPost, target, `{"appearanceIds":[7,3],"targetId":9}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if got := decode[gen.Person](t, rec); got.Id != 9 {
		t.Errorf("body = %+v, want the target 9", got)
	}
	if want := []appearanceCall{{personID: 5, appearances: []int64{7, 3}, target: 9}}; !reflect.DeepEqual(credits.calls, want) {
		t.Errorf("store calls = %+v, want %+v", credits.calls, want)
	}
	credits.err = domain.ErrMergeSameCredit
	rec = f.owned(http.MethodPost, target, `{"appearanceIds":[7],"targetId":5}`)
	assertErrorBody(t, "same person", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest})

	// A missing or zero target would read as a new person in the store.
	credits.calls = nil
	for _, body := range []string{`{"appearanceIds":[7]}`, `{"appearanceIds":[7],"targetId":0}`} {
		rec = f.owned(http.MethodPost, target, body)
		assertErrorBody(t, body, rec.Code, rec.Body.Bytes(),
			wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest})
	}
	if len(credits.calls) != 0 {
		t.Errorf("store calls = %+v, want none for an invalid target", credits.calls)
	}
}

func TestListSimilarPeopleAnswersEachPersonWithTheirCrops(t *testing.T) {
	credits := &faceAppearanceCredits{similar: []domain.SimilarPerson{{
		Person:     domain.Credit{Kind: domain.CreditPerson, ID: 9, Provisional: true},
		Similarity: 0.7, FaceCount: 6, SampleAppearanceIDs: []int64{12, 11}, FirstSeenTitle: "Beach",
	}}}
	f := newFaceAppearanceFixture(t, credits)

	rec := f.owned(http.MethodGet, "/api/people/5/similar-people?appearanceId=7&appearanceId=3&limit=4", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	got := decode[gen.SimilarPersonList](t, rec)
	if len(got.Items) != 1 || got.Items[0].Person.Id != 9 || got.Items[0].FaceCount != 6 ||
		!reflect.DeepEqual(got.Items[0].SampleAppearanceIds, []int64{12, 11}) || got.Items[0].FirstSeenTitle == nil {
		t.Errorf("body = %+v, want person 9 with six faces, two crops and a first video", got)
	}
	if want := []appearanceCall{{personID: 5, appearances: []int64{7, 3}, limit: 4}}; !reflect.DeepEqual(credits.calls, want) {
		t.Errorf("store calls = %+v, want %+v", credits.calls, want)
	}
	credits.calls = nil
	if rec := f.owned(http.MethodGet, "/api/people/5/similar-people", ""); rec.Code != http.StatusOK ||
		!reflect.DeepEqual(credits.calls, []appearanceCall{{personID: 5, limit: 8}}) {
		t.Errorf("without parameters: status %d, calls %+v; want every face and 8 people", rec.Code, credits.calls)
	}
	rec = f.owned(http.MethodGet, "/api/people/5/similar-people?limit=51", "")
	assertErrorBody(t, "limit over 50", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest})
}
