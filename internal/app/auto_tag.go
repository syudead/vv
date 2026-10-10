package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// autoTagRetryDelay は、待ち行列からの取り出し自体が失敗したときに次に試すまでの間隔である。
const autoTagRetryDelay = 5 * time.Second

// autoTagCheckTimeout は接続の確かめの上限時間である。模型の読み込みを含むので長めにとる。
const autoTagCheckTimeout = 5 * time.Minute

// maxAutoTagErrorLength は残す失敗の理由の長さの上限である。
const maxAutoTagErrorLength = 500

// AutoTagQueue は自動タグ付けの待ち行列である。internal/store の *AutoTagStore が満たす。
type AutoTagQueue interface {
	QueueVideos(ctx context.Context, videoIDs []int64, mode domain.AutoTagQueueMode) (int, error)
	QueueLibrary(ctx context.Context, mode domain.AutoTagQueueMode) (int, error)
	// Claim は積んだ順にいちばん古い判定を専有する。無ければ domain.ErrNoJob。
	Claim(ctx context.Context) (domain.AutoTagJob, error)
	// Subject は内容の動画を返す。無ければ domain.ErrNotFound。
	Subject(ctx context.Context, contentKey string) (domain.Video, error)
	Candidates(ctx context.Context) ([]domain.AutoTagCandidate, error)
	// Finish は判定を終え、tagIDs のタグを付ける。
	Finish(ctx context.Context, contentKey string, tagIDs []int64) error
	Fail(ctx context.Context, contentKey, reason string) error
	RequeueRunning(ctx context.Context) (int, error)
	Counts(ctx context.Context) (domain.AutoTagCounts, error)
}

// AutoTagSettingsStore は自動タグ付けの設定の保存先である。internal/store の *SettingsStore が満たす。
type AutoTagSettingsStore interface {
	AutoTagSettings(ctx context.Context) (domain.AutoTagSettings, error)
	SaveAutoTagSettings(ctx context.Context, settings domain.AutoTagSettings) error
}

// AutoTagClassifier は判定モデルに質問ごとの「はい」の確率を聞く。internal/clef の *Client が満たす。
type AutoTagClassifier interface {
	Classify(ctx context.Context, req domain.AutoTagRequest) (map[string]float64, error)
}

// ThumbnailSource は代表サムネイルの JPEG を読む。無ければ誤りを返す。
type ThumbnailSource interface {
	ThumbnailJPEG(contentKey string) ([]byte, error)
}

// AutoTaggerOptions は自動タグ付けに必要な依存である。
type AutoTaggerOptions struct {
	Queue      AutoTagQueue
	Settings   AutoTagSettingsStore
	Classifier AutoTagClassifier
	Thumbnails ThumbnailSource
	// Logger は nil なら slog の既定を使う。
	Logger *slog.Logger
}

// AutoTagger は動画を判定モデルに回し、確率が閾値以上の既存のタグを付ける
// （docs/design-docs/auto-tagging.md）。判定は 1 件ずつ、待ち行列の順に行う。
type AutoTagger struct {
	queue      AutoTagQueue
	settings   AutoTagSettingsStore
	classifier AutoTagClassifier
	thumbnails ThumbnailSource
	logger     *slog.Logger
	wake       chan struct{}

	// saveMu は設定の保存を 1 つずつにする。
	saveMu sync.Mutex
}

// NewAutoTagger は自動タグ付けを組み立てる。判定は Run で始める。
func NewAutoTagger(opts AutoTaggerOptions) *AutoTagger {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &AutoTagger{
		queue:      opts.Queue,
		settings:   opts.Settings,
		classifier: opts.Classifier,
		thumbnails: opts.Thumbnails,
		logger:     logger.With(slog.String("component", "auto-tagging")),
		// 容量 1 で、起こす知らせを 1 つだけ貯める。
		wake: make(chan struct{}, 1),
	}
}

