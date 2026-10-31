package httpapi

import (
	"context"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/store"
)

// remotePersonPath is the screens' path of the peer's person peerID.
func remotePersonPath(peerID int64, suffix string) string {
	return "/api/people/" + strconv.FormatInt(domain.ToRemoteID(peerID), 10) + suffix
}

// faceNodes pairs b with a, whose people store is wrapped by peer, so b reaches a's people by remote id.
func faceNodes(t *testing.T, peer func(Credits) Credits) (a, b *peerNode) {
	t.Helper()
	first := true
	_, _, a, b, _ = mergedNodesWith(t, func(_ *store.DB, opts *Options) {
		if first {
			opts.Credits, first = peer(opts.Credits), false
		}
	})
	return a, b
}

// createPerson creates name on n and returns its id there.
func createPerson(t *testing.T, n *peerNode, name string) int64 {
	t.Helper()
	rec := n.owner(http.MethodPost, "/api/people", `{"name":"`+name+`"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create %s: status = %d: %s", name, rec.Code, rec.Body)
	}
	return decode[gen.Person](t, rec).Id
}

// The peer's person's face candidates join by name both ways: the answer names a same-named local person by its
// local id, and a confirm naming that local id reaches the peer's person of the name.
func TestRemotePersonFaceCandidatesJoinPeopleByName(t *testing.T) {
	credits := &faceCandidateCredits{}
	a, b := faceNodes(t, func(people Credits) Credits { credits.Credits = people; return credits })
	// b creates Ken first, so Mio has different ids on the two nodes.
	kenB := createPerson(t, b, "Ken")
	mioA, mioB := createPerson(t, a, "Mio"), createPerson(t, b, "Mio")
	credits.pairs = []domain.FacePair{
		{Person: domain.CreditRef{ID: mioA, Name: "Mio"}, Similarity: 0.5},
		{Person: domain.CreditRef{ID: 31, Provisional: true}, Similarity: 0.25},
	}
	credits.merged = domain.FaceCandidateConfirmation{
		Person: domain.Credit{Kind: domain.CreditPerson, ID: mioA, Name: "Mio"}, UndoActivityID: 12,
	}

	rec := b.owner(http.MethodGet, remotePersonPath(5, "/face-candidates"), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("face-candidates: status = %d: %s", rec.Code, rec.Body)
	}
	list := decode[gen.FaceCandidateList](t, rec)
	if got := []int64{list.Items[0].Person.Id, list.Items[1].Person.Id}; !slices.Equal(got, []int64{mioB, domain.ToRemoteID(31)}) {
		t.Errorf("candidate ids = %v, want the local Mio %d and the peer's provisional person in the offset form", got, mioB)
	}

	rec = b.owner(http.MethodPost, remotePersonPath(5, "/face-candidates/"+strconv.FormatInt(mioB, 10)+"/confirm"), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("confirm: status = %d: %s", rec.Code, rec.Body)
	}
	// The undo entry is in the peer's activity log, which this node's undo cannot reach.
	if got := decode[gen.FaceCandidateConfirmation](t, rec); got.Person.Id != mioB || got.UndoActivityId != nil {
		t.Errorf("confirm = %+v, want the local Mio %d and no undo entry", got, mioB)
	}
	rec = b.owner(http.MethodPost, remotePersonPath(5, "/face-candidates/"+strconv.FormatInt(kenB, 10)+"/dismiss"), "")
	assertErrorBody(t, "a name the peer lacks", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: gen.ErrorReasonFaceCandidateNotFound})
	if want := [][2]int64{{5, 0}, {5, mioA}}; !reflect.DeepEqual(credits.calls, want) {
		t.Errorf("the peer's calls = %v, want %v", credits.calls, want)
	}
}

// Moving the peer's faces to a local person moves them to the peer's person of that name, made there on the first
// move and found on the next; this node's appearance cannot join the move.
func TestRemotePersonFacesMoveToThePeersPersonOfTheTargetsName(t *testing.T) {
	credits := &faceAppearanceCredits{}
	_, b := faceNodes(t, func(people Credits) Credits { credits.Credits = people; return credits })
	// b creates Ken first, so the Mio made on the peer cannot share the local id.
	createPerson(t, b, "Ken")
	mioB := createPerson(t, b, "Mio")
	move := `{"appearanceIds":[` + strconv.FormatInt(domain.ToRemoteID(7), 10) + `],"targetId":` + strconv.FormatInt(mioB, 10) + `}`

	var targets []int64
	for range 2 {
		credits.calls = nil
		rec := b.owner(http.MethodPost, remotePersonPath(5, "/appearances/move"), move)
		if rec.Code != http.StatusOK {
			t.Fatalf("move: status = %d: %s", rec.Code, rec.Body)
		}
		if len(credits.calls) != 1 || credits.calls[0].personID != 5 || !slices.Equal(credits.calls[0].appearances, []int64{7}) {
			t.Fatalf("the peer's move = %+v, want appearance 7 of person 5", credits.calls)
		}
		targets = append(targets, credits.calls[0].target)
		credits.person = domain.Credit{Kind: domain.CreditPerson, ID: credits.calls[0].target, Name: "Mio"}
	}
	made, err := credits.Get(context.Background(), domain.CreditPerson, targets[0])
	if err != nil || made.Name != "Mio" || targets[1] != targets[0] {
		t.Errorf("targets = %v (%+v, %v), want the peer's Mio made once", targets, made, err)
	}

	rec := b.owner(http.MethodPost, remotePersonPath(5, "/appearances/move"), move)
	if got := decode[gen.FaceAppearanceMove](t, rec); got.Person.Id != mioB {
		t.Errorf("moved person = %d, want the local Mio %d", got.Person.Id, mioB)
	}
	credits.calls = nil
	rec = b.owner(http.MethodPost, remotePersonPath(5, "/appearances/move"), `{"appearanceIds":[7],"targetId":`+strconv.FormatInt(mioB, 10)+`}`)
	assertErrorBody(t, "this node's appearance", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: gen.ErrorReasonMixedNodes})
	if len(credits.calls) != 0 {
		t.Errorf("the peer's calls = %+v, want none for a mixed move", credits.calls)
	}
}

func TestRemoteFaceAppearanceCropStreamsThePeersJPEG(t *testing.T) {
	credits := &faceAppearanceCredits{crop: []byte{0xff, 0xd8, 0xff, 0xd9}}
	_, b := faceNodes(t, func(people Credits) Credits { credits.Credits = people; return credits })

	rec := b.owner(http.MethodGet, "/api/face-appearances/"+strconv.FormatInt(domain.ToRemoteID(7), 10)+"/crop", "")
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/jpeg" || !slices.Equal(rec.Body.Bytes(), credits.crop) {
		t.Errorf("crop: status = %d, Content-Type = %q, body = %x; want the peer's JPEG", rec.Code, rec.Header().Get("Content-Type"), rec.Body)
	}
	if want := []appearanceCall{{personID: 7}}; !reflect.DeepEqual(credits.calls, want) {
		t.Errorf("the peer's reads = %+v, want appearance 7", credits.calls)
	}
}

// A similar-people read of the peer's person compares the peer's appearances only; with none of them it is empty
// rather than a comparison of every appearance.
func TestPeerPersonQueryKeepsThePeersAppearances(t *testing.T) {
	r := strconv.FormatInt(domain.ToRemoteID(7), 10)
	for _, tc := range []struct {
		in, want []string
		empty    bool
	}{
		{in: []string{r, "3"}, want: []string{"7"}},
		{in: []string{"3"}, want: nil, empty: true},
	} {
		query := url.Values{"appearanceId": tc.in}
		var op remoteOperation
		peerPersonQuery(similarPeoplePattern, query, &op)
		if !slices.Equal(query["appearanceId"], tc.want) || (op.rewrite != nil) != tc.empty {
			t.Errorf("%v: appearanceId = %v, rewrite set %t; want %v and %t", tc.in, query["appearanceId"], op.rewrite != nil, tc.want, tc.empty)
		}
		if tc.empty {
			if got, err := op.rewrite([]byte(`{"items":[{}]}`), domain.VocabularyJoin{}); err != nil ||
				!reflect.DeepEqual(got, gen.SimilarPersonList{Items: []gen.SimilarPerson{}}) {
				t.Errorf("%v: answer = %+v (%v), want an empty list", tc.in, got, err)
			}
		}
	}
}
