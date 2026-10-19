package store

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

func facePairPeople(pairs []domain.FacePair) []int64 {
	out := make([]int64, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, p.Person.ID)
	}
	return out
}

func TestFaceCandidatesListTheOtherPersonOfEachOpenPair(t *testing.T) {
	f := newLinkedFixture(t)
	ctx := context.Background()

	pairs, err := f.db.Credits().FaceCandidates(ctx, f.provisional)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := facePairPeople(pairs), []int64{f.nearer, f.near}; !slices.Equal(got, want) {
		t.Fatalf("candidates of the provisional person = %v, want %v, highest similarity first", got, want)
	}
	if p := pairs[0]; p.Person.Name != "Nearer" || p.Person.Provisional || p.Similarity <= pairs[1].Similarity {
		t.Errorf("first candidate = %+v, want Nearer, named, more similar than %v", p, pairs[1].Similarity)
	}

	pairs, err = f.db.Credits().FaceCandidates(ctx, f.near)
	if err != nil {
		t.Fatal(err)
	}
	if len(pairs) != 1 || pairs[0].Person.ID != f.provisional || !pairs[0].Person.Provisional ||
		pairs[0].Person.Name != "" || pairs[0].ImageRevision == 0 {
		t.Errorf("candidates of Near = %+v, want the provisional person with its face", pairs)
	}
	if _, err := f.db.Credits().FaceCandidates(ctx, f.provisional+100); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("candidates of a missing person: err = %v, want ErrNotFound", err)
	}
}

func TestConfirmFaceCandidateMergesTheProvisionalPersonWhicheverIDComesFirst(t *testing.T) {
	for _, provisionalFirst := range []bool{true, false} {
		f := newLinkedFixture(t)
		ctx := context.Background()
		id, other := f.near, f.provisional
		if provisionalFirst {
			id, other = other, id
		}

		person, err := f.db.Credits().ConfirmFaceCandidate(ctx, id, other)
		if err != nil {
			t.Fatalf("provisional first %v: %v", provisionalFirst, err)
		}
		if person.ID != f.near || person.Name != "Near" {
			t.Errorf("provisional first %v: merged person = %+v, want Near", provisionalFirst, person)
		}
		if _, err := f.db.Credits().Get(ctx, domain.CreditPerson, f.provisional); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("provisional first %v: provisional person after confirm: err = %v, want ErrNotFound", provisionalFirst, err)
		}
		if got := linkedPeople(t, f.db, f.key, "performer"); !slices.Equal(got, []int64{f.named, f.near}) {
			t.Errorf("provisional first %v: performers = %v, want [%d %d]", provisionalFirst, got, f.named, f.near)
		}
		// The provisional person's other pair moved to Near, so Near is now offered with Nearer.
		if got, want := faceCandidatePairs(t, f.db), [][2]int64{{f.near, f.nearer}}; !slices.Equal(got, want) {
			t.Errorf("provisional first %v: candidates = %v, want %v", provisionalFirst, got, want)
		}
	}
}

func TestDismissFaceCandidateHidesThePairForGood(t *testing.T) {
	f := newLinkedFixture(t)
	ctx := context.Background()

	if err := f.db.Credits().DismissFaceCandidate(ctx, f.near, f.provisional); err != nil {
		t.Fatal(err)
	}
	// A later result offers the same pair again.
	tx, err := f.db.sql.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeFaceCandidates(ctx, tx, f.provisional,
		[]domain.FaceCandidate{{PersonID: f.near, Similarity: 0.5}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	pairs, err := f.db.Credits().FaceCandidates(ctx, f.provisional)
	if err != nil {
		t.Fatal(err)
	}
	if got := facePairPeople(pairs); !slices.Equal(got, []int64{f.nearer}) {
		t.Errorf("candidates after dismiss = %v, want only Nearer %d", got, f.nearer)
	}
	if err := f.db.Credits().DismissFaceCandidate(ctx, f.provisional, f.near); !errors.Is(err, domain.ErrFaceCandidateNotFound) {
		t.Errorf("dismiss again: err = %v, want ErrFaceCandidateNotFound", err)
	}
	if _, err := f.db.Credits().ConfirmFaceCandidate(ctx, f.provisional, f.near); !errors.Is(err, domain.ErrFaceCandidateNotFound) {
		t.Errorf("confirm a dismissed pair: err = %v, want ErrFaceCandidateNotFound", err)
	}
}
