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
	// Finish は判定を終え、確定と仮のタグを付ける。
	Finish(ctx context.Context, contentKey string, selection domain.AutoTagSelection) error
	// ConfirmTentative と RejectTentative は動画に仮に付いたタグを確定・却下する。仮に付いて
	// いなければ domain.ErrNotFound。
	ConfirmTentative(ctx context.Context, videoID, tagID int64) error
	RejectTentative(ctx context.Context, videoID, tagID int64) error
	Fail(ctx context.Context, contentKey, reason string) error
	RequeueRunning(ctx context.Context) (int, error)
	Counts(ctx context.Context) (domain.AutoTagCounts, error)
}

// AutoTagSettingsStore は自動タグ付けの設定の保存先である。internal/store の *SettingsStore が満たす。
type AutoTagSettingsStore interface {
	AutoTagSettings(ctx context.Context) (domain.AutoTagSettings, error)
	SaveAutoTagSettings(ctx context.Context, settings domain.AutoTagSettings) error
}

// OllamaSettingsStore は Ollama の接続先の保存先である。internal/store の *SettingsStore が満たす。
type OllamaSettingsStore interface {
	OllamaSettings(ctx context.Context) (domain.OllamaSettings, error)
	SaveOllamaSettings(ctx context.Context, settings domain.OllamaSettings) error
}

// AutoTagClassifier は判定モデルに質問ごとの「はい」の確率を聞く。internal/clef の *Client が満たす。
type AutoTagClassifier interface {
	Classify(ctx context.Context, req domain.AutoTagRequest) (map[string]float64, error)
}

// VideoImageSource は判定に添える画像を生成物の置き場から読む。
type VideoImageSource interface {
	// ThumbnailJPEG は代表サムネイルの JPEG を読む。無ければ誤りを返す。
	ThumbnailJPEG(contentKey string) ([]byte, error)
	// SeekFramesJPEG はシーク用プレビューから動画全体に等間隔に n コマを選び、1 コマずつの
	// JPEG にして時間の順に返す。無ければ誤りを返す。
	SeekFramesJPEG(contentKey string, n int) ([][]byte, error)
}

// AutoTaggerOptions は自動タグ付けに必要な依存である。
type AutoTaggerOptions struct {
	Queue      AutoTagQueue
	Settings   AutoTagSettingsStore
	Ollama     OllamaSettingsStore
	Classifier AutoTagClassifier
	Images     VideoImageSource
	// Logger は nil なら slog の既定を使う。
	Logger *slog.Logger
}

// AutoTagger は動画を判定モデルに回し、確率が閾値以上の既存のタグを付ける
// （docs/design-docs/auto-tagging.md）。判定は 1 件ずつ、待ち行列の順に行う。
type AutoTagger struct {
	queue      AutoTagQueue
	settings   AutoTagSettingsStore
	ollama     OllamaSettingsStore
	classifier AutoTagClassifier
	images     VideoImageSource
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
		ollama:     opts.Ollama,
		classifier: opts.Classifier,
		images:     opts.Images,
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
// domain.ErrInvalidAutoTagSettings を包んだ誤りを返す。機能を入れたときに待ち行列に残っていた
// 判定を再開するため、保存のあとに判定の処理を起こす。
func (a *AutoTagger) SaveSettings(ctx context.Context, settings domain.AutoTagSettings) (domain.AutoTagStatus, error) {
	validated, err := domain.ValidateAutoTagSettings(settings)
	if err != nil {
		return domain.AutoTagStatus{}, err
	}
	a.saveMu.Lock()
	err = a.settings.SaveAutoTagSettings(ctx, validated)
	a.saveMu.Unlock()
	if err != nil {
		return domain.AutoTagStatus{}, err
	}
	a.Wake()
	return a.Status(ctx)
}

// OllamaSettings は Ollama の接続先を返す。
func (a *AutoTagger) OllamaSettings(ctx context.Context) (domain.OllamaSettings, error) {
	return a.ollama.OllamaSettings(ctx)
}

// SaveOllamaSettings は接続先を確かめて保存し、保存後の値を返す。規則に合わなければ
// domain.ErrInvalidOllamaSettings を包んだ誤りを返す。
func (a *AutoTagger) SaveOllamaSettings(ctx context.Context, settings domain.OllamaSettings) (domain.OllamaSettings, error) {
	normalized, err := domain.NormalizeOllamaSettings(settings)
	if err != nil {
		return domain.OllamaSettings{}, err
	}
	a.saveMu.Lock()
	err = a.ollama.SaveOllamaSettings(ctx, normalized)
	a.saveMu.Unlock()
	if err != nil {
		return domain.OllamaSettings{}, err
	}
	return normalized, nil
}

// Check は保存した接続先と模型で、短い質問を 1 つ判定させて、使えるかを確かめる。使えなければ
// 理由を包んだ誤りを返す。接続先は保存した設定だけから取り、呼び出し側が渡した URL へは要求を
// 送らない。自動タグ付けの機能の入切には関係なく確かめられる。
func (a *AutoTagger) Check(ctx context.Context) error {
	ollama, err := a.ollama.OllamaSettings(ctx)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, autoTagCheckTimeout)
	defer cancel()
	_, err = a.classifier.Classify(ctx, domain.AutoTagRequest{
		Endpoint: ollama.Endpoint,
		Model:    ollama.Model,
		Subject:  domain.NewAutoTagSubject("Connection check", "/videos/check.mp4"),
		Questions: []domain.AutoTagQuestion{{
			Key:          "check",
			Instructions: "Is this a connection check?",
		}},
	})
	return err
}

