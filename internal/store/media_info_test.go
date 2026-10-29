package store

import (
	"context"
	"errors"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func sampleMediaInfo() *domain.MediaInfo {
	interlaced := false
	return &domain.MediaInfo{
		Video:       &domain.MediaVideo{FrameRate: 29.97, BitDepth: 10, Interlaced: &interlaced, HDR: domain.HDRKindHDR10},
		AudioTracks: []domain.MediaAudioTrack{{Index: 1, Codec: "aac", Channels: 2, Language: "jpn", Default: true}},
		Embedded:    &domain.MediaEmbedded{Title: "夏の記録", RecordedAt: "2024-05-01T12:34:56+09:00"},
		Chapters:    []domain.MediaChapter{{StartMs: 0, EndMs: 1000, Title: "1"}},
	}
}

func claimProbeJob(t *testing.T, db *DB, videoID int64) domain.Job {
	t.Helper()
	ctx := context.Background()
	if err := db.Ingest().EnqueueJob(ctx, domain.JobProbe, videoID, domain.ActivityByRule); err != nil {
		t.Fatal(err)
	}
	job, err := db.Ingest().ClaimJob(ctx, domain.JobProbe)
	if err != nil {
		t.Fatal(err)
	}
	return job
}

func TestApplyProbeForJobWritesMediaInfo(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	job := claimProbeJob(t, db, videoID)

	probe := domain.Probe{DurationMs: 1000, VideoCodec: "h264", Media: sampleMediaInfo()}
	written, err := db.Ingest().ApplyProbeForJob(ctx, job, probe, domain.Playability{Playable: true})
	if err != nil || !written {
		t.Fatalf("written = %v, err = %v", written, err)
	}
	got, err := db.Library().MediaInfo(ctx, videoID)
	if err != nil || got == nil {
		t.Fatalf("got = %+v, err = %v", got, err)
	}
	if got.Video == nil || got.Video.HDR != domain.HDRKindHDR10 || got.Video.Interlaced == nil || *got.Video.Interlaced ||
		len(got.AudioTracks) != 1 || got.AudioTracks[0].Language != "jpn" ||
		got.Embedded == nil || got.Embedded.RecordedAt != "2024-05-01T12:34:56+09:00" || len(got.Chapters) != 1 {
		t.Fatalf("got = %+v", got)
	}
	if infos, err := db.Library().MediaInfos(ctx, []int64{videoID, videoID + 1000}); err != nil || len(infos) != 1 ||
		infos[videoID] == nil || infos[videoID].Video.HDR != domain.HDRKindHDR10 {
		t.Fatalf("MediaInfos = %+v, %v, want 情報のある動画だけ", infos, err)
	}

	if _, err := db.sql.Exec(`update video_media_info set version = version + 1 where video_id = ?`, videoID); err != nil {
		t.Fatal(err)
	}
	if got, err := db.Library().MediaInfo(ctx, videoID); err != nil || got != nil {
		t.Fatalf("版が違う行: got = %+v, err = %v", got, err)
	}
	if _, err := db.sql.Exec(`update video_media_info set version = ?, info = '{"Video":' where video_id = ?`,
		domain.MediaInfoVersion, videoID); err != nil {
		t.Fatal(err)
	}
	if got, err := db.Library().MediaInfo(ctx, videoID); err != nil || got != nil {
		t.Fatalf("JSON が読めない行: got = %+v, err = %v", got, err)
	}

	if _, err := db.sql.Exec(`delete from videos where id = ?`, videoID); err != nil {
		t.Fatal(err)
	}
	var rows int
	if err := db.sql.QueryRow(`select count(*) from video_media_info where video_id = ?`, videoID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf("動画を消したあと rows = %d, want 0", rows)
	}
}

func TestStaleProbeJobDoesNotWriteMediaInfo(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	job := claimProbeJob(t, db, videoID)
	if _, err := db.sql.Exec(`update video_locations set version = version + 1 where id = ?`, job.LocationID); err != nil {
		t.Fatal(err)
	}

	probe := domain.Probe{DurationMs: 1000, VideoCodec: "h264", Media: sampleMediaInfo()}
	written, err := db.Ingest().ApplyProbeForJob(ctx, job, probe, domain.Playability{Playable: true})
	if err != nil || written {
		t.Fatalf("written = %v, err = %v", written, err)
	}
	var rows int
	if err := db.sql.QueryRow(`select count(*) from video_media_info where video_id = ?`, videoID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf("rows = %d, want 0", rows)
	}
}

func TestApplyProbeForJobRollsBackWhenMediaInfoFails(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	job := claimProbeJob(t, db, videoID)
	if _, err := db.sql.Exec(`create trigger fail_media_info before insert on video_media_info
		begin select raise(abort, 'boom'); end`); err != nil {
		t.Fatal(err)
	}
	probe := domain.Probe{DurationMs: 9000, VideoCodec: "h264", Media: sampleMediaInfo()}
	if _, err := db.Ingest().ApplyProbeForJob(ctx, job, probe, domain.Playability{Playable: true}); err == nil {
		t.Fatal("行の書き込みの失敗が返らない")
	}
	var state string
	if err := db.sql.QueryRow(`select probe_state from videos where id = ?`, videoID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state == string(domain.ProbeStateDone) {
		t.Fatal("videos の更新が残った")
	}
}

func TestMediaInfoBackfillReplacesOlderRowsAndRemembersFailures(t *testing.T) {
	db, id, keyframes := keyframeFixture(t)
	ctx := context.Background()
	source := keyframes.Source
	if _, err := db.sql.Exec(`insert into video_media_info (video_id, version, info, updated_at)
		values (?, 1, '{"Video":{"BitDepth":8,"HDR":"sdr"}}', 0)`, id); err != nil {
		t.Fatal(err)
	}
	if got, err := db.Library().MediaInfo(ctx, id); err != nil || got == nil || got.Video.BitDepth != 8 {
		t.Fatalf("古い版の行 = %+v, %v, want 読み直すまで今の値のまま", got, err)
	}
	job, err := nextBackfill(ctx, db, domain.JobMediaInfo)
	if err != nil || job.VideoID != id {
		t.Fatalf("候補 = %+v, %v", job, err)
	}

	changed := domain.FileStamp{SizeBytes: source.SizeBytes + 1, ModTimeNs: source.ModTimeNs}
	if err := db.Ingest().SaveMediaInfoProbe(ctx, job, domain.MediaInfoProbe{Source: changed, Info: sampleMediaInfo()}); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.Library().MediaInfo(ctx, id); got == nil || got.Video.BitDepth != 8 {
		t.Fatalf("差し替わったファイルの値 = %+v, want 書かない", got)
	}

	if err := db.Ingest().SaveMediaInfoProbe(ctx, job, domain.MediaInfoProbe{Source: source}); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.Library().MediaInfo(ctx, id); got == nil || got.Video.BitDepth != 8 {
		t.Fatalf("読めなかったあとの値 = %+v, want 今の値を残す", got)
	}
	if _, err := nextBackfill(ctx, db, domain.JobMediaInfo); !errors.Is(err, domain.ErrNoJob) {
		t.Fatalf("失敗した同じファイル: %v, want ErrNoJob", err)
	}
	if _, err := db.sql.Exec(`update video_locations set size_bytes = size_bytes + 1 where id = ?`, job.LocationID); err != nil {
		t.Fatal(err)
	}
	if job, err = nextBackfill(ctx, db, domain.JobMediaInfo); err != nil || job.VideoID != id {
		t.Fatalf("変わったファイル: %+v, %v, want もう一度候補", job, err)
	}

	changed.SizeBytes = source.SizeBytes + 1
	if err := db.Ingest().SaveMediaInfoProbe(ctx, job, domain.MediaInfoProbe{Source: changed, Info: sampleMediaInfo()}); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.Library().MediaInfo(ctx, id); got == nil || got.Video.BitDepth != 10 {
		t.Fatalf("読み直した値 = %+v, want 新しい値", got)
	}
	if _, err := nextBackfill(ctx, db, domain.JobMediaInfo); !errors.Is(err, domain.ErrNoJob) {
		t.Fatalf("読み直したあと: %v, want ErrNoJob", err)
	}
}
