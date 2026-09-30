package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/syudead/vv/internal/domain"
)

// IngestStore は取り込みのジョブが読み書きする保存先である。
type IngestStore interface {
	ContentIndex
	GetVideo(ctx context.Context, id int64) (domain.Video, error)
	// JobIdentityCurrent は専有した時点の所在と内容が今も同じかを返す。
	JobIdentityCurrent(ctx context.Context, job domain.Job) (bool, error)
	// ContentKeyCurrent は動画の内容が key のままかを返す。
	ContentKeyCurrent(ctx context.Context, videoID int64, key string) (bool, error)
	// PreviewSourceCurrent はプレビューの元が専有した時点と同じかを返す。
	PreviewSourceCurrent(ctx context.Context, job domain.Job) (bool, error)
	// ThumbnailSourceCurrent は動画の内容が key のままで、所在 locationPath が今も内容 key の
	// 動画の所在かを返す。
	ThumbnailSourceCurrent(ctx context.Context, videoID int64, key, locationPath string) (bool, error)
	// ApplyProbeForJob は解析の結果を書き、同じ取引で一覧用プレビューの仕事を積む。
	ApplyProbeForJob(ctx context.Context, job domain.Job, probe domain.Probe, play domain.Playability) (bool, error)
	// SetThumbnailStateForJob と SetSeekThumbnailStateForJob は、成功を書く取引で
	// substitution に従って代用の問題を入れる・消す。
	SetThumbnailStateForJob(
		ctx context.Context, job domain.Job, state domain.ThumbnailState, substitution domain.Substitution) (bool, error)
	SetSeekThumbnailStateForJob(
		ctx context.Context, job domain.Job, state domain.SeekThumbnailState, substitution domain.Substitution) (bool, error)
	CompletePreviewForContent(ctx context.Context, job domain.Job) (bool, error)
	// ApplyFingerprintForJob は専有した時点の内容と所在が今も同じときだけ、その内容の
	// 映像の指紋を置き換え、反映したかを返す。
	ApplyFingerprintForJob(ctx context.Context, job domain.Job, fingerprint domain.Fingerprint) (bool, error)
	// SetThumbnailPosition は代表サムネイルの位置を記録し（nil は解除）、同じ取引でその内容の
	// 動画の thumbnail_state を done にする。画像を公開したあと、生成の錠の中で呼ぶ
	// （specs/029-video-overrides/data-model.md §3）。
	SetThumbnailPosition(ctx context.Context, videoID int64, positionMs *int64) (domain.Video, error)
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
	// StashThumbnail は今の代表サムネイルを写しておき、PublishThumbnail で置き換えたあとに
	// 前の画像へ戻す restore と、写しを捨てる discard を返す。前の画像が無ければ restore は
	// 置き換えた画像を消す。
	StashThumbnail(contentKey string) (restore func() error, discard func(), err error)
	// PublishSeekThumbnails は完成したもの（配置情報のある置き場）があれば write を
	// 呼ばない。write は一時置き場のディレクトリを受け、layout の配置でシートを書き、
	// 全編の復号から作ったかを返す。配置情報の無い置き場（旧形式・途中で壊れたもの）は
	// 公開の直前に消す。公開した、または採用したスプライトが代用で作られたかを返し、
	// 採用したものに記録が無ければ SubstitutionUnknown を返す。
	PublishSeekThumbnails(contentKey string, layout domain.SeekSpriteLayout,
		write func(outputDir string) (fullDecode bool, err error)) (domain.Substitution, error)
	// PublishPreview は完成したものがあれば write を呼ばない。公開の直前に current を
	// 呼び、false なら公開せずに domain.ErrPreviewStale を返す。
	PublishPreview(ctx context.Context, contentKey string, write func(output string) error,
		current func(context.Context) (bool, error)) error
	// SeekSprite は完成したシーク用スプライトの配置情報を読む。無ければ誤りを返す。
	SeekSprite(contentKey string) (domain.SeekSprite, error)
	// SeekSpriteSheet はシーク用スプライトのシート sheet（0 から）の JPEG を読む。
	SeekSpriteSheet(contentKey string, sheet int) ([]byte, error)
}

