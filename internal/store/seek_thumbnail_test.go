package store

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"reflect"
	"testing"

	"github.com/pressly/goose/v3"

	"github.com/syudead/vv/internal/domain"
)

func seekAndThumbnailState(t *testing.T, db *DB, videoID int64) (string, string) {
	t.Helper()
	var seek, thumbnail string
	if err := db.sql.QueryRow(`select seek_thumbnail_state, thumbnail_state from videos where id = ?`, videoID).
		Scan(&seek, &thumbnail); err != nil {
		t.Fatal(err)
	}
	return seek, thumbnail
}

// Seek jobs wait for probe completion and all registered thumbnail work, including thumbnails still waiting on their probes.
func TestClaimSeekThumbnailWaitsForProbeAndRemainingThumbnails(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	other, err := db.ScanIndex().UpsertVideo(ctx, sampleFile(fixturePath("/media/b.mp4"), "b", "key-b", 2048, 0))
	if err != nil {
		t.Fatal(err)
	}
	outside, err := db.ScanIndex().UpsertVideo(ctx, sampleFile(fixturePath("/elsewhere/c.mp4"), "c", "key-c", 4096, 0))
	if err != nil {
		t.Fatal(err)
	}
	for _, job := range []struct {
		kind domain.JobKind
		id   int64
	}{
		{domain.JobSeekThumbnail, videoID}, {domain.JobThumbnail, other.ID}, {domain.JobThumbnail, outside.ID},
	} {
		if err := db.Ingest().EnqueueJob(ctx, job.kind, job.id, domain.ActivityByRule); err != nil {
			t.Fatal(err)
		}
	}
	noJob := func(step string) {
		t.Helper()
		if _, err := db.Ingest().ClaimJob(ctx, domain.JobSeekThumbnail); !errors.Is(err, domain.ErrNoJob) {
			t.Fatalf("%s: ClaimJob error = %v, want domain.ErrNoJob", step, err)
		}
	}

	noJob("解析前")
	probeDone(t, db, videoID)
	// The other thumbnail still blocks seek work even though its own probe is pending.
	noJob("解析待ちの代表サムネイルが残る間")
	probeDone(t, db, other.ID)
	thumbnail, err := db.Ingest().ClaimJob(ctx, domain.JobThumbnail)
	if err != nil {
		t.Fatal(err)
	}
	if thumbnail.VideoID != other.ID {
		t.Fatalf("thumbnail VideoID = %d, want %d", thumbnail.VideoID, other.ID)
	}
	noJob("代表サムネイルが処理中の間")
	if err := db.Ingest().CompleteClaimedJob(ctx, thumbnail); err != nil {
		t.Fatal(err)
	}

	job, err := db.Ingest().ClaimJob(ctx, domain.JobSeekThumbnail)
	if err != nil {
		t.Fatalf("代表サムネイルの残りが登録外だけになっても取り出せない: %v", err)
	}
	if job.VideoID != videoID || job.Kind != domain.JobSeekThumbnail {
		t.Fatalf("claimed = %+v, want seek_thumbnail of video %d", job, videoID)
	}
}

func TestRemainingJobsCountSeekThumbnailSeparately(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	other, err := db.ScanIndex().UpsertVideo(ctx, sampleFile(fixturePath("/media/b.mp4"), "b", "key-b", 2048, 0))
	if err != nil {
		t.Fatal(err)
	}
	for _, job := range []struct {
		kind domain.JobKind
		id   int64
	}{
		{domain.JobThumbnail, videoID}, {domain.JobSeekThumbnail, videoID}, {domain.JobSeekThumbnail, other.ID},
	} {
		if err := db.Ingest().EnqueueJob(ctx, job.kind, job.id, domain.ActivityByRule); err != nil {
			t.Fatal(err)
		}
	}
	got := remainingByKind(t, db)
	want := map[domain.JobKind]int{domain.JobThumbnail: 1, domain.JobSeekThumbnail: 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("残り = %v, want %v", got, want)
	}
}

