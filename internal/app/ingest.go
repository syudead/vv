package app

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/syudead/vv/internal/domain"
)

// IngestStore は取り込みのジョブが読み書きする保存先である。
type IngestStore interface {
	ContentIndex
	GetVideo(ctx context.Context, id int64) (domain.Video, error)
	EnqueueJob(ctx context.Context, kind domain.JobKind, videoID int64) error
	// JobIdentityCurrent は専有した時点の所在と内容が今も同じかを返す。
	JobIdentityCurrent(ctx context.Context, job domain.Job) (bool, error)
	// ContentKeyCurrent は動画の内容が key のままかを返す。
	ContentKeyCurrent(ctx context.Context, videoID int64, key string) (bool, error)
	// PreviewSourceCurrent はプレビューの元が専有した時点と同じかを返す。
	PreviewSourceCurrent(ctx context.Context, job domain.Job) (bool, error)
	ApplyProbeForJob(ctx context.Context, job domain.Job, probe domain.Probe, play domain.Playability) (bool, error)
	SetThumbnailStateForJob(ctx context.Context, job domain.Job, state domain.ThumbnailState) (bool, error)
	CompletePreviewForContent(ctx context.Context, job domain.Job) (bool, error)
}

// ContentIndex は内容の識別子を参照する動画があるかの問い合わせ先である。
type ContentIndex interface {
	ContentKeyReferenced(ctx context.Context, key string) (bool, error)
}

// ArtifactRemover は内容1つ分の生成物を消す。
type ArtifactRemover interface {
	RemoveContent(contentKey string) error
}

// ArtifactStore は生成物の置き場である。internal/artifacts の *Store がこれを
// 満たす。置き場の並べ方と一時置き場からの公開はそちらが持ち、ここは生成を
// write として渡す。
type ArtifactStore interface {
	ArtifactRemover
	// PublishThumbnail は write に一時置き場のパスを渡して書かせ、公開する。
	PublishThumbnail(contentKey string, write func(output string) error) error
	// PublishSeekThumbnails は完成したものがあれば write を呼ばない。write は
	// 連番のファイル名の型を受ける。
	PublishSeekThumbnails(contentKey string, write func(outputPattern string) error) error
	// PublishPreview は完成したものがあれば write を呼ばない。公開の直前に current を
	// 呼び、false なら公開せずに domain.ErrPreviewStale を返す。
	PublishPreview(ctx context.Context, contentKey string, write func(output string) error,
		current func(context.Context) (bool, error)) error
}

// Generator は元の動画を読み、渡されたパスへ生成物を書く。internal/media の
// *Assets がこれを満たす。
type Generator interface {
	// CheckSource は元の動画が読める通常ファイルかを確かめる。
	CheckSource(path string) error
	Probe(ctx context.Context, path string) (domain.Probe, error)
	Thumbnail(ctx context.Context, path string, durationMs int64, output string) error
	SeekThumbnails(ctx context.Context, path, outputPattern string) error
	Preview(ctx context.Context, path, output string, durationMs int64) error
}

// Waker は1つの段階のワーカーを起こす。internal/jobs の *Worker がこれを満たす。
type Waker interface {
	Wake()
}

// IngestOptions は取り込みの組み立てに必要な依存である。
type IngestOptions struct {
	Store     IngestStore
	Generator Generator
	Artifacts ArtifactStore
	// Notifier は nil なら知らせない。
	Notifier Notifier
	// Logger は nil なら slog の既定を使う。
	Logger *slog.Logger
}

// Ingest は取り込みの各段階（解析・サムネイル・プレビュー）のジョブの処理と、
// 段階の間の受け渡しを受け持つ。
//
// ジョブを取り出して成否を記録する進め方は internal/jobs が持ち、1件で何を
// するかはここが決める。
type Ingest struct {
	store     IngestStore
	generator Generator
	files     ArtifactStore
	notifier  Notifier
	artifacts *artifacts

	mu     sync.RWMutex
	wakers map[domain.JobKind]Waker
}