// Generator は元の動画を読み、渡されたパスへ生成物を書く。生成物から映像の指紋も
// 作る。internal/media の *Assets がこれを満たす。
type Generator interface {
	// CheckSource は元の動画が読める通常ファイルかを確かめる。
	CheckSource(path string) error
	Probe(ctx context.Context, path string) (domain.Probe, error)
	// Thumbnail は代表サムネイルを output へ書き、指定位置で取れず先頭のコマで
	// 作ったかを返す。
	Thumbnail(ctx context.Context, path string, durationMs int64, output string) (firstFrame bool, err error)
	// ThumbnailAt は positionMs の場面の代表サムネイルを output へ書く。先頭のコマへの
	// 代用はしない。
	ThumbnailAt(ctx context.Context, path string, positionMs int64, output string) error
	// SeekSprite はシーク用サムネイルのシートを layout の配置で outputDir へ書き、
	// 区間ごとの抽出にも失敗して全編から作ったかを返す。
	SeekSprite(ctx context.Context, path, outputDir string, layout domain.SeekSpriteLayout) (fullDecode bool, err error)
	Preview(ctx context.Context, path, output string, durationMs int64) error
	// SpriteFingerprint は完成したシーク用スプライトのシート（シート 0 から順の JPEG）から
	// 映像の指紋を作る。ffmpeg は起動しない。
	SpriteFingerprint(sprite domain.SeekSprite, sheets [][]byte) (domain.Fingerprint, error)
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

// Ingest は取り込みの各段階（解析・代表サムネイル・シーク用サムネイル・
// プレビュー・映像の指紋）のジョブの処理と、
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
	case domain.JobSeekThumbnail:
		return i.SeekThumbnails
	case domain.JobPreview:
		return i.Preview
	case domain.JobFingerprint:
		return i.Fingerprint
	}
	return nil
}

// JobFinished は1件の成否が記録されたあとに呼ばれる。ワーカーの Finished に渡す。
//
// その動画の取り込みの状態と、段階ごとの残りが変わったことを発行する。
// 解析の成否が決まったら待っていたサムネイルのワーカーを起こす、代表サムネイルの
// 成否が決まったら待っていたシーク用サムネイルのワーカーを起こす、という段階の
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

// Probe は ffprobe の結果を索引へ反映する。プレビューの仕事は保存側が同じ取引で積む。
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
	// 一覧用プレビューの仕事は、結果を書くのと同じ取引で保存側が積む
	// （specs/024-import-progress/research.md R-2）。
	_, err = i.store.ApplyProbeForJob(ctx, job, probe, playability)
	return err
}

