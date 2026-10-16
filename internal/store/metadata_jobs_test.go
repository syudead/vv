package store

import (
	"context"
	"errors"
	"io/fs"
	"slices"
	"testing"

	"github.com/pressly/goose/v3"

	"github.com/syudead/vv/internal/domain"
)

// 解析し直しの仕事（specs/056-media-info/research.md R-5、data-model.md「Rules」）。

// 00051 は解析が done で登録フォルダの下に所在がある動画にだけ解析し直しの仕事を積み、
// 既存の仕事を残す。Down は解析し直しの仕事を消し、ほかの仕事を戻す。
func TestMetadataJobsMigrationBackfillsAndRollsBack(t *testing.T) {
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
	if _, err := provider.UpTo(ctx, 50); err != nil {
		t.Fatal(err)
	}
	if _, err := db.sql.Exec(`insert into media_folders(path, version, created_at, updated_at) values (?, 1, 1, 1)`,
		fixturePath("/media")); err != nil {
		t.Fatal(err)
	}
	ids := map[string]int64{}
	for _, item := range []struct {
		key, probe, dir string
	}{
		{key: "done", probe: "done", dir: "/media/"},
		{key: "pending", probe: "pending", dir: "/media/"},
		{key: "failed", probe: "failed", dir: "/media/"},
		{key: "unregistered", probe: "done", dir: "/other/"},
		{key: "unlocated", probe: "done"},
	} {
		res, err := db.sql.Exec(`insert into videos(content_key, probe_state) values (?, ?)`, item.key, item.probe)
		if err != nil {
			t.Fatal(err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			t.Fatal(err)
		}
		ids[item.key] = id
		if item.dir != "" {
			if _, err := db.sql.Exec(`insert into video_locations
				(video_id, path, title, size_bytes, mtime, created_at, updated_at)
				values (?, ?, ?, 1, 1, 1, 1)`, id, fixturePath(item.dir)+item.key+".mp4", item.key); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := db.sql.Exec(`insert into jobs(kind, video_id, state, attempts, last_error, created_at, updated_at)
		values ('thumbnail', ?, 'failed', 3, 'boom', 1, 1)`, ids["done"]); err != nil {
		t.Fatal(err)
	}

	if _, err := provider.UpTo(ctx, 51); err != nil {
		t.Fatal(err)
	}
	if got := jobCounts(t, db, ids["done"]); !equalCounts(got, map[string]int{"metadata:queued": 1, "thumbnail:failed": 1}) {
		t.Errorf("done の jobs = %v", got)
	}
	for _, key := range []string{"pending", "failed", "unregistered", "unlocated"} {
		if got := jobCounts(t, db, ids[key]); len(got) != 0 {
			t.Errorf("%s の jobs = %v, want none", key, got)
		}
	}

	if _, err := provider.DownTo(ctx, 50); err != nil {
		t.Fatal(err)
	}
	if got := jobCounts(t, db, ids["done"]); !equalCounts(got, map[string]int{"thumbnail:failed": 1}) {
		t.Errorf("Down のあとの done の jobs = %v", got)
	}
	if _, err := db.sql.Exec(`insert into jobs(kind, video_id, state, attempts, created_at, updated_at)
		values ('metadata', ?, 'queued', 0, 1, 1)`, ids["done"]); err == nil {
		t.Error("Down のあとも metadata の仕事を積める")
	}
}

// 解析の仕事が待ちか実行中の間は、解析し直しの仕事を取り出さない。登録外の所在だけを持つ動画の
// 解析の仕事は数えない。解析の仕事が終われば取り出す。
func TestMetadataJobWaitsForClaimableProbes(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	probeDone(t, db, videoID)
	other, err := db.ScanIndex().UpsertVideo(ctx, sampleFile(fixturePath("/media/b.mp4"), "b", "key-b", 2048, 0))
	if err != nil {
		t.Fatal(err)
	}
	outside, err := db.ScanIndex().UpsertVideo(ctx, sampleFile(fixturePath("/other/c.mp4"), "c", "key-c", 4096, 0))
	if err != nil {
		t.Fatal(err)
	}
	for _, job := range []struct {
		kind domain.JobKind
		id   int64
	}{{domain.JobMetadata, videoID}, {domain.JobProbe, other.ID}, {domain.JobProbe, outside.ID}} {
		if err := db.Ingest().EnqueueJob(ctx, job.kind, job.id); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := db.Ingest().ClaimJob(ctx, domain.JobMetadata); !errors.Is(err, domain.ErrNoJob) {
		t.Fatalf("解析の仕事が待っている間の取り出し: err = %v, want ErrNoJob", err)
	}
	probe, err := db.Ingest().ClaimJob(ctx, domain.JobProbe)
	if err != nil || probe.VideoID != other.ID {
		t.Fatalf("解析の仕事 = %+v, %v", probe, err)
	}
	if _, err := db.Ingest().ClaimJob(ctx, domain.JobMetadata); !errors.Is(err, domain.ErrNoJob) {
		t.Fatalf("解析の仕事が実行中の間の取り出し: err = %v, want ErrNoJob", err)
	}
	if err := db.Ingest().CompleteClaimedJob(ctx, probe); err != nil {
		t.Fatal(err)
	}
	job, err := db.Ingest().ClaimJob(ctx, domain.JobMetadata)
	if err != nil || job.VideoID != videoID || job.Kind != domain.JobMetadata {
		t.Fatalf("解析のあとの取り出し = %+v, %v", job, err)
	}
}

// 走査の索引は、追加の情報の行が無い動画と古い版の行の動画を MediaInfoMissing にし、EnsureJob が
// 解析し直しの仕事を積む。今の版の行があれば積まない。解析がまだの動画は対象にしない。
func TestScanQueuesMetadataForMissingOrOldMediaInfo(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	missing := func() bool {
		t.Helper()
		indexed, err := db.ScanIndex().IndexedVideosByPath(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return indexed[fixturePath("/media/a.mp4")].MediaInfoMissing
	}
	if missing() {
		t.Fatal("解析がまだの動画が MediaInfoMissing になる")
	}
	if err := db.Ingest().EnsureJob(ctx, domain.JobMetadata, videoID); err != nil {
		t.Fatal(err)
	}
	if got := jobCounts(t, db, videoID); len(got) != 0 {
		t.Fatalf("解析がまだの動画の jobs = %v", got)
	}

	probeDone(t, db, videoID)
	if !missing() {
		t.Fatal("行の無い動画が MediaInfoMissing にならない")
	}
	if err := db.Ingest().EnsureJob(ctx, domain.JobMetadata, videoID); err != nil {
		t.Fatal(err)
	}
	if got := jobCounts(t, db, videoID); !equalCounts(got, map[string]int{"metadata:queued": 1}) {
		t.Fatalf("jobs = %v, want one queued metadata", got)
	}
	job, err := db.Ingest().ClaimJob(ctx, domain.JobMetadata)
	if err != nil {
		t.Fatal(err)
	}
	if applied, err := db.Ingest().ApplyMediaInfoForJob(ctx, job, *sampleMediaInfo()); err != nil || !applied {
		t.Fatalf("ApplyMediaInfoForJob = %v, %v", applied, err)
	}
	if err := db.Ingest().CompleteClaimedJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	if missing() {
		t.Fatal("今の版の行がある動画が MediaInfoMissing になる")
	}
	if err := db.Ingest().EnsureJob(ctx, domain.JobMetadata, videoID); err != nil {
		t.Fatal(err)
	}
	if got := jobCounts(t, db, videoID); !equalCounts(got, map[string]int{"metadata:done": 1}) {
		t.Fatalf("今の版の行がある動画の jobs = %v", got)
	}

	if _, err := db.sql.Exec(`update video_media_info set version = ? where video_id = ?`,
		domain.MediaInfoVersion-1, videoID); err != nil {
		t.Fatal(err)
	}
	if !missing() {
		t.Fatal("古い版の行が MediaInfoMissing にならない")
	}
	// 完了した行が残っていても、版を上げたら積み直す。
	if err := db.Ingest().EnsureJob(ctx, domain.JobMetadata, videoID); err != nil {
		t.Fatal(err)
	}
	if got := jobCounts(t, db, videoID); !equalCounts(got, map[string]int{"metadata:queued": 1}) {
		t.Fatalf("古い版の行の動画の jobs = %v", got)
	}
}

// ApplyMediaInfoForJob は行だけを書き、videos の行に触れない。所在の版が変わっていれば何も書かない。
func TestApplyMediaInfoForJobWritesOnlyTheRowUnderIdentity(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	probeDone(t, db, videoID)
	videoRow := func() string {
		t.Helper()
		var row string
		if err := db.sql.QueryRow(`select probe_state || ':' || duration_ms || ':' || video_codec || ':' || playable ||
			':' || indexed_at from videos where id = ?`, videoID).Scan(&row); err != nil {
			t.Fatal(err)
		}
		return row
	}
	before := videoRow()

	if err := db.Ingest().EnqueueJob(ctx, domain.JobMetadata, videoID); err != nil {
		t.Fatal(err)
	}
	job, err := db.Ingest().ClaimJob(ctx, domain.JobMetadata)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.sql.Exec(`update video_locations set version = version + 1 where id = ?`, job.LocationID); err != nil {
		t.Fatal(err)
	}
	if applied, err := db.Ingest().ApplyMediaInfoForJob(ctx, job, *sampleMediaInfo()); err != nil || applied {
		t.Fatalf("所在の版が変わったあと: applied = %v, err = %v", applied, err)
	}
	if got, err := db.Library().MediaInfo(ctx, videoID); err != nil || got != nil {
		t.Fatalf("所在の版が変わったあとの行 = %+v, %v", got, err)
	}

	job.LocationVersion++
	if applied, err := db.Ingest().ApplyMediaInfoForJob(ctx, job, *sampleMediaInfo()); err != nil || !applied {
		t.Fatalf("applied = %v, err = %v", applied, err)
	}
	if got, err := db.Library().MediaInfo(ctx, videoID); err != nil || got == nil || got.Video.HDR != domain.HDRKindHDR10 {
		t.Fatalf("行 = %+v, %v", got, err)
	}
	if after := videoRow(); after != before {
		t.Fatalf("videos の行が変わった: %s → %s", before, after)
	}
}

// 上限まで失敗した解析し直しの仕事は行をそのまま残し、直近の走査の対象の動画なら
// metadata_failed を記録する。対象でなければ記録しない。次の成功で記録を消す。
func TestFailedMetadataJobKeepsRowAndRecordsIssue(t *testing.T) {
	for _, inScan := range []bool{true, false} {
		db, videoID := jobsFixture(t)
		ctx := context.Background()
		probe := domain.Probe{DurationMs: 1000, VideoCodec: "h264", Media: sampleMediaInfo()}
		if _, _, err := db.Scans().StartScan(ctx); err != nil {
			t.Fatal(err)
		}
		probeJob := claimProbeJob(t, db, videoID)
		if written, err := db.Ingest().ApplyProbeForJob(ctx, probeJob, probe, domain.Playability{Playable: true}); err != nil || !written {
			t.Fatalf("written = %v, err = %v", written, err)
		}
		if err := db.Ingest().CompleteClaimedJob(ctx, probeJob); err != nil {
			t.Fatal(err)
		}
		job := claimAtLastAttempt(t, db, domain.JobMetadata, videoID)
		if !inScan {
			if _, err := db.sql.Exec(`delete from scan_videos where video_id = ?`, videoID); err != nil {
				t.Fatal(err)
			}
		}
		if err := db.Ingest().FailClaimedJob(ctx, job, errors.New("moov atom not found")); err != nil {
			t.Fatal(err)
		}
		if got := jobCounts(t, db, videoID); got["metadata:failed"] != 1 {
			t.Fatalf("inScan=%v: jobs = %v, want one failed metadata", inScan, got)
		}
		if got, err := db.Library().MediaInfo(ctx, videoID); err != nil || got == nil || got.Video.HDR != domain.HDRKindHDR10 {
			t.Fatalf("inScan=%v: 失敗のあとの行 = %+v, %v", inScan, got, err)
		}
		issues := func() int {
			t.Helper()
			var count int
			if err := db.sql.QueryRow(`select count(*) from scan_issues where video_id = ? and kind = 'metadata_failed'`,
				videoID).Scan(&count); err != nil {
				t.Fatal(err)
			}
			return count
		}
		want := 0
		if inScan {
			want = 1
		}
		if got := issues(); got != want {
			t.Fatalf("inScan=%v: metadata_failed = %d, want %d", inScan, got, want)
		}
		if !inScan {
			continue
		}
		// 次の成功で記録を消す。
		if _, err := db.sql.Exec(`delete from jobs where kind = 'metadata'`); err != nil {
			t.Fatal(err)
		}
		if err := db.Ingest().EnqueueJob(ctx, domain.JobMetadata, videoID); err != nil {
			t.Fatal(err)
		}
		retry, err := db.Ingest().ClaimJob(ctx, domain.JobMetadata)
		if err != nil {
			t.Fatal(err)
		}
		if applied, err := db.Ingest().ApplyMediaInfoForJob(ctx, retry, *sampleMediaInfo()); err != nil || !applied {
			t.Fatalf("applied = %v, err = %v", applied, err)
		}
		if got := issues(); got != 0 {
			t.Fatalf("成功のあとも metadata_failed = %d", got)
		}
	}
}

// 解析の仕事を持つ動画の登録済みの所在が無くなる（フォルダの登録を外す、所在を消す）と、動画の
// 行が残っても待っていた解析し直しの仕事が取り出せるようになる。どちらの経路も解析し直しの
// ワーカーを起こす知らせを出す。
func TestLosingProbeLocationWakesMetadata(t *testing.T) {
	for _, tc := range []struct {
		name   string
		remove func(t *testing.T, db *DB)
	}{
		{name: "DeleteMediaFolder", remove: func(t *testing.T, db *DB) {
			t.Helper()
			var folderID, version int64
			if err := db.sql.QueryRow(`select id, version from media_folders where path = ?`,
				fixturePath("/media")).Scan(&folderID, &version); err != nil {
				t.Fatal(err)
			}
			if err := db.Settings().DeleteMediaFolder(context.Background(), folderID, version); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "DeleteVideoLocations", remove: func(t *testing.T, db *DB) {
			t.Helper()
			var locationID int64
			if err := db.sql.QueryRow(`select id from video_locations where path = ?`,
				fixturePath("/media/a.mp4")).Scan(&locationID); err != nil {
				t.Fatal(err)
			}
			if err := db.ScanIndex().DeleteVideoLocations(context.Background(), []int64{locationID}); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := migratedDB(t)
			ctx := context.Background()
			if _, err := db.sql.Exec(`insert into media_folders(path, version, created_at, updated_at) values (?, 1, 1, 1)`,
				fixturePath("/other")); err != nil {
				t.Fatal(err)
			}
			blocking, err := db.ScanIndex().UpsertVideo(ctx, sampleFile(fixturePath("/media/a.mp4"), "a", "key-a", 1024, 0))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.ScanIndex().UpsertVideo(ctx, sampleFile(fixturePath("/legacy/a.mp4"), "a", "key-a", 1024, 0)); err != nil {
				t.Fatal(err)
			}
			waiting, err := db.ScanIndex().UpsertVideo(ctx, sampleFile(fixturePath("/other/b.mp4"), "b", "key-b", 2048, 0))
			if err != nil {
				t.Fatal(err)
			}
			probeDone(t, db, waiting.ID)
			if err := db.Ingest().EnqueueJob(ctx, domain.JobProbe, blocking.ID); err != nil {
				t.Fatal(err)
			}
			if err := db.Ingest().EnqueueJob(ctx, domain.JobMetadata, waiting.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Ingest().ClaimJob(ctx, domain.JobMetadata); !errors.Is(err, domain.ErrNoJob) {
				t.Fatalf("解析の仕事が残る間の ClaimJob error = %v, want domain.ErrNoJob", err)
			}

			recorder := &queuedRecorder{}
			db.PublishTo(recorder)
			tc.remove(t, db)

			if got := recorder.take(); !slices.Contains(got, domain.JobMetadata) {
				t.Errorf("積まれた段階の知らせ = %v, want metadata を含む", got)
			}
			job, err := db.Ingest().ClaimJob(ctx, domain.JobMetadata)
			if err != nil {
				t.Fatalf("登録済みの所在が無くなった後も取り出せない: %v", err)
			}
			if job.VideoID != waiting.ID {
				t.Errorf("VideoID = %d, want %d", job.VideoID, waiting.ID)
			}
		})
	}
}
