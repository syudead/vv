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

const versionJudgeRetryDelay = 5 * time.Second

// An unanswered question is a failure, not zero probability.
var errNoVersionAnswer = errors.New("the classifier did not answer the question")

type VersionJudgeStore interface {
	// Claim the oldest eligible pair with both fingerprints, or return ErrNoJob.
	ClaimJudgement(ctx context.Context) (domain.VersionJudgeJob, error)
	ApplyJudgement(ctx context.Context, keys [2]string, result domain.VersionJudgeResult) (bool, error)
	RecoverRunningJudgements(ctx context.Context) (int, error)
	NextJudgementRetry(ctx context.Context) (at time.Time, found bool, err error)
	ResetNoConnectionJudgements(ctx context.Context) (int, error)
}

type VersionJudgeSettingsStore interface {
	VersionJudgingEnabled(ctx context.Context) (bool, error)
	SaveVersionJudgingEnabled(ctx context.Context, enabled bool) error
	OllamaConnectionSaved(ctx context.Context) (bool, error)
}

type VersionFrameSource interface {
	SeekFramePairs(keyA, keyB string, pairs []domain.FramePair) ([][]byte, error)
}

type VersionJudgeOptions struct {
	Store      VersionJudgeStore
	Settings   VersionJudgeSettingsStore
	Classifier Classifier
	Frames     VersionFrameSource
	// Nil uses slog.Default.
	Logger *slog.Logger
	// Nil uses time.Now.
	Now func() time.Time
}

// VersionJudge compares fingerprints before asking the classifier about mismatched intervals,
// processing pairs serially only while enabled; see docs/design-docs/duplicate-judging.md.
type VersionJudge struct {
	store      VersionJudgeStore
	settings   VersionJudgeSettingsStore
	classifier Classifier
	frames     VersionFrameSource
	logger     *slog.Logger
	now        func() time.Time
	wake       chan struct{}

	// Serialize settings with result application so disabled judging cannot commit an in-flight pair
	// afterward.
	saveMu sync.Mutex
	// Requeue inside serial Run after connection saves or failed result writes, avoiding races with
	// active judgements.
	requeuePending atomic.Bool
}

// NewVersionJudge starts no work until Run.
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
		// Coalesce wakeups in a one-element buffer.
		wake: make(chan struct{}, 1),
	}
}

// Wake is nonblocking and reacts to processing completion or candidate changes.
func (j *VersionJudge) Wake() {
	select {
	case j.wake <- struct{}{}:
	default:
	}
}

func (j *VersionJudge) Enabled(ctx context.Context) (bool, error) {
	return j.settings.VersionJudgingEnabled(ctx)
}

// Saving enablement wakes pending candidates.
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

// Queue connection recovery in Run so request cancellation and in-flight no_connection writes
// cannot lose it; failed requeueing retries before later judgements.
func (j *VersionJudge) ConnectionSaved(context.Context) {
	j.requeuePending.Store(true)
	j.Wake()
}

// Recover before Run; failed recovery is retried before the first judgement.
func (j *VersionJudge) Recover(ctx context.Context) error {
	if err := j.requeue(ctx); err != nil {
		j.requeuePending.Store(true)
		return err
	}
	return nil
}

// Call requeue only before Run or between judgements, never while a pair is being processed.
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

// Run checks immediately, processes serially while enabled, and waits for wakeups or the next retry
// time when no pair is eligible.
func (j *VersionJudge) Run(ctx context.Context) {
	for ctx.Err() == nil {
		j.waitFor(ctx, j.step(ctx))
	}
}

// Zero delay means continue immediately; negative means wait for a wakeup.
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

// Return a negative delay when no retry is scheduled.
func (j *VersionJudge) untilNextRetry(ctx context.Context) time.Duration {
	at, found, err := j.store.NextJudgementRetry(ctx)
	if err != nil {
		j.logError(ctx, "cannot read the next duplicate judgement retry", err)
		return versionJudgeRetryDelay
	}
	if !found {
		return -1
	}
	// Wait until after the timestamp stored at second precision.
	return max(at.Sub(j.now()), 0) + time.Second
}

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

// Do not log shutdown cancellation as an operational error.
func (j *VersionJudge) logError(ctx context.Context, message string, err error) {
	if ctx.Err() == nil {
		j.logger.Error(message, slog.Any("error", err))
	}
}

// Leave shutdown-interrupted work for startup recovery; failed result writes trigger requeueing
// before the next judgement.
func (j *VersionJudge) process(ctx context.Context, job domain.VersionJudgeJob) bool {
	rule := domain.JudgeVersionFrames(job.Fingerprints[0], job.Fingerprints[1])
	properties := domain.CompareVersionProperties(job.Properties[0], job.Properties[1])
	leaning := domain.LeanVersionProperties(properties)
	result, probability := j.decide(ctx, job, rule, leaning)
	if ctx.Err() != nil {
		return true
	}
	result.Evidence = domain.NewVersionJudgeEvidence(job.Fingerprints[0], job.Fingerprints[1], rule)
	result.Evidence.Properties = properties
	result = settleByProperties(result, leaning)
	attrs := []any{
		slog.String("key_a", job.Keys[0]), slog.String("key_b", job.Keys[1]),
		slog.String("rule", rule.Outcome.String()), slog.Int("frames_compared", rule.Compared),
		slog.Int("frames_differing", rule.Differing), slog.String("verdict", result.Verdict.String()),
		slog.Bool("by_properties", result.Evidence.ByProperties),
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

// Apply results under the settings lock only while enabled; disabling returns in-flight pairs to
// pending so no bundling/rejection can commit after SaveEnabled(false) returns.
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

// Matching fingerprints need no classifier; incomparable pairs remain for the owner, while
// mismatches require saved connection settings or return no_connection.
func (j *VersionJudge) decide(
	ctx context.Context, job domain.VersionJudgeJob, rule domain.VersionFrameRule, leaning domain.VersionLeaning,
) (domain.VersionJudgeResult, *float64) {
	switch rule.Outcome {
	case domain.VersionFramesSame:
		return domain.VersionJudgeResult{Verdict: domain.VersionSame, Decider: domain.ActivityByRule}, nil
	case domain.VersionFramesDiffer:
		// A strong same from the properties settles the pair without asking the classifier.
		if leaning == domain.VersionLeansStronglySame {
			return domain.VersionJudgeResult{Verdict: domain.VersionUndecided, Decider: domain.ActivityByRule}, nil
		}
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

// Send only side-by-side frames and the fixed question, excluding filenames, folders, and duration;
// missing answers return errNoVersionAnswer.
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

// settleByProperties lets the properties decide or hold the verdict; a verdict they decide is the rule's, and the
// classifier's answer stays as evidence.
func settleByProperties(result domain.VersionJudgeResult, leaning domain.VersionLeaning) domain.VersionJudgeResult {
	verdict, byProperties := domain.SettleVersionVerdict(result.Verdict, leaning, result.Probability)
	if !byProperties {
		return result
	}
	result.Evidence.ByProperties = true
	result.Verdict, result.Error = verdict, ""
	if verdict != domain.VersionUndecided || result.Probability == nil {
		result.Decider = domain.ActivityByRule
	}
	return result
}

func failedJudgement(err error) domain.VersionJudgeResult {
	return domain.VersionJudgeResult{Verdict: domain.VersionFailed, Error: failureReason(err)}
}