// NewIngest は取り込みを組み立てる。
func NewIngest(opts IngestOptions) *Ingest {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Ingest{
		store:     opts.Store,
		generator: opts.Generator,
		files:     opts.Artifacts,
		notifier:  opts.Notifier,
		artifacts: newArtifacts(opts.Store, opts.Artifacts, logger),
		wakers:    map[domain.JobKind]Waker{},
	}
}

// Handler は kind のジョブ1件を処理する関数を返す。知らない種類なら nil。
func (i *Ingest) Handler(kind domain.JobKind) func(context.Context, domain.Job) error {
	switch kind {
	case domain.JobProbe:
		return i.Probe
	case domain.JobThumbnail:
		return i.Thumbnail
	case domain.JobPreview:
		return i.Preview
	}
	return nil
}

// AttachWorker は kind の段階のワーカーを登録する。仕事が積まれたときと、
// 前の段階が終わったときに起こす。
func (i *Ingest) AttachWorker(kind domain.JobKind, worker Waker) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.wakers[kind] = worker
}

func (i *Ingest) wake(kind domain.JobKind) {
	i.mu.RLock()
	worker, ok := i.wakers[kind]
	i.mu.RUnlock()
	if ok {
		worker.Wake()
	}
}

// JobFinished は1件の成否が記録されたあとに呼ばれる。ワーカーの Finished に渡す。
//
// サムネイルは解析が終わるまで取り出されない（store.ClaimJob）。解析の成否が
// 決まったら、待っていたサムネイルのワーカーを起こす。そのうえで、その動画と
// 段階ごとの残りが変わったことを画面へ知らせる。
func (i *Ingest) JobFinished(job domain.Job) {
	if job.Kind == domain.JobProbe {
		i.wake(domain.JobThumbnail)
	}
	if i.notifier == nil {
		return
	}
	i.notifier.VideoChanged(job.VideoID)
	i.notifier.ProcessingChanged()
}

// JobsChanged は仕事が積まれた段階のワーカーを起こし、残りが変わったことを
// 画面へ知らせる。保存層の OnJobsChanged に渡す。
func (i *Ingest) JobsChanged(kinds []domain.JobKind) {
	for _, kind := range kinds {
		i.wake(kind)
	}
	if i.notifier != nil {
		i.notifier.ProcessingChanged()
	}
}

// VideosDeleted は、動画の行が消えたときに、参照の無くなった内容の生成物だけを
// 消し、開いている画面へ消えたことを知らせる（取り直すと見つからないので、画面が
// 外す）。保存層の OnVideosDeleted に渡す。
func (i *Ingest) VideosDeleted(deleted []domain.DeletedVideo) {
	keys := make([]string, 0, len(deleted))
	for _, video := range deleted {
		keys = append(keys, video.ContentKey)
		if i.notifier != nil {
			i.notifier.VideoChanged(video.ID)
		}
	}
	i.artifacts.release(keys)
	if i.notifier != nil {
		i.notifier.ProcessingChanged()
	}
}

// Wait は背後で動いている生成物の削除の終わりを待つ。データベースを閉じる前に
// 呼ぶ。途中で閉じると、消すはずの生成物が残り続ける。
func (i *Ingest) Wait() {
	i.artifacts.wait()
}

// Probe は ffprobe の結果を索引へ反映し、プレビューの仕事を積む。
//
// 再生可否の判定は internal/domain の純粋関数が行い、ここはその結果を
// 保存層へ渡すだけである。
func (i *Ingest) Probe(ctx context.Context, job domain.Job) error {
	current, err := i.store.JobIdentityCurrent(ctx, job)
	if err != nil {
		return err
	}
	if !current {
		return nil
	}
	if err := i.generator.CheckSource(job.LocationPath); err != nil {
		return err
	}

	// 上限まで試して駄目なときの失敗は、ジョブを failed にするのと同じ取引で
	// FailClaimedJob が動画側へ記録する。行は残したままで、一覧からは消さない。
	probe, err := i.generator.Probe(ctx, job.LocationPath)
	if err != nil {
		return err
	}

	playability := domain.EvaluatePlayability(domain.ContainerFromPath(job.LocationPath), probe)
	applied, err := i.store.ApplyProbeForJob(ctx, job, probe, playability)
	if err != nil || !applied {
		return err
	}
	return i.store.EnqueueJob(ctx, domain.JobPreview, job.VideoID)
}

