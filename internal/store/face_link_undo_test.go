package store

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// undoFaceLink undoes one link, or every link for appearance 0, of the fixture's first face_analysis entry.
func (f linkedFixture) undoFaceLink(t *testing.T, appearanceID int64) domain.ActivityEntry {
	t.Helper()
	entry := f.firstFaceEntry(t)
	undo, err := f.db.Faces().UndoFaceLinks(context.Background(), entry.ID, appearanceID)
	if err != nil {
		t.Fatalf("UndoFaceLinks(%d, %d): %v", entry.ID, appearanceID, err)
	}
	return undo
}

// firstFaceEntry is the entry of the fixture's analysis, which made both links; later analyses come before it.
func (f linkedFixture) firstFaceEntry(t *testing.T) domain.ActivityEntry {
	t.Helper()
	entries := entriesOfKind(t, f.db, domain.ActivityFaceAnalysis)
	return entries[len(entries)-1]
}

func (f linkedFixture) setPerformers(t *testing.T, ids ...int64) {
	t.Helper()
	info, _ := workInfoOf(t, f.db, f.videoID)
	info.People = nil
	for _, id := range ids {
		info.People = append(info.People, performer(id))
	}
	if _, err := f.db.WorkInfo().SetDetails(context.Background(), f.videoID, "", info); err != nil {
		t.Fatal(err)
	}
}

func TestUndoingOneFaceLinkRemovesOnlyThatPerformer(t *testing.T) {
	f := newLinkedFixture(t)
	named := faceAppearances(t, f.db)[0].id

	undo := f.undoFaceLink(t, named)

	if undo.Kind != domain.ActivityUndo || undo.Outcome != domain.ActivityCompleted || undo.Detail != "Named" {
		t.Errorf("undo = %+v, want a completed undo naming the person", undo)
	}
	if got := linkedPeople(t, f.db, f.key, "performer"); !slices.Equal(got, []int64{f.provisional}) {
		t.Errorf("performers = %v, want [%d]", got, f.provisional)
	}
	if a := faceAppearances(t, f.db); !a[0].dropped || a[1].dropped {
		t.Errorf("dropped = %v, %v; want only the undone link's appearance", a[0].dropped, a[1].dropped)
	}
	entry, err := f.db.Activity().ActivityEntry(context.Background(), undo.UndoesID)
	if err != nil || len(entry.FaceLinks) != 2 || entry.FaceLinks[0].UndoneBy != undo.ID ||
		entry.FaceLinks[1].UndoneBy != 0 || !entry.UndoOffered() {
		t.Errorf("entry = %+v, %v; want only the first link undone and undo still offered", entry.FaceLinks, err)
	}

	f.analyse(t, facePerson(0.25, 0, faceBasis(0), "crop-1"), facePerson(0.75, 0, faceBasis(2), "crop-2"))
	if got := linkedPeople(t, f.db, f.key, "performer"); !slices.Equal(got, []int64{f.provisional}) {
		t.Errorf("performers after a re-analysis = %v, want [%d]", got, f.provisional)
	}
}

func TestUndoingEveryFaceLinkDeletesTheProvisionalPersonTheRunCreated(t *testing.T) {
	f := newLinkedFixture(t)
	people := personCount(t, f.db)

	if undo := f.undoFaceLink(t, 0); undo.Outcome != domain.ActivityCompleted || undo.Detail != "" {
		t.Errorf("undo = %+v, want completed without a name", undo)
	}

	if got := linkedPeople(t, f.db, f.key, "performer"); len(got) != 0 {
		t.Errorf("performers = %v, want none", got)
	}
	if _, err := f.db.Credits().Get(context.Background(), domain.CreditPerson, f.provisional); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("provisional person = %v, want deleted", err)
	}
	if got := personCount(t, f.db); got != people-1 {
		t.Errorf("people = %d, want %d: only the created person goes", got, people-1)
	}
}

func TestUndoingAFaceLinkTheOwnerChangedKeepsTheCredit(t *testing.T) {
	f := newLinkedFixture(t)
	f.setPerformers(t, f.provisional)
	f.setPerformers(t, f.provisional, f.named)

	undo := f.undoFaceLink(t, faceAppearances(t, f.db)[0].id)

	if undo.Outcome != domain.ActivityAlreadyReverted {
		t.Errorf("outcome = %q, want already_reverted", undo.Outcome)
	}
	if got := linkedPeople(t, f.db, f.key, "performer"); !slices.Equal(got, []int64{f.provisional, f.named}) {
		t.Errorf("performers = %v, want the owner's [%d %d]", got, f.provisional, f.named)
	}
}

func TestUndoingAFaceLinkAfterAReanalysisDropsTheCarriedAppearance(t *testing.T) {
	f := newLinkedFixture(t)
	before := faceAppearances(t, f.db)
	f.analyse(t, facePerson(0.25, 1, faceBasis(0), "crop-1"), facePerson(0.75, 1, faceBasis(2), "crop-2"))

	f.undoFaceLink(t, before[0].id)

	after := faceAppearances(t, f.db)
	if after[0].id != before[0].id || !after[0].dropped || after[0].firstStart != 1 {
		t.Errorf("appearance = %+v, want the carried-over row %d dropped", after[0], before[0].id)
	}
	if got := linkedPeople(t, f.db, f.key, "performer"); !slices.Equal(got, []int64{f.provisional}) {
		t.Errorf("performers = %v, want [%d]", got, f.provisional)
	}
}

func TestUndoingAFaceLinkTwiceIsRefused(t *testing.T) {
	f := newLinkedFixture(t)
	ctx := context.Background()
	entry := f.firstFaceEntry(t)
	named := faceAppearances(t, f.db)[0].id
	f.undoFaceLink(t, named)

	if _, err := f.db.Faces().UndoFaceLinks(ctx, entry.ID, named); !errors.Is(err, domain.ErrActivityNotUndoable) {
		t.Errorf("second undo = %v, want ErrActivityNotUndoable", err)
	}
	if _, err := f.db.Faces().UndoFaceLinks(ctx, entry.ID, named+100); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("undo of a link the entry did not make = %v, want ErrNotFound", err)
	}
}

func TestUndoingAFaceLinkKeepsThePerformerWhileAnotherAppearanceLinksThePerson(t *testing.T) {
	f := linkedFixture{faceFixture: newFaceFixture(t)}
	f.named = f.analyseOnePersonTwice(t, "crop")
	appearances := faceAppearances(t, f.db)

	f.undoFaceLink(t, appearances[0].id)
	if got := linkedPeople(t, f.db, f.key, "performer"); !slices.Equal(got, []int64{f.named}) {
		t.Errorf("performers after undoing one link = %v, want [%d]: the other appearance still links it", got, f.named)
	}

	f.undoFaceLink(t, appearances[1].id)
	if got := linkedPeople(t, f.db, f.key, "performer"); len(got) != 0 {
		t.Errorf("performers after undoing both links = %v, want none", got)
	}
}
