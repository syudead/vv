package app

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// このファイルの偽物は、アプリケーション層が宣言した interface を満たすだけの
// 決め打ちである。SQLite・ffmpeg/ffprobe・HTTP サーバーのどれも起動しない。

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fakePublisher は発行された変化を記録する。
type fakePublisher struct {
	mu     sync.Mutex
	events []domain.Event
}

func (f *fakePublisher) Publish(events ...domain.Event) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, events...)
}

func (f *fakePublisher) published() []domain.Event {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]domain.Event(nil), f.events...)
}

// fakeIngestStore は取り込みのジョブが読み書きする保存先の偽物である。
type fakeIngestStore struct {
	mu sync.Mutex

	videos map[int64]domain.Video
	// stale が真なら、専有した時点から所在や内容が変わったものとして答える。
	stale bool
	// gone が真なら、結果を反映する時点で動画が消えていたものとして答える。
	gone bool
	// referenced は内容の識別子を参照する動画があるか。
	referenced map[string]bool

	appliedProbes   []domain.Probe
	enqueued        []domain.JobKind
	thumbnailStates []domain.ThumbnailState
	previewsDone    int
}

func newFakeIngestStore(videos ...domain.Video) *fakeIngestStore {
	store := &fakeIngestStore{videos: map[int64]domain.Video{}, referenced: map[string]bool{}}
	for _, video := range videos {
		store.videos[video.ID] = video
		store.referenced[video.ContentKey] = true
	}
	return store
}

func (f *fakeIngestStore) ContentKeyReferenced(_ context.Context, key string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.referenced[key], nil
}

func (f *fakeIngestStore) GetVideo(_ context.Context, id int64) (domain.Video, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	video, ok := f.videos[id]
	if !ok {
		return domain.Video{}, domain.ErrNotFound
	}
	return video, nil
}

func (f *fakeIngestStore) EnqueueJob(_ context.Context, kind domain.JobKind, _ int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.enqueued = append(f.enqueued, kind)
	return nil
}

func (f *fakeIngestStore) JobIdentityCurrent(context.Context, domain.Job) (bool, error) {
	return !f.stale, nil
}

func (f *fakeIngestStore) ContentKeyCurrent(context.Context, int64, string) (bool, error) {
	return !f.stale, nil
}

func (f *fakeIngestStore) PreviewSourceCurrent(context.Context, domain.Job) (bool, error) {
	return !f.stale, nil
}

func (f *fakeIngestStore) ApplyProbeForJob(
	_ context.Context, _ domain.Job, probe domain.Probe, _ domain.Playability,
) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.gone {
		return false, nil
	}
	f.appliedProbes = append(f.appliedProbes, probe)
	return true, nil
}

func (f *fakeIngestStore) SetThumbnailStateForJob(
	_ context.Context, job domain.Job, state domain.ThumbnailState,
) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.gone {
		delete(f.referenced, job.ContentKey)
		return false, nil
	}
	f.thumbnailStates = append(f.thumbnailStates, state)
	return true, nil
}

func (f *fakeIngestStore) CompletePreviewForContent(_ context.Context, job domain.Job) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.gone {
		delete(f.referenced, job.ContentKey)
		return false, nil
	}
	f.previewsDone++
	return true, nil
}

// fakeGenerator は生成の呼び出しを記録し、決め打ちの結果を返す。生成物の置き場
// （ArtifactStore）も兼ね、公開は write を呼ぶだけにする。
type fakeGenerator struct {
	mu sync.Mutex

	sourceErr    error
	probe        domain.Probe
	probeErr     error
	previewErr   error
	thumbnailErr error
	seekErr      error
	// validated はプレビューの公開の直前に確かめた元の同一性。
	validated []bool
	// outputs は生成に渡した書き出し先。置き場が渡したものと同じであること。
	outputs []string

	calls   []string
	removed []string
}

func (f *fakeGenerator) record(call string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
}

func (f *fakeGenerator) recordOutput(output string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.outputs = append(f.outputs, output)
}

func (f *fakeGenerator) CheckSource(string) error {
	f.record("check")
	return f.sourceErr
}

func (f *fakeGenerator) Probe(context.Context, string) (domain.Probe, error) {
	f.record("probe")
	return f.probe, f.probeErr
}

func (f *fakeGenerator) Thumbnail(_ context.Context, _ string, _ int64, output string) error {
	f.record("thumbnail")
	f.recordOutput(output)
	return f.thumbnailErr
}

func (f *fakeGenerator) SeekThumbnails(_ context.Context, _, outputPattern string) error {
	f.record("seek")
	f.recordOutput(outputPattern)
	return f.seekErr
}

func (f *fakeGenerator) Preview(_ context.Context, _, output string, _ int64) error {
	f.record("preview")
	f.recordOutput(output)
	return f.previewErr
}

func (f *fakeGenerator) PublishThumbnail(contentKey string, write func(string) error) error {
	return write("tmp/thumbnail/" + contentKey)
}

func (f *fakeGenerator) PublishSeekThumbnails(contentKey string, write func(string) error) error {
	return write("tmp/seek/" + contentKey)
}

func (f *fakeGenerator) PublishPreview(
	ctx context.Context, contentKey string, write func(string) error, current func(context.Context) (bool, error),
) error {
	if err := write("tmp/preview/" + contentKey); err != nil {
		return err
	}
	ok, err := current(ctx)
	if err != nil {
		return err
	}
	f.mu.Lock()
	f.validated = append(f.validated, ok)
	f.mu.Unlock()
	if !ok {
		return domain.ErrPreviewStale
	}
	return nil
}

func (f *fakeGenerator) RemoveContent(contentKey string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removed = append(f.removed, contentKey)
	return nil
}

func (f *fakeGenerator) snapshot() (calls, removed []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...), append([]string(nil), f.removed...)
}

// probedVideo は解析済みの動画を1件返す。
func probedVideo(id int64, contentKey string) domain.Video {
	duration := int64(60_000)
	return domain.Video{
		ID:             id,
		Path:           "/media/show/" + contentKey + ".mp4",
		Title:          contentKey,
		AddedAt:        time.Unix(1_757_000_000, 0),
		ContentKey:     contentKey,
		DurationMs:     &duration,
		Container:      "mp4",
		VideoCodec:     "h264",
		AudioCodec:     "aac",
		ProbeState:     domain.ProbeStateDone,
		ThumbnailState: domain.ThumbnailStatePending,
		PreviewState:   domain.PreviewStatePending,
	}
}

func jobFor(kind domain.JobKind, video domain.Video) domain.Job {
	return domain.Job{
		ID: 1, Kind: kind, VideoID: video.ID, LocationPath: video.Path, ContentKey: video.ContentKey,
	}
}