func TestFailClaimedSeekThumbnailMarksOnlySeekState(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	probeDone(t, db, videoID)

	seekJob := claimAtLastAttempt(t, db, domain.JobSeekThumbnail, videoID)
	if err := db.Ingest().FailClaimedJob(ctx, seekJob, errors.New("ffmpeg failed")); err != nil {
		t.Fatal(err)
	}
	if seek, thumbnail := seekAndThumbnailState(t, db, videoID); seek != "failed" || thumbnail != "pending" {
		t.Fatalf("seek = %s, thumbnail = %s, want failed and pending", seek, thumbnail)
	}

	if _, err := db.sql.Exec(`update videos set seek_thumbnail_state = 'pending' where id = ?`, videoID); err != nil {
		t.Fatal(err)
	}
	thumbnailJob := claimAtLastAttempt(t, db, domain.JobThumbnail, videoID)
	if err := db.Ingest().FailClaimedJob(ctx, thumbnailJob, errors.New("ffmpeg failed")); err != nil {
		t.Fatal(err)
	}
	if seek, thumbnail := seekAndThumbnailState(t, db, videoID); seek != "pending" || thumbnail != "failed" {
		t.Fatalf("seek = %s, thumbnail = %s, want pending and failed", seek, thumbnail)
	}
}

func TestFailClaimedSeekThumbnailKeepsDone(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	probeDone(t, db, videoID)
	job := claimAtLastAttempt(t, db, domain.JobSeekThumbnail, videoID)
	if _, err := db.sql.Exec(`update videos set seek_thumbnail_state = 'done' where id = ?`, videoID); err != nil {
		t.Fatal(err)
	}
	if err := db.Ingest().FailClaimedJob(ctx, job, errors.New("late failure")); err != nil {
		t.Fatal(err)
	}
	if seek, _ := seekAndThumbnailState(t, db, videoID); seek != "done" {
		t.Fatalf("seek = %s, want done", seek)
	}
}

func TestSetSeekThumbnailStateForJobRequiresClaimIdentity(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	probeDone(t, db, videoID)
	if err := db.Ingest().EnqueueJob(ctx, domain.JobSeekThumbnail, videoID, domain.ActivityByRule); err != nil {
		t.Fatal(err)
	}
	job, err := db.Ingest().ClaimJob(ctx, domain.JobSeekThumbnail)
	if err != nil {
		t.Fatal(err)
	}

	stale := job
	stale.LocationGeneration++
	if applied, err := db.Ingest().SetSeekThumbnailStateForJob(ctx, stale, domain.SeekThumbnailDone, domain.SubstitutionNone); err != nil || applied {
		t.Fatalf("古い世代の記録 = %v, %v, want false", applied, err)
	}
	if seek, _ := seekAndThumbnailState(t, db, videoID); seek != "pending" {
		t.Fatalf("seek = %s, want pending", seek)
	}
	if applied, err := db.Ingest().SetSeekThumbnailStateForJob(ctx, job, domain.SeekThumbnailDone, domain.SubstitutionNone); err != nil || !applied {
		t.Fatalf("SetSeekThumbnailStateForJob = %v, %v, want true", applied, err)
	}
	video, err := db.Library().GetVideo(ctx, domain.AudienceOwner, videoID)
	if err != nil {
		t.Fatal(err)
	}
	if video.SeekThumbnailState != domain.SeekThumbnailDone || video.ThumbnailState != domain.ThumbnailStatePending {
		t.Fatalf("seek = %s, thumbnail = %s, want done and pending", video.SeekThumbnailState, video.ThumbnailState)
	}
}

