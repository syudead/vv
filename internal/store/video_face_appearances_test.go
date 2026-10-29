package store

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// setFaceQueueState records a done or failed result of key, or queues its face job.
func setFaceQueueState(t *testing.T, db *DB, key, state string) {
	t.Helper()
	statement := `insert into face_results (content_key, state, updated_at) values (?, ?, 0)`
	if state == string(domain.JobQueued) {
		statement = `insert into jobs (kind, subject_key, state, created_at, updated_at) values ('face', ?, ?, 0, 0)`
	}
	if _, err := db.sql.Exec(statement, key, state); err != nil {
		t.Fatal(err)
	}
}

// A done video lists its living, not-dropped appearances by first span with their people; an unfinished one lists
// nothing, and a missing video is not found.
func TestVideoFaceAppearancesListsTheAnalysedVideosPeople(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	paths := []string{fixturePath("/media/done.mp4"), fixturePath("/media/queued.mp4"), fixturePath("/media/new.mp4")}
	ids := upsertAll(t, db, listingFile(paths[0], "Done", "key-done", 1), listingFile(paths[1], "Queued", "key-queued", 2),
		listingFile(paths[2], "New", "key-new", 3))
	setFaceQueueState(t, db, "key-done", "done")
	setFaceQueueState(t, db, "key-queued", "queued")
	named := createPerson(t, db, "Hanako").ID
	withFace, err := db.Credits().SetPersonImage(ctx, named, testFace)
	if err != nil {
		t.Fatal(err)
	}
	unnamed := createProvisionalPerson(t, db)
	deleted := createPerson(t, db, "Taro").ID

	later := insertFaceAppearance(t, db, "key-done", named, false, `[[20, 30, 0.5, 0.5, 0.1], [25, 40, 0.5, 0.5, 0.1]]`,
		[]byte("crop"))
	earlier := insertFaceAppearance(t, db, "key-done", unnamed, false, `[[5, 8, 0.5, 0.5, 0.1]]`, nil)
	insertFaceAppearance(t, db, "key-done", named, true, `[[0, 1, 0.5, 0.5, 0.1]]`, nil)
	insertFaceAppearance(t, db, "key-done", deleted, false, `[[0, 1, 0.5, 0.5, 0.1]]`, nil)
	insertFaceAppearance(t, db, "key-queued", named, false, `[[0, 1, 0.5, 0.5, 0.1]]`, nil)
	if err := db.Credits().Delete(ctx, domain.CreditPerson, deleted); err != nil {
		t.Fatal(err)
	}

	got, err := db.Credits().VideoFaceAppearances(ctx, ids[paths[0]])
	want := domain.VideoFaceAppearances{Analysed: true, Items: []domain.VideoFaceAppearance{
		{ID: earlier, Person: domain.CreditRef{ID: unnamed, Provisional: true}, Spans: []domain.FaceTimeSpan{{Start: 5, End: 8}}},
		{ID: later, Person: domain.CreditRef{ID: named, Name: "Hanako"}, ImageRevision: withFace.ImageRevision, HasCrop: true,
			Spans: []domain.FaceTimeSpan{{Start: 20, End: 40}}},
	}}
	if err != nil || !reflect.DeepEqual(got, want) || withFace.ImageRevision == 0 {
		t.Errorf("done video = %+v, %v; want %+v", got, err, want)
	}

	for _, path := range paths[1:] {
		got, err := db.Credits().VideoFaceAppearances(ctx, ids[path])
		if want := (domain.VideoFaceAppearances{Items: []domain.VideoFaceAppearance{}}); err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("%s = %+v, %v; want not analysed and no items", path, got, err)
		}
	}
	if _, err := db.Credits().VideoFaceAppearances(ctx, ids[paths[2]]+100); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("missing video: err = %v, want ErrNotFound", err)
	}
}
