package app

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// versionJudgeRetryDelay は、取り出しや設定の読み込み自体が失敗したときに次に試すまでの間隔である。
const versionJudgeRetryDelay = 5 * time.Second

// errNoVersionAnswer は判定の口が問いに答えなかったことを表す。確率 0 ではなく失敗として扱う
var errNoVersionAnswer = errors.New("the classifier did not answer the question")

// VersionJudgeStore は「同じ動画かもしれない」候補の判定の保存先である。internal/store の
// *VersionStore が満たす。
type VersionJudgeStore interface {
	// ClaimJudgement は取り出せる最も古い候補を判定中にし、両方の指紋と共に返す。無ければ
	// domain.ErrNoJob。
	ClaimJudgement(ctx context.Context) (domain.VersionJudgeJob, error)
	// ApplyJudgement は判定中の組に結果を反映し、反映したかを返す。
	ApplyJudgement(ctx context.Context, keys [2]string, result domain.VersionJudgeResult) (bool, error)
	// RecoverRunningJudgements は判定中のまま残った行を待ちに戻す。
	RecoverRunningJudgements(ctx context.Context) (int, error)
	// NextJudgementRetry は failed の組のうち最も早く取り出せる時刻を返す。無ければ found は偽。
	NextJudgementRetry(ctx context.Context) (at time.Time, found bool, err error)
	// ResetNoConnectionJudgements は no_connection の組を待ちに戻す。
	ResetNoConnectionJudgements(ctx context.Context) (int, error)
}

// VersionJudgeSettingsStore は重複の自動判定の入切と、Ollama の接続先を保存したかを読む。
// internal/store の *SettingsStore が満たす。
type VersionJudgeSettingsStore interface {
	VersionJudgingEnabled(ctx context.Context) (bool, error)
	SaveVersionJudgingEnabled(ctx context.Context, enabled bool) error
	// OllamaConnectionSaved は所有者が Ollama の接続先を保存したかを返す。
	OllamaConnectionSaved(ctx context.Context) (bool, error)
}

// VersionFrameSource は 2 つの内容のシーク用プレビューのコマを左右に並べた JPEG を読む。
// internal/artifacts の *Store が満たす。
type VersionFrameSource interface {
	SeekFramePairs(keyA, keyB string, pairs []domain.FramePair) ([][]byte, error)
}

// VersionJudgeOptions は重複の自動判定に必要な依存である。
type VersionJudgeOptions struct {
	Store    VersionJudgeStore
	Settings VersionJudgeSettingsStore
	// Classifier は判定モデルの口で、どの提供元かは知らない。
	Classifier Classifier
	Frames     VersionFrameSource
	// Logger は nil なら slog の既定を使う。
	Logger *slog.Logger
	// Now は今の時刻で、nil なら time.Now。
	Now func() time.Time
}

// VersionJudge は「同じ動画かもしれない」候補を 1 組ずつ判定する（docs/design-docs/duplicate-judging.md）。
// まず 2 本の指紋をコマごとに比べ、一致すればモデルに聞かずに同じ動画とし、コマの連なりが食い違う
// ときだけ、その区間のコマを左右に並べて Classifier の口に聞く。結果は保存層が所有者の操作と同じ
// 行に反映する。設定が入のあいだだけ取り出す。
type VersionJudge struct {
	store      VersionJudgeStore
	settings   VersionJudgeSettingsStore
	classifier Classifier
	frames     VersionFrameSource
	logger     *slog.Logger
	now        func() time.Time
	wake       chan struct{}

	// saveMu は設定の保存を 1 つずつにし、判定の結果の反映と入れ違わないようにする。切を保存し
	// 終えたあとに、判定中だった組を確定しない。
	saveMu sync.Mutex
	// requeuePending は、次の判定の前に待ちへ戻す行（requeue）を見直すことを表す。接続の保存と、
	// 結果を保存できなかった判定で立てる。判定を 1 組ずつ行う Run の中で戻すので、判定中の組と
	// 入れ違わない。
	requeuePending atomic.Bool
}

// NewVersionJudge は重複の自動判定を組み立てる。判定は Run で始める。
func NewVersionJudge(opts VersionJudgeOptions) *VersionJudge {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &VersionJudge{
		store:      opts.Store,
		settings:   opts.Settings,
		classifier: opts.Classifier,
		frames:     opts.Frames,
		logger:     logger.With(slog.String("component", "duplicate-judging")),
		now:        now,
		// 容量 1 で、起こす知らせを 1 つだけ貯める。
		wake: make(chan struct{}, 1),
	}
}

// Wake は判定する組が増えたかもしれないことを知らせる。ブロックしない。仕事の終わり
// （domain.ProcessingChanged）と候補の変化（domain.VersionCandidatesChanged）で呼ぶ。
func (j *VersionJudge) Wake() {
	select {
	case j.wake <- struct{}{}:
	default:
	}
}

// Enabled は重複の自動判定が入かを返す。
func (j *VersionJudge) Enabled(ctx context.Context) (bool, error) {
	return j.settings.VersionJudgingEnabled(ctx)
}

