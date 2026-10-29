package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

func keyframeFixture(t *testing.T) (*DB, int64, domain.KeyframeProbe) {
	t.Helper()
	db, id := jobsFixture(t)
	file := sampleFile(fixturePath("/media/a.mp4"), "a", "key-a", 1024, 0)
	p := domain.KeyframeProbe{Version: domain.KeyframeProbeVersion,
		Source: domain.FileStamp{SizeBytes: file.SizeBytes, ModTimeNs: file.MTime.UnixNano()}, Spacing: domain.KeyframesSparse}
	if err := db.Ingest().ApplyProbe(context.Background(), id, domain.Probe{DurationMs: 300000, VideoCodec: "h264"}, domain.Playability{Playable: true}); err != nil {
		t.Fatal(err)
	}
	return db, id, p
}

func TestKeyframeBackfillAndImport(t *testing.T) {
	for _, spacing := range []domain.KeyframeSpacing{domain.KeyframesSparse, domain.KeyframesShort, domain.KeyframesUnknown} {
		t.Run(string(spacing), func(t *testing.T) {
			db, id, p := keyframeFixture(t)
			ctx := context.Background()
			p.Spacing = spacing
			job, err := nextBackfill(ctx, db, domain.JobKeyframes)
			if err != nil || job.VideoID != id {
				t.Fatalf("candidate = %+v, %v", job, err)
			}
			if err := db.Ingest().SaveKeyframeProbe(ctx, job, p); err != nil {
				t.Fatal(err)
			}
			got, err := db.Library().KeyframeSpacing(ctx, id)
			if err != nil || got != p {
				t.Fatalf("saved = %+v, %v", got, err)
			}
			if _, err := nextBackfill(ctx, db, domain.JobKeyframes); !errors.Is(err, domain.ErrNoJob) {
				t.Fatalf("completed candidate: %v", err)
			}
			p.Spacing = domain.KeyframesShort
			if err := db.Ingest().ApplyProbe(ctx, id, domain.Probe{DurationMs: 300000, VideoCodec: "h264", Keyframes: &p}, domain.Playability{Playable: true}); err != nil {
				t.Fatal(err)
			}
			p.Spacing = domain.KeyframesSparse
			if err := db.Ingest().SaveKeyframeProbe(ctx, job, p); err != nil {
				t.Fatal(err)
			}
			got, err = db.Library().KeyframeSpacing(ctx, id)
			if err != nil || got.Spacing != domain.KeyframesShort {
				t.Fatalf("concurrent import overwritten: %+v, %v", got, err)
			}
		})
	}
}

func TestKeyframeBackfillRejectsChangedSource(t *testing.T) {
	for _, change := range []string{"file stamp", "subsecond stamp", "location", "sample changed", "in progress"} {
		t.Run(change, func(t *testing.T) {
			db, id, p := keyframeFixture(t)
			ctx := context.Background()
			job, err := nextBackfill(ctx, db, domain.JobKeyframes)
			if err != nil {
				t.Fatal(err)
			}
			switch change {
			case "file stamp":
				p.Source.SizeBytes++
			case "subsecond stamp":
				p.Source.ModTimeNs++
			case "location":
				if _, err := db.sql.ExecContext(ctx, `update video_locations set version = version + 1 where video_id = ?`, id); err != nil {
					t.Fatal(err)
				}
			case "sample changed":
				p.Version = 0
			case "in progress":
				if err := db.Ingest().EnqueueJob(ctx, domain.JobProbe, id, domain.ActivityByRule); err != nil {
					t.Fatal(err)
				}
				if _, err := nextBackfill(ctx, db, domain.JobKeyframes); !errors.Is(err, domain.ErrNoJob) {
					t.Fatalf("candidate during probe: %v", err)
				}
				return
			}
			if err := db.Ingest().SaveKeyframeProbe(ctx, job, p); err != nil {
				t.Fatal(err)
			}
			got, err := db.Library().KeyframeSpacing(ctx, id)
			if err != nil || got.Version != 0 {
				t.Fatalf("stale saved = %+v, %v", got, err)
			}
		})
	}
}