// enabled は自動タグ付けの機能が入かを返す。
func (a *AutoTagger) enabled(ctx context.Context) (bool, error) {
	settings, err := a.settings.AutoTagSettings(ctx)
	return settings.Enabled, err
}

// QueueVideo は動画 1 本を判定に回す（判定を終えていても回し直す）。積んだら true を返す。
// 登録フォルダの下に所在の無い動画や判定中の動画は積まない。機能が切なら
// domain.ErrAutoTaggingOff。
func (a *AutoTagger) QueueVideo(ctx context.Context, videoID int64) (bool, error) {
	if on, err := a.enabled(ctx); err != nil {
		return false, err
	} else if !on {
		return false, domain.ErrAutoTaggingOff
	}
	queued, err := a.queue.QueueVideos(ctx, []int64{videoID}, domain.AutoTagQueueAgain)
	if err != nil {
		return false, err
	}
	if queued > 0 {
		a.Wake()
	}
	return queued > 0, nil
}

// QueueLibrary はライブラリの動画を scope に従って判定に回し、積んだ件数を返す。機能が切なら
// domain.ErrAutoTaggingOff。
func (a *AutoTagger) QueueLibrary(ctx context.Context, scope domain.AutoTagScope) (int, error) {
	if !scope.Valid() {
		return 0, fmt.Errorf("unknown auto-tagging scope %q", scope)
	}
	if on, err := a.enabled(ctx); err != nil {
		return 0, err
	} else if !on {
		return 0, domain.ErrAutoTaggingOff
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

// ConfirmTag は動画に自動タグ付けが仮に付けたタグを確定する。
func (a *AutoTagger) ConfirmTag(ctx context.Context, videoID, tagID int64) error {
	return a.queue.ConfirmTentative(ctx, videoID, tagID)
}

// RejectTag は動画に自動タグ付けが仮に付けたタグを外し、この動画には付け直さない。
func (a *AutoTagger) RejectTag(ctx context.Context, videoID, tagID int64) error {
	return a.queue.RejectTentative(ctx, videoID, tagID)
}

// VideoImagesFinished は取り込みで動画のシーク用プレビューの段階が終わった（成否を問わない）
// ことを受け取る。代表サムネイルはそれより前に終わっているので、判定に添える画像が揃っている。
// 機能と取り込みのあとの自動の判定が入なら、まだ判定していない動画を積む。
func (a *AutoTagger) VideoImagesFinished(ctx context.Context, videoID int64) {
	settings, err := a.settings.AutoTagSettings(ctx)
	if err != nil {
		a.logger.Warn("cannot read the auto-tagging settings", slog.Any("error", err))
		return
	}
	if !settings.Enabled || !settings.AfterImport {
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
// 待ち行列を見る。機能が切のあいだは待ち行列から取り出さず、判定モデルに接続しない。積んで
// あった判定は残り、機能を入れたとき（SaveSettings が起こす）に続きから処理する。
func (a *AutoTagger) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		on, err := a.enabled(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			a.logger.Error("cannot read the auto-tagging settings", slog.Any("error", err))
			select {
			case <-ctx.Done():
				return
			case <-time.After(autoTagRetryDelay):
			}
			continue
		}
		if !on {
			// 切のあいだは、設定の保存（SaveSettings）に起こされるまで待つ。
			select {
			case <-ctx.Done():
				return
			case <-a.wake:
			}
			continue
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
	selection, err := a.decide(ctx, job)
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
	if err := a.queue.Finish(ctx, job.ContentKey, selection); err != nil {
		a.logger.Error("cannot record the auto-tagging result", slog.String("content_key", job.ContentKey), slog.Any("error", err))
	}
}

// decide は動画の手がかりを判定モデルに渡し、確定と仮に付けるタグの id を返す。動画が
// 消えていれば空を返す（Finish が行を消す）。
func (a *AutoTagger) decide(ctx context.Context, job domain.AutoTagJob) (domain.AutoTagSelection, error) {
	var selection domain.AutoTagSelection
	settings, err := a.settings.AutoTagSettings(ctx)
	if err != nil {
		return selection, err
	}
	ollama, err := a.ollama.OllamaSettings(ctx)
	if err != nil {
		return selection, err
	}
	video, err := a.queue.Subject(ctx, job.ContentKey)
	if errors.Is(err, domain.ErrNotFound) {
		return selection, nil
	}
	if err != nil {
		return selection, err
	}
	candidates, err := a.queue.Candidates(ctx)
	if err != nil {
		return selection, err
	}
	if len(candidates) == 0 {
		return selection, nil
	}

	subject := domain.NewAutoTagSubject(video.Title, video.Path)
	subject.Images = a.videoImages(job.ContentKey, video)

	for _, chunk := range domain.ChunkAutoTagCandidates(candidates) {
		questions := make([]domain.AutoTagQuestion, len(chunk))
		for i, candidate := range chunk {
			questions[i] = domain.AutoTagQuestionFor(candidate)
		}
		probabilities, err := a.classifier.Classify(ctx, domain.AutoTagRequest{
			Endpoint:  ollama.Endpoint,
			Model:     ollama.Model,
			Subject:   subject,
			Questions: questions,
		})
		if err != nil {
			return selection, err
		}
		chunkSelection := domain.SelectAutoTags(chunk, probabilities, settings)
		selection.Confirmed = append(selection.Confirmed, chunkSelection.Confirmed...)
		selection.Tentative = append(selection.Tentative, chunkSelection.Tentative...)
	}
	return selection, nil
}

// videoImages は判定に添える画像（代表サムネイルと、動画全体から等間隔に選んだコマ）を
// 読む。読めないものは飛ばし、題名・ファイル名・フォルダと読めた画像だけで判定する。
func (a *AutoTagger) videoImages(contentKey string, video domain.Video) [][]byte {
	if a.images == nil {
		return nil
	}
	var images [][]byte
	if video.HasThumbnail() {
		image, err := a.images.ThumbnailJPEG(contentKey)
		if err != nil {
			a.logger.Warn("cannot read the thumbnail for auto-tagging",
				slog.String("content_key", contentKey), slog.Any("error", err))
		} else {
			images = append(images, image)
		}
	}
	if video.SeekThumbnailState == domain.SeekThumbnailDone {
		frames, err := a.images.SeekFramesJPEG(contentKey, domain.AutoTagFrameCount)
		if err != nil {
			a.logger.Warn("cannot read the seek preview frames for auto-tagging",
				slog.String("content_key", contentKey), slog.Any("error", err))
		} else {
			images = append(images, frames...)
		}
	}
	return images
}
