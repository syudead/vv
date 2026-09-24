package main

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/artifacts"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/eventbus"
	"github.com/syudead/vv/internal/httpapi"
	"github.com/syudead/vv/internal/jobs"
	"github.com/syudead/vv/internal/store"
)

// countingWaker は起こされた回数を数える。
type countingWaker struct{ woken atomic.Int32 }

func (w *countingWaker) Wake() { w.woken.Add(1) }

func newTestBus() *eventbus.Bus {
	return eventbus.New(slog.New(slog.DiscardHandler))
}

// 仕事が積まれた段階のワーカーだけを起こし、解析の成否が決まったらサムネイルの
// ワーカーを起こす。購読をやめたら、止めたワーカーを起こさない。
func TestSubscribeEventsWakesWorkers(t *testing.T) {
	bus := newTestBus()
	wakers := map[domain.JobKind]*countingWaker{}
	workers := map[domain.JobKind]waker{}
	for _, kind := range domain.JobKinds {
		wakers[kind] = &countingWaker{}
		workers[kind] = wakers[kind]
	}
	subs := subscribeEvents(bus, eventSubscribers{Workers: workers})

	bus.Publish(domain.JobsQueued{Kinds: []domain.JobKind{domain.JobProbe, domain.JobPreview}})
	bus.Publish(domain.VideoIngestChanged{VideoID: 1, Stage: domain.JobProbe})
	bus.Publish(domain.VideoIngestChanged{VideoID: 1, Stage: domain.JobThumbnail})
	bus.Publish(domain.VideoIngestChanged{VideoID: 1})
	want := map[domain.JobKind]int32{domain.JobProbe: 1, domain.JobThumbnail: 1, domain.JobPreview: 1}
	check := func() bool {
		for kind, w := range wakers {
			if w.woken.Load() != want[kind] {
				return false
			}
		}
		return true
	}
	// 購読者へは非同期に渡るので、届くまで待つ。
	for deadline := time.Now().Add(5 * time.Second); !check() && time.Now().Before(deadline); {
		time.Sleep(time.Millisecond)
	}
	subs.StopWorkers()
	bus.Publish(domain.JobsQueued{Kinds: domain.JobKinds})
	bus.Close()

	for kind, w := range wakers {
		if got := w.woken.Load(); got != want[kind] {
			t.Errorf("%s の起床 = %d, want %d", kind, got, want[kind])
		}
	}
}

// probeOnlyGenerator は解析だけを返す。ffprobe を起動しない。
type probeOnlyGenerator struct{}

func (probeOnlyGenerator) CheckSource(string) error { return nil }
func (probeOnlyGenerator) Probe(context.Context, string) (domain.Probe, error) {
	return domain.Probe{DurationMs: 60_000, VideoCodec: "h264", AudioCodec: "aac"}, nil
}
func (probeOnlyGenerator) Thumbnail(context.Context, string, int64, string) error { return nil }
func (probeOnlyGenerator) SeekThumbnails(context.Context, string, string) error   { return nil }
func (probeOnlyGenerator) Preview(context.Context, string, string, int64) error   { return nil }

// emptyClaims は、取り出しが空振りしたことを知らせる待ち行列である。
type emptyClaims struct {
	jobs.Queue
	empty chan struct{}
}

func (q emptyClaims) ClaimJob(ctx context.Context, kind domain.JobKind) (domain.Job, error) {
	job, err := q.Queue.ClaimJob(ctx, kind)
	if errors.Is(err, domain.ErrNoJob) {
		select {
		case q.empty <- struct{}{}:
		default:
		}
	}
	return job, err
}

func openTestDB(t *testing.T) (context.Context, *store.DB, string) {
	t.Helper()
	ctx := context.Background()
	dataDir := t.TempDir()
	db, err := store.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := store.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	mediaDir := t.TempDir()
	if _, err := db.AddMediaFolder(ctx, mediaDir); err != nil {
		t.Fatal(err)
	}
	return ctx, db, mediaDir
}

