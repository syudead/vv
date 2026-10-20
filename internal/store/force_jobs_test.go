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

// The forced jobs and the scan's forced mark commit together, so a resumed rescan skips exactly the queued videos.
func TestForceScanJobsMarksTheVideoInTheSameTransaction(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	videoID := queueNewFile(t, db, "clip", domain.ActivityByRule)
	runImportStage(t, db, domain.JobProbe)
	scan := startTestRescan(t, db, domain.RescanScope{Kind: domain.RescanLibrary})

	err := db.Ingest().ForceScanJobs(ctx, scan.ID, videoID, domain.ActivityByOwner,
		domain.JobProbe, domain.JobKind("unknown"))
	if err == nil {
		t.Fatal("ForceScanJobs with an unknown kind succeeded, want an error")
	}
	assertForceRows(t, db, domain.JobProbe, videoID, forceRow{state: domain.JobDone})
	if forced, err := db.ScanIndex().ForcedVideos(ctx, scan.ID); err != nil || len(forced) != 0 {
		t.Fatalf("ForcedVideos after the failure = %v, err %v, want none", forced, err)
	}

	if err := db.Ingest().ForceScanJobs(ctx, scan.ID, videoID, domain.ActivityByOwner, domain.JobProbe); err != nil {
		t.Fatal(err)
	}
	assertForceRows(t, db, domain.JobProbe, videoID, queuedForced)
	if forced, err := db.ScanIndex().ForcedVideos(ctx, scan.ID); err != nil || !slices.Equal(forced, []int64{videoID}) {
		t.Fatalf("ForcedVideos = %v, err %v, want [%d]", forced, err, videoID)
	}
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

func videoBuilds(t *testing.T, db *DB, videoID int64) domain.ArtifactBuilds {
	t.Helper()
	return getOwnerVideo(t, db, videoID).Builds
}

// completePreview records a preview the way the preview worker does.
func completePreview(t *testing.T, db *DB) {
	t.Helper()
	job := claimKind(t, db, domain.JobPreview)
	if applied, err := db.Ingest().CompletePreviewForContent(context.Background(), job); err != nil || !applied {
		t.Fatalf("CompletePreviewForContent = %v, %v", applied, err)
	}
}

func TestForcedStageResultCountsTheBuild(t *testing.T) {
	db := migratedDB(t)
	videoID := queueNewFile(t, db, "clip", domain.ActivityByRule)
	for _, kind := range []domain.JobKind{domain.JobProbe, domain.JobThumbnail, domain.JobSeekThumbnail} {
		runImportStage(t, db, kind)
	}
	completePreview(t, db)
	if got := videoBuilds(t, db, videoID); got != (domain.ArtifactBuilds{}) {
		t.Fatalf("builds after ordinary results = %+v, want all 0", got)
	}

	forceJobs(t, db, videoID, domain.JobThumbnail, domain.JobSeekThumbnail, domain.JobPreview)
	runImportStage(t, db, domain.JobThumbnail)
	runImportStage(t, db, domain.JobSeekThumbnail)
	completePreview(t, db)
	want := domain.ArtifactBuilds{Thumbnail: 1, SeekThumbnail: 1, Preview: 1}
	if got := videoBuilds(t, db, videoID); got != want {
		t.Fatalf("builds after forced results = %+v, want %+v", got, want)
	}
}

// A forced stage that gives up records the stage failed over a done state, so a failed rebuild does not read as
// success; an unforced job that gives up keeps the done state.
func TestForcedStageFailureReplacesTheDoneState(t *testing.T) {
	for _, tc := range []struct {
		name  string
		kind  domain.JobKind
		force bool
		state func(domain.Video) string
		want  string
	}{
		{"forced probe", domain.JobProbe, true, func(v domain.Video) string { return string(v.ProbeState) }, "failed"},
		{"forced thumbnail", domain.JobThumbnail, true, func(v domain.Video) string { return string(v.ThumbnailState) }, "failed"},
		{"forced seek", domain.JobSeekThumbnail, true, func(v domain.Video) string { return string(v.SeekThumbnailState) }, "failed"},
		{"unforced thumbnail", domain.JobThumbnail, false, func(v domain.Video) string { return string(v.ThumbnailState) }, "done"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := migratedDB(t)
			ctx := context.Background()
			videoID := queueNewFile(t, db, "clip", domain.ActivityByRule)
			for _, kind := range []domain.JobKind{domain.JobProbe, domain.JobThumbnail, domain.JobSeekThumbnail} {
				runImportStage(t, db, kind)
			}
			queue := db.Ingest().EnqueueJobs
			if tc.force {
				queue = db.Ingest().ForceJobs
			}
			if err := queue(ctx, videoID, domain.ActivityByOwner, tc.kind); err != nil {
				t.Fatal(err)
			}

			giveUpStage(t, db, tc.kind, errors.New("ffmpeg failed"))

			if got := tc.state(getOwnerVideo(t, db, videoID)); got != tc.want {
				t.Fatalf("%s state = %s, want %s", tc.kind, got, tc.want)
			}
			if !tc.force {
				return
			}
			// The next force rescan runs the failed stage again, and its success clears the failure.
			forceJobs(t, db, videoID, tc.kind)
			runImportStage(t, db, tc.kind)
			if got := tc.state(getOwnerVideo(t, db, videoID)); got != "done" {
				t.Errorf("%s state after a forced success = %s, want done", tc.kind, got)
			}
		})
	}
}

func getOwnerVideo(t *testing.T, db *DB, videoID int64) domain.Video {
	t.Helper()
	video, err := db.Library().GetVideo(context.Background(), domain.AudienceOwner, videoID)
	if err != nil {
		t.Fatal(err)
	}
	return video
}
