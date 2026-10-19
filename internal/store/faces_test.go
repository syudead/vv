package store

import (
	"context"
	"errors"
	"math"
	"slices"
	"strconv"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

const faceDims = 128 // sface_int8

// faceBasis is a unit vector along axis i: equal axes have similarity 1, different ones 0.
func faceBasis(i int) []float32 {
	v := make([]float32, faceDims)
	v[i] = 1
	return v
}

// faceNear is a unit vector with similarity s to faceBasis(i).
func faceNear(i, other int, s float64) []float32 {
	v := make([]float32, faceDims)
	v[i], v[other] = float32(s), float32(math.Sqrt(1-s*s))
	return v
}

// facePerson is a main person on screen at x for two tracks; shift moves the tracks in time, as a re-analysis may.
func facePerson(x float64, shift float64, centroid []float32, crop string) domain.FaceAnalysedPerson {
	return domain.FaceAnalysedPerson{
		Spans: []domain.FaceSpan{
			{Start: 0 + shift, End: 10 + shift, X: x, Y: 0.5, W: 0.1},
			{Start: 20 + shift, End: 30 + shift, X: x, Y: 0.5, W: 0.1},
		},
		GoodSeconds: 12, Centroid: centroid,
		Crop: domain.PersonImage{JPEG: []byte(crop), Width: 2, Height: 2},
	}
}

type faceFixture struct {
	db      *DB
	videoID int64
	key     string
}

func newFaceFixture(t *testing.T) faceFixture {
	t.Helper()
	db := migratedDB(t)
	path := fixturePath("/media/a.mp4")
	ids := upsertAll(t, db, listingFile(path, "a", "key-a", 1))
	return faceFixture{db: db, videoID: ids[path], key: "key-a"}
}

// analyse queues the video again, claims it and stores the people as one result of the current generation.
func (f faceFixture) analyse(t *testing.T, people ...domain.FaceAnalysedPerson) {
	t.Helper()
	ctx := context.Background()
	f.claim(t)
	if err := f.db.Faces().Store(ctx, domain.FaceAnalysis{
		ContentKey: f.key, Recognizer: domain.FaceRecognizerSFaceInt8, People: people,
	}); err != nil {
		t.Fatal(err)
	}
}

func (f faceFixture) claim(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	if n, err := f.db.Faces().QueueLibrary(ctx, domain.FaceQueueAll, domain.ActivityByOwner); err != nil || n != 1 {
		t.Fatalf("QueueLibrary = %d, %v", n, err)
	}
	job, err := f.db.Faces().Claim(ctx)
	if err != nil || job.ContentKey != f.key {
		t.Fatalf("Claim = %+v, %v", job, err)
	}
}

// setFaceVector gives the person a current face-image vector.
func setFaceVector(t *testing.T, db *DB, personID int64, v []float32) {
	t.Helper()
	if _, err := db.sql.Exec(`insert into person_face_vectors (person_id, image_revision, recognizer, vector)
		values (?, (select image_revision from people where id = ?), 'sface_int8', ?)`,
		personID, personID, encodeFaceVector(v)); err != nil {
		t.Fatal(err)
	}
}

type storedAppearance struct {
	id, personID        int64
	dropped             bool
	centroid, crop      []byte
	trackCount          int
	firstStart, lastEnd float64
}

func faceAppearances(t *testing.T, db *DB) []storedAppearance {
	t.Helper()
	rows, err := db.sql.Query(`select id, coalesce(person_id, 0), dropped, centroid, crop, track_count, tracks
		from face_appearances order by id`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []storedAppearance
	for rows.Next() {
		var a storedAppearance
		var tracks string
		if err := rows.Scan(&a.id, &a.personID, &a.dropped, &a.centroid, &a.crop, &a.trackCount, &tracks); err != nil {
			t.Fatal(err)
		}
		spans, err := decodeFaceTracks(tracks)
		if err != nil || len(spans) == 0 {
			t.Fatalf("tracks %q: %v", tracks, err)
		}
		a.firstStart, a.lastEnd = spans[0].Start, spans[len(spans)-1].End
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

type faceLinkRow struct {
	appearanceID, personID int64
	created                bool
}

func faceLinksOf(t *testing.T, db *DB, entryID int64) []faceLinkRow {
	t.Helper()
	rows, err := db.sql.Query(`select appearance_id, person_id, created_person from activity_entry_face_links
		where entry_id = ? order by appearance_id`, entryID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []faceLinkRow
	for rows.Next() {
		var l faceLinkRow
		if err := rows.Scan(&l.appearanceID, &l.personID, &l.created); err != nil {
			t.Fatal(err)
		}
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func faceCandidatePairs(t *testing.T, db *DB) [][2]int64 {
	t.Helper()
	rows, err := db.sql.Query(`select provisional_id, person_id from face_candidates order by provisional_id, person_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out [][2]int64
	for rows.Next() {
		var p [2]int64
		if err := rows.Scan(&p[0], &p[1]); err != nil {
			t.Fatal(err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// linkedFixture analyses a video with two main people: one matches the named person, the other is new and close to
// two other named people, so it becomes a provisional person offered with both.
type linkedFixture struct {
	faceFixture
	named, near, nearer, provisional int64
}

func newLinkedFixture(t *testing.T) linkedFixture {
	t.Helper()
	f := linkedFixture{faceFixture: newFaceFixture(t)}
	f.named = createPerson(t, f.db, "Named").ID
	f.near = createPerson(t, f.db, "Near").ID
	f.nearer = createPerson(t, f.db, "Nearer").ID
	setFaceVector(t, f.db, f.named, faceBasis(0))
	setFaceVector(t, f.db, f.near, faceNear(2, 3, 0.4))
	setFaceVector(t, f.db, f.nearer, faceNear(2, 4, 0.42))
	f.analyse(t, facePerson(0.25, 0, faceBasis(0), "crop-named"), facePerson(0.75, 0, faceBasis(2), "crop-new"))
	appearances := faceAppearances(t, f.db)
	if len(appearances) != 2 {
		t.Fatalf("appearances = %+v, want 2", appearances)
	}
	f.provisional = appearances[1].personID
	return f
}

func TestStoreFaceAnalysisLinksPeopleAndCreatesAProvisionalPerson(t *testing.T) {
	f := newLinkedFixture(t)
	ctx := context.Background()

	appearances := faceAppearances(t, f.db)
	if appearances[0].personID != f.named || appearances[1].personID == 0 ||
		slices.Contains([]int64{f.named, f.near, f.nearer}, appearances[1].personID) {
		t.Fatalf("appearances = %+v, want the named person and a new one", appearances)
	}
	provisional, err := f.db.Credits().Get(ctx, domain.CreditPerson, f.provisional)
	if err != nil || !provisional.Provisional {
		t.Fatalf("new person = %+v, %v; want provisional", provisional, err)
	}
	image, err := f.db.Credits().PersonImage(ctx, f.provisional)
	if err != nil || string(image.JPEG) != "crop-new" || string(appearances[1].crop) != "crop-new" {
		t.Errorf("face of the new person = %q, %v; appearance crop %q; want crop-new", image.JPEG, err,
			appearances[1].crop)
	}
	if got := linkedPeople(t, f.db, f.key, "performer"); !slices.Equal(got, []int64{f.named, f.provisional}) {
		t.Errorf("performers = %v, want [%d %d]", got, f.named, f.provisional)
	}
	if got, want := faceCandidatePairs(t, f.db), [][2]int64{{f.provisional, f.near}, {f.provisional, f.nearer}}; !slices.Equal(got, want) {
		t.Errorf("candidates = %v, want %v", got, want)
	}

	entry := entriesOfKind(t, f.db, domain.ActivityFaceAnalysis)[0]
	if entry.Feature != domain.ActivityFeatureFaceRecognition || entryState(entry) != activityDone ||
		entry.Decider != domain.ActivityByRule {
		t.Errorf("entry = %+v, want face_recognition done by rule", entry)
	}
	want := []faceLinkRow{
		{appearanceID: appearances[0].id, personID: f.named},
		{appearanceID: appearances[1].id, personID: f.provisional, created: true},
	}
	if got := faceLinksOf(t, f.db, entry.ID); !slices.Equal(got, want) {
		t.Errorf("links = %+v, want %+v", got, want)
	}
}

func TestStoreFaceAnalysisOffersTwoNewPeopleOfOneVideoToEachOther(t *testing.T) {
	// SFace's candidate threshold is 0.363: 0.6 reaches it, an orthogonal vector does not.
	f := newFaceFixture(t)
	f.analyse(t,
		facePerson(0.25, 0, faceBasis(0), "crop-a"),
		facePerson(0.5, 0, faceNear(0, 1, 0.6), "crop-b"),
		facePerson(0.75, 0, faceBasis(5), "crop-c"))

	appearances := faceAppearances(t, f.db)
	if len(appearances) != 3 {
		t.Fatalf("appearances = %+v, want 3", appearances)
	}
	a, b := appearances[0].personID, appearances[1].personID
	if got, want := faceCandidatePairs(t, f.db), [][2]int64{{b, a}}; !slices.Equal(got, want) {
		t.Errorf("candidates = %v, want only the close pair %v", got, want)
	}
}

func TestStoreFaceAnalysisAgainKeepsPeopleIDsAndCrops(t *testing.T) {
	f := newLinkedFixture(t)
	before := faceAppearances(t, f.db)
	people := personCount(t, f.db)

	// The same faces in the other order, a second later, with new crops.
	f.analyse(t, facePerson(0.75, 1, faceBasis(2), "crop-2"), facePerson(0.25, 1, faceBasis(0), "crop-1"))

	after := faceAppearances(t, f.db)
	if len(after) != 2 || personCount(t, f.db) != people {
		t.Fatalf("appearances = %+v, people = %d; want 2 and %d", after, personCount(t, f.db), people)
	}
	for i := range after {
		if after[i].id != before[i].id || after[i].personID != before[i].personID ||
			string(after[i].crop) != string(before[i].crop) || after[i].firstStart != 1 {
			t.Errorf("appearance %d = %+v, want id, person and crop of %+v with the new tracks", i, after[i], before[i])
		}
	}
	if links := faceLinksOf(t, f.db, entriesOfKind(t, f.db, domain.ActivityFaceAnalysis)[0].ID); len(links) != 0 {
		t.Errorf("links of the second run = %+v, want none", links)
	}
}

func TestStoreFaceAnalysisRefusesAnOlderGeneration(t *testing.T) {
	f := newFaceFixture(t)
	ctx := context.Background()
	f.claim(t)
	if err := writeSetting(ctx, f.db.sql, faceGenerationKey, strconv.Itoa(1)); err != nil {
		t.Fatal(err)
	}
	people := personCount(t, f.db)

	err := f.db.Faces().Store(ctx, domain.FaceAnalysis{
		ContentKey: f.key, Recognizer: domain.FaceRecognizerSFaceInt8, Generation: 0,
		People: []domain.FaceAnalysedPerson{facePerson(0.5, 0, faceBasis(0), "crop")},
	})
	if !errors.Is(err, domain.ErrFaceGenerationChanged) {
		t.Fatalf("Store = %v, want ErrFaceGenerationChanged", err)
	}
	if got := faceAppearances(t, f.db); len(got) != 0 || personCount(t, f.db) != people ||
		len(linkedPeople(t, f.db, f.key, "performer")) != 0 {
		t.Errorf("after a refused result: appearances %+v, people %d; want nothing written", got, personCount(t, f.db))
	}
}

func TestRemovingAFaceLinkedPerformerDropsTheAppearance(t *testing.T) {
	f := newLinkedFixture(t)
	ctx := context.Background()

	info, _ := workInfoOf(t, f.db, f.videoID)
	info.People = []domain.WorkPerson{performer(f.provisional)}
	if _, err := f.db.WorkInfo().SetDetails(ctx, f.videoID, "", info); err != nil {
		t.Fatal(err)
	}

	appearances := faceAppearances(t, f.db)
	if !appearances[0].dropped || appearances[1].dropped {
		t.Errorf("dropped = %v, %v; want only the removed performer's appearance", appearances[0].dropped,
			appearances[1].dropped)
	}
}

func TestDeletingAProvisionalPersonDeletesItsFaceData(t *testing.T) {
	f := newLinkedFixture(t)
	ctx := context.Background()
	if err := f.db.Credits().Delete(ctx, domain.CreditPerson, f.provisional); err != nil {
		t.Fatal(err)
	}

	check := func(when string) {
		t.Helper()
		a := faceAppearances(t, f.db)[1]
		if a.personID != 0 || !a.dropped || a.centroid != nil || a.crop != nil || a.trackCount != 2 {
			t.Errorf("%s: appearance = %+v, want dropped tracks without person, centroid or crop", when, a)
		}
	}
	check("after the delete")
	if rows := tableRows(t, f.db, "person_images"); len(rows) != 0 {
		t.Errorf("faces after the delete = %v, want none", rows)
	}

	// Acceptance 7: the same face found again creates nobody and gets no centroid or crop back.
	people := personCount(t, f.db)
	f.analyse(t, facePerson(0.25, 0, faceBasis(0), "crop-1"), facePerson(0.75, 0, faceBasis(2), "crop-2"))
	check("after a re-analysis")
	if personCount(t, f.db) != people {
		t.Errorf("people = %d, want %d", personCount(t, f.db), people)
	}
	if got := linkedPeople(t, f.db, f.key, "performer"); !slices.Equal(got, []int64{f.named}) {
		t.Errorf("performers = %v, want [%d]", got, f.named)
	}
}

func TestMergingMovesFaceAppearancesAndCandidates(t *testing.T) {
	f := newLinkedFixture(t)
	ctx := context.Background()
	if _, err := f.db.Credits().Merge(ctx, domain.CreditPerson, f.near, []int64{f.provisional}); err != nil {
		t.Fatal(err)
	}

	if got := faceAppearances(t, f.db)[1].personID; got != f.near {
		t.Errorf("appearance person = %d, want the target %d", got, f.near)
	}
	// The pair with the target itself is gone; the pair with the other person now starts from the target.
	if got, want := faceCandidatePairs(t, f.db), [][2]int64{{f.near, f.nearer}}; !slices.Equal(got, want) {
		t.Errorf("candidates = %v, want %v", got, want)
	}
}

func TestAfterImportFaceAnalysisJoinsTheImportPipeline(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(strconv.FormatBool(enabled), func(t *testing.T) {
			db := migratedDB(t)
			if err := writeSetting(context.Background(), db.sql, faceEnabledKey, strconv.FormatBool(enabled)); err != nil {
				t.Fatal(err)
			}
			importThroughSeek(t, db, "clip", false)

			entries := entriesOfKind(t, db, domain.ActivityFaceAnalysis)
			if !enabled {
				if len(entries) != 0 {
					t.Errorf("face_analysis entries = %+v, want none while off", entries)
				}
				return
			}
			if len(entries) != 1 || entryState(entries[0]) != activityWaiting {
				t.Fatalf("face_analysis entries = %+v, want one waiting", entries)
			}
			pipeline := onlyPipeline(t, db, importEntries(t, db)[0].ID)
			if got := onlyPipeline(t, db, entries[0].ID); got.ID != pipeline.ID {
				t.Errorf("face_analysis pipeline = %d, want the import pipeline %d", got.ID, pipeline.ID)
			}
		})
	}
}
