package store

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func jpegCrop(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewGray(image.Rect(0, 0, 4, 3)), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// newSplitFixture is the linked fixture with a decodable crop on its first appearance, linked to the named person.
func newSplitFixture(t *testing.T) (linkedFixture, []byte) {
	t.Helper()
	f := newLinkedFixture(t)
	crop := jpegCrop(t)
	if _, err := f.db.sql.Exec(`update face_appearances set crop = ? where id = ?`, crop, faceAppearances(t, f.db)[0].id); err != nil {
		t.Fatal(err)
	}
	return f, crop
}

func performersOf(t *testing.T, f faceFixture) []int64 {
	t.Helper()
	info, _ := workInfoOf(t, f.db, f.videoID)
	return performerIDs(&info)
}

func personVectorCount(t *testing.T, db *DB, personID int64) int {
	t.Helper()
	tx, err := db.sql.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	vectors, err := facePersonVectors(context.Background(), tx, domain.FaceRecognizerSFaceInt8)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, v := range vectors {
		if v.PersonID == personID {
			n++
		}
	}
	return n
}

func TestSplitFaceAppearanceMakesAProvisionalPersonOfTheFace(t *testing.T) {
	f, crop := newSplitFixture(t)
	ctx := context.Background()
	moved := faceAppearances(t, f.db)[0]
	if got := performersOf(t, f.faceFixture); !slices.Equal(got, []int64{f.named, f.provisional}) {
		t.Fatalf("performers before = %v, want named then provisional", got)
	}
	vectorsBefore := personVectorCount(t, f.db, f.named)

	created, err := f.db.Credits().SplitFaceAppearances(ctx, f.named, []int64{moved.id})
	if err != nil {
		t.Fatal(err)
	}
	if !created.Provisional || created.ID == f.named || created.ImageRevision == 0 {
		t.Fatalf("created = %+v, want a new provisional person with a face", created)
	}
	var face []byte
	var width, height int
	if err := f.db.sql.QueryRow(`select image, width, height from person_images where person_id = ?`, created.ID).
		Scan(&face, &width, &height); err != nil || !bytes.Equal(face, crop) || width != 4 || height != 3 {
		t.Errorf("face = %d bytes %dx%d, %v; want the appearance's crop at 4x3", len(face), width, height, err)
	}
	after := faceAppearances(t, f.db)[0]
	if after.personID != created.ID || !bytes.Equal(after.centroid, moved.centroid) || after.dropped {
		t.Errorf("appearance = %+v, want it moved to the new person with its centroid", after)
	}
	if got := performersOf(t, f.faceFixture); !slices.Equal(got, []int64{created.ID, f.provisional}) {
		t.Errorf("performers = %v, want the new person in the old person's place", got)
	}
	if pairs, err := f.db.Credits().FaceCandidates(ctx, created.ID); err != nil || len(pairs) != 0 {
		t.Errorf("candidates of the new person = %+v, %v; want none", pairs, err)
	}
	var dismissed int
	if err := f.db.sql.QueryRow(`select count(*) from face_candidates where dismissed = 1
		and (provisional_id = ?1 and person_id = ?2 or provisional_id = ?2 and person_id = ?1)`,
		created.ID, f.named).Scan(&dismissed); err != nil || dismissed != 1 {
		t.Errorf("dismissed pairs of the two people = %d, %v; want 1", dismissed, err)
	}
	if got := personVectorCount(t, f.db, f.named); got != vectorsBefore {
		t.Errorf("vectors of the old person = %d, want %d: their face stays the one they are matched by", got, vectorsBefore)
	}

	// A re-analysis keeps the appearance with the new person.
	f.analyse(t, facePerson(0.25, 1, faceBasis(0), "x"), facePerson(0.75, 1, faceBasis(2), "y"))
	if got := faceAppearances(t, f.db)[0]; got.id != moved.id || got.personID != created.ID {
		t.Errorf("appearance after re-analysis = %+v, want it kept on person %d", got, created.ID)
	}
}

func TestSplitFaceAppearanceKeepsTheCreditAnotherAppearanceStillLinks(t *testing.T) {
	f := newFaceFixture(t)
	ctx := context.Background()
	named := createPerson(t, f.db, "Named").ID
	setFaceVector(t, f.db, named, faceBasis(0))
	crop := string(jpegCrop(t))
	f.analyse(t, facePerson(0.25, 0, faceBasis(0), crop), facePerson(0.75, 0, faceBasis(0), crop))
	appearances := faceAppearances(t, f.db)
	if len(appearances) != 2 || appearances[0].personID != named || appearances[1].personID != named {
		t.Fatalf("appearances = %+v, want both linked to the named person", appearances)
	}

	created, err := f.db.Credits().SplitFaceAppearances(ctx, named, []int64{appearances[1].id})
	if err != nil {
		t.Fatal(err)
	}
	if got := performersOf(t, f); !slices.Equal(got, []int64{named, created.ID}) {
		t.Errorf("performers = %v, want the named person kept and the new person added", got)
	}
}

func TestSplitFaceAppearancePublishesTheNewFaceWithoutACreditChange(t *testing.T) {
	f, _ := newSplitFixture(t)
	recorder := &committedFaceRecorder{t: t, db: f.db}
	f.db.PublishTo(recorder)
	if _, err := f.db.sql.Exec(`delete from video_work_people where role = 'performer'`); err != nil {
		t.Fatal(err)
	}

	created, err := f.db.Credits().SplitFaceAppearances(context.Background(), f.named, []int64{faceAppearances(t, f.db)[0].id})
	if err != nil {
		t.Fatal(err)
	}
	if got := recorder.take(); !slices.Equal(got, []int64{created.ID}) {
		t.Errorf("face events = %v, want [%d] for the new person", got, created.ID)
	}
}

func TestSplitFaceAppearanceRefusesAnAppearanceOfAnotherPerson(t *testing.T) {
	f, _ := newSplitFixture(t)
	ctx := context.Background()
	appearances := faceAppearances(t, f.db)
	people := personCount(t, f.db)

	if _, err := f.db.Credits().SplitFaceAppearances(ctx, f.named, []int64{appearances[1].id}); !errors.Is(err, domain.ErrFaceAppearanceNotFound) {
		t.Errorf("another person's appearance: err = %v, want ErrFaceAppearanceNotFound", err)
	}
	if _, err := f.db.Credits().SplitFaceAppearances(ctx, f.named+1000, []int64{appearances[0].id}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("missing person: err = %v, want ErrNotFound", err)
	}
	if personCount(t, f.db) != people || faceAppearances(t, f.db)[1].personID != f.provisional {
		t.Error("a refused split changed people or appearances")
	}
}

// Several faces go to one new person, whose face is the first one's crop; a face the person does not have splits none.
func TestSplitFaceAppearancesMovesEveryFaceToOnePerson(t *testing.T) {
	f, crop := newSplitFixture(t)
	ctx := context.Background()
	first := faceAppearances(t, f.db)[0].id
	gone := insertFaceAppearance(t, f.db, "key-gone", f.named, false, `[[0, 10, 0.5, 0.5, 0.1]]`, nil)
	other := faceAppearances(t, f.db)[1].id

	if _, err := f.db.Credits().SplitFaceAppearances(ctx, f.named, []int64{first, other}); !errors.Is(err, domain.ErrFaceAppearanceNotFound) {
		t.Fatalf("with another person's face: err = %v, want ErrFaceAppearanceNotFound", err)
	}
	if got := faceAppearances(t, f.db)[0].personID; got != f.named {
		t.Fatalf("after a refused split the first face belongs to %d, want %d", got, f.named)
	}

	created, err := f.db.Credits().SplitFaceAppearances(ctx, f.named, []int64{first, gone})
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range faceAppearances(t, f.db) {
		if (a.id == first || a.id == gone) && a.personID != created.ID {
			t.Errorf("appearance %d belongs to %d, want the new person %d", a.id, a.personID, created.ID)
		}
	}
	image, err := f.db.Credits().PersonImage(ctx, created.ID)
	if err != nil || !bytes.Equal(image.JPEG, crop) {
		t.Errorf("face of the new person = %d bytes, %v; want the first face's crop", len(image.JPEG), err)
	}
}

// Faces moved to an existing person take the old person's credit, or leave the target's own credit alone.
func TestMoveFaceAppearancesHandsFacesAndCreditsToTheTarget(t *testing.T) {
	f, _ := newSplitFixture(t)
	ctx := context.Background()
	appearances := faceAppearances(t, f.db)

	moved, err := f.db.Credits().MoveFaceAppearances(ctx, f.named, []int64{appearances[0].id}, f.near, nil)
	if err != nil || moved.Person.ID != f.near || len(moved.Restore) != 0 {
		t.Fatalf("move = %+v, %v; want Near with nothing to restore", moved, err)
	}
	if got := faceAppearances(t, f.db)[0].personID; got != f.near {
		t.Errorf("the face belongs to %d, want Near %d", got, f.near)
	}
	if got := performersOf(t, f.faceFixture); !slices.Equal(got, []int64{f.near, f.provisional}) {
		t.Errorf("performers = %v, want Near in Named's place", got)
	}

	// Near is credited now, so the provisional person's face moving to Near drops only the provisional credit.
	if _, err := f.db.Credits().MoveFaceAppearances(ctx, f.provisional, []int64{appearances[1].id}, f.near, nil); err != nil {
		t.Fatal(err)
	}
	if got := performersOf(t, f.faceFixture); !slices.Equal(got, []int64{f.near}) {
		t.Errorf("performers = %v, want Near alone", got)
	}
	if _, err := f.db.Credits().MoveFaceAppearances(ctx, f.near, []int64{appearances[1].id}, f.near, nil); !errors.Is(err, domain.ErrMergeSameCredit) {
		t.Errorf("move to the same person: err = %v, want ErrMergeSameCredit", err)
	}
	if _, err := f.db.Credits().MoveFaceAppearances(ctx, f.near, []int64{appearances[1].id}, f.near+1000, nil); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("move to a missing person: err = %v, want ErrNotFound", err)
	}
}