// SaveEnabled は重複の自動判定の入切を保存し、判定の処理を起こす。入れたときに待っていた候補を
// 判定し始める。
func (j *VersionJudge) SaveEnabled(ctx context.Context, enabled bool) error {
	j.saveMu.Lock()
	err := j.settings.SaveVersionJudgingEnabled(ctx, enabled)
	j.saveMu.Unlock()
	if err != nil {
		return err
	}
	j.Wake()
	return nil
}

// ConnectionSaved は Ollama の接続先が保存されたことを受け取る。接続が無くて待っていた組
// （no_connection）を待ちに戻すよう判定の処理に頼んで起こす。戻すのは Run の中で
// 行うので、保存の要求の取り消しに左右されず、判定中の組が後から no_connection を書いても次の判定の
// 前に戻り、戻せなければ再び試す。
func (j *VersionJudge) ConnectionSaved(context.Context) {
	j.requeuePending.Store(true)
	j.Wake()
}

// Recover は前回の終了で判定中のまま残った行と、接続が保存されていれば no_connection の行を待ちに
// 戻す。Run より前に呼ぶ。戻せなければ Run が最初の判定の前に再び試す。
func (j *VersionJudge) Recover(ctx context.Context) error {
	if err := j.requeue(ctx); err != nil {
		j.requeuePending.Store(true)
		return err
	}
	return nil
}

// requeue は判定中のまま残った行を待ちに戻し、Ollama の接続先が保存されていれば no_connection の行も
// 待ちに戻す。判定中の組が無いとき（Run の前か、Run の判定と判定のあいだ）だけ呼ぶ。
func (j *VersionJudge) requeue(ctx context.Context) error {
	recovered, err := j.store.RecoverRunningJudgements(ctx)
	if err != nil {
		return err
	}
	if recovered > 0 {
		j.logger.Info("requeued interrupted duplicate judgements", slog.Int("count", recovered))
	}
	saved, err := j.settings.OllamaConnectionSaved(ctx)
	if err != nil || !saved {
		return err
	}
	reset, err := j.store.ResetNoConnectionJudgements(ctx)
	if err != nil {
		return err
	}
	if reset > 0 {
		j.logger.Info("duplicate judgements resume with the saved connection", slog.Int("count", reset))
	}
	return nil
}

// Run は ctx が終わるまで候補を 1 組ずつ判定する。起動直後は知らせを待たずに 1 度見る。設定が切の
// あいだは取り出さず、起こされるまで待つ。取り出せる組が無ければ、起こされるか、次の失敗した組の
// 再試行の時刻まで待つ。
func (j *VersionJudge) Run(ctx context.Context) {
	for ctx.Err() == nil {
		j.waitFor(ctx, j.step(ctx))
	}
}

// step は 1 組を判定し、次の判定までの待ち方を返す。0 は待たない、負は起こされるまで待つ。
func (j *VersionJudge) step(ctx context.Context) time.Duration {
	if j.requeuePending.Swap(false) {
		if err := j.requeue(ctx); err != nil {
			j.requeuePending.Store(true)
			j.logError(ctx, "cannot requeue the duplicate judgements", err)
			return versionJudgeRetryDelay
		}
	}
	on, err := j.Enabled(ctx)
	if err != nil {
		j.logError(ctx, "cannot read the duplicate judging setting", err)
		return versionJudgeRetryDelay
	}
	if !on {
		return -1
	}
	job, err := j.store.ClaimJudgement(ctx)
	if errors.Is(err, domain.ErrNoJob) {
		return j.untilNextRetry(ctx)
	}
	if err != nil {
		j.logError(ctx, "cannot claim a possible duplicate", err)
		return versionJudgeRetryDelay
	}
	if !j.process(ctx, job) {
		return versionJudgeRetryDelay
	}
	return 0
}

// untilNextRetry は次の失敗した組を再び取り出せるまでの時間を返す。無ければ起こされるまで待つ（負）。
func (j *VersionJudge) untilNextRetry(ctx context.Context) time.Duration {
	at, found, err := j.store.NextJudgementRetry(ctx)
	if err != nil {
		j.logError(ctx, "cannot read the next duplicate judgement retry", err)
		return versionJudgeRetryDelay
	}
	if !found {
		return -1
	}
	// 秒の単位で保存した時刻を過ぎてから取り出す。
	return max(at.Sub(j.now()), 0) + time.Second
}

// waitFor は wait だけ（負なら限りなく）待つ。ctx が終わるか起こされれば待つのをやめる。
func (j *VersionJudge) waitFor(ctx context.Context, wait time.Duration) {
	if wait == 0 {
		return
	}
	var timeout <-chan time.Time
	if wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		timeout = timer.C
	}
	select {
	case <-ctx.Done():
	case <-j.wake:
	case <-timeout:
	}
}

// logError は停止で途切れたのでなければ誤りを記録する。
func (j *VersionJudge) logError(ctx context.Context, message string, err error) {
	if ctx.Err() == nil {
		j.logger.Error(message, slog.Any("error", err))
	}
}

