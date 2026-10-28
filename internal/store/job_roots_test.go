package store

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// twoRootsDB registers /media (the fixture's folder) and /other, and returns their ids.
func twoRootsDB(t *testing.T) (db *DB, media, other int64) {
	t.Helper()
	db = migratedDB(t)
	if _, err := db.sql.Exec(`insert into media_folders(path, version, created_at, updated_at) values (?, 1, 1, 1)`,
		fixturePath("/other")); err != nil {
		t.Fatal(err)
	}
	media = countRows(t, db, `select id from media_folders where path = '`+fixturePath("/media")+`'`)
	other = countRows(t, db, `select id from media_folders where path = '`+fixturePath("/other")+`'`)
	return db, media, other
}

func addVideoAt(t *testing.T, db *DB, path, key string) int64 {
	t.Helper()
	added, err := db.ScanIndex().UpsertVideo(context.Background(), sampleFile(fixturePath(path), key, key, 1024, 0))
	if err != nil {
		t.Fatal(err)
	}
	return added.ID
}

func claimOn(t *testing.T, db *DB, claim domain.JobClaim) domain.Job {
	t.Helper()
	job, err := db.Ingest().ClaimJobOn(context.Background(), claim)
	if err != nil {
		t.Fatalf("ClaimJobOn(%+v): %v", claim, err)
	}
	return job
}

func TestClaimJobOnTakesOnlyJobsUnderTheVisitedRoots(t *testing.T) {
	db, _, other := twoRootsDB(t)
	older := addVideoAt(t, db, "/media/older.mp4", "older")
	newer := addVideoAt(t, db, "/other/newer.mp4", "newer")
	enqueueProbe(t, db, older)
	enqueueProbe(t, db, newer)

	job := claimOn(t, db, domain.JobClaim{Kind: domain.JobProbe, MaxPriority: domain.JobPriorityBackfill, Roots: []int64{other}})
	if job.VideoID != newer || job.RootID != other {
		t.Fatalf("claimed video %d under root %d, want video %d under %d", job.VideoID, job.RootID, newer, other)
	}
}

// A file in a folder nested under another belongs to the inner folder, which may sit on another disk, so only a claim
// on the inner folder takes it, and parking the inner folder parks it even though the outer one is available.
func TestClaimJobOnPlacesANestedFoldersJobInTheInnerFolder(t *testing.T) {
	db := migratedDB(t)
	if _, err := db.sql.Exec(`insert into media_folders(path, version, created_at, updated_at) values (?, 1, 1, 1)`,
		fixturePath("/media/inner")); err != nil {
		t.Fatal(err)
	}
	outer := countRows(t, db, `select id from media_folders where path = '`+fixturePath("/media")+`'`)
	inner := countRows(t, db, `select id from media_folders where path = '`+fixturePath("/media/inner")+`'`)
	videoID := addVideoAt(t, db, "/media/inner/movie.mp4", "movie")
	enqueueProbe(t, db, videoID)
	ctx := context.Background()

	for _, claim := range []domain.JobClaim{
		{Kind: domain.JobProbe, MaxPriority: domain.JobPriorityBackfill, Roots: []int64{outer}},
		{Kind: domain.JobProbe, MaxPriority: domain.JobPriorityBackfill, Unavailable: []int64{inner}},
	} {
		if job, err := db.Ingest().ClaimJobOn(ctx, claim); !errors.Is(err, domain.ErrNoJob) {
			t.Fatalf("ClaimJobOn(%+v) = %+v, %v; want domain.ErrNoJob", claim, job, err)
		}
	}
	job := claimOn(t, db, domain.JobClaim{Kind: domain.JobProbe, MaxPriority: domain.JobPriorityBackfill, Roots: []int64{inner}})
	if job.VideoID != videoID || job.RootID != inner {
		t.Fatalf("claimed video %d under root %d, want video %d under the inner folder %d", job.VideoID, job.RootID,
			videoID, inner)
	}
}

func TestClaimJobOnParksAnUnavailableRootsJobUntilItIsAvailable(t *testing.T) {
	db, media, other := twoRootsDB(t)
	parked := addVideoAt(t, db, "/media/parked.mp4", "parked")
	readable := addVideoAt(t, db, "/other/readable.mp4", "readable")
	enqueueProbe(t, db, parked)
	enqueueProbe(t, db, readable)
	// Both folders are on one disk; the older job's folder cannot be read.
	disk := domain.JobClaim{
		Kind: domain.JobProbe, MaxPriority: domain.JobPriorityBackfill, Roots: []int64{media, other},
		Unavailable: []int64{media},
	}

	if job := claimOn(t, db, disk); job.VideoID != readable {
		t.Fatalf("claimed video %d, want video %d under the readable folder", job.VideoID, readable)
	}
	if job, err := db.Ingest().ClaimJobOn(context.Background(), disk); !errors.Is(err, domain.ErrNoJob) {
		t.Fatalf("second claim = %+v, %v; want domain.ErrNoJob", job, err)
	}
	if got := countRows(t, db, `select count(*) from jobs where state = 'queued' and attempts = 0`); got != 1 {
		t.Fatalf("queued jobs without an attempt = %d, want the parked one", got)
	}

	// A recheck found the folder readable, so the scheduler no longer passes it over.
	disk.Unavailable = nil
	if job := claimOn(t, db, disk); job.VideoID != parked || job.Attempts != 1 {
		t.Fatalf("claimed video %d at attempt %d, want video %d at its first", job.VideoID, job.Attempts, parked)
	}
}

