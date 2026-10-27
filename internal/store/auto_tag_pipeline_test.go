package store

import (
	"context"
	"errors"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func saveAutoTagSwitches(t *testing.T, db *DB, enabled, afterImport bool) {
	t.Helper()
	if err := db.Settings().SaveAutoTagSettings(context.Background(), domain.AutoTagSettings{
		Enabled: enabled, AfterImport: afterImport, Threshold: 0.8, TentativeThreshold: 0.5,
	}); err != nil {
		t.Fatal(err)
	}
}

// importThroughSeek runs a new file's stages up to the seek stage's final result, which either succeeds or is given up.
func importThroughSeek(t *testing.T, db *DB, name string, seekFails bool) int64 {
	t.Helper()
	videoID := queueNewFile(t, db, name, domain.ActivityByRule)
	for _, kind := range []domain.JobKind{domain.JobProbe, domain.JobThumbnail, domain.JobPreview} {
		runImportStage(t, db, kind)
	}
	if seekFails {
		giveUpStage(t, db, domain.JobSeekThumbnail, errors.New("ffmpeg failed"))
	} else {
		runImportStage(t, db, domain.JobSeekThumbnail)
	}
	return videoID
}

func TestAfterImportAutoTagJoinsTheImportPipeline(t *testing.T) {
	for _, seekFails := range []bool{false, true} {
		name := "seek-done"
		if seekFails {
			name = "seek-failed"
		}
		t.Run(name, func(t *testing.T) {
			db := migratedDB(t)
			saveAutoTagSwitches(t, db, true, true)
			importThroughSeek(t, db, name, seekFails)
			if !seekFails {
				runImportStage(t, db, domain.JobFingerprint)
			}

			imported := importEntries(t, db)[0]
			if imported.Status == domain.ActivityRunning {
				t.Fatalf("import entry = %+v, want it finished", imported)
			}
			pipeline := onlyPipeline(t, db, imported.ID)
			tagging := latestAutoTagEntry(t, db)
			if got := onlyPipeline(t, db, tagging.ID); got.ID != pipeline.ID {
				t.Errorf("auto_tag pipeline = %d, want the import pipeline %d", got.ID, pipeline.ID)
			}
			if entryState(tagging) != activityWaiting || tagging.Decider != domain.ActivityByRule {
				t.Errorf("auto_tag entry = %+v, want running/waiting by rule", tagging)
			}
			if entryState(pipeline) != activityWaiting {
				t.Errorf("pipeline = %+v, want running/waiting while tagging waits", pipeline)
			}
		})
	}
}

func TestAfterImportAutoTagFailureLeavesTheImportMemberDone(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	saveAutoTagSwitches(t, db, true, true)
	importThroughSeek(t, db, "clip", false)
	runImportStage(t, db, domain.JobFingerprint)
	store := db.AutoTags()
	job := claimAutoTag(t, store)
	if err := failAutoTag(ctx, store, job.ContentKey, "cannot reach Ollama"); err != nil {
		t.Fatal(err)
	}

	imported := importEntries(t, db)[0]
	failed := latestAutoTagEntry(t, db)
	pipeline := onlyPipeline(t, db, imported.ID)
	if entryState(imported) != activityDone || entryState(pipeline) != activityFailed {
		t.Fatalf("import = %+v, pipeline = %+v; want done and attention/failed", imported, pipeline)
	}

	// A retry from the activity log continues the failed member inside the same pipeline.
	if n, err := store.RetryVideo(ctx, failed.VideoIDs[0], failed.ID); err != nil || n != 1 {
		t.Fatalf("RetryVideo = %d, %v", n, err)
	}
	retried := latestAutoTagEntry(t, db)
	if got := onlyPipeline(t, db, retried.ID); retried.ID == failed.ID || got.ID != pipeline.ID ||
		entryState(got) != activityWaiting {
		t.Errorf("retried entry %d in pipeline %+v, want a new member of %d running/waiting", retried.ID, got, pipeline.ID)
	}

	// A claim that reopens the interrupted member joins the same pipeline too.
	claimAutoTag(t, store)
	if _, err := db.Activity().RecoverInterruptedActivity(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Ingest().RequeueRunningJobs(ctx); err != nil {
		t.Fatal(err)
	}
	claimAutoTag(t, store)
	reopened := latestAutoTagEntry(t, db)
	if got := onlyPipeline(t, db, reopened.ID); reopened.ID == retried.ID || got.ID != pipeline.ID ||
		entryState(got) != activityClaimed {
		t.Errorf("reopened entry %d in pipeline %+v, want a new member of %d running/processing", reopened.ID, got,
			pipeline.ID)
	}
}

func TestAfterImportAutoTagWritesNothingWhenItDoesNotRun(t *testing.T) {
	cases := []struct {
		name                 string
		enabled, afterImport bool
		queued               bool
	}{
		{name: "feature off", afterImport: true},
		{name: "after import off", enabled: true},
		{name: "queue row present", enabled: true, afterImport: true, queued: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := migratedDB(t)
			ctx := context.Background()
			videoID := queueNewFile(t, db, "clip", domain.ActivityByRule)
			if tc.queued {
				if _, err := db.AutoTags().QueueVideos(ctx, []int64{videoID}, domain.AutoTagQueueIfNew,
					domain.ActivityByOwner); err != nil {
					t.Fatal(err)
				}
			}
			saveAutoTagSwitches(t, db, tc.enabled, tc.afterImport)
			before := len(autoTagEntries(t, db))
			for _, kind := range []domain.JobKind{domain.JobProbe, domain.JobThumbnail, domain.JobPreview,
				domain.JobSeekThumbnail, domain.JobFingerprint} {
				runImportStage(t, db, kind)
			}

			if got := len(autoTagEntries(t, db)); got != before {
				t.Errorf("auto_tag entries = %d, want %d", got, before)
			}
			pipeline := onlyPipeline(t, db, importEntries(t, db)[0].ID)
			if entryState(pipeline) != activityDone {
				t.Errorf("pipeline = %+v, want done/completed", pipeline)
			}
		})
	}
}

func TestSuggestTagsWritesAStandaloneEntry(t *testing.T) {
	db := migratedDB(t)
	saveAutoTagSwitches(t, db, true, false)
	videoID := importThroughSeek(t, db, "clip", false)

	if _, err := db.AutoTags().QueueVideos(context.Background(), []int64{videoID}, domain.AutoTagQueueAgain,
		domain.ActivityByOwner); err != nil {
		t.Fatal(err)
	}
	if pipelines := entryPipelines(t, db, latestAutoTagEntry(t, db).ID); len(pipelines) != 0 {
		t.Errorf("pipelines of the owner's run = %v, want none", pipelines)
	}
}