func TestRetryProbeRequeuesFailedSeekThumbnail(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	for _, kind := range []domain.JobKind{domain.JobProbe, domain.JobSeekThumbnail} {
		job := claimAtLastAttempt(t, db, kind, videoID)
		if err := db.Ingest().FailClaimedJob(ctx, job, errors.New(string(kind)+" failed")); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.sql.Exec(`update videos set thumbnail_state = 'done' where id = ?`, videoID); err != nil {
		t.Fatal(err)
	}
	recorder := &queuedRecorder{}
	db.PublishTo(recorder)

	if err := db.Ingest().RetryProbe(ctx, videoID); err != nil {
		t.Fatal(err)
	}
	if seek, thumbnail := seekAndThumbnailState(t, db, videoID); seek != "pending" || thumbnail != "done" {
		t.Fatalf("seek = %s, thumbnail = %s, want pending and done", seek, thumbnail)
	}
	want := map[string]int{"probe:queued": 1, "seek_thumbnail:queued": 1}
	if got := jobCounts(t, db, videoID); !equalCounts(got, want) {
		t.Fatalf("jobs = %v, want %v", got, want)
	}
	if got := recorder.take(); fmt.Sprint(got) != "[probe seek_thumbnail]" {
		t.Errorf("知らせ = %v, want [probe seek_thumbnail]", got)
	}
}

func TestRequeueMissingSeekThumbnails(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	probeDone(t, db, videoID)

	if requeued, err := db.Ingest().RequeueMissingSeekThumbnails(ctx, videoID, "key-a"); err != nil || requeued {
		t.Fatalf("pending の動画 = %v, %v, want false", requeued, err)
	}
	if _, err := db.sql.Exec(`update videos set seek_thumbnail_state = 'done' where id = ?`, videoID); err != nil {
		t.Fatal(err)
	}
	recorder := &queuedRecorder{}
	db.PublishTo(recorder)
	if requeued, err := db.Ingest().RequeueMissingSeekThumbnails(ctx, videoID, "other-key"); err != nil || requeued {
		t.Fatalf("内容の違う要求 = %v, %v, want false", requeued, err)
	}
	requeued, err := db.Ingest().RequeueMissingSeekThumbnails(ctx, videoID, "key-a")
	if err != nil || !requeued {
		t.Fatalf("RequeueMissingSeekThumbnails = %v, %v, want true", requeued, err)
	}
	if got := recorder.take(); fmt.Sprint(got) != "[seek_thumbnail]" {
		t.Errorf("知らせ = %v, want [seek_thumbnail]", got)
	}
	if seek, _ := seekAndThumbnailState(t, db, videoID); seek != "pending" {
		t.Errorf("seek_thumbnail_state = %s, want pending", seek)
	}
	if requeued, err := db.Ingest().RequeueMissingSeekThumbnails(ctx, videoID, "key-a"); err != nil || requeued {
		t.Fatalf("2度目 = %v, %v, want false", requeued, err)
	}
	if got := jobCounts(t, db, videoID); !equalCounts(got, map[string]int{"seek_thumbnail:queued": 1}) {
		t.Fatalf("jobs = %v, want one queued seek_thumbnail", got)
	}
}

func TestUpsertVideoReportsSeekThumbnailNeed(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	added, err := db.ScanIndex().UpsertVideo(ctx, sampleFile(fixturePath("/media/a.mp4"), "a", "key-a", 1024, 0))
	if err != nil {
		t.Fatal(err)
	}
	if !added.NeedsSeekThumbnail {
		t.Fatal("新しい内容で NeedsSeekThumbnail が false")
	}
	if _, err := db.sql.Exec(`update videos set seek_thumbnail_state = 'done' where id = ?`, added.ID); err != nil {
		t.Fatal(err)
	}
	moved, err := db.ScanIndex().UpsertVideo(ctx, sampleFile(fixturePath("/media/b.mp4"), "b", "key-a", 1024, 0))
	if err != nil {
		t.Fatal(err)
	}
	if moved.ID != added.ID || moved.NeedsSeekThumbnail {
		t.Fatalf("done の内容の別の所在 = %+v, want same video without seek need", moved)
	}
	indexed, err := db.ScanIndex().IndexedVideosByPath(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := indexed[fixturePath("/media/a.mp4")].SeekThumbnailState; got != domain.SeekThumbnailDone {
		t.Fatalf("IndexedVideo.SeekThumbnailState = %q, want done", got)
	}

	if _, err := db.sql.Exec(`update videos set seek_thumbnail_state = 'pending' where id = ?`, added.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.Ingest().EnsureJob(ctx, domain.JobSeekThumbnail, added.ID, domain.ActivityByRule); err != nil {
		t.Fatal(err)
	}
	if got := jobCounts(t, db, added.ID); !equalCounts(got, map[string]int{"seek_thumbnail:queued": 1}) {
		t.Fatalf("jobs = %v, want one queued seek_thumbnail", got)
	}
}

func TestSeekThumbnailStageMigrationBackfillsAndRollsBack(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	fsy, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, db.sql, fsy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(context.Background(), 13); err != nil {
		t.Fatal(err)
	}
	ids := map[string]int64{}
	for _, item := range []struct {
		key, probe, thumbnail string
		located               bool
	}{
		{key: "done", probe: "done", thumbnail: "done", located: true},
		{key: "failed", probe: "failed", thumbnail: "failed", located: true},
		{key: "pending", probe: "pending", thumbnail: "pending", located: true},
		{key: "unlocated", probe: "done", thumbnail: "done", located: false},
	} {
		res, err := db.sql.Exec(`insert into videos(content_key, probe_state, thumbnail_state) values (?, ?, ?)`,
			item.key, item.probe, item.thumbnail)
		if err != nil {
			t.Fatal(err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			t.Fatal(err)
		}
		ids[item.key] = id
		if item.located {
			if _, err := db.sql.Exec(`insert into video_locations
				(video_id, path, title, size_bytes, mtime, created_at, updated_at)
				values (?, ?, ?, 1, 1, 1, 1)`, id, fixturePath("/media/")+item.key+".mp4", item.key); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := db.sql.Exec(`insert into jobs(kind, video_id, state, attempts, created_at, updated_at)
		values ('thumbnail', ?, 'running', 1, 1, 1)`, ids["pending"]); err != nil {
		t.Fatal(err)
	}

	if _, err := provider.UpTo(context.Background(), 14); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]map[string]int{
		"done":      {"seek_thumbnail:queued": 1},
		"failed":    {"seek_thumbnail:queued": 1},
		"pending":   {"thumbnail:running": 1},
		"unlocated": {},
	} {
		if got := jobCounts(t, db, ids[key]); !equalCounts(got, want) {
			t.Errorf("%s: jobs = %v, want %v", key, got, want)
		}
		if seek, _ := seekAndThumbnailState(t, db, ids[key]); seek != "pending" {
			t.Errorf("%s: seek_thumbnail_state = %s, want pending", key, seek)
		}
	}
	if _, err := db.sql.Exec(`update videos set seek_thumbnail_state = 'invalid' where id = ?`, ids["done"]); err == nil {
		t.Fatal("seek_thumbnail_state accepted an invalid value")
	}
	if _, err := db.sql.Exec(`insert into jobs(kind, video_id, state, created_at, updated_at)
		values ('seek_thumbnail', ?, 'queued', 1, 1)`, ids["done"]); err == nil {
		t.Fatal("unique index over pending jobs was not rebuilt")
	}

	if _, err := provider.DownTo(context.Background(), 13); err != nil {
		t.Fatal(err)
	}
	if _, ok := tableColumns(t, db, "videos")["seek_thumbnail_state"]; ok {
		t.Fatal("down kept seek_thumbnail_state")
	}
	if got := jobCounts(t, db, ids["done"]); !equalCounts(got, map[string]int{}) {
		t.Errorf("down kept seek_thumbnail jobs: %v", got)
	}
	if got := jobCounts(t, db, ids["pending"]); !equalCounts(got, map[string]int{"thumbnail:running": 1}) {
		t.Errorf("down lost thumbnail jobs: %v", got)
	}
	if _, err := db.sql.Exec(`insert into jobs(kind, video_id, state, created_at, updated_at)
		values ('seek_thumbnail', ?, 'queued', 1, 1)`, ids["done"]); err == nil {
		t.Fatal("down kept seek_thumbnail in the kind check")
	}
	if _, err := Migrate(context.Background(), db); err != nil {
		t.Fatalf("up after down: %v", err)
	}
}

func TestSeekThumbnailSpriteMigrationRequeuesDone(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	fsy, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, db.sql, fsy)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := provider.UpTo(ctx, 14); err != nil {
		t.Fatal(err)
	}
	ids := map[string]int64{}
	for _, item := range []struct {
		key, probe, seek string
		located          bool
	}{
		{key: "done", probe: "done", seek: "done", located: true},
		{key: "done-queued", probe: "done", seek: "done", located: true},
		{key: "failed", probe: "done", seek: "failed", located: true},
		{key: "pending", probe: "done", seek: "pending", located: true},
		{key: "unprobed", probe: "pending", seek: "done", located: true},
		{key: "unlocated", probe: "done", seek: "done", located: false},
	} {
		res, err := db.sql.Exec(`insert into videos(content_key, probe_state, thumbnail_state, preview_state,
			seek_thumbnail_state) values (?, ?, 'done', 'done', ?)`, item.key, item.probe, item.seek)
		if err != nil {
			t.Fatal(err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			t.Fatal(err)
		}
		ids[item.key] = id
		if item.located {
			if _, err := db.sql.Exec(`insert into video_locations
				(video_id, path, title, size_bytes, mtime, created_at, updated_at)
				values (?, ?, ?, 1, 1, 1, 1)`, id, fixturePath("/media/")+item.key+".mp4", item.key); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, job := range []struct {
		kind, state, key string
	}{
		{"seek_thumbnail", "running", "done-queued"},
		{"thumbnail", "queued", "done"},
		{"preview", "failed", "done"},
	} {
		if _, err := db.sql.Exec(`insert into jobs(kind, video_id, state, attempts, created_at, updated_at)
			values (?, ?, ?, 1, 1, 1)`, job.kind, ids[job.key], job.state); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := provider.UpTo(ctx, 15); err != nil {
		t.Fatal(err)
	}
	want := map[string]struct {
		seek string
		jobs map[string]int
	}{
		"done":        {"pending", map[string]int{"seek_thumbnail:queued": 1, "thumbnail:queued": 1, "preview:failed": 1}},
		"done-queued": {"pending", map[string]int{"seek_thumbnail:running": 1}},
		"failed":      {"failed", map[string]int{}},
		"pending":     {"pending", map[string]int{}},
		"unprobed":    {"pending", map[string]int{}},
		"unlocated":   {"pending", map[string]int{}},
	}
	for key, w := range want {
		if got := jobCounts(t, db, ids[key]); !equalCounts(got, w.jobs) {
			t.Errorf("%s: jobs = %v, want %v", key, got, w.jobs)
		}
		if seek, thumbnail := seekAndThumbnailState(t, db, ids[key]); seek != w.seek || thumbnail != "done" {
			t.Errorf("%s: seek = %s, thumbnail = %s, want %s and done", key, seek, thumbnail, w.seek)
		}
		var preview string
		if err := db.sql.QueryRow(`select preview_state from videos where id = ?`, ids[key]).Scan(&preview); err != nil {
			t.Fatal(err)
		}
		if preview != "done" {
			t.Errorf("%s: preview_state = %s, want done", key, preview)
		}
	}

	if _, err := provider.DownTo(ctx, 14); err != nil {
		t.Fatal(err)
	}
	if _, ok := tableColumns(t, db, "videos")["seek_thumbnail_state"]; !ok {
		t.Fatal("down dropped seek_thumbnail_state")
	}
	if _, err := Migrate(ctx, db); err != nil {
		t.Fatalf("up after down: %v", err)
	}
	if got := jobCounts(t, db, ids["done"]); got["seek_thumbnail:queued"] != 1 {
		t.Errorf("up after down: done jobs = %v, want one queued seek_thumbnail", got)
	}
}
