package store

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// eventRecorder は発行された変化を記録する。
type eventRecorder struct {
	events []domain.Event
}

func (r *eventRecorder) Publish(events ...domain.Event) {
	r.events = append(r.events, events...)
}

// 取引の中で変化を集めても、確定できなければ1件も発行しない。
func TestCommitPublishesNothingWhenTransactionRollsBack(t *testing.T) {
	db, videoID := jobsFixture(t)
	recorder := &eventRecorder{}
	db.PublishTo(recorder)

	ctx, cancel := context.WithCancel(context.Background())
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	deleted, err := collectDeletedVideos(tx.QueryContext(ctx,
		`delete from videos where id = ? returning id, content_key`, videoID))
	if err != nil {
		t.Fatal(err)
	}
	var c changes
	c.videosDeleted(deleted)
	c.jobsQueued(domain.JobProbe)
	// 確定の前に取り消すと、取引はロールバックする。
	cancel()
	if err := db.commit(tx, &c); err == nil {
		t.Fatal("取り消した取引が確定した")
	}

	if len(recorder.events) != 0 {
		t.Fatalf("ロールバックした取引から発行した: %v", recorder.events)
	}
	if _, err := db.Library().GetVideo(context.Background(), videoID); err != nil {
		t.Fatalf("ロールバックしたのに動画が消えた: %v", err)
	}
}

// 途中で断った書き込み（ロールバックする取引）は発行しない。
func TestRejectedWritesPublishNothing(t *testing.T) {
	db, videoID := jobsFixture(t)
	ctx := context.Background()
	folders, err := db.Settings().ListMediaFolders(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.Scans().StartScan(ctx); err != nil {
		t.Fatal(err)
	}
	recorder := &eventRecorder{}
	db.PublishTo(recorder)

	if err := db.Settings().DeleteMediaFolder(ctx, folders[0].ID, folders[0].Version); !errors.Is(err, domain.ErrScanRunning) {
		t.Fatalf("走査中の DeleteMediaFolder error = %v, want domain.ErrScanRunning", err)
	}
	if err := db.Ingest().RetryProbe(ctx, videoID, false); !errors.Is(err, domain.ErrProbeNotFailed) {
		t.Fatalf("RetryProbe error = %v, want domain.ErrProbeNotFailed", err)
	}
	if requeued, err := db.Ingest().RequeueMissingPreview(ctx, videoID, "key-a"); err != nil || requeued {
		t.Fatalf("RequeueMissingPreview = %v, %v, want false", requeued, err)
	}
	if len(recorder.events) != 0 {
		t.Fatalf("ロールバックした取引から発行した: %v", recorder.events)
	}
}

// 1つの取引の中で同じ種類の変化が何度起きても、確定後の発行は種類ごとに1つにまとめる。
func TestOneTransactionPublishesEachKindOfChangeOnce(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()
	var ids []int64
	for _, file := range []domain.VideoFile{
		sampleFile("/media/a.mp4", "a", "key-a", 1, 0),
		sampleFile("/media/b.mp4", "b", "key-b", 2, 0),
		sampleFile("/media/b2.mp4", "b", "key-b", 2, 0),
	} {
		added, err := db.ScanIndex().UpsertVideo(ctx, file)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, added.ID)
	}
	folders, err := db.Settings().ListMediaFolders(ctx)
	if err != nil {
		t.Fatal(err)
	}
	recorder := &eventRecorder{}
	db.PublishTo(recorder)

	// 付け替えで2本の動画が消え、すべての段階の仕事が取り出せるようになる。
	if _, err := db.Settings().ReplaceMediaFolder(ctx, folders[0].ID, folders[0].Version, "/other"); err != nil {
		t.Fatal(err)
	}

	want := []domain.Event{
		domain.VideoIngestChanged{VideoID: ids[0]},
		domain.VideoIngestChanged{VideoID: ids[1]},
		domain.ContentUnreferenced{ContentKeys: []string{"key-a", "key-b"}},
		domain.JobsQueued{Kinds: domain.JobKinds},
		domain.ProcessingChanged{},
	}
	if !reflect.DeepEqual(recorder.events, want) {
		t.Fatalf("発行 = %#v, want %#v", recorder.events, want)
	}
}

func TestChangesCoalesceRepeatedChanges(t *testing.T) {
	var c changes
	c.jobsQueued(domain.JobProbe, domain.JobProbe)
	c.jobsQueued(domain.JobThumbnail, domain.JobProbe)
	c.processingChanged()
	c.videosDeleted([]domain.DeletedVideo{{ID: 1, ContentKey: "k"}})
	c.videosDeleted([]domain.DeletedVideo{{ID: 2, ContentKey: "k"}})

	want := []domain.Event{
		domain.VideoIngestChanged{VideoID: 1},
		domain.VideoIngestChanged{VideoID: 2},
		domain.ContentUnreferenced{ContentKeys: []string{"k"}},
		domain.JobsQueued{Kinds: []domain.JobKind{domain.JobProbe, domain.JobThumbnail}},
		domain.ProcessingChanged{},
	}
	if got := c.events(); !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %#v, want %#v", got, want)
	}
	var empty changes
	if got := empty.events(); len(got) != 0 {
		t.Fatalf("変化が無いのに発行する: %v", got)
	}
}
