package store

import (
	"context"
	"errors"
	"testing"

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
			job, err := db.Ingest().NextKeyframeProbe(ctx, 0)
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
			if _, err := db.Ingest().NextKeyframeProbe(ctx, 0); !errors.Is(err, domain.ErrNoJob) {
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
	for _, change := range []string{"file stamp", "location", "sample changed", "in progress"} {
		t.Run(change, func(t *testing.T) {
			db, id, p := keyframeFixture(t)
			ctx := context.Background()
			job, err := db.Ingest().NextKeyframeProbe(ctx, 0)
			if err != nil {
				t.Fatal(err)
			}
			switch change {
			case "file stamp":
				p.Source.SizeBytes++
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
				if _, err := db.Ingest().NextKeyframeProbe(ctx, 0); !errors.Is(err, domain.ErrNoJob) {
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

func TestKeyframeBackfillUnavailableAndVersionChange(t *testing.T) {
	db, id, p := keyframeFixture(t)
	ctx := context.Background()
	job, err := db.Ingest().NextKeyframeProbe(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	p.Source, p.Spacing = domain.FileStamp{}, domain.KeyframesUnknown
	if err := db.Ingest().SaveKeyframeProbe(ctx, job, p); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Ingest().NextKeyframeProbe(ctx, 0); !errors.Is(err, domain.ErrNoJob) {
		t.Fatalf("unavailable retried: %v", err)
	}
	if _, err := db.sql.ExecContext(ctx, `update video_keyframe_spacing set version = 0 where video_id = ?`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Ingest().NextKeyframeProbe(ctx, 0); err != nil {
		t.Fatalf("old version not retried: %v", err)
	}
}