// Wake は判定が積まれたことを知らせる。ブロックしない。
func (a *AutoTagger) Wake() {
	select {
	case a.wake <- struct{}{}:
	default:
	}
}

// Status は設定と待ち行列の今の状態を返す。
func (a *AutoTagger) Status(ctx context.Context) (domain.AutoTagStatus, error) {
	settings, err := a.settings.AutoTagSettings(ctx)
	if err != nil {
		return domain.AutoTagStatus{}, err
	}
	counts, err := a.queue.Counts(ctx)
	if err != nil {
		return domain.AutoTagStatus{}, err
	}
	return domain.AutoTagStatus{Settings: settings, Counts: counts}, nil
}

// SaveSettings は設定を確かめて保存し、保存後の状態を返す。規則に合わなければ
// domain.ErrInvalidAutoTagSettings を包んだ誤りを返す。
func (a *AutoTagger) SaveSettings(ctx context.Context, settings domain.AutoTagSettings) (domain.AutoTagStatus, error) {
	normalized, err := domain.NormalizeAutoTagSettings(settings)
	if err != nil {
		return domain.AutoTagStatus{}, err
	}
	a.saveMu.Lock()
	err = a.settings.SaveAutoTagSettings(ctx, normalized)
	a.saveMu.Unlock()
	if err != nil {
		return domain.AutoTagStatus{}, err
	}
	return a.Status(ctx)
}

// Check は保存した問い合わせ先と模型で、短い質問を 1 つ判定させて、使えるかを確かめる。
// 使えなければ理由を包んだ誤りを返す。問い合わせ先は保存した設定だけから取り、呼び出し側が
// 渡した URL へは要求を送らない。
func (a *AutoTagger) Check(ctx context.Context) error {
	settings, err := a.settings.AutoTagSettings(ctx)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, autoTagCheckTimeout)
	defer cancel()
	_, err = a.classifier.Classify(ctx, domain.AutoTagRequest{
		Endpoint: settings.Endpoint,
		Model:    settings.Model,
		Subject:  domain.NewAutoTagSubject("Connection check", "/videos/check.mp4"),
		Questions: []domain.AutoTagQuestion{{
			Key:          "check",
			Instructions: "Is this a connection check?",
		}},
	})
	return err
}

// QueueVideo は動画 1 本を判定に回す（判定を終えていても回し直す）。積んだら true を返す。
// 登録フォルダの下に所在の無い動画や判定中の動画は積まない。
func (a *AutoTagger) QueueVideo(ctx context.Context, videoID int64) (bool, error) {
	queued, err := a.queue.QueueVideos(ctx, []int64{videoID}, domain.AutoTagQueueAgain)
	if err != nil {
		return false, err
	}
	if queued > 0 {
		a.Wake()
	}
	return queued > 0, nil
}

// QueueLibrary はライブラリの動画を scope に従って判定に回し、積んだ件数を返す。
func (a *AutoTagger) QueueLibrary(ctx context.Context, scope domain.AutoTagScope) (int, error) {
	if !scope.Valid() {
		return 0, fmt.Errorf("unknown auto-tagging scope %q", scope)
	}
	queued, err := a.queue.QueueLibrary(ctx, scope.QueueMode())
	if err != nil {
		return 0, err
	}
	if queued > 0 {
		a.Wake()
	}
	return queued, nil
}

// VideoThumbnailFinished は取り込みで動画の代表サムネイルの段階が終わった（成否を問わない）
// ことを受け取る。自動の判定が入なら、まだ判定していない動画を積む。
func (a *AutoTagger) VideoThumbnailFinished(ctx context.Context, videoID int64) {
	settings, err := a.settings.AutoTagSettings(ctx)
	if err != nil {
		a.logger.Warn("cannot read the auto-tagging settings", slog.Any("error", err))
		return
	}
	if !settings.Enabled {
		return
	}
	queued, err := a.queue.QueueVideos(ctx, []int64{videoID}, domain.AutoTagQueueIfNew)
	if err != nil {
		a.logger.Warn("cannot queue a video for auto-tagging", slog.Int64("video_id", videoID), slog.Any("error", err))
		return
	}
	if queued > 0 {
		a.Wake()
	}
}

