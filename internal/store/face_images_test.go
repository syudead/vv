package store

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strconv"
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

func TestAFaceMergeRefusedForGoodStoresTheVectorWithoutIt(t *testing.T) {
	f := newMergeFixture(t, false)
	ctx := context.Background()
	own := []domain.ExternalID{{Source: "fanza", Value: "provisional"}}
	if _, err := f.db.Credits().Update(ctx, domain.CreditPerson, f.provisional, domain.CreditPatch{ExternalIDs: &own}); err != nil {
		t.Fatal(err)
	}
	// The target is at the limit, so taking the provisional person's id would exceed it.
	full := make([]domain.ExternalID, domain.ExternalIDsMaxCount)
	for i := range full {
		full[i] = domain.ExternalID{Source: "fanza", Value: "full-" + strconv.Itoa(i)}
	}
	if _, err := f.db.Credits().Update(ctx, domain.CreditPerson, f.target, domain.CreditPatch{ExternalIDs: &full}); err != nil {
		t.Fatal(err)
	}
	job := setFace(t, f.db, f.target)

	decision, err := f.db.Faces().StoreFaceImageVector(ctx, job, faceBasis(2))
	if decision.MergeID != 0 || !errors.Is(err, domain.ErrFaceMergeRefused) {
		t.Fatalf("StoreFaceImageVector = %+v, %v; want a refusal without a merge", decision, err)
	}
	// The stored vector keeps the face from being embedded, and refused, again.
	if got := storedFaceVectors(t, f.db); got[f.target] != string(domain.FaceRecognizerSFaceInt8) {
		t.Errorf("vectors = %v, want the target's", got)
	}
	if entries := entriesOfKind(t, f.db, domain.ActivityFaceMerge); len(entries) != 0 {
		t.Errorf("face_merge entries = %+v, want none", entries)
	}
	person, err := f.db.Credits().Get(ctx, domain.CreditPerson, f.provisional)
	if err != nil || !slices.Equal(person.ExternalIDs, own) {
		t.Errorf("provisional person = %+v, %v; want it kept with its external id", person, err)
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

// A vector is written only for the face and the recognizer that are current when it is stored; a vector of a replaced
// face is refused, so its job reads the face again.
func TestAFaceVectorIsStoredOnlyForTheCurrentFaceAndGeneration(t *testing.T) {
	ctx := context.Background()
	db := migratedDB(t)
	named := createPerson(t, db, "Named").ID
	job := setFace(t, db, named)
	if job.Embedded {
		t.Fatalf("FaceImage = %+v, want the new face without a vector", job)
	}

	replaced := setFace(t, db, named)
	if _, err := db.Faces().StoreFaceImageVector(ctx, job, faceBasis(0)); !errors.Is(err, domain.ErrFaceImageChanged) {
		t.Fatalf("vector of a replaced face: %v, want ErrFaceImageChanged", err)
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

	current, err := db.Faces().FaceImage(ctx, named)
	if err != nil || current.Recognizer != domain.FaceRecognizerArcFaceR50 || current.Generation != 1 {
		t.Fatalf("FaceImage = %+v, %v; want the face under ArcFace and generation 1", current, err)
	}
	if _, err := db.Faces().StoreFaceImageVector(ctx, current, nil); err != nil {
		t.Fatal(err)
	}
	if after, err := db.Faces().FaceImage(ctx, named); err != nil || !after.Embedded || after.Vector != nil {
		t.Fatalf("FaceImage after a face without a good face = %+v, %v; want it embedded without a vector", after, err)
	}
}

// takeFaceImageJobs returns the people of the queued face_image jobs with their class, and deletes the jobs.
func takeFaceImageJobs(t *testing.T, db *DB) [][]any {
	t.Helper()
	jobs := queryRows(t, db.sql, `select cast(subject_key as integer), priority from jobs
		where kind = 'face_image' and state = 'queued' order by id`, 2)
	execAll(t, db, `delete from jobs where kind = 'face_image'`)
	return jobs
}

// Each write that sets, replaces or brings a face, or names a provisional person with one, queues the person's
// face_image job in its own transaction, in the owner class.
func TestFaceChangesQueueAFaceImageJob(t *testing.T) {
	ctx := context.Background()
	db := migratedDB(t)
	named := createPerson(t, db, "Named").ID
	faceless := createPerson(t, db, "Faceless").ID
	provisional := provisionalPerson(t, db, faceBasis(0))
	owner := int64(domain.JobPriorityOwner)

	setFace(t, db, named)
	if got, want := takeFaceImageJobs(t, db), [][]any{{named, owner}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("jobs of a set face = %v, want %v", got, want)
	}
	if _, err := db.Credits().SetPersonImage(ctx, named+100, testFace); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("SetPersonImage of a missing person = %v", err)
	}
	if _, err := db.Credits().DeletePersonImage(ctx, named); err != nil {
		t.Fatal(err)
	}
	if got := takeFaceImageJobs(t, db); len(got) != 0 {
		t.Fatalf("jobs of a refused write and a removed face = %v, want none", got)
	}

	setFace(t, db, provisional)
	takeFaceImageJobs(t, db)
	name := "Named later"
	if _, err := db.Credits().Update(ctx, domain.CreditPerson, provisional, domain.CreditPatch{Name: &name}); err != nil {
		t.Fatal(err)
	}
	if got, want := takeFaceImageJobs(t, db), [][]any{{provisional, owner}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("jobs of naming a provisional person = %v, want %v", got, want)
	}

	if _, err := db.Credits().Merge(ctx, domain.CreditPerson, faceless, []int64{provisional}); err != nil {
		t.Fatal(err)
	}
	if got, want := takeFaceImageJobs(t, db), [][]any{{faceless, owner}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("jobs of a merge that brought a face = %v, want %v", got, want)
	}
}

// A face replaced while its face_image job runs may come after the run read the old face, so the job runs again
// after it ends, whether it succeeded or failed for good; without a request it ends.
func TestAFaceReplacedWhileItsJobRunsQueuesTheJobAgain(t *testing.T) {
	ctx := context.Background()
	db := migratedDB(t)
	person := createPerson(t, db, "Named").ID
	setFace(t, db, person)
	want := [][]any{{"queued", strconv.FormatInt(person, 10), int64(0), int64(domain.JobPriorityOwner)}}
	for _, end := range []struct {
		name string
		run  func(job domain.Job) error
	}{
		{"completed", func(job domain.Job) error { return db.Ingest().CompleteClaimedJob(ctx, job) }},
		{"failed for good", func(job domain.Job) error {
			return db.Ingest().FailClaimedJob(ctx, job, domain.FailureRepeats(errors.New("unreadable face")))
		}},
	} {
		job, err := db.Ingest().ClaimJob(ctx, domain.JobFaceImage)
		if err != nil {
			t.Fatal(err)
		}
		setFace(t, db, person)
		if err := end.run(job); err != nil {
			t.Fatal(err)
		}
		got := queryRows(t, db.sql, `select state, subject_key, attempts, priority from jobs where kind = 'face_image'`, 4)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("jobs after the run %s = %v, want %v", end.name, got, want)
		}
	}

	job, err := db.Ingest().ClaimJob(ctx, domain.JobFaceImage)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ingest().CompleteClaimedJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	if got := queryRows(t, db.sql, `select state from jobs where kind = 'face_image'`, 1); len(got) != 0 {
		t.Fatalf("jobs after a run without a request = %v, want none", got)
	}
}
