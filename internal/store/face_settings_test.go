package store

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// A switch of recognizer fences results of the old one; saving the same recognizer again must not.
func TestSaveFaceSettingsIncreasesTheGenerationOnlyOnARecognizerSwitch(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	saves := []struct {
		settings   domain.FaceSettings
		generation int64
	}{
		{domain.FaceSettings{Enabled: true, Recognizer: domain.FaceRecognizerSFaceInt8}, 0},
		{domain.FaceSettings{Enabled: true, Recognizer: domain.FaceRecognizerArcFaceR50}, 1},
		{domain.FaceSettings{Enabled: false, Recognizer: domain.FaceRecognizerArcFaceR50}, 1},
	}
	for _, save := range saves {
		if err := db.Settings().SaveFaceSettings(ctx, save.settings); err != nil {
			t.Fatal(err)
		}
		got, err := db.Settings().FaceSettings(ctx)
		if err != nil || got != save.settings {
			t.Fatalf("FaceSettings = %+v, %v; want %+v", got, err, save.settings)
		}
		if generation, err := faceGeneration(ctx, db.sql); err != nil || generation != save.generation {
			t.Fatalf("after saving %+v: generation = %d, %v; want %d", save.settings, generation, err, save.generation)
		}
	}
}

// A switch removes vectors the new recognizer's cannot be compared with and queues the analysed videos again; a
// video never analysed stays for "Analyse missing", and the next claim carries the new recognizer and generation.
func TestRecognizerSwitchClearsOldVectorsAndQueuesAnalysedVideosAgain(t *testing.T) {
	f := newFaceFixture(t)
	ctx := context.Background()
	named := createPerson(t, f.db, "Named").ID
	setFaceVector(t, f.db, named, faceBasis(1))
	f.analyse(t, facePerson(0.5, 0, faceBasis(0), "crop"))
	upsertAll(t, f.db, listingFile(fixturePath("/media/b.mp4"), "b", "key-b", 2))

	if err := f.db.Settings().SaveFaceSettings(ctx, domain.FaceSettings{
		Enabled: true, Recognizer: domain.FaceRecognizerArcFaceR50,
	}); err != nil {
		t.Fatal(err)
	}
	var vectors int
	if err := f.db.sql.QueryRow(`select count(*) from person_face_vectors`).Scan(&vectors); err != nil || vectors != 0 {
		t.Fatalf("face image vectors = %d, %v; want the old recognizer's removed", vectors, err)
	}
	appearances := faceAppearances(t, f.db)
	if len(appearances) != 1 || appearances[0].centroid != nil || string(appearances[0].crop) != "crop" ||
		appearances[0].personID == 0 {
		t.Fatalf("appearances = %+v, want the centroid removed and the person and crop kept", appearances)
	}
	job, err := claimFace(f.db)
	if err != nil || job != (domain.FaceJob{ContentKey: f.key, Recognizer: domain.FaceRecognizerArcFaceR50, Generation: 1}) {
		t.Fatalf("claimFace = %+v, %v; want the analysed video under ArcFace and generation 1", job, err)
	}
	if _, err := claimFace(f.db); !errors.Is(err, domain.ErrNoJob) {
		t.Fatalf("second claimFace = %v, want ErrNoJob: a video never analysed is not queued by a switch", err)
	}
}

// A recognizer switch queues every face for embedding again through one queue_face_images job, which queues a job per
// stale face.
func TestRecognizerSwitchQueuesOneJobPerStaleFace(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	stale := createPerson(t, db, "Stale").ID
	setFace(t, db, stale)
	current := createPerson(t, db, "Current").ID
	setFace(t, db, current)
	createPerson(t, db, "Faceless")
	execAll(t, db, `delete from jobs`)

	if err := db.Settings().SaveFaceSettings(ctx, domain.FaceSettings{
		Enabled: true, Recognizer: domain.FaceRecognizerArcFaceR50,
	}); err != nil {
		t.Fatal(err)
	}
	queuer, err := db.Ingest().ClaimJob(ctx, domain.JobQueueFaceImages)
	if err != nil {
		t.Fatalf("claim the queue_face_images job the switch queued: %v", err)
	}
	if _, err := db.sql.Exec(`insert into person_face_vectors (person_id, image_revision, recognizer, vector)
		select id, image_revision, 'arcface_r50', null from people where id = ?`, current); err != nil {
		t.Fatal(err)
	}
	if n, err := db.Faces().QueueStaleFaceImages(ctx); err != nil || n != 1 {
		t.Fatalf("QueueStaleFaceImages = %d, %v; want the one face without a current vector", n, err)
	}
	if err := db.Ingest().CompleteClaimedJob(ctx, queuer); err != nil {
		t.Fatal(err)
	}
	want := [][]any{{"face_image", strconv.FormatInt(stale, 10), int64(domain.JobPriorityImport)}}
	if got := queryRows(t, db.sql, `select kind, subject_key, priority from jobs order by id`, 3); !reflect.DeepEqual(got, want) {
		t.Fatalf("jobs = %v, want %v", got, want)
	}
}

// A retry queues the failed video again unless it is running.
func TestRetryFaceAnalysisQueuesTheFailedVideo(t *testing.T) {
	f := newFaceFixture(t)
	ctx := context.Background()
	queueAllFaces(t, f.db)
	job, err := f.db.Ingest().ClaimJob(ctx, domain.JobFace)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := f.db.Faces().RetryVideo(ctx, f.key, 1); err != nil || n != 0 {
		t.Fatalf("RetryVideo while running = %d, %v; want 0", n, err)
	}
	if err := f.db.Ingest().FailClaimedJob(ctx, job,
		domain.FailureRepeats(errors.New("the face helper exited"))); err != nil {
		t.Fatal(err)
	}
	if n, err := f.db.Faces().RetryVideo(ctx, f.key, 1); err != nil || n != 1 {
		t.Fatalf("RetryVideo after a failure = %d, %v; want 1", n, err)
	}
	if job, err := claimFace(f.db); err != nil || job.ContentKey != f.key {
		t.Fatalf("claimFace = %+v, %v; want the retried video", job, err)
	}
}
