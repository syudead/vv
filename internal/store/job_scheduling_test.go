package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// fixJobClock stops the claim and failure clock an hour ahead of the real one, so rows queued with the real time are
// already due; the test moves the returned time.
func fixJobClock(t *testing.T) *time.Time {
	t.Helper()
	now := time.Now().Add(time.Hour).Truncate(time.Second)
	jobNow = func() time.Time { return now }
	t.Cleanup(func() { jobNow = time.Now })
	return &now
}

func setJobRetryPolicy(t *testing.T, policy domain.JobRetryPolicy) {
	t.Helper()
	jobRetryPolicy = func(domain.JobKind) domain.JobRetryPolicy { return policy }
	t.Cleanup(func() { jobRetryPolicy = domain.JobRetryPolicyFor })
}

func addVideo(t *testing.T, db *DB, name string) int64 {
	t.Helper()
	added, err := db.ScanIndex().UpsertVideo(context.Background(),
		sampleFile(fixturePath("/media/"+name+".mp4"), name, "key-"+name, 1024, 0))
	if err != nil {
		t.Fatal(err)
	}
	return added.ID
}

func enqueueProbe(t *testing.T, db *DB, videoID int64) {
	t.Helper()
	if err := db.Ingest().EnqueueJob(context.Background(), domain.JobProbe, videoID, domain.ActivityByRule); err != nil {
		t.Fatal(err)
	}
}

func claimProbe(t *testing.T, db *DB) domain.Job {
	t.Helper()
	job, err := db.Ingest().ClaimJob(context.Background(), domain.JobProbe)
	if err != nil {
		t.Fatalf("ClaimJob: %v", err)
	}
	return job
}

func assertNoDueProbe(t *testing.T, db *DB) {
	t.Helper()
	if job, err := db.Ingest().ClaimJob(context.Background(), domain.JobProbe); !errors.Is(err, domain.ErrNoJob) {
		t.Fatalf("ClaimJob = %+v, %v; want domain.ErrNoJob", job, err)
	}
}

func failJob(t *testing.T, db *DB, job domain.Job, cause error) {
	t.Helper()
	if err := db.Ingest().FailClaimedJob(context.Background(), job, cause); err != nil {
		t.Fatal(err)
	}
}

// jobWait is the job's run-after time minus now, in seconds.
func jobWait(t *testing.T, db *DB, id int64, now time.Time) int64 {
	t.Helper()
	return countRows(t, db, fmt.Sprintf(`select run_after - %d from jobs where id = %d`, now.Unix(), id))
}

func TestClaimJobTakesOwnerClassBeforeOlderImport(t *testing.T) {
	db := migratedDB(t)
	older, newer := addVideo(t, db, "older"), addVideo(t, db, "newer")
	enqueueProbe(t, db, older)
	enqueueProbe(t, db, newer)
	if err := db.Ingest().ForceJobs(context.Background(), newer, domain.ActivityByOwner, domain.JobProbe); err != nil {
		t.Fatal(err)
	}

	if job := claimProbe(t, db); job.VideoID != newer {
		t.Fatalf("first claim = video %d, want the owner's video %d", job.VideoID, newer)
	}
	if job := claimProbe(t, db); job.VideoID != older {
		t.Fatalf("second claim = video %d, want video %d", job.VideoID, older)
	}
}