// Thumbnail は代表サムネイルを1枚とシーク用プレビューを生成し、状態を記録する。
func (i *Ingest) Thumbnail(ctx context.Context, job domain.Job) error {
	video, err := i.store.GetVideo(ctx, job.VideoID)
	if err != nil {
		return err
	}

	var durationMs int64
	if video.DurationMs != nil {
		durationMs = *video.DurationMs
	}

	current, err := i.store.JobIdentityCurrent(ctx, job)
	if err != nil {
		return err
	}
	if !current {
		return nil
	}
	if err := i.generator.CheckSource(job.LocationPath); err != nil {
		return err
	}
	// 生成（既存のファイルの採用を含む）から完了の記録までを、同じ内容の
	// 生成物の削除と直列にする。
	unlock := i.artifacts.lock(job.ContentKey)
	defer unlock()
	// 上限まで試して駄目なときの失敗は、ジョブを failed にするのと同じ取引で
	// FailClaimedJob が動画側へ記録する。
	if video.ThumbnailState != domain.ThumbnailStateDone {
		if err := i.files.PublishThumbnail(job.ContentKey, func(output string) error {
			return i.generator.Thumbnail(ctx, job.LocationPath, durationMs, output)
		}); err != nil {
			return err
		}
		applied, err := i.store.SetThumbnailStateForJob(ctx, job, domain.ThumbnailStateDone)
		if err != nil {
			return err
		}
		if !applied {
			// 生成中に動画が消えていたら、書き終えたサムネイルを残さない。
			return i.artifacts.removeIfUnreferencedLocked(context.WithoutCancel(ctx), job.ContentKey)
		}
	}
	if err := i.files.PublishSeekThumbnails(job.ContentKey, func(outputPattern string) error {
		return i.generator.SeekThumbnails(ctx, job.LocationPath, outputPattern)
	}); err != nil {
		return err
	}

	// 生成中にスキャンが動画を消すことがある。書き終えたあとで確かめ直し、
	// 参照の無くなった生成物を残さない。
	return i.artifacts.removeIfUnreferencedLocked(context.WithoutCancel(ctx), job.ContentKey)
}

// errMissingDuration は解析済みなのに長さが無く、プレビューを作れないことを表す。
var errMissingDuration = errors.New("プレビュー生成に必要な動画の長さがありません")

// Preview は一覧用プレビューを生成し、完了を記録する。
func (i *Ingest) Preview(ctx context.Context, job domain.Job) error {
	video, err := i.store.GetVideo(ctx, job.VideoID)
	if err != nil {
		return err
	}
	if video.ProbeState != domain.ProbeStateDone {
		return nil
	}
	if video.DurationMs == nil || *video.DurationMs <= 0 {
		return errMissingDuration
	}
	current, err := i.store.ContentKeyCurrent(ctx, job.VideoID, job.ContentKey)
	if err != nil || !current {
		return err
	}
	if err := i.generator.CheckSource(job.LocationPath); err != nil {
		return err
	}
	validateContent := func(validateCtx context.Context) (bool, error) {
		return i.store.PreviewSourceCurrent(validateCtx, job)
	}
	// 生成（既存のファイルの採用を含む）から完了の記録までを、同じ内容の
	// 生成物の削除と直列にする。
	unlock := i.artifacts.lock(job.ContentKey)
	defer unlock()
	durationMs := *video.DurationMs
	if err := i.files.PublishPreview(ctx, job.ContentKey, func(output string) error {
		return i.generator.Preview(ctx, job.LocationPath, output, durationMs)
	}, validateContent); err != nil {
		return err
	}
	applied, err := i.store.CompletePreviewForContent(context.WithoutCancel(ctx), job)
	if err != nil {
		return err
	}
	if !applied {
		// 生成中に動画が消えていたら、書き終えたプレビューを残さない。
		if err := i.artifacts.removeIfUnreferencedLocked(context.WithoutCancel(ctx), job.ContentKey); err != nil {
			return err
		}
		return domain.ErrPreviewStale
	}
	return nil
}