// A move to a person already credited takes the old person's credit; the reverse move given what it returned keeps
// the target's own credit and puts the old person's back in place.
func TestMoveFaceAppearancesUndoKeepsTheTargetsOwnCredit(t *testing.T) {
	f, _ := newSplitFixture(t)
	ctx := context.Background()
	face := faceAppearances(t, f.db)[0].id
	// Near is credited in the work without a face of their own there.
	if _, err := f.db.sql.Exec(`insert into video_work_people (content_key, role, position, person_id)
		select content_key, 'performer', 2, ? from video_work_people where role = 'performer' limit 1`, f.near); err != nil {
		t.Fatal(err)
	}
	if got := performersOf(t, f.faceFixture); !slices.Equal(got, []int64{f.named, f.provisional, f.near}) {
		t.Fatalf("performers before = %v", got)
	}

	moved, err := f.db.Credits().MoveFaceAppearances(ctx, f.named, []int64{face}, f.near, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := performersOf(t, f.faceFixture); !slices.Equal(got, []int64{f.provisional, f.near}) || len(moved.Restore) != 1 {
		t.Fatalf("after the move: performers %v, restore %+v; want Named's credit taken", got, moved.Restore)
	}

	if _, err := f.db.Credits().MoveFaceAppearances(ctx, f.near, []int64{face}, f.named, moved.Restore); err != nil {
		t.Fatal(err)
	}
	if got := performersOf(t, f.faceFixture); !slices.Equal(got, []int64{f.named, f.provisional, f.near}) {
		t.Errorf("after the undo: performers %v, want Named back first and Near kept", got)
	}
	if got := faceAppearances(t, f.db)[0].personID; got != f.named {
		t.Errorf("the face belongs to %d, want Named %d", got, f.named)
	}
}

// The people closest to the faces come first, the person themself is left out, and each carries their crops, newest
// video first and a face whose video is gone last.
func TestSimilarPeopleRanksByTheClosestFace(t *testing.T) {
	f, crop := newSplitFixture(t)
	ctx := context.Background()
	appearances := faceAppearances(t, f.db)
	gone := insertFaceAppearance(t, f.db, "key-gone", f.named, false, `[[0, 10, 0.5, 0.5, 0.1]]`, crop)

	got, err := f.db.Credits().SimilarPeople(ctx, f.provisional, nil, 3)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]int64, 0, len(got))
	for _, p := range got {
		ids = append(ids, p.Person.ID)
	}
	if want := []int64{f.nearer, f.near, f.named}; !slices.Equal(ids, want) {
		t.Fatalf("people = %v, want %v", ids, want)
	}
	if named := got[2]; named.FaceCount != 2 || !slices.Equal(named.SampleAppearanceIDs, []int64{appearances[0].id, gone}) ||
		named.FirstSeenTitle == "" {
		t.Errorf("Named = %+v, want two faces, the one in a video first, and a first video", named)
	}
	if got, err := f.db.Credits().SimilarPeople(ctx, f.provisional, []int64{appearances[0].id}, 3); err != nil || len(got) != 0 {
		t.Errorf("with another person's face only = %+v, %v; want none", got, err)
	}
}
