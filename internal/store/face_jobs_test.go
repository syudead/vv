package store

import (
	"context"
	"errors"
	"io/fs"
	"reflect"
	"strconv"
	"testing"

	"github.com/pressly/goose/v3"

	"github.com/syudead/vv/internal/domain"
)

const faceJobsVersion = 87

// Unfinished face analysis becomes face jobs and outcomes become results, and a library with faces gets one
// queue_face_images job, which queues the face that was stale at the upgrade; going back restores the queue.
func TestFaceJobsMigrationRoundTripsTheQueueAndQueuesStaleFaces(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	fsys, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, db.sql, fsys)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := provider.UpTo(ctx, faceJobsVersion-1); err != nil {
		t.Fatal(err)
	}
	execAll(t, db, `insert into face_queue (content_key, state, recognizer, reason, activity_id, updated_at)
		values ('key-queued', 'queued', null, null, 5, 100000),
		       ('key-running', 'running', null, null, 6, 200000),
		       ('key-done', 'done', 'sface_int8', null, null, 300000),
		       ('key-failed', 'failed', 'sface_int8', 'boom', null, 400000)`,
		`insert into people (id, created_at) values (7, 0), (8, 0)`,
		`insert into person_images (person_id, image, width, height, updated_at) values (7, x'00', 1, 1, 0)`)

	if _, err := provider.UpTo(ctx, faceJobsVersion); err != nil {
		t.Fatal(err)
	}
	wantJobs := [][]any{
		{"face", "key-queued", "queued", int64(0), int64(1), int64(100), int64(5)},
		{"face", "key-running", "queued", int64(0), int64(1), int64(200), int64(6)},
		{"queue_face_images", nil, "queued", int64(0), int64(1), nil, nil},
	}
	got := queryRows(t, db.sql, `select kind, subject_key, state, attempts, priority,
		case when kind = 'face' then run_after end, activity_id from jobs order by id`, 7)
	if !reflect.DeepEqual(got, wantJobs) {
		t.Fatalf("jobs = %v, want %v", got, wantJobs)
	}
	wantResults := [][]any{{"key-done", "done", "sface_int8", nil}, {"key-failed", "failed", "sface_int8", "boom"}}
	if got := queryRows(t, db.sql, `select content_key, state, recognizer, reason from face_results
		order by content_key`, 4); !reflect.DeepEqual(got, wantResults) {
		t.Fatalf("results = %v, want %v", got, wantResults)
	}
	if n, err := db.Faces().QueueStaleFaceImages(ctx); err != nil || n != 1 {
		t.Fatalf("QueueStaleFaceImages = %d, %v; want the one face without a vector", n, err)
	}
	if n := countRows(t, db, `select count(*) from jobs where kind = 'face_image' and subject_key = '7'`); n != 1 {
		t.Fatalf("face_image jobs of the stale person = %d, want 1", n)
	}

	if _, err := provider.DownTo(ctx, faceJobsVersion-1); err != nil {
		t.Fatal(err)
	}
	wantQueue := [][]any{
		{"key-done", "done", "sface_int8", nil, nil},
		{"key-failed", "failed", "sface_int8", "boom", nil},
		{"key-queued", "queued", nil, nil, int64(5)},
		{"key-running", "queued", nil, nil, int64(6)},
	}
	if got := queryRows(t, db.sql, `select content_key, state, recognizer, reason, activity_id from face_queue
		order by content_key`, 5); !reflect.DeepEqual(got, wantQueue) {
		t.Fatalf("queue after down = %v, want %v", got, wantQueue)
	}
	if n := countRows(t, db, `select count(*) from jobs`); n != 0 {
		t.Fatalf("jobs after down = %d, want the face kinds removed", n)
	}
}

// A face job reads from the first available location of its content in path order, under the visited folders; one
// whose content has no registered video left is dropped, and one stored only under an unavailable folder stays queued.
func TestFaceClaimTakesTheContentsFirstAvailableLocation(t *testing.T) {
	db, media, other := twoRootsDB(t)
	ctx := context.Background()
	addVideoAt(t, db, "/media/gone.mp4", "gone")
	addVideoAt(t, db, "/media/shared.mp4", "shared")
	addVideoAt(t, db, "/other/shared.mp4", "shared")
	if n, err := db.Faces().QueueLibrary(ctx, domain.FaceQueueMissing, domain.ActivityByRule); err != nil || n != 2 {
		t.Fatalf("QueueLibrary = %d, %v; want gone and shared", n, err)
	}
	execAll(t, db, `delete from video_locations where path = '`+fixturePath("/media/gone.mp4")+`'`)

	parked := domain.JobClaim{
		Kind: domain.JobFace, MaxPriority: domain.JobPriorityBackfill, Roots: []int64{media}, Unavailable: []int64{media},
	}
	if job, err := db.Ingest().ClaimJobOn(ctx, parked); !errors.Is(err, domain.ErrNoJob) {
		t.Fatalf("claim under the unavailable folder = %+v, %v; want domain.ErrNoJob", job, err)
	}
	if got := queryRows(t, db.sql, `select subject_key, state, attempts from jobs where kind = 'face'`, 3); !reflect.DeepEqual(
		got, [][]any{{"shared", "queued", int64(0)}}) {
		t.Fatalf("face jobs = %v, want gone dropped and shared parked without an attempt", got)
	}

	job := claimOn(t, db, domain.JobClaim{
		Kind: domain.JobFace, MaxPriority: domain.JobPriorityBackfill, Roots: []int64{other}, Unavailable: []int64{media},
	})
	if job.ContentKey != "shared" || job.LocationPath != fixturePath("/other/shared.mp4") || job.RootID != other ||
		job.Attempts != 1 {
		t.Fatalf("claimed %+v, want shared at its location under the other folder on its first attempt", job)
	}
	if got := queryRows(t, db.sql, `select location_path from jobs where id = `+strconv.FormatInt(job.ID, 10), 1); !reflect.DeepEqual(
		got, [][]any{{fixturePath("/other/shared.mp4")}}) {
		t.Fatalf("stored location = %v, want the claimed one", got)
	}
}

