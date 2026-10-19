package store

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

type forceRow struct {
	state          domain.JobState
	force, request bool
}

// forceRows lists every row of the kind for the video in insertion order.
func forceRows(t *testing.T, db *DB, kind domain.JobKind, videoID int64) []forceRow {
	t.Helper()
	rows, err := db.sql.Query(`select state, force, force_requested from jobs where kind = ? and video_id = ? order by id`,
		string(kind), videoID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var got []forceRow
	for rows.Next() {
		var row forceRow
		if err := rows.Scan(&row.state, &row.force, &row.request); err != nil {
			t.Fatal(err)
		}
		got = append(got, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return got
}

func assertForceRows(t *testing.T, db *DB, kind domain.JobKind, videoID int64, want ...forceRow) {
	t.Helper()
	if got := forceRows(t, db, kind, videoID); !slices.Equal(got, want) {
		t.Errorf("%s rows = %+v, want %+v", kind, got, want)
	}
}

func forceJobs(t *testing.T, db *DB, videoID int64, kinds ...domain.JobKind) {
	t.Helper()
	if err := db.Ingest().ForceJobs(context.Background(), videoID, domain.ActivityByOwner, kinds...); err != nil {
		t.Fatal(err)
	}
}

func claimKind(t *testing.T, db *DB, kind domain.JobKind) domain.Job {
	t.Helper()
	job, err := db.Ingest().ClaimJob(context.Background(), kind)
	if err != nil {
		t.Fatalf("ClaimJob(%s): %v", kind, err)
	}
	return job
}

var (
	queuedForced  = forceRow{state: domain.JobQueued, force: true}
	runningForced = forceRow{state: domain.JobRunning, force: true}
)

func TestForceJobsFollowsTheExistingRow(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	videoID := queueNewFile(t, db, "clip", domain.ActivityByRule)

	forceJobs(t, db, videoID, domain.JobProbe)
	assertForceRows(t, db, domain.JobProbe, videoID, queuedForced)

	runImportStage(t, db, domain.JobProbe)
	forceJobs(t, db, videoID, domain.JobProbe)
	assertForceRows(t, db, domain.JobProbe, videoID, queuedForced)

	probe := claimKind(t, db, domain.JobProbe)
	if !probe.Force {
		t.Errorf("claimed probe = %+v, want Force", probe)
	}
	forceJobs(t, db, videoID, domain.JobProbe)
	assertForceRows(t, db, domain.JobProbe, videoID, runningForced)

	thumbnail := claimKind(t, db, domain.JobThumbnail)
	forceJobs(t, db, videoID, domain.JobThumbnail)
	assertForceRows(t, db, domain.JobThumbnail, videoID, forceRow{state: domain.JobRunning, request: true})
	if err := db.Ingest().CompleteClaimedJob(ctx, thumbnail); err != nil {
		t.Fatal(err)
	}
	assertForceRows(t, db, domain.JobThumbnail, videoID, forceRow{state: domain.JobDone}, queuedForced)
}

func TestRequeueRunningJobsTurnsAForceRequestIntoForce(t *testing.T) {
	db := migratedDB(t)
	videoID := queueNewFile(t, db, "clip", domain.ActivityByRule)
	claimKind(t, db, domain.JobProbe)
	forceJobs(t, db, videoID, domain.JobProbe)

	if _, err := db.Ingest().RequeueRunningJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertForceRows(t, db, domain.JobProbe, videoID, queuedForced)
}

func TestProbeResultPassesItsForceToThePreview(t *testing.T) {
	db := migratedDB(t)
	forced := queueNewFile(t, db, "forced", domain.ActivityByRule)
	forceJobs(t, db, forced, domain.JobProbe)
	runImportStage(t, db, domain.JobProbe)
	assertForceRows(t, db, domain.JobPreview, forced, queuedForced)

	ordinary := queueNewFile(t, db, "ordinary", domain.ActivityByRule)
	runImportStage(t, db, domain.JobProbe)
	assertForceRows(t, db, domain.JobPreview, ordinary, forceRow{state: domain.JobQueued})
}

func TestForcedSeekStageQueuesNoFollowUpProcess(t *testing.T) {
	for _, seekFails := range []bool{false, true} {
		name := "seek-done"
		if seekFails {
			name = "seek-failed"
		}
		t.Run(name, func(t *testing.T) {
			db := migratedDB(t)
			ctx := context.Background()
			saveAutoTagSwitches(t, db, true, true)
			if err := db.Settings().SaveFaceSettings(ctx, domain.FaceSettings{
				Enabled: true, Recognizer: domain.FaceRecognizerArcFaceR50,
			}); err != nil {
				t.Fatal(err)
			}
			videoID := queueNewFile(t, db, name, domain.ActivityByRule)
			for _, kind := range []domain.JobKind{domain.JobProbe, domain.JobThumbnail, domain.JobPreview} {
				runImportStage(t, db, kind)
			}
			forceJobs(t, db, videoID, domain.JobSeekThumbnail)
			if seekFails {
				giveUpStage(t, db, domain.JobSeekThumbnail, errors.New("ffmpeg failed"))
			} else {
				runImportStage(t, db, domain.JobSeekThumbnail)
				assertForceRows(t, db, domain.JobFingerprint, videoID, queuedForced)
			}

			for _, table := range []string{"auto_tag_queue", "face_queue"} {
				var count int
				if err := db.sql.QueryRow(`select count(*) from ` + table).Scan(&count); err != nil {
					t.Fatal(err)
				}
				if count != 0 {
					t.Errorf("%s rows = %d, want none after a forced seek stage", table, count)
				}
			}
		})
	}
}