func TestClaimJobOnTakesAReadableSecondLocation(t *testing.T) {
	db, media, other := twoRootsDB(t)
	addVideoAt(t, db, "/media/movie.mp4", "shared")
	videoID := addVideoAt(t, db, "/other/movie.mp4", "shared")
	enqueueProbe(t, db, videoID)

	job := claimOn(t, db, domain.JobClaim{
		Kind: domain.JobProbe, MaxPriority: domain.JobPriorityBackfill, Roots: []int64{other},
		Unavailable: []int64{media},
	})
	if job.LocationPath != fixturePath("/other/movie.mp4") || job.RootID != other || !job.LastLocation {
		t.Fatalf("claimed %s under root %d (last %v), want the readable location under %d as the round's last",
			job.LocationPath, job.RootID, job.LastLocation, other)
	}
}

func TestClaimJobOnCountsTheRoundASnoozeUndidAtTheNextLocation(t *testing.T) {
	db, media, _ := twoRootsDB(t)
	addVideoAt(t, db, "/media/movie.mp4", "shared")
	videoID := addVideoAt(t, db, "/other/movie.mp4", "shared")
	enqueueProbe(t, db, videoID)
	failJob(t, db, claimProbe(t, db), fmt.Errorf("root unreadable: %w", domain.ErrJobSnoozed))

	// The snoozed claim started the round, so the claim at the second location starts and counts it.
	job := claimOn(t, db, domain.JobClaim{
		Kind: domain.JobProbe, MaxPriority: domain.JobPriorityBackfill, Unavailable: []int64{media},
	})
	if job.LocationPath != fixturePath("/other/movie.mp4") || job.Attempts != 1 || !job.StartsRound {
		t.Fatalf("claimed %s at attempt %d (starts round %v), want the second location starting attempt 1",
			job.LocationPath, job.Attempts, job.StartsRound)
	}
}

func TestClaimJobOnPassesOverAParkedThumbnailForASeekSprite(t *testing.T) {
	db, media, other := twoRootsDB(t)
	parked := addVideoAt(t, db, "/media/parked.mp4", "parked")
	readable := addVideoAt(t, db, "/other/readable.mp4", "readable")
	enqueueProbe(t, db, parked)
	enqueueProbe(t, db, readable)
	runImportStage(t, db, domain.JobProbe)
	runImportStage(t, db, domain.JobProbe)
	ctx := context.Background()
	if err := db.Ingest().EnqueueJob(ctx, domain.JobThumbnail, parked, domain.ActivityByRule); err != nil {
		t.Fatal(err)
	}
	if err := db.Ingest().EnqueueJob(ctx, domain.JobSeekThumbnail, readable, domain.ActivityByRule); err != nil {
		t.Fatal(err)
	}

	job := claimOn(t, db, domain.JobClaim{
		Kind: domain.JobSeekThumbnail, MaxPriority: domain.JobPriorityBackfill, Roots: []int64{other},
		Unavailable: []int64{media},
	})
	if job.VideoID != readable {
		t.Fatalf("claimed video %d, want video %d while the only thumbnail is parked", job.VideoID, readable)
	}
}

func TestDueJobKindsReportsTheMostUrgentDueClass(t *testing.T) {
	db := migratedDB(t)
	now := fixJobClock(t)
	ctx := context.Background()
	imported, owned := addVideo(t, db, "imported"), addVideo(t, db, "owned")
	enqueueProbe(t, db, imported)
	if err := db.Ingest().ForceJobs(ctx, owned, domain.ActivityByOwner, domain.JobProbe); err != nil {
		t.Fatal(err)
	}
	if err := db.Ingest().EnqueueJob(ctx, domain.JobThumbnail, imported, domain.ActivityByRule); err != nil {
		t.Fatal(err)
	}
	if _, err := db.sql.Exec(`update jobs set run_after = ? where kind = 'thumbnail'`, now.Unix()+1); err != nil {
		t.Fatal(err)
	}

	due, err := db.Ingest().DueJobKinds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[domain.JobKind]domain.JobPriority{domain.JobProbe: domain.JobPriorityOwner}; len(due) != 1 ||
		due[domain.JobProbe] != want[domain.JobProbe] {
		t.Fatalf("due = %v, want %v", due, want)
	}
}

func TestClaimJobOnTakesAJobThatReadsNoSourceUnderAnUnavailableRoot(t *testing.T) {
	db, media, _ := twoRootsDB(t)
	videoID := addVideoAt(t, db, "/media/movie.mp4", "movie")
	enqueueProbe(t, db, videoID)
	runImportStage(t, db, domain.JobProbe)
	ctx := context.Background()
	if err := db.Ingest().EnqueueJob(ctx, domain.JobSeekThumbnail, videoID, domain.ActivityByRule); err != nil {
		t.Fatal(err)
	}
	runImportStage(t, db, domain.JobSeekThumbnail)
	if err := db.Ingest().EnqueueJob(ctx, domain.JobFingerprint, videoID, domain.ActivityByRule); err != nil {
		t.Fatal(err)
	}

	// The fingerprint reads saved sprites, so the unplugged folder does not park it.
	job := claimOn(t, db, domain.JobClaim{
		Kind: domain.JobFingerprint, MaxPriority: domain.JobPriorityBackfill, Unavailable: []int64{media},
	})
	if job.VideoID != videoID || job.RootID != media || job.Attempts != 1 {
		t.Fatalf("claimed video %d under root %d at attempt %d, want video %d under %d at its first",
			job.VideoID, job.RootID, job.Attempts, videoID, media)
	}
}
