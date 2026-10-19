package store

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

var testFace = domain.PersonImage{JPEG: []byte("face"), Width: 2, Height: 2}

// setFace gives the person a face image and returns its face-image job.
func setFace(t *testing.T, db *DB, personID int64) domain.FaceImageJob {
	t.Helper()
	ctx := context.Background()
	if _, err := db.Credits().SetPersonImage(ctx, personID, testFace); err != nil {
		t.Fatal(err)
	}
	job, err := db.Faces().FaceImage(ctx, personID)
	if err != nil {
		t.Fatal(err)
	}
	return job
}

// provisionalPerson inserts a person without names whose face-image vector is v.
func provisionalPerson(t *testing.T, db *DB, v []float32) int64 {
	t.Helper()
	res, err := db.sql.Exec(`insert into people (created_at) values (0)`)
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	setFaceVector(t, db, id, v)
	return id
}

func storedFaceVectors(t *testing.T, db *DB) map[int64]string {
	t.Helper()
	rows, err := db.sql.Query(`select person_id, recognizer from person_face_vectors`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	out := map[int64]string{}
	for rows.Next() {
		var id int64
		var recognizer string
		if err := rows.Scan(&id, &recognizer); err != nil {
			t.Fatal(err)
		}
		out[id] = recognizer
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// Acceptance 4: a named person's face that reaches one provisional person merges it, so both of its videos list the
// named person.
func TestAFaceMergesTheOnlyProvisionalPersonItReaches(t *testing.T) {
	f := newMergeFixture(t, false)
	ctx := context.Background()
	job := setFace(t, f.db, f.target)

	decision, err := f.db.Faces().StoreFaceImageVector(ctx, job, faceBasis(2))
	if err != nil || decision.MergeID != f.provisional {
		t.Fatalf("StoreFaceImageVector = %+v, %v; want a merge of %d", decision, err, f.provisional)
	}
	for _, key := range []string{mergeKeyA, mergeKeyB} {
		if got := linkedPeople(t, f.db, key, "performer"); !slices.Equal(got, []int64{f.target}) {
			t.Errorf("performers of %s = %v, want the target", key, got)
		}
	}
	if entries := entriesOfKind(t, f.db, domain.ActivityFaceMerge); len(entries) != 1 {
		t.Errorf("face_merge entries = %+v, want one", entries)
	}
	if got := storedFaceVectors(t, f.db); got[f.target] != string(domain.FaceRecognizerSFaceInt8) {
		t.Errorf("vectors = %v, want the target's", got)
	}
}

func TestAFaceOffersProvisionalPeopleItCannotTellApart(t *testing.T) {
	ctx := context.Background()
	db := migratedDB(t)
	named := createPerson(t, db, "Named").ID
	first, second := provisionalPerson(t, db, faceNear(0, 1, 0.8)), provisionalPerson(t, db, faceNear(0, 2, 0.8))
	job := setFace(t, db, named)

	decision, err := db.Faces().StoreFaceImageVector(ctx, job, faceBasis(0))
	if err != nil || decision.MergeID != 0 {
		t.Fatalf("StoreFaceImageVector = %+v, %v; want no merge", decision, err)
	}
	if got, want := faceCandidatePairs(t, db), [][2]int64{{first, named}, {second, named}}; !slices.Equal(got, want) {
		t.Fatalf("candidates = %v, want %v", got, want)
	}

	// A pair the owner dismissed neither merges nor counts, so the face, applied again, merges the other one.
	if err := db.Credits().DismissFaceCandidate(ctx, named, first); err != nil {
		t.Fatal(err)
	}
	job, err = db.Faces().FaceImage(ctx, named)
	if err != nil || !job.Embedded {
		t.Fatalf("FaceImage = %+v, %v; want the stored vector", job, err)
	}
	decision, err = db.Faces().StoreFaceImageVector(ctx, job, job.Vector)
	if err != nil || decision.MergeID != second {
		t.Fatalf("StoreFaceImageVector after the dismissal = %+v, %v; want a merge of %d", decision, err, second)
	}
}

func TestAProvisionalPersonsOwnFaceMergesNobody(t *testing.T) {
	ctx := context.Background()
	db := migratedDB(t)
	other := provisionalPerson(t, db, faceBasis(0))
	crop := provisionalPerson(t, db, faceBasis(5))
	job := setFace(t, db, crop)

	decision, err := db.Faces().StoreFaceImageVector(ctx, job, faceBasis(0))
	if err != nil || decision.MergeID != 0 {
		t.Fatalf("StoreFaceImageVector = %+v, %v; want no merge", decision, err)
	}
	if _, err := db.Credits().Get(ctx, domain.CreditPerson, other); err != nil {
		t.Fatalf("the other provisional person = %v, want kept", err)
	}
	if got, want := faceCandidatePairs(t, db), [][2]int64{{other, crop}}; !slices.Equal(got, want) {
		t.Fatalf("candidates = %v, want %v", got, want)
	}
}

// A vector is written only for the face and the recognizer that are current when it is stored.
func TestAFaceVectorIsStoredOnlyForTheCurrentFaceAndGeneration(t *testing.T) {
	ctx := context.Background()
	db := migratedDB(t)
	named := createPerson(t, db, "Named").ID
	job := setFace(t, db, named)
	if stale, err := db.Faces().StaleFaceImage(ctx); err != nil || stale.PersonID != named || stale.Embedded {
		t.Fatalf("StaleFaceImage = %+v, %v; want the new face", stale, err)
	}

	replaced := setFace(t, db, named)
	if _, err := db.Faces().StoreFaceImageVector(ctx, job, faceBasis(0)); err != nil {
		t.Fatalf("vector of a replaced face: %v", err)
	}
	if got := storedFaceVectors(t, db); len(got) != 0 {
		t.Fatalf("vectors after the face was replaced = %v, want none", got)
	}

	if err := db.Settings().SaveFaceSettings(ctx, domain.FaceSettings{
		Enabled: true, Recognizer: domain.FaceRecognizerArcFaceR50,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Faces().StoreFaceImageVector(ctx, replaced, faceBasis(0)); !errors.Is(err, domain.ErrFaceGenerationChanged) {
		t.Fatalf("vector of an older generation: %v, want ErrFaceGenerationChanged", err)
	}
	if got := storedFaceVectors(t, db); len(got) != 0 {
		t.Fatalf("vectors after the switch = %v, want none", got)
	}

	current, err := db.Faces().StaleFaceImage(ctx)
	if err != nil || current.Recognizer != domain.FaceRecognizerArcFaceR50 || current.Generation != 1 {
		t.Fatalf("StaleFaceImage = %+v, %v; want the face under ArcFace and generation 1", current, err)
	}
	if _, err := db.Faces().StoreFaceImageVector(ctx, current, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Faces().StaleFaceImage(ctx); !errors.Is(err, domain.ErrNoJob) {
		t.Fatalf("StaleFaceImage after a face without a good face = %v, want ErrNoJob", err)
	}
}

// committedFaceRecorder checks through the read pool, which sees only committed rows, that each face event follows
// its commit.
type committedFaceRecorder struct {
	t      *testing.T
	db     *DB
	people []int64
}

func (r *committedFaceRecorder) Publish(events ...domain.Event) {
	for _, e := range events {
		changed, ok := e.(domain.PersonImageChanged)
		if !ok {
			continue
		}
		var faces int
		if err := r.db.read.QueryRow(`select count(*) from person_images where person_id = ?`, changed.PersonID).
			Scan(&faces); err != nil || faces != 1 {
			r.t.Errorf("face of person %d at its event = %d, %v; want committed", changed.PersonID, faces, err)
		}
		r.people = append(r.people, changed.PersonID)
	}
}

func (r *committedFaceRecorder) take() []int64 {
	people := r.people
	r.people = nil
	return people
}

func TestFaceChangesArePublishedAfterCommit(t *testing.T) {
	ctx := context.Background()
	db := migratedDB(t)
	recorder := &committedFaceRecorder{t: t, db: db}
	db.PublishTo(recorder)
	named := createPerson(t, db, "Named").ID
	faceless := createPerson(t, db, "Faceless").ID
	provisional := provisionalPerson(t, db, faceBasis(0))

	setFace(t, db, named)
	if got := recorder.take(); !slices.Equal(got, []int64{named}) {
		t.Fatalf("events of a set face = %v, want [%d]", got, named)
	}
	if _, err := db.Credits().SetPersonImage(ctx, named+100, testFace); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("SetPersonImage of a missing person = %v", err)
	}
	if _, err := db.Credits().DeletePersonImage(ctx, named); err != nil {
		t.Fatal(err)
	}
	if got := recorder.take(); len(got) != 0 {
		t.Fatalf("events of a refused write and a removed face = %v, want none", got)
	}

	setFace(t, db, provisional)
	recorder.take()
	name := "Named later"
	if _, err := db.Credits().Update(ctx, domain.CreditPerson, provisional, domain.CreditPatch{Name: &name}); err != nil {
		t.Fatal(err)
	}
	if got := recorder.take(); !slices.Equal(got, []int64{provisional}) {
		t.Fatalf("events of naming a provisional person = %v, want [%d]", got, provisional)
	}

	if _, err := db.Credits().Merge(ctx, domain.CreditPerson, faceless, []int64{provisional}); err != nil {
		t.Fatal(err)
	}
	if got := recorder.take(); !slices.Equal(got, []int64{faceless}) {
		t.Fatalf("events of a merge that brought a face = %v, want [%d]", got, faceless)
	}
}