// 解析が終わると、待ち行列を一定間隔で問い合わせずにサムネイルの生成が始まる。
// 解析の完了からサムネイルのワーカーの起床への依存は、subscribeEvents の購読だけで
// 表されている。
func TestProbeCompletionStartsThumbnailWithoutPolling(t *testing.T) {
	ctx, db, mediaDir := openTestDB(t)
	video, err := db.UpsertVideo(ctx, domain.VideoFile{
		Path: filepath.Join(mediaDir, "movie.mp4"), Title: "movie", ContentKey: "key-movie", SizeBytes: 1,
		MTime: time.Unix(1, 0), Container: "mp4",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.EnqueueJob(ctx, domain.JobThumbnail, video.ID); err != nil {
		t.Fatal(err)
	}

	bus := newTestBus()
	db.PublishTo(bus)
	logger := slog.New(slog.DiscardHandler)
	ingest := app.NewIngest(app.IngestOptions{
		Store: db.Ingest(), Generator: probeOnlyGenerator{}, Artifacts: artifacts.New(t.TempDir()),
		Publisher: bus, Logger: logger,
	})
	// 取り出しの失敗による再試行でも起きないよう、再試行の間隔は長くする。
	const noRetry = time.Hour
	thumbnailStarted := make(chan domain.Job, 1)
	thumbnailQueue := emptyClaims{Queue: db.Ingest(), empty: make(chan struct{}, 1)}
	thumbnail := jobs.New(jobs.Options{
		Kind: domain.JobThumbnail, Queue: thumbnailQueue, RetryDelay: noRetry, Logger: logger,
		Handler: func(_ context.Context, job domain.Job) error {
			thumbnailStarted <- job
			return nil
		},
		Finished: ingest.JobFinished,
	})
	probe := jobs.New(jobs.Options{
		Kind: domain.JobProbe, Queue: db.Ingest(), RetryDelay: noRetry, Logger: logger,
		Handler: ingest.Handler(domain.JobProbe), Finished: ingest.JobFinished,
	})
	subs := subscribeEvents(bus, eventSubscribers{Workers: map[domain.JobKind]waker{
		domain.JobProbe: probe, domain.JobThumbnail: thumbnail,
	}})

	runCtx, stop := context.WithCancel(ctx)
	var running sync.WaitGroup
	defer func() {
		subs.StopWorkers()
		stop()
		running.Wait()
		bus.Close()
	}()
	running.Go(func() { thumbnail.Run(runCtx) })
	// 解析の前なので、サムネイルのワーカーは空振りして眠る。
	select {
	case <-thumbnailQueue.empty:
	case <-time.After(5 * time.Second):
		t.Fatal("サムネイルのワーカーが待ち行列を見ない")
	}

	running.Go(func() { probe.Run(runCtx) })
	if err := db.EnqueueJob(ctx, domain.JobProbe, video.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case job := <-thumbnailStarted:
		if job.VideoID != video.ID {
			t.Fatalf("VideoID = %d, want %d", job.VideoID, video.ID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("解析が終わってもサムネイルの生成が始まらない")
	}
}

// eventLog は受け取った変化の種類を記録する、テスト用の購読者である。
type eventLog struct {
	mu     sync.Mutex
	events []domain.Event
}

func (l *eventLog) handle(event domain.Event) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, event)
}

func (l *eventLog) has(match func(domain.Event) bool) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.ContainsFunc(l.events, match)
}

type finishedScanner struct{}

func (finishedScanner) Scan(context.Context) (domain.ScanResult, error) {
	return domain.ScanResult{}, nil
}

// 購読者を1つ加えるには、購読者と、配り先への登録だけがあればよい。発行する
// 側（保存層・取り込み・走査）は変えずに、定めたすべての変化が届く。
func TestAddedSubscriberReceivesEveryEventWithoutPublisherChanges(t *testing.T) {
	ctx, db, mediaDir := openTestDB(t)
	video, err := db.UpsertVideo(ctx, domain.VideoFile{
		Path: filepath.Join(mediaDir, "movie.mp4"), Title: "movie", ContentKey: "key-movie", SizeBytes: 1,
		MTime: time.Unix(1, 0), Container: "mp4",
	})
	if err != nil {
		t.Fatal(err)
	}

	bus := newTestBus()
	db.PublishTo(bus)
	logger := slog.New(slog.DiscardHandler)
	ingest := app.NewIngest(app.IngestOptions{
		Store: db.Ingest(), Artifacts: artifacts.New(t.TempDir()), Publisher: bus, Logger: logger,
	})
	scans := app.NewScans(app.ScansOptions{
		Store: db.Scans(), Jobs: db.Ingest(),
		NewScanner: func(app.ScanReporter) app.Scanner { return finishedScanner{} },
		Publisher:  bus, Logger: logger,
	})
	subscribeEvents(bus, eventSubscribers{
		Screen:           httpapi.NewEvents().Handle,
		Workers:          map[domain.JobKind]waker{domain.JobProbe: &countingWaker{}},
		ReleaseArtifacts: ingest.ReleaseArtifacts,
	})
	// テスト用の購読者。加えるのはこの1行だけである。
	log := &eventLog{}
	bus.Subscribe("テスト用の購読者", log.handle)

	if _, err := scans.StartScan(ctx); err != nil {
		t.Fatal(err)
	}
	scans.Wait()
	if err := db.EnqueueJob(ctx, domain.JobProbe, video.ID); err != nil {
		t.Fatal(err)
	}
	ingest.JobFinished(domain.Job{Kind: domain.JobProbe, VideoID: video.ID})
	if err := db.DeleteVideos(ctx, []int64{video.ID}); err != nil {
		t.Fatal(err)
	}
	bus.Close()
	ingest.Wait()

	for name, match := range map[string]func(domain.Event) bool{
		"走査の状態の変化":   func(e domain.Event) bool { return e == domain.ScanChanged{} },
		"段階ごとの残りの変化": func(e domain.Event) bool { return e == domain.ProcessingChanged{} },
		"仕事が積まれたこと": func(e domain.Event) bool {
			queued, ok := e.(domain.JobsQueued)
			return ok && slices.Equal(queued.Kinds, []domain.JobKind{domain.JobProbe})
		},
		"動画の ingest 状態の変化": func(e domain.Event) bool {
			return e == domain.VideoIngestChanged{VideoID: video.ID, Stage: domain.JobProbe}
		},
		"動画の行が消えたこと": func(e domain.Event) bool { return e == domain.VideoIngestChanged{VideoID: video.ID} },
		"content key の参照が無くなったこと": func(e domain.Event) bool {
			released, ok := e.(domain.ContentUnreferenced)
			return ok && slices.Equal(released.ContentKeys, []string{"key-movie"})
		},
	} {
		if !log.has(match) {
			t.Errorf("%s が届かない", name)
		}
	}
}