// process は 1 組を判定し、結果を反映して、反映できたかを返す。停止で途切れた判定は反映せず、次の
// 起動で Recover が待ちに戻す。結果を保存できなければ、組は判定中のまま残るので、次の判定の前に
// 待ちへ戻す。
func (j *VersionJudge) process(ctx context.Context, job domain.VersionJudgeJob) bool {
	rule := domain.JudgeVersionFrames(job.Fingerprints[0], job.Fingerprints[1])
	result, probability := j.decide(ctx, job, rule)
	if ctx.Err() != nil {
		return true
	}
	attrs := []any{
		slog.String("key_a", job.Keys[0]), slog.String("key_b", job.Keys[1]),
		slog.String("rule", rule.Outcome.String()), slog.Int("frames_compared", rule.Compared),
		slog.Int("frames_differing", rule.Differing), slog.String("verdict", result.Verdict.String()),
	}
	if probability != nil {
		attrs = append(attrs, slog.Float64("probability", *probability))
	}
	if result.Error != "" {
		attrs = append(attrs, slog.String("error", result.Error))
	}
	j.logger.Info("judged a possible duplicate", attrs...)
	if err := j.record(ctx, job.Keys, result); err != nil {
		j.logError(ctx, "cannot record the duplicate judgement", err)
		j.requeuePending.Store(true)
		return false
	}
	return true
}

// record は設定が入なら判定の結果を反映し、判定のあいだに切られていれば結果を捨てて組を待ちに戻す。
// 戻した組は、入れ直したときに判定し直す。設定の保存と同じ saveMu の中で読んで反映するので、
// SaveEnabled(false) が返ったあとに組を束ねたり却下したりしない（親 Issue の要件 7: 切なら
// すべての候補が候補一覧に残る）。
func (j *VersionJudge) record(ctx context.Context, keys [2]string, result domain.VersionJudgeResult) error {
	j.saveMu.Lock()
	defer j.saveMu.Unlock()
	on, err := j.Enabled(ctx)
	if err != nil {
		return err
	}
	if on {
		_, err = j.store.ApplyJudgement(ctx, keys, result)
		return err
	}
	if _, err := j.store.RecoverRunningJudgements(ctx); err != nil {
		return err
	}
	j.logger.Info("discarded a duplicate judgement because judging was turned off",
		slog.String("key_a", keys[0]), slog.String("key_b", keys[1]))
	return nil
}

// decide はコマごとの規則の結果から判定の結果を決め、モデルに聞いたときはその確率も返す。コマが
// 一致すれば同じ、比べられなければ決められない（所有者に残す）。食い違えば、接続が保存されて
// いなければ聞かずに no_connection、保存されていればその区間のコマを聞く。
func (j *VersionJudge) decide(
	ctx context.Context, job domain.VersionJudgeJob, rule domain.VersionFrameRule,
) (domain.VersionJudgeResult, *float64) {
	switch rule.Outcome {
	case domain.VersionFramesSame:
		return domain.VersionJudgeResult{Verdict: domain.VersionSame, Decider: domain.ActivityByRule}, nil
	case domain.VersionFramesDiffer:
	default:
		return domain.VersionJudgeResult{Verdict: domain.VersionUndecided, Decider: domain.ActivityByRule}, nil
	}
	saved, err := j.settings.OllamaConnectionSaved(ctx)
	if err != nil {
		return failedJudgement(err), nil
	}
	if !saved {
		return domain.VersionJudgeResult{Verdict: domain.VersionNoConnection}, nil
	}
	probability, err := j.ask(ctx, job.Keys, rule.Shown)
	if err != nil {
		return failedJudgement(err), nil
	}
	return domain.VersionJudgeResult{
		Verdict: domain.VersionVerdictFor(probability), Probability: &probability, Decider: domain.ActivityByAI,
	}, &probability
}

// ask は 2 つの内容の組ごとのコマを左右に並べた画像と R-3 の問いを判定の口に送り、答えの確率を
// 返す。ファイル名・フォルダ・長さは送らない。答えが無ければ errNoVersionAnswer。
func (j *VersionJudge) ask(ctx context.Context, keys [2]string, pairs []domain.FramePair) (float64, error) {
	images, err := j.frames.SeekFramePairs(keys[0], keys[1], pairs)
	if err != nil {
		return 0, err
	}
	probabilities, err := j.classifier.Classify(ctx, domain.ClassifierRequest{
		Input:     domain.ClassifierInput{Images: images},
		Questions: []domain.ClassifierQuestion{{Key: domain.VersionJudgeQuestionKey, Text: domain.VersionJudgeQuestion}},
	})
	if err != nil {
		return 0, err
	}
	probability, ok := probabilities[domain.VersionJudgeQuestionKey]
	if !ok {
		return 0, errNoVersionAnswer
	}
	return probability, nil
}

// failedJudgement は誤りを失敗の結果にする。
func failedJudgement(err error) domain.VersionJudgeResult {
	return domain.VersionJudgeResult{Verdict: domain.VersionFailed, Error: failureReason(err)}
}
