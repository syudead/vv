package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

func queueAutoTag(t *testing.T, store *AutoTagStore, videoID int64, mode domain.AutoTagQueueMode, decider domain.ActivityDecider) {
	t.Helper()
	if n, err := store.QueueVideos(context.Background(), []int64{videoID}, mode, decider); err != nil || n != 1 {
		t.Fatalf("QueueVideos = %d, %v; want 1 queued", n, err)
	}
}

func assertNoDueAutoTag(t *testing.T, db *DB) {
	t.Helper()
	if job, err := db.Ingest().ClaimJob(context.Background(), domain.JobAutoTag); !errors.Is(err, domain.ErrNoJob) {
		t.Fatalf("ClaimJob = %+v, %v; want domain.ErrNoJob", job, err)
	}
}

// A failed classification waits out the retry delay with its entry waiting, then is claimed again, and its success
// clears the failure from the entry; before jobs it stayed failed until the owner queued it again.
func TestAutoTagFailureRetriesAfterItsDelay(t *testing.T) {
	db, ids := itemsFixture(t)
	now := fixJobClock(t)
	// The middle of the spread is the delay itself.
	previousJitter := jobJitter
	jobJitter = func() float64 { return 0.5 }
	t.Cleanup(func() { jobJitter = previousJitter })
	store := db.AutoTags()
	soloKey := "key" + fixturePath("/media/solo.mp4")
	queueAutoTag(t, store, ids[fixturePath("/media/solo.mp4")], domain.AutoTagQueueIfNew, domain.ActivityByRule)

	job := claimAutoTag(t, store)
	if err := db.Ingest().FailClaimedJob(context.Background(), job, errors.New("cannot reach Ollama")); err != nil {
		t.Fatal(err)
	}
	if got := autoTagState(t, db, soloKey); got != "queued" {
		t.Fatalf("state after the first failure = %q, want queued", got)
	}
	if wait := jobWait(t, db, job.ID, *now); wait != 30 {
		t.Fatalf("wait = %ds, want 30s", wait)
	}
	waiting := latestAutoTagEntry(t, db)
	if entryState(waiting) != activityWaiting || waiting.Detail != "cannot reach Ollama" {
		t.Fatalf("entry while waiting = %+v, want waiting with the failure", waiting)
	}
	*now = now.Add(29 * time.Second)
	assertNoDueAutoTag(t, db)
	*now = now.Add(time.Second)
	if again := claimAutoTag(t, store); again.ID != job.ID || again.Attempts != 2 {
		t.Fatalf("claim after the delay = %+v, want job %d on its second attempt", again, job.ID)
	}
	if got := latestAutoTagEntry(t, db); got.ID != waiting.ID || got.Reason != domain.ActivityProcessing {
		t.Fatalf("entry on the retry = %+v, want entry %d processing", got, waiting.ID)
	}
	if err := store.Finish(context.Background(), soloKey, domain.AutoTagSelection{}, domain.ActivityByAI); err != nil {
		t.Fatal(err)
	}
	if done := latestAutoTagEntry(t, db); entryState(done) != activityDone || done.Detail != "" {
		t.Fatalf("entry after the retry succeeded = %+v, want done without the earlier failure", done)
	}
}

// The owner's retry is claimed before after-import tagging queued earlier, and an owner request on a queued job moves
// it first.
func TestAutoTagOwnerRequestsAreClaimedFirst(t *testing.T) {
	db, ids := itemsFixture(t)
	ctx := context.Background()
	store := db.AutoTags()
	solo, p1, p2 := ids[fixturePath("/media/solo.mp4")], ids[fixturePath("/media/pair/p1.mp4")],
		ids[fixturePath("/media/pair/p2.mp4")]
	queueAutoTag(t, store, solo, domain.AutoTagQueueIfNew, domain.ActivityByRule)
	failed := claimAutoTag(t, store)
	if err := failAutoTag(ctx, store, failed.ContentKey, "cannot reach Ollama"); err != nil {
		t.Fatal(err)
	}
	queueAutoTag(t, store, p1, domain.AutoTagQueueIfNew, domain.ActivityByRule)
	queueAutoTag(t, store, p2, domain.AutoTagQueueIfNew, domain.ActivityByRule)

	if n, err := store.RetryVideo(ctx, solo, latestAutoTagEntry(t, db).ID); err != nil || n != 1 {
		t.Fatalf("RetryVideo = %d, %v", n, err)
	}
	if job := claimAutoTag(t, store); job.ContentKey != failed.ContentKey || job.Priority != domain.JobPriorityOwner {
		t.Fatalf("first claim = %+v, want the owner's retry of %q", job, failed.ContentKey)
	}
	if n, err := store.QueueVideos(ctx, []int64{p2}, domain.AutoTagQueueAgain, domain.ActivityByOwner); err != nil || n != 0 {
		t.Fatalf("QueueVideos over a queued job = %d, %v; want nothing new queued", n, err)
	}
	if job := claimAutoTag(t, store); job.ContentKey != "key"+fixturePath("/media/pair/p2.mp4") {
		t.Fatalf("second claim = %q, want the owner's video p2 before the older p1", job.ContentKey)
	}
}

// Content whose last registered video is gone is dropped at claim with its entry completed, and the next job is
// claimed instead.
func TestAutoTagClaimDropsContentWithoutARegisteredVideo(t *testing.T) {
	db, ids := itemsFixture(t)
	store := db.AutoTags()
	queueAutoTag(t, store, ids[fixturePath("/media/solo.mp4")], domain.AutoTagQueueIfNew, domain.ActivityByRule)
	queueAutoTag(t, store, ids[fixturePath("/media/pair/p1.mp4")], domain.AutoTagQueueIfNew, domain.ActivityByRule)
	soloKey := "key" + fixturePath("/media/solo.mp4")
	execAll(t, db, `delete from video_locations where video_id = (select id from videos where content_key = '`+
		soloKey+`')`)

	if job := claimAutoTag(t, store); job.ContentKey != "key"+fixturePath("/media/pair/p1.mp4") {
		t.Fatalf("claim = %q, want p1 after dropping solo", job.ContentKey)
	}
	if got := autoTagState(t, db, soloKey); got != "" {
		t.Fatalf("state of the dropped content = %q, want none", got)
	}
	for _, entry := range autoTagEntries(t, db) {
		if entry.Subject.Key == soloKey && entry.Outcome != domain.ActivityCompleted {
			t.Fatalf("dropped entry = %+v, want completed", entry)
		}
	}
}
