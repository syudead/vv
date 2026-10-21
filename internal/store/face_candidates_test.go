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

// offerFaceCandidate stores the pair as an analysis result would.
func offerFaceCandidate(t *testing.T, db *DB, provisionalID, personID int64) {
	t.Helper()
	ctx := context.Background()
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := writeFaceCandidates(ctx, tx, provisionalID,
		[]domain.FaceCandidate{{PersonID: personID, Similarity: 0.5}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
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

// createFacedProvisionalPerson creates a person without names whose face is the given crop, as an analysis does.
func createFacedProvisionalPerson(t *testing.T, db *DB, crop string) int64 {
	t.Helper()
	ctx := context.Background()
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	id, err := createFacePerson(ctx, tx, domain.PersonImage{JPEG: []byte(crop), Width: 1, Height: 1}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return id
}

// Between two people without names, the person whose faces page confirms stays with their own face, whichever of
// them the pair stores as its provisional side.
func TestConfirmFaceCandidateKeepsTheConfirmingPersonBetweenUnnamedPeople(t *testing.T) {
	for _, fromStoredSide := range []bool{true, false} {
		f := newFaceFixture(t)
		ctx := context.Background()
		stored := createFacedProvisionalPerson(t, f.db, "crop-stored")
		other := createFacedProvisionalPerson(t, f.db, "crop-other")
		offerFaceCandidate(t, f.db, stored, other)
		from, merged, crop := other, stored, "crop-other"
		if fromStoredSide {
			from, merged, crop = stored, other, "crop-stored"
		}

		person, err := f.db.Credits().ConfirmFaceCandidate(ctx, from, merged)
		if err != nil {
			t.Fatalf("from the stored side %v: %v", fromStoredSide, err)
		}
		if person.ID != from {
			t.Errorf("from the stored side %v: merged into %d, want %d", fromStoredSide, person.ID, from)
		}
		if _, err := f.db.Credits().Get(ctx, domain.CreditPerson, merged); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("from the stored side %v: the other person after confirm: err = %v, want ErrNotFound", fromStoredSide, err)
		}
		if image, err := f.db.Credits().PersonImage(ctx, from); err != nil || string(image.JPEG) != crop {
			t.Errorf("from the stored side %v: face = %q, %v; want %s", fromStoredSide, image.JPEG, err, crop)
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
	offerFaceCandidate(t, f.db, f.provisional, f.near)

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

func TestReopenFaceCandidateOffersADismissedPairAgain(t *testing.T) {
	f := newLinkedFixture(t)
	ctx := context.Background()
	if err := f.db.Credits().ReopenFaceCandidate(ctx, f.near, f.provisional); !errors.Is(err, domain.ErrFaceCandidateNotFound) {
		t.Errorf("reopen an open pair: err = %v, want ErrFaceCandidateNotFound", err)
	}
	if err := f.db.Credits().DismissFaceCandidate(ctx, f.provisional, f.near); err != nil {
		t.Fatal(err)
	}

	if err := f.db.Credits().ReopenFaceCandidate(ctx, f.near, f.provisional); err != nil {
		t.Fatal(err)
	}
	pairs, err := f.db.Credits().FaceCandidates(ctx, f.provisional)
	if err != nil {
		t.Fatal(err)
	}
	if got := facePairPeople(pairs); !slices.Equal(got, []int64{f.nearer, f.near}) {
		t.Errorf("candidates after reopen = %v, want Nearer %d and Near %d", got, f.nearer, f.near)
	}
}

// A merge can bring one pair of people in both orientations; only one row may remain, or the pair is listed twice.
func TestMergingKeepsOneCandidateRowPerPairOfPeople(t *testing.T) {
	t.Run("the target's dismissed pair stays", func(t *testing.T) {
		f := newLinkedFixture(t)
		ctx := context.Background()
		if err := f.db.Credits().DismissFaceCandidate(ctx, f.near, f.provisional); err != nil {
			t.Fatal(err)
		}
		offerFaceCandidate(t, f.db, f.nearer, f.provisional)
		if _, err := f.db.Credits().Merge(ctx, domain.CreditPerson, f.near, []int64{f.nearer}); err != nil {
			t.Fatal(err)
		}

		if got, want := faceCandidatePairs(t, f.db), [][2]int64{{f.provisional, f.near}}; !slices.Equal(got, want) {
			t.Errorf("candidates = %v, want %v", got, want)
		}
		pairs, err := f.db.Credits().FaceCandidates(ctx, f.near)
		if err != nil {
			t.Fatal(err)
		}
		if len(pairs) != 0 {
			t.Errorf("candidates of the target = %+v, want none", pairs)
		}
	})

	t.Run("two sources bring one pair", func(t *testing.T) {
		f := newLinkedFixture(t)
		ctx := context.Background()
		offerFaceCandidate(t, f.db, f.nearer, f.provisional)
		if _, err := f.db.Credits().Merge(ctx, domain.CreditPerson, f.named, []int64{f.near, f.nearer}); err != nil {
			t.Fatal(err)
		}

		if got, want := faceCandidatePairs(t, f.db), [][2]int64{{f.provisional, f.named}}; !slices.Equal(got, want) {
			t.Errorf("candidates = %v, want %v", got, want)
		}
		pairs, err := f.db.Credits().FaceCandidates(ctx, f.named)
		if err != nil {
			t.Fatal(err)
		}
		if got := facePairPeople(pairs); !slices.Equal(got, []int64{f.provisional}) {
			t.Errorf("candidates of the target = %v, want the provisional person %d once", got, f.provisional)
		}
	})
}