// A face job reading from a folder nested under another is placed in the inner folder, whose disk may differ.
func TestFaceClaimPlacesANestedFoldersLocationInTheInnerFolder(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	execAll(t, db, `insert into media_folders(path, version, created_at, updated_at) values ('`+
		fixturePath("/media/inner")+`', 1, 1, 1)`)
	outer := countRows(t, db, `select id from media_folders where path = '`+fixturePath("/media")+`'`)
	inner := countRows(t, db, `select id from media_folders where path = '`+fixturePath("/media/inner")+`'`)
	addVideoAt(t, db, "/media/inner/movie.mp4", "movie")
	if _, err := db.Faces().QueueLibrary(ctx, domain.FaceQueueMissing, domain.ActivityByRule); err != nil {
		t.Fatal(err)
	}

	onOuter := domain.JobClaim{Kind: domain.JobFace, MaxPriority: domain.JobPriorityBackfill, Roots: []int64{outer}}
	if job, err := db.Ingest().ClaimJobOn(ctx, onOuter); !errors.Is(err, domain.ErrNoJob) {
		t.Fatalf("claim on the outer folder = %+v, %v; want domain.ErrNoJob", job, err)
	}
	job := claimOn(t, db, domain.JobClaim{Kind: domain.JobFace, MaxPriority: domain.JobPriorityBackfill, Roots: []int64{inner}})
	if job.ContentKey != "movie" || job.RootID != inner {
		t.Fatalf("claimed %+v, want movie under the inner folder %d", job, inner)
	}
}

// A failed face job tries its content's next location at once within the same attempt, and only a failure at the
// last location waits for the retry delay.
func TestFaceJobTriesTheContentsNextLocationAfterAFailure(t *testing.T) {
	db, _, other := twoRootsDB(t)
	ctx := context.Background()
	addVideoAt(t, db, "/media/shared.mp4", "shared")
	addVideoAt(t, db, "/other/shared.mp4", "shared")
	if _, err := db.Faces().QueueLibrary(ctx, domain.FaceQueueMissing, domain.ActivityByRule); err != nil {
		t.Fatal(err)
	}
	claim := domain.JobClaim{Kind: domain.JobFace, MaxPriority: domain.JobPriorityBackfill}

	first := claimOn(t, db, claim)
	if first.LocationPath != fixturePath("/media/shared.mp4") || first.Attempts != 1 || first.LastLocation {
		t.Fatalf("first claim = %+v, want the first location on attempt 1 with another after it", first)
	}
	if err := db.Ingest().FailClaimedJob(ctx, first, errors.New("cannot open the video's file")); err != nil {
		t.Fatal(err)
	}
	second := claimOn(t, db, domain.JobClaim{
		Kind: domain.JobFace, MaxPriority: domain.JobPriorityBackfill, Roots: []int64{other},
	})
	if second.LocationPath != fixturePath("/other/shared.mp4") || second.Attempts != 1 || !second.LastLocation {
		t.Fatalf("second claim = %+v, want the next location within attempt 1, as the last one", second)
	}
	if err := db.Ingest().FailClaimedJob(ctx, second, errors.New("cannot open the video's file")); err != nil {
		t.Fatal(err)
	}
	if job, err := db.Ingest().ClaimJobOn(ctx, claim); !errors.Is(err, domain.ErrNoJob) {
		t.Fatalf("claim after the last location failed = %+v, %v; want the job waiting for its retry delay", job, err)
	}
}

// A work-information change queues the cast rule for each changed video while face recognition is on, once while
// the job waits; a change while it is off is not checked later.
func TestWorkInfoChangeQueuesTheCastRuleWhileFaceRecognitionIsOn(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	ids := upsertAll(t, db, listingFile(fixturePath("/media/a.mp4"), "a", "key-a", 1))
	video := ids[fixturePath("/media/a.mp4")]

	setWorkInfo(t, db, video, domain.WorkInfo{Code: "OFF-1"})
	if err := writeSetting(ctx, db.sql, faceEnabledKey, strconv.FormatBool(true)); err != nil {
		t.Fatal(err)
	}
	setWorkInfo(t, db, video, domain.WorkInfo{Code: "ON-1"})
	setWorkInfo(t, db, video, domain.WorkInfo{Code: "ON-2"})

	want := [][]any{{video, int64(domain.JobPriorityImport)}}
	if got := queryRows(t, db.sql, `select video_id, priority from jobs where kind = 'face_cast'`, 2); !reflect.DeepEqual(got, want) {
		t.Fatalf("face_cast jobs = %v, want %v", got, want)
	}
}