// Recover は前回の終了で判定中のまま残った行を積み直す。Run より前に呼ぶ。
func (a *AutoTagger) Recover(ctx context.Context) error {
	requeued, err := a.queue.RequeueRunning(ctx)
	if err != nil {
		return err
	}
	if requeued > 0 {
		a.logger.Info("requeued interrupted auto-tagging jobs", slog.Int("count", requeued))
	}
	return nil
}

// Run は ctx が終わるまで判定を 1 件ずつ処理する。起動直後は知らせを待たずに 1 度
// 待ち行列を見る。
func (a *AutoTagger) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		job, err := a.queue.Claim(ctx)
		switch {
		case errors.Is(err, domain.ErrNoJob):
			select {
			case <-ctx.Done():
				return
			case <-a.wake:
			}
			continue
		case err != nil:
			if ctx.Err() != nil {
				return
			}
			a.logger.Error("cannot claim an auto-tagging job", slog.Any("error", err))
			select {
			case <-ctx.Done():
				return
			case <-time.After(autoTagRetryDelay):
			}
			continue
		}
		a.process(ctx, job)
	}
}

// process は 1 件を判定し、結果を記録する。停止で途切れた判定は記録せず、次の起動で
// Recover が積み直す。
func (a *AutoTagger) process(ctx context.Context, job domain.AutoTagJob) {
	tagIDs, err := a.decide(ctx, job)
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		reason := err.Error()
		if len(reason) > maxAutoTagErrorLength {
			reason = reason[:maxAutoTagErrorLength]
		}
		a.logger.Warn("auto-tagging failed", slog.String("content_key", job.ContentKey), slog.Any("error", err))
		if err := a.queue.Fail(ctx, job.ContentKey, reason); err != nil {
			a.logger.Error("cannot record the auto-tagging failure", slog.Any("error", err))
		}
		return
	}
	if err := a.queue.Finish(ctx, job.ContentKey, tagIDs); err != nil {
		a.logger.Error("cannot record the auto-tagging result", slog.String("content_key", job.ContentKey), slog.Any("error", err))
	}
}

// decide は動画の手がかりを判定モデルに渡し、付けるタグの id を返す。動画が消えていれば
// 空を返す（Finish が行を消す）。
func (a *AutoTagger) decide(ctx context.Context, job domain.AutoTagJob) ([]int64, error) {
	settings, err := a.settings.AutoTagSettings(ctx)
	if err != nil {
		return nil, err
	}
	video, err := a.queue.Subject(ctx, job.ContentKey)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	candidates, err := a.queue.Candidates(ctx)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	subject := domain.NewAutoTagSubject(video.Title, video.Path)
	if video.HasThumbnail() && a.thumbnails != nil {
		image, err := a.thumbnails.ThumbnailJPEG(job.ContentKey)
		if err != nil {
			// サムネイルが読めなくても、題名・ファイル名・フォルダだけで判定する。
			a.logger.Warn("cannot read the thumbnail; judging from the text only",
				slog.String("content_key", job.ContentKey), slog.Any("error", err))
		} else {
			subject.Thumbnail = image
		}
	}

	var selected []int64
	for _, chunk := range domain.ChunkAutoTagCandidates(candidates) {
		questions := make([]domain.AutoTagQuestion, len(chunk))
		for i, candidate := range chunk {
			questions[i] = domain.AutoTagQuestionFor(candidate)
		}
		probabilities, err := a.classifier.Classify(ctx, domain.AutoTagRequest{
			Endpoint:  settings.Endpoint,
			Model:     settings.Model,
			Subject:   subject,
			Questions: questions,
		})
		if err != nil {
			return nil, err
		}
		selected = append(selected, domain.SelectAutoTags(chunk, probabilities, settings.Threshold)...)
	}
	return selected, nil
}