func TestKeyframeBackfillAfterSubsecondUpdate(t *testing.T) {
	db, id, p := keyframeFixture(t)
	ctx := context.Background()
	file := sampleFile(fixturePath("/media/a.mp4"), "a", "key-a", 1024, 100*time.Millisecond)
	for _, offset := range []time.Duration{0, 100 * time.Millisecond} {
		file.MTime = file.MTime.Add(offset)
		result, err := db.ScanIndex().UpsertVideo(ctx, file)
		if err != nil || result.ID != id || result.Outcome != domain.OutcomeUpdated {
			t.Fatalf("subsecond update = %+v, %v", result, err)
		}
		indexed, err := db.ScanIndex().IndexedVideosByPath(ctx)
		if err != nil || !indexed[file.Path].MTime.Equal(file.MTime) {
			t.Fatalf("indexed timestamp = %v, %v", indexed[file.Path].MTime, err)
		}
		job, err := nextBackfill(ctx, db, domain.JobKeyframes)
		if err != nil || job.VideoID != id {
			t.Fatalf("updated candidate = %+v, %v", job, err)
		}
		p.Source.ModTimeNs = file.MTime.UnixNano()
		if err := db.Ingest().SaveKeyframeProbe(ctx, job, p); err != nil {
			t.Fatal(err)
		}
		got, err := db.Library().KeyframeSpacing(ctx, id)
		if err != nil || !got.SparseFor(p.Source) {
			t.Fatalf("updated sparse hint = %+v, %v", got, err)
		}
		if _, err := nextBackfill(ctx, db, domain.JobKeyframes); !errors.Is(err, domain.ErrNoJob) {
			t.Fatalf("completed sample retried: %v", err)
		}
	}
}

func TestKeyframeBackfillLearnsLegacyTimestamp(t *testing.T) {
	for _, available := range []bool{true, false} {
		t.Run(map[bool]string{true: "available", false: "unavailable"}[available], func(t *testing.T) {
			db, id, p := keyframeFixture(t)
			ctx := context.Background()
			// Existing locations have no nanosecond stamp after migration.
			if _, err := db.sql.ExecContext(ctx, `update video_locations set mtime_ns = null where video_id = ?`, id); err != nil {
				t.Fatal(err)
			}
			p.Source.ModTimeNs += int64(100 * time.Millisecond)
			if !available {
				p.Source = domain.FileStamp{}
			}
			job, err := nextBackfill(ctx, db, domain.JobKeyframes)
			if err != nil {
				t.Fatal(err)
			}
			if err := db.Ingest().SaveKeyframeProbe(ctx, job, p); err != nil {
				t.Fatal(err)
			}
			got, err := db.Library().KeyframeSpacing(ctx, id)
			if err != nil || got.Version != domain.KeyframeProbeVersion {
				t.Fatalf("legacy sample = %+v, %v", got, err)
			}
			if available && !got.SparseFor(p.Source) {
				t.Fatalf("legacy sparse hint = %+v", got)
			}
			if !available && got.Spacing != domain.KeyframesUnknown {
				t.Fatalf("unavailable sample = %+v", got)
			}
			if _, err := nextBackfill(ctx, db, domain.JobKeyframes); !errors.Is(err, domain.ErrNoJob) {
				t.Fatalf("legacy sample retried: %v", err)
			}
		})
	}
}

func TestLocationTimestampMigration(t *testing.T) {
	db, provider := migrateTo(t, 56)
	ctx := context.Background()
	execSQL(t, db, `insert into videos (id, content_key, added_at, indexed_at) values (1, 'key-a', 1, 1)`)
	execSQL(t, db, `insert into video_locations (video_id, path, title, size_bytes, mtime, created_at, updated_at)
		values (1, ?, 'a', 1024, 1757000000, 1, 1)`, fixturePath("/media/a.mp4"))
	if _, err := provider.UpTo(ctx, 57); err != nil {
		t.Fatal(err)
	}
	var precise sql.NullInt64
	if err := db.sql.QueryRow(`select mtime_ns from video_locations`).Scan(&precise); err != nil || precise.Valid {
		t.Fatalf("legacy timestamp = %+v, %v", precise, err)
	}
	indexed, err := db.ScanIndex().IndexedVideosByPath(ctx)
	if err != nil || !indexed[fixturePath("/media/a.mp4")].MTime.Equal(fixedTime) {
		t.Fatalf("legacy index = %+v, %v", indexed, err)
	}
	if _, err := provider.DownTo(ctx, 56); err != nil {
		t.Fatal(err)
	}
	var seconds int64
	if err := db.sql.QueryRow(`select mtime from video_locations`).Scan(&seconds); err != nil || seconds != fixedTime.Unix() {
		t.Fatalf("restored timestamp = %d, %v", seconds, err)
	}
}

func TestKeyframeBackfillUnavailableAndVersionChange(t *testing.T) {
	db, id, p := keyframeFixture(t)
	ctx := context.Background()
	job, err := nextBackfill(ctx, db, domain.JobKeyframes)
	if err != nil {
		t.Fatal(err)
	}
	p.Source, p.Spacing = domain.FileStamp{}, domain.KeyframesUnknown
	if err := db.Ingest().SaveKeyframeProbe(ctx, job, p); err != nil {
		t.Fatal(err)
	}
	if _, err := nextBackfill(ctx, db, domain.JobKeyframes); !errors.Is(err, domain.ErrNoJob) {
		t.Fatalf("unavailable retried: %v", err)
	}
	if _, err := db.sql.ExecContext(ctx, `update video_keyframe_spacing set version = 0 where video_id = ?`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := nextBackfill(ctx, db, domain.JobKeyframes); err != nil {
		t.Fatalf("old version not retried: %v", err)
	}
}
