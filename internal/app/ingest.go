package app

import (
	"context"
	"errors"
	"log/slog"

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

// IngestOptions は取り込みの組み立てに必要な依存である。
type IngestOptions struct {
	Store     IngestStore
	Generator Generator
	Artifacts ArtifactStore
	// Publisher は nil なら発行しない。
	Publisher Publisher
	// Logger は nil なら slog の既定を使う。
	Logger *slog.Logger
}

// Ingest は取り込みの各段階（解析・サムネイル・プレビュー）のジョブの処理と、
// 参照の無くなった内容の生成物の削除を受け持つ。
//
// ジョブを取り出して成否を記録する進め方は internal/jobs が持ち、1件で何を
// するかはここが決める。
type Ingest struct {
	store     IngestStore
	generator Generator
	files     ArtifactStore
	publisher Publisher
	artifacts *artifacts
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
		publisher: opts.Publisher,
		artifacts: newArtifacts(opts.Store, opts.Artifacts, logger),
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

// JobFinished は1件の成否が記録されたあとに呼ばれる。ワーカーの Finished に渡す。
//
// その動画の取り込みの状態と、段階ごとの残りが変わったことを発行する。
// 解析の成否が決まったら待っていたサムネイルのワーカーを起こす、という段階の
// 間の受け渡しは、この発行の購読として cmd/mdm が登録する。
func (i *Ingest) JobFinished(job domain.Job) {
	if i.publisher == nil {
		return
	}
	i.publisher.Publish(
		domain.VideoIngestChanged{VideoID: job.VideoID, Stage: job.Kind},
		domain.ProcessingChanged{},
	)
}

// ReleaseArtifacts は、参照の無くなった内容の生成物を消す。消す直前に参照を
// 確かめ直し、同じ内容の動画が取り込み直されていれば残す。
// domain.ContentUnreferenced の購読として cmd/mdm が登録する。
func (i *Ingest) ReleaseArtifacts(event domain.ContentUnreferenced) {
	i.artifacts.release(event.ContentKeys)
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
