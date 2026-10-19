package store

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// mergeFixture has one provisional person linked by face as performer of two videos, offered with a near person,
// with a height and the only face, and named in a smart folder rule; the target is a named person without a face.
type mergeFixture struct {
	db                                  *DB
	videoA, videoB                      int64
	target, provisional, near, folderID int64
	face                                []byte
}

const (
	mergeKeyA = "key-a"
	mergeKeyB = "key-b"
)

func newMergeFixture(t *testing.T, targetOnA bool) mergeFixture {
	t.Helper()
	ctx := context.Background()
	db := migratedDB(t)
	pathA, pathB := fixturePath("/media/a.mp4"), fixturePath("/media/b.mp4")
	ids := upsertAll(t, db, listingFile(pathA, "a", mergeKeyA, 1), listingFile(pathB, "b", mergeKeyB, 2))
	f := mergeFixture{db: db, videoA: ids[pathA], videoB: ids[pathB]}
	f.target = createPerson(t, db, "Target").ID
	f.near = createPerson(t, db, "Near").ID
	setFaceVector(t, db, f.near, faceNear(2, 3, 0.4))
	if targetOnA {
		if _, err := db.WorkInfo().Set(ctx, f.videoA, domain.WorkInfo{People: []domain.WorkPerson{performer(f.target)}}); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := db.Faces().QueueLibrary(ctx, domain.FaceQueueAll, domain.ActivityByOwner); err != nil || n != 2 {
		t.Fatalf("QueueLibrary = %d, %v", n, err)
	}
	// The first video creates the provisional person; the second links the same face to it.
	for range 2 {
		job, err := db.Faces().Claim(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Faces().Store(ctx, domain.FaceAnalysis{
			ContentKey: job.ContentKey, Recognizer: domain.FaceRecognizerSFaceInt8,
			People: []domain.FaceAnalysedPerson{facePerson(0.5, 0, faceBasis(2), "crop-"+job.ContentKey)},
		}); err != nil {
			t.Fatal(err)
		}
	}
	f.provisional = faceAppearances(t, db)[0].personID
	height := domain.PersonProfilePatch{Fields: []domain.PersonProfileField{domain.ProfileHeightCm},
		Values: domain.PersonProfile{HeightCm: intPtr(160)}}
	if _, err := db.Credits().Update(ctx, domain.CreditPerson, f.provisional, domain.CreditPatch{Profile: &height}); err != nil {
		t.Fatal(err)
	}
	folder, err := db.SmartFolders().CreateSmartFolder(ctx,
		domain.SmartFolderCreate{Name: "Face", Rule: domain.SmartRule{Person: f.provisional}})
	if err != nil {
		t.Fatal(err)
	}
	f.folderID = folder.ID
	image, err := db.Credits().PersonImage(ctx, f.provisional)
	if err != nil {
		t.Fatal(err)
	}
	f.face = image.JPEG
	return f
}

// merge merges the provisional person into the target and returns the face_merge entry.
func (f mergeFixture) merge(t *testing.T) domain.ActivityEntry {
	t.Helper()
	if _, err := f.db.Faces().MergeProvisional(context.Background(), f.provisional, f.target); err != nil {
		t.Fatal(err)
	}
	entries := entriesOfKind(t, f.db, domain.ActivityFaceMerge)
	if len(entries) != 1 {
		t.Fatalf("face_merge entries = %+v, want one", entries)
	}
	return entries[0]
}

func (f mergeFixture) undo(t *testing.T, entry domain.ActivityEntry) {
	t.Helper()
	undo, err := f.db.Activity().UndoFaceMerge(context.Background(), entry.ID)
	if err != nil || undo.Outcome != domain.ActivityCompleted {
		t.Fatalf("UndoFaceMerge = %+v, %v; want completed", undo, err)
	}
}

func (f mergeFixture) setPerformers(t *testing.T, videoID int64, ids ...int64) {
	t.Helper()
	info := domain.WorkInfo{}
	for _, id := range ids {
		info.People = append(info.People, performer(id))
	}
	if _, err := f.db.WorkInfo().SetDetails(context.Background(), videoID, "", info); err != nil {
		t.Fatal(err)
	}
}

func TestUndoFaceMergeRestoresTheProvisionalPerson(t *testing.T) {
	f := newMergeFixture(t, true)
	ctx := context.Background()
	entry := f.merge(t)
	if entry.Subject.Kind != domain.ActivitySubjectPerson || entry.Subject.Label != "Target" ||
		entry.Decider != domain.ActivityByRule || !entry.UndoOffered() {
		t.Fatalf("entry = %+v, want an undoable person entry by rule", entry)
	}
	if got := linkedPeople(t, f.db, mergeKeyB, "performer"); !slices.Equal(got, []int64{f.target}) {
		t.Fatalf("performers of B after the merge = %v, want the target", got)
	}

	f.undo(t, entry)

	person, err := f.db.Credits().Get(ctx, domain.CreditPerson, f.provisional)
	if err != nil || !person.Provisional || person.Profile.HeightCm == nil || *person.Profile.HeightCm != 160 {
		t.Fatalf("restored person = %+v, %v; want provisional with its height", person, err)
	}
	// Acceptance 5: both performer rows come back; the performer the target had before the merge stays.
	if got := linkedPeople(t, f.db, mergeKeyA, "performer"); !slices.Equal(got, []int64{f.target, f.provisional}) {
		t.Errorf("performers of A = %v, want [%d %d]", got, f.target, f.provisional)
	}
	if got := linkedPeople(t, f.db, mergeKeyB, "performer"); !slices.Equal(got, []int64{f.provisional}) {
		t.Errorf("performers of B = %v, want [%d]", got, f.provisional)
	}
	for _, a := range faceAppearances(t, f.db) {
		if a.personID != f.provisional {
			t.Errorf("appearance %d person = %d, want %d", a.id, a.personID, f.provisional)
		}
	}
	if got, want := faceCandidatePairs(t, f.db), [][2]int64{{f.provisional, f.near}}; !slices.Equal(got, want) {
		t.Errorf("candidates = %v, want %v", got, want)
	}
	folder, err := f.db.SmartFolders().SmartFolder(ctx, f.folderID)
	if err != nil || folder.Rule.Person != f.provisional {
		t.Errorf("smart folder rule person = %d, %v; want %d", folder.Rule.Person, err, f.provisional)
	}
	// The face and the height the merge gave the target go back with the person.
	if image, err := f.db.Credits().PersonImage(ctx, f.provisional); err != nil || string(image.JPEG) != string(f.face) {
		t.Errorf("face of the restored person = %q, %v; want %q", image.JPEG, err, f.face)
	}
	if _, err := f.db.Credits().PersonImage(ctx, f.target); !errors.Is(err, domain.ErrPersonImageNotFound) {
		t.Errorf("face of the target = %v, want none", err)
	}
	if target, _ := f.db.Credits().Get(ctx, domain.CreditPerson, f.target); target.Profile.HeightCm != nil {
		t.Errorf("target height = %d, want none", *target.Profile.HeightCm)
	}
	if _, err := f.db.Activity().UndoFaceMerge(ctx, entry.ID); !errors.Is(err, domain.ErrActivityNotUndoable) {
		t.Errorf("second undo = %v, want ErrActivityNotUndoable", err)
	}
}

// A person whose face was removed keeps its revision counter through merge and undo, so the next face gets a new ETag.
func TestUndoFaceMergeKeepsTheImageRevisionOfAPersonWithoutAFace(t *testing.T) {
	f := newMergeFixture(t, false)
	ctx := context.Background()
	if _, err := f.db.Credits().DeletePersonImage(ctx, f.provisional); err != nil {
		t.Fatal(err)
	}
	var used int64
	if err := f.db.sql.QueryRowContext(ctx, `select image_revision from people where id = ?`, f.provisional).
		Scan(&used); err != nil {
		t.Fatal(err)
	}

	f.undo(t, f.merge(t))

	person, err := f.db.Credits().SetPersonImage(ctx, f.provisional, domain.PersonImage{JPEG: f.face, Width: 1, Height: 1})
	if err != nil || person.ImageRevision <= used {
		t.Errorf("revision of the next face = %d, %v; want above %d", person.ImageRevision, err, used)
	}
}

func TestUndoFaceMergeKeepsTheOwnersLaterEdits(t *testing.T) {
	f := newMergeFixture(t, false)
	ctx := context.Background()
	entry := f.merge(t)
	renamed := "Renamed"
	if _, err := f.db.Credits().Update(ctx, domain.CreditPerson, f.target, domain.CreditPatch{Name: &renamed}); err != nil {
		t.Fatal(err)
	}
	f.setPerformers(t, f.videoA)
	// Removed and added again in the details editor: the credit is now the owner's.
	f.setPerformers(t, f.videoB)
	f.setPerformers(t, f.videoB, f.target)

	f.undo(t, entry)

	if target, err := f.db.Credits().Get(ctx, domain.CreditPerson, f.target); err != nil || target.Name != renamed {
		t.Errorf("target = %+v, %v; want the name %q", target, err, renamed)
	}
	if got := linkedPeople(t, f.db, mergeKeyA, "performer"); len(got) != 0 {
		t.Errorf("performers of A = %v, want none", got)
	}
	if got := linkedPeople(t, f.db, mergeKeyB, "performer"); !slices.Equal(got, []int64{f.target}) {
		t.Errorf("performers of B = %v, want [%d]", got, f.target)
	}
}
