package store

import (
	"context"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// backgroundJob is one jobs row the count reads; a video or a content key names its subject.
type backgroundJob struct {
	kind     domain.JobKind
	videoID  int64
	key      string
	state    domain.JobState
	runAfter time.Time
}

func insertBackgroundJobs(t *testing.T, db *DB, jobs ...backgroundJob) {
	t.Helper()
	if _, err := db.sql.Exec(`delete from jobs`); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for _, job := range jobs {
		var videoID, key any
		if job.videoID != 0 {
			videoID = job.videoID
		}
		if job.key != "" {
			key = job.key
		}
		runAfter := now
		if !job.runAfter.IsZero() {
			runAfter = job.runAfter
		}
		if _, err := db.sql.Exec(`insert into jobs (kind, video_id, subject_key, state, run_after, created_at, updated_at)
			values (?, ?, ?, ?, ?, ?, ?)`, string(job.kind), videoID, key, string(job.state), runAfter.Unix(),
			now.Unix(), now.Unix()); err != nil {
			t.Fatalf("insert %+v: %v", job, err)
		}
	}
}

// The import group counts videos, not stages; a job waiting out a retry delay is left to do; finished jobs and jobs
// of a video only under a switched-off folder are not; a job that names no video is.
func TestBackgroundRemainingCountsUnfinishedWorkTheOwnerLists(t *testing.T) {
	f := switchedOffFolderFixture(t)
	insertBackgroundJobs(t, f.db,
		backgroundJob{kind: domain.JobProbe, videoID: f.shown, state: domain.JobQueued},
		backgroundJob{kind: domain.JobThumbnail, videoID: f.shown, state: domain.JobRunning},
		backgroundJob{kind: domain.JobPreview, videoID: f.outID, state: domain.JobQueued},
		backgroundJob{kind: domain.JobAutoTag, key: "key-on", state: domain.JobQueued,
			runAfter: time.Now().Add(time.Hour)},
		backgroundJob{kind: domain.JobAutoTag, key: "key-off", state: domain.JobQueued},
		backgroundJob{kind: domain.JobKeyframes, videoID: f.shown, state: domain.JobDone},
		backgroundJob{kind: domain.JobMediaInfo, videoID: f.shown, state: domain.JobFailed},
		backgroundJob{kind: domain.JobQueueKeyframes, state: domain.JobQueued},
	)

	got, err := f.db.Ingest().BackgroundRemaining(context.Background(), owner)
	if err != nil {
		t.Fatal(err)
	}
	want := map[domain.BackgroundGroup]int{
		domain.BackgroundImport: 1, domain.BackgroundAutoTag: 1, domain.BackgroundBackfill: 1,
	}
	if !maps.Equal(got, want) {
		t.Fatalf("remaining = %v, want %v", got, want)
	}
}

// A feature's switch decides its group's featureOff, so each save tells the background status even when no job row
// changes.
func TestFeatureSwitchesTellTheBackgroundStatus(t *testing.T) {
	ctx := context.Background()
	for name, save := range map[string]func(*SettingsStore) error{
		"auto_tag": func(s *SettingsStore) error {
			return s.SaveAutoTagSettings(ctx, domain.AutoTagSettings{Threshold: 0.8, TentativeThreshold: 0.5})
		},
		"face": func(s *SettingsStore) error {
			return s.SaveFaceSettings(ctx, domain.FaceSettings{Recognizer: domain.DefaultFaceRecognizer})
		},
		"version_judge": func(s *SettingsStore) error { return s.SaveVersionJudgingEnabled(ctx, false) },
	} {
		t.Run(name, func(t *testing.T) {
			db := migratedDB(t)
			recorder := &eventRecorder{}
			db.PublishTo(recorder)
			if err := save(db.Settings()); err != nil {
				t.Fatal(err)
			}
			if !slices.Contains(recorder.events, domain.Event(domain.BackgroundChanged{})) {
				t.Fatalf("events = %v, want BackgroundChanged", recorder.events)
			}
		})
	}
}