// Thumbnail は代表サムネイルを1枚生成し、状態を記録する。シーク用サムネイルは
// 別の段階（SeekThumbnails）が作るので、ここでは待たない。
//
// 所有者が位置を指定していればその場面で（ThumbnailAt）、無ければ自動の位置で作る
// （specs/029-video-overrides/research.md R-5）。位置と thumbnail_state は生成の錠に
// 入ってから読み直した値で決める。錠を待つ間に SetThumbnailPosition が位置と done を
// 記録していたら、古い値で画像を上書きせずに生成を飛ばす。
func (i *Ingest) Thumbnail(ctx context.Context, job domain.Job) error {
	// 錠の外の読み出しは、動画がまだあるかの確認だけに使う。
	if _, err := i.store.GetVideo(ctx, job.VideoID); err != nil {
		return err
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
	// 生成物の削除と直列にする。別の種類の生成は待たない。
	return i.artifacts.generate(ctx, job.ContentKey, artifactThumbnail, func() (bool, error) {
		video, err := i.store.GetVideo(ctx, job.VideoID)
		if errors.Is(err, domain.ErrNotFound) {
			// 錠を待つ間に動画が消えた。残った生成物は後始末に任せる。
			return true, nil
		}
		if err != nil {
			return false, err
		}
		// 上限まで試して駄目なときの失敗は、ジョブを failed にするのと同じ取引で
		// FailClaimedJob が動画側へ記録する。指定の位置で取れなくても行は消さない。
		if video.ThumbnailState != domain.ThumbnailStateDone {
			// 先頭のコマでの代用は、成功を書く取引で問題として記録する
			// （specs/024-import-progress/research.md R-7）。指定の位置では代用しない。
			substitution := domain.SubstitutionUnknown
			if err := i.files.PublishThumbnail(job.ContentKey, func(output string) error {
				if video.ThumbnailPositionMs != nil {
					substitution = domain.SubstitutionNone
					return i.generator.ThumbnailAt(ctx, job.LocationPath, *video.ThumbnailPositionMs, output)
				}
				firstFrame, err := i.generator.Thumbnail(ctx, job.LocationPath, durationOf(video), output)
				substitution = domain.SubstitutionOf(firstFrame)
				return err
			}); err != nil {
				return false, err
			}
			// 生成中に動画が消えていたら（applied が偽）、書き終えたサムネイルを
			// 後始末で残さない。
			if _, err := i.store.SetThumbnailStateForJob(ctx, job, domain.ThumbnailStateDone, substitution); err != nil {
				return false, err
			}
		}
		// 生成中にスキャンが動画を消すことがある。書き終えたあとで確かめ直し、
		// 参照の無くなった生成物を残さない。
		return true, nil
	})
}

// SetThumbnailPosition は動画 videoID の代表サムネイルを positionMs の場面で作り直して
// 公開し、位置を記録する。positionMs が nil なら位置を解除し、自動の位置で作り直す
// （specs/029-video-overrides/research.md R-4）。locationPath は読む元の動画の所在（登録の
// パス）、path はそれを辿った先で、呼び出し側が開けることを確かめたものを渡す。
//
// 位置は domain.CheckThumbnailPosition で確かめ、解析前は domain.ErrDurationUnknown、尺の外は
// domain.ErrThumbnailPositionOutOfRange を返す。生成と記録は取り込みの job と同じ生成の錠の
// 中で行うので、同じ内容への指定と job は直列になり、最後に記録した位置の画像が残る。
// 生成に失敗したら何も記録せず domain.ErrThumbnailFrameUnavailable を返す。置き場は
// 一時置き場から置き換えるので、前の画像はそのまま残る。
//
// 走査は生成の錠を取らないので、要求が所在を決めたあとに、その所在が別の内容へ付け替わる
// ことがある。生成の直前、生成のあとで公開する前、記録の直前に、所在が今も動画の内容のものかを
// 確かめ直し、違えば別の動画のコマを公開・記録せずに domain.ErrMediaFileUnavailable を返す。公開のあとで記録
// できなかったときは（確かめ直しの失敗・取引の失敗・要求の取り消し）、前の画像へ戻し、
// 記録の位置と版に画像を揃える。
func (i *Ingest) SetThumbnailPosition(
	ctx context.Context, videoID int64, locationPath, path string, positionMs *int64,
) (domain.Video, error) {
	video, err := i.store.GetVideo(ctx, videoID)
	if err != nil {
		return domain.Video{}, err
	}
	if positionMs != nil {
		if err := domain.CheckThumbnailPosition(video, *positionMs); err != nil {
			return domain.Video{}, err
		}
	}
	if video.ContentKey == "" {
		// 内容を読めていない動画は、生成物の置き場も上書きの行も持てない。
		return domain.Video{}, domain.ErrNotFound
	}

	var saved domain.Video
	err = i.artifacts.generate(ctx, video.ContentKey, artifactThumbnail, func() (bool, error) {
		if err := i.checkThumbnailSource(ctx, videoID, video.ContentKey, locationPath); err != nil {
			return false, err
		}
		restore, discard, err := i.files.StashThumbnail(video.ContentKey)
		if err != nil {
			return false, err
		}
		var generateErr, sourceErr error
		if err := i.files.PublishThumbnail(video.ContentKey, func(output string) error {
			if positionMs != nil {
				generateErr = i.generator.ThumbnailAt(ctx, path, *positionMs, output)
			} else {
				_, generateErr = i.generator.Thumbnail(ctx, path, durationOf(video), output)
			}
			if generateErr != nil {
				return generateErr
			}
			// 生成の間に付け替わっていたら、別の動画のコマを置き場へ移さない。公開の改名の
			// 前に確かめるので、並ぶ要求にもその画像は見えない。
			sourceErr = i.checkThumbnailSource(ctx, videoID, video.ContentKey, locationPath)
			return sourceErr
		}); err != nil {
			discard()
			if generateErr != nil {
				return false, errors.Join(domain.ErrThumbnailFrameUnavailable, generateErr)
			}
			if sourceErr != nil {
				return false, sourceErr
			}
			return false, err
		}
		// 公開した画像は、記録できたときだけ残す。公開から記録までの間に付け替わったときや
		// 記録できなければ前の画像へ戻し、生成中に動画が消えていたら、戻した画像も後始末で
		// 残さない。
		if err := i.checkThumbnailSource(ctx, videoID, video.ContentKey, locationPath); err != nil {
			return true, errors.Join(err, restore())
		}
		recorded, err := i.store.SetThumbnailPosition(ctx, videoID, positionMs)
		if err != nil {
			return true, errors.Join(err, restore())
		}
		discard()
		saved = recorded
		return true, nil
	})
	if err != nil {
		return domain.Video{}, err
	}
	return saved, nil
}

// checkThumbnailSource は、所在 locationPath が今も動画 videoID の内容 key のものかを確かめる。
// 動画が消えていれば domain.ErrNotFound、所在が消えたか別の内容へ付け替わっていれば
// domain.ErrMediaFileUnavailable を返す。
func (i *Ingest) checkThumbnailSource(ctx context.Context, videoID int64, key, locationPath string) error {
	current, err := i.store.ThumbnailSourceCurrent(ctx, videoID, key, locationPath)
	if err != nil {
		return err
	}
	if current {
		return nil
	}
	if _, err := i.store.GetVideo(ctx, videoID); err != nil {
		return err
	}
	return domain.ErrMediaFileUnavailable
}

// durationOf は動画の尺（ミリ秒）を返す。分からなければ 0。
func durationOf(video domain.Video) int64 {
	if video.DurationMs == nil {
		return 0
	}
	return *video.DurationMs
}

// SeekThumbnails はシーク用サムネイル（スプライトシート）を生成し、
// 状態を記録する。配置は動画の長さから domain.NewSeekSpriteLayout で決める。
// 置き場に完成したものがあれば生成せず、状態だけを記録する。
//
// 代表サムネイルと同じく、完了は専有した時点の内容・所在・所在の世代が今も
// 同じときだけ記録する。上限まで試して駄目なときの失敗は、FailClaimedJob が
// seek_thumbnail_state だけへ記録し、代表サムネイルの状態には触れない。
func (i *Ingest) SeekThumbnails(ctx context.Context, job domain.Job) error {
	video, err := i.store.GetVideo(ctx, job.VideoID)
	if err != nil {
		return err
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
	var durationMs int64
	if video.DurationMs != nil {
		durationMs = *video.DurationMs
	}
	layout := domain.NewSeekSpriteLayout(durationMs)
	// 生成（既存のファイルの採用を含む）から完了の記録までを、同じ内容の
	// 生成物の削除と直列にする。別の種類の生成は待たない。
	return i.artifacts.generate(ctx, job.ContentKey, artifactSeekThumbnails, func() (bool, error) {
		// 全編からの作り直しは、成功を書く取引で問題として記録する（R-7）。置き場に
		// 完成したものがあって生成しなかったときは、置き場が生成時に残した記録を使う。
		// 公開と完了の記録の間で止まった後の再実行や、同じ内容の別の動画でも代用を
		// 取りこぼさない。記録の無い旧いものは代用したかが分からないので、問題の行を
		// 変えない。
		substitution, err := i.files.PublishSeekThumbnails(job.ContentKey, layout, func(outputDir string) (bool, error) {
			return i.generator.SeekSprite(ctx, job.LocationPath, outputDir, layout)
		})
		if err != nil {
			return false, err
		}
		// 生成中に動画が消えていたら（applied が偽）、書き終えたシーク用サムネイルを
		// 後始末で残さない。
		if _, err := i.store.SetSeekThumbnailStateForJob(ctx, job, domain.SeekThumbnailDone, substitution); err != nil {
			return false, err
		}
		// 生成中にスキャンが動画を消すことがある。書き終えたあとで確かめ直し、
		// 参照の無くなった生成物を残さない。
		return true, nil
	})
}

// errMissingDuration は解析済みなのに長さが無く、プレビューを作れないことを表す。
var errMissingDuration = errors.New("the video has no duration to generate a preview from")

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
	durationMs := *video.DurationMs
	// 生成（既存のファイルの採用を含む）から完了の記録までを、同じ内容の
	// 生成物の削除と直列にする。別の種類の生成は待たない。
	return i.artifacts.generate(ctx, job.ContentKey, artifactPreview, func() (bool, error) {
		if err := i.files.PublishPreview(ctx, job.ContentKey, func(output string) error {
			return i.generator.Preview(ctx, job.LocationPath, output, durationMs)
		}, validateContent); err != nil {
			return false, err
		}
		applied, err := i.store.CompletePreviewForContent(context.WithoutCancel(ctx), job)
		if err != nil {
			return false, err
		}
		if !applied {
			// 生成中に動画が消えていたら、書き終えたプレビューを後始末で残さない。
			return true, domain.ErrPreviewStale
		}
		return false, nil
	})
}

// Fingerprint は完成したシーク用スプライトから映像の指紋を作り、記録する
// （specs/030-video-versions/data-model.md §6）。ffmpeg を起動せず、元の動画も読まない。
//
// スプライトが無い、またはシートが読めなければ誤りを返す。仕事は再試行し、上限まで
// 失敗すると FailClaimedJob が問題として記録する。積み直しは次の走査が行う。
func (i *Ingest) Fingerprint(ctx context.Context, job domain.Job) error {
	current, err := i.store.JobIdentityCurrent(ctx, job)
	if err != nil {
		return err
	}
	if !current {
		return nil
	}
	sprite, err := i.files.SeekSprite(job.ContentKey)
	if err != nil {
		return fmt.Errorf("cannot read the seek sprite for the fingerprint: %w", err)
	}
	sheets := make([][]byte, sprite.SheetCount)
	for sheet := range sheets {
		if sheets[sheet], err = i.files.SeekSpriteSheet(job.ContentKey, sheet); err != nil {
			return fmt.Errorf("cannot read seek sprite sheet %d for the fingerprint: %w", sheet, err)
		}
	}
	fingerprint, err := i.generator.SpriteFingerprint(sprite, sheets)
	if err != nil {
		return err
	}
	_, err = i.store.ApplyFingerprintForJob(ctx, job, fingerprint)
	return err
}