func TestFailedRoundsWaitGrowingDelaysBeforeTheirRunAfterTime(t *testing.T) {
	db, videoID := jobsFixture(t)
	now := fixJobClock(t)
	// One round more than the default limit shows the third delay.
	setJobRetryPolicy(t, domain.JobRetryPolicy{MaxAttempts: domain.MaxJobAttempts + 1})
	enqueueProbe(t, db, videoID)

	for round, base := range []int64{30, 120, 480} {
		job := claimProbe(t, db)
		failJob(t, db, job, errors.New("unreadable"))
		wait := jobWait(t, db, job.ID, *now)
		if wait < base*9/10 || wait > base*11/10 {
			t.Fatalf("round %d: wait = %ds, want %ds ±10%%", round+1, wait, base)
		}
		*now = now.Add(time.Duration(wait-1) * time.Second)
		assertNoDueProbe(t, db)
		claimedAt := *now
		*now = now.Add(time.Second)
		// The job came due after the claim, so the lookup from the claim still reports it.
		next, err := db.Ingest().NextJobRunAfter(context.Background(), claimedAt)
		if err != nil || !next.Equal(*now) {
			t.Fatalf("round %d: NextJobRunAfter = %v, %v; want the job's run-after time %v", round+1, next, err, *now)
		}
		// A claim from now sees the due job, so the lookup must not report it again.
		if next, err := db.Ingest().NextJobRunAfter(context.Background(), *now); err != nil || !next.IsZero() {
			t.Fatalf("round %d: NextJobRunAfter from the due time = %v, %v; want none", round+1, next, err)
		}
	}
	job := claimProbe(t, db)
	failJob(t, db, job, errors.New("unreadable"))
	if got := jobState(t, db, job.ID); got != "failed" {
		t.Fatalf("state after the last round = %s, want failed", got)
	}
}

func TestFailureInsideARoundRequeuesAtOnce(t *testing.T) {
	db := migratedDB(t)
	fixJobClock(t)
	ctx := context.Background()
	var videoID int64
	for i := range 2 {
		added, err := db.ScanIndex().UpsertVideo(ctx,
			sampleFile(fmt.Sprintf(fixturePath("/media/%d/movie.mp4"), i), "movie", "shared", 1, 0))
		if err != nil {
			t.Fatal(err)
		}
		videoID = added.ID
	}
	enqueueProbe(t, db, videoID)

	first := claimProbe(t, db)
	failJob(t, db, first, errors.New("unreadable"))
	if second := claimProbe(t, db); second.LocationPath == first.LocationPath || second.Attempts != 1 {
		t.Fatalf("claim after the first location = %+v, want the other location in round 1", second)
	}
}

func TestRepeatingFailureFailsAtItsRoundEndWhileAPlainOneRequeues(t *testing.T) {
	db := migratedDB(t)
	fixJobClock(t)
	broken, flaky := addVideo(t, db, "broken"), addVideo(t, db, "flaky")
	enqueueProbe(t, db, broken)
	enqueueProbe(t, db, flaky)

	for range 2 {
		job := claimProbe(t, db)
		cause := errors.New("Input/output error")
		if job.VideoID == broken {
			cause = domain.FailureRepeats(errors.New("Invalid data found when processing input"))
		}
		failJob(t, db, job, cause)
		want := map[bool]string{true: "failed", false: "queued"}[job.VideoID == broken]
		if got := jobState(t, db, job.ID); got != want {
			t.Fatalf("video %d after round 1 = %s, want %s", job.VideoID, got, want)
		}
	}
}

func TestOverriddenRetryPolicyReplacesDelayAndLimit(t *testing.T) {
	db, videoID := jobsFixture(t)
	now := fixJobClock(t)
	setJobRetryPolicy(t, domain.JobRetryPolicy{Delay: func(int) time.Duration { return 10 * time.Minute }})
	enqueueProbe(t, db, videoID)

	for round := 1; round <= domain.MaxJobAttempts+1; round++ {
		job := claimProbe(t, db)
		failJob(t, db, job, errors.New("unanswered"))
		if got, wait := jobState(t, db, job.ID), jobWait(t, db, job.ID, *now); got != "queued" || wait != 600 {
			t.Fatalf("round %d: state = %s, wait = %ds; want queued after 600s", round, got, wait)
		}
		*now = now.Add(10 * time.Minute)
	}
}

