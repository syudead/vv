package store

import (
	"context"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func sampleTranscodeProbe(width int) domain.TranscodeProbe {
	return domain.TranscodeProbe{
		FormatName: "mov,mp4,m4a,3gp,3g2,mj2",
		Video: domain.TranscodeVideo{
			Index: 0, CodecName: "h264", Profile: "High", Level: 41, PixelFormat: "yuv420p", BitsPerRawSample: 8,
			Width: width, Height: 1080, SampleAspectNum: 1, SampleAspectDen: 1, FPS: 30, RealFPS: 30,
		},
		Audio: &domain.TranscodeAudio{Index: 1, CodecName: "aac", Profile: "LC", SampleRate: 48000, Channels: 2},
	}
}

// usableTranscodeProbe は保存済みの行を読み、opened と比べて使える値を返す。
func usableTranscodeProbe(t *testing.T, db *DB, videoID int64, opened domain.FileStamp) (domain.TranscodeProbe, bool) {
	t.Helper()
	stored, err := db.Library().TranscodeProbe(context.Background(), videoID)
	if err != nil {
		t.Fatal(err)
	}
	return domain.TranscodeProbeUsable(stored, opened)
}

// 取り込みの解析ジョブが終わった動画に行があり、読み取り直し（POST /api/videos/{id}/probe
// が積むジョブ）でも書き直される。
func TestProbeJobWritesAndRewritesTranscodeProbe(t *testing.T) {
	db, videoID := failedVideoFixture(t)
	ctx := context.Background()

	old := domain.FileStamp{SizeBytes: 1, ModTimeNs: 1}
	if err := db.Ingest().SaveTranscodeProbe(ctx, videoID, old, sampleTranscodeProbe(640)); err != nil {
		t.Fatal(err)
	}
	if err := db.Ingest().RetryProbe(ctx, videoID, false); err != nil {
		t.Fatal(err)
	}
	job, err := db.Ingest().ClaimJob(ctx, domain.JobProbe)
	if err != nil {
		t.Fatal(err)
	}
	source := domain.FileStamp{SizeBytes: 1024, ModTimeNs: 1_700_000_000_123_456_789}
	want := sampleTranscodeProbe(1920)
	probe := domain.Probe{DurationMs: 1000, VideoCodec: "h264", AudioCodec: "aac", Transcode: &want, Source: source}
	written, err := db.Ingest().ApplyProbeForJob(ctx, job, probe, domain.Playability{Playable: true})
	if err != nil || !written {
		t.Fatalf("written = %v, err = %v", written, err)
	}

	got, ok := usableTranscodeProbe(t, db, videoID, source)
	if !ok || got.Video.Width != 1920 || got.Audio == nil || got.Audio.SampleRate != 48000 {
		t.Fatalf("stored = %+v (usable=%v)", got, ok)
	}
	if _, ok := usableTranscodeProbe(t, db, videoID, old); ok {
		t.Fatal("書き直す前のファイルの大きさと更新時刻で使えてしまう")
	}
}

func TestApplyProbeWritesTranscodeProbeInSameTransaction(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	source := domain.FileStamp{SizeBytes: 2048, ModTimeNs: 42}

	// 使える映像 stream の無い動画では行を書かない。
	if err := db.Ingest().ApplyProbe(ctx, videoID, domain.Probe{DurationMs: 1000, AudioCodec: "mp3", Source: source}, domain.Playability{}); err != nil {
		t.Fatal(err)
	}
	if stored, err := db.Library().TranscodeProbe(ctx, videoID); err != nil || stored != nil {
		t.Fatalf("stored = %+v, err = %v", stored, err)
	}

	transcode := sampleTranscodeProbe(1280)
	if err := db.Ingest().ApplyProbe(ctx, videoID, domain.Probe{DurationMs: 1000, VideoCodec: "h264", Transcode: &transcode, Source: source}, domain.Playability{Playable: true}); err != nil {
		t.Fatal(err)
	}
	if got, ok := usableTranscodeProbe(t, db, videoID, source); !ok || got.Video.Width != 1280 {
		t.Fatalf("stored = %+v (usable=%v)", got, ok)
	}

	// 行の書き込みが失敗すれば、videos の更新も残らない。
	if _, err := db.sql.Exec(`create trigger fail_transcode_probe before update on video_transcode_probes
		begin select raise(abort, 'boom'); end`); err != nil {
		t.Fatal(err)
	}
	if err := db.Ingest().ApplyProbe(ctx, videoID, domain.Probe{DurationMs: 9000, VideoCodec: "h264", Transcode: &transcode, Source: source}, domain.Playability{Playable: true}); err == nil {
		t.Fatal("行の書き込みの失敗が返らない")
	}
	var duration int64
	if err := db.sql.QueryRow(`select duration_ms from videos where id = ?`, videoID).Scan(&duration); err != nil {
		t.Fatal(err)
	}
	if duration != 1000 {
		t.Fatalf("duration_ms = %d, want 1000 (rolled back)", duration)
	}
}

// 所在が変わって古くなったジョブの結果は、videos と同じく解析情報も書かない。
func TestStaleProbeJobDoesNotWriteTranscodeProbe(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	if err := db.Ingest().EnqueueJob(ctx, domain.JobProbe, videoID); err != nil {
		t.Fatal(err)
	}
	job, err := db.Ingest().ClaimJob(ctx, domain.JobProbe)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ScanIndex().UpsertVideo(ctx, sampleFile("/media/z.mp4", "z", "key-a", 1024, 0)); err != nil {
		t.Fatal(err)
	}
	transcode := sampleTranscodeProbe(1920)
	written, err := db.Ingest().ApplyProbeForJob(ctx, job, domain.Probe{VideoCodec: "h264", Transcode: &transcode}, domain.Playability{Playable: true})
	if err != nil || written {
		t.Fatalf("written = %v, err = %v", written, err)
	}
	if stored, err := db.Library().TranscodeProbe(ctx, videoID); err != nil || stored != nil {
		t.Fatalf("stored = %+v, err = %v", stored, err)
	}
}

// SaveTranscodeProbe は upsert で、後に書いた値が残る。版が違う行と JSON が読めない行は
// 「無い」と読まれ、動画の行が消えると連鎖で消える。
func TestSaveTranscodeProbeUpsertsAndReadsAsMissingWhenStale(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	first := domain.FileStamp{SizeBytes: 10, ModTimeNs: 100}
	second := domain.FileStamp{SizeBytes: 20, ModTimeNs: 200}
	if err := db.Ingest().SaveTranscodeProbe(ctx, videoID, first, sampleTranscodeProbe(640)); err != nil {
		t.Fatal(err)
	}
	if err := db.Ingest().SaveTranscodeProbe(ctx, videoID, second, sampleTranscodeProbe(1920)); err != nil {
		t.Fatal(err)
	}
	var rows int
	if err := db.sql.QueryRow(`select count(*) from video_transcode_probes where video_id = ?`, videoID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("rows = %d, want 1", rows)
	}
	if got, ok := usableTranscodeProbe(t, db, videoID, second); !ok || got.Video.Width != 1920 {
		t.Fatalf("stored = %+v (usable=%v)", got, ok)
	}

	if _, err := db.sql.Exec(`update video_transcode_probes set version = version + 1 where video_id = ?`, videoID); err != nil {
		t.Fatal(err)
	}
	if _, ok := usableTranscodeProbe(t, db, videoID, second); ok {
		t.Fatal("版が違う行が使えてしまう")
	}
	if _, err := db.sql.Exec(`update video_transcode_probes set version = ?, probe = '{"Video":' where video_id = ?`, domain.TranscodeProbeVersion, videoID); err != nil {
		t.Fatal(err)
	}
	if _, ok := usableTranscodeProbe(t, db, videoID, second); ok {
		t.Fatal("JSON が読めない行が使えてしまう")
	}

	if _, err := db.sql.Exec(`delete from videos where id = ?`, videoID); err != nil {
		t.Fatal(err)
	}
	if stored, err := db.Library().TranscodeProbe(ctx, videoID); err != nil || stored != nil {
		t.Fatalf("動画を消したあと stored = %+v, err = %v", stored, err)
	}
}