func TestOwnerRequestEndsARetryWait(t *testing.T) {
	db, videoID := jobsFixture(t)
	fixJobClock(t)
	enqueueProbe(t, db, videoID)
	failJob(t, db, claimProbe(t, db), errors.New("unreadable"))
	assertNoDueProbe(t, db)

	if err := db.Ingest().ForceJobs(context.Background(), videoID, domain.ActivityByOwner, domain.JobProbe); err != nil {
		t.Fatal(err)
	}
	if job := claimProbe(t, db); job.Attempts != 2 || !job.Force {
		t.Fatalf("claim after the request = %+v, want the forced job in round 2", job)
	}
}

func TestStagesQueuedByAnOwnerRetryKeepTheOwnerClass(t *testing.T) {
	db, retried := failedVideoFixture(t)
	imported := addVideo(t, db, "imported")
	ctx := context.Background()
	if err := db.Ingest().RetryProbe(ctx, retried); err != nil {
		t.Fatal(err)
	}
	enqueueProbe(t, db, imported)

	for _, want := range []struct {
		videoID  int64
		priority domain.JobPriority
	}{{retried, domain.JobPriorityOwner}, {imported, domain.JobPriorityImport}} {
		job := claimProbe(t, db)
		if job.VideoID != want.videoID || job.Priority != want.priority {
			t.Fatalf("claimed video %d in class %d, want video %d in class %d", job.VideoID, job.Priority, want.videoID, want.priority)
		}
		if written, err := db.Ingest().ApplyProbeForJob(ctx, job, domain.Probe{VideoCodec: "h264"},
			domain.Playability{Playable: true}); err != nil || !written {
			t.Fatalf("ApplyProbeForJob = %v, %v", written, err)
		}
		got := countRows(t, db, fmt.Sprintf(`select priority from jobs where kind = 'preview' and video_id = %d`, job.VideoID))
		if got != int64(want.priority) {
			t.Fatalf("video %d: preview class = %d, want %d", job.VideoID, got, want.priority)
		}
	}
}

func TestStartupRecoveryRaisesAForceRequestToTheOwnerClass(t *testing.T) {
	db := migratedDB(t)
	requested, plain := addVideo(t, db, "requested"), addVideo(t, db, "plain")
	enqueueProbe(t, db, requested)
	enqueueProbe(t, db, plain)
	requestedJob, plainJob := claimProbe(t, db), claimProbe(t, db)
	ctx := context.Background()
	if err := db.Ingest().ForceJobs(ctx, requested, domain.ActivityByOwner, domain.JobProbe); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Ingest().RequeueRunningJobs(ctx); err != nil {
		t.Fatal(err)
	}

	for _, want := range []struct {
		id       int64
		force    int64
		priority domain.JobPriority
	}{{requestedJob.ID, 1, domain.JobPriorityOwner}, {plainJob.ID, 0, domain.JobPriorityImport}} {
		force := countRows(t, db, fmt.Sprintf(`select force from jobs where id = %d`, want.id))
		priority := countRows(t, db, fmt.Sprintf(`select priority from jobs where id = %d`, want.id))
		if force != want.force || priority != int64(want.priority) {
			t.Fatalf("job %d: force = %d, class = %d; want %d, %d", want.id, force, priority, want.force, want.priority)
		}
	}
}

func TestStartupRecoveryCountsTheInterruptedClaim(t *testing.T) {
	db, videoID := jobsFixture(t)
	enqueueProbe(t, db, videoID)
	claimProbe(t, db)

	if _, err := db.Ingest().RequeueRunningJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	if job := claimProbe(t, db); job.Attempts != 2 {
		t.Fatalf("attempts after recovery = %d, want 2", job.Attempts)
	}
}

func TestSnoozedJobRequeuesAtOnceWithoutAnAttempt(t *testing.T) {
	db, videoID := jobsFixture(t)
	fixJobClock(t)
	enqueueProbe(t, db, videoID)
	failJob(t, db, claimProbe(t, db), fmt.Errorf("root unreadable: %w", domain.ErrJobSnoozed))

	if job := claimProbe(t, db); job.Attempts != 1 {
		t.Fatalf("attempts after a snooze = %d, want 1", job.Attempts)
	}
}
