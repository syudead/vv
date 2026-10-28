package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// An unanswered question is a failure, not zero probability.
var errNoVersionAnswer = errors.New("the classifier did not answer the question")

// errJudgingTurnedOff snoozes a pair whose judging was turned off mid-judgement, so it waits without spending an
// attempt.
var errJudgingTurnedOff = fmt.Errorf("duplicate judging was turned off: %w", domain.ErrJobSnoozed)

// errConnectionSavedMidway snoozes a pair judged no_connection while the connection was being saved, so it is judged
// again at once with the saved connection.
var errConnectionSavedMidway = fmt.Errorf("the connection was saved during the judgement: %w", domain.ErrJobSnoozed)

type VersionJudgeStore interface {
	// JudgementInput reads both fingerprints and properties of a claimed pair.
	JudgementInput(ctx context.Context, keys [2]string) (domain.VersionJudgeJob, error)
	// ApplyJudgement records the result and ends the pair's job, or leaves it waiting out its retry delay.
	ApplyJudgement(ctx context.Context, keys [2]string, result domain.VersionJudgeResult) (bool, error)
	RecoverRunningJudgements(ctx context.Context) (int, error)
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
	// Wake wakes the job scheduler, which runs version_judge jobs through HandleJob.
	Wake Waker
	// Nil uses slog.Default.
	Logger *slog.Logger
}

// VersionJudge judges the version_judge jobs the scheduler grants, one at a time on the classifier slot, comparing
// fingerprints before asking the classifier about mismatched intervals; see docs/design-docs/duplicate-judging.md.
type VersionJudge struct {
	store      VersionJudgeStore
	settings   VersionJudgeSettingsStore
	classifier Classifier
	frames     VersionFrameSource
	logger     *slog.Logger
	// Wake reacts to candidate changes and to settings that make pairs claimable.
	Waker

	// Serialize result application with the judging switch and the connection reset, so no result commits after
	// judging is turned off and no no_connection result outlives a saved connection.
	saveMu sync.Mutex
	// resetPending is a reset owed to a saved connection that has not committed; each scheduler round retries it.
	resetPending atomic.Bool
}

// NewVersionJudge does not process work until the scheduler runs its HandleJob.
func NewVersionJudge(opts VersionJudgeOptions) *VersionJudge {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &VersionJudge{
		store:      opts.Store,
		settings:   opts.Settings,
		classifier: opts.Classifier,
		frames:     opts.Frames,
		logger:     logger.With(slog.String("component", "duplicate-judging")),
		Waker:      opts.Wake,
	}
}

func (j *VersionJudge) Enabled(ctx context.Context) (bool, error) {
	return j.settings.VersionJudgingEnabled(ctx)
}

// SaveEnabled wakes the scheduler, so turning judging on resumes the jobs it kept queued while paused.
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

// PausedJobKinds pauses version_judge jobs while judging is off, so they keep their attempts until it is turned on.
// The scheduler calls it every round, so it first retries a reset owed to a saved connection.
func (j *VersionJudge) PausedJobKinds(ctx context.Context) ([]domain.JobKind, error) {
	j.resetOwed(ctx)
	on, err := j.Enabled(ctx)
	if err != nil || on {
		return nil, err
	}
	return []domain.JobKind{domain.JobVersionJudge}, nil
}

// ConnectionSaved returns no_connection pairs to waiting with their jobs and wakes the scheduler. A reset that fails,
// or whose request ends first, stays owed and the scheduler retries it. A judgement that decided no_connection before
// the save snoozes instead of recording it, so it cannot outlive this reset.
func (j *VersionJudge) ConnectionSaved(ctx context.Context) {
	j.resetPending.Store(true)
	j.resetOwed(ctx)
	j.Wake()
}

// resetOwed runs an owed reset; a failure keeps it owed and wakes the scheduler after domain.WorkerRetry, so a
// scheduler round retries it even when no other work wakes one.
func (j *VersionJudge) resetOwed(ctx context.Context) {
	if !j.resetPending.Load() {
		return
	}
	j.saveMu.Lock()
	defer j.saveMu.Unlock()
	if !j.resetPending.Load() {
		return
	}
	if err := j.resetNoConnection(ctx); err != nil {
		if ctx.Err() == nil {
			j.logger.Error("cannot return the duplicate judgements waiting for a connection", slog.Any("error", err))
		}
		time.AfterFunc(time.Duration(domain.WorkerRetry), j.Wake)
		return
	}
	j.resetPending.Store(false)
}

// Recover returns pairs interrupted by a restart to waiting, and pairs waiting for a connection that was saved; it
// runs before the scheduler starts.
func (j *VersionJudge) Recover(ctx context.Context) error {
	recovered, err := j.store.RecoverRunningJudgements(ctx)
	if err != nil {
		return err
	}
	if recovered > 0 {
		j.logger.Info("requeued interrupted duplicate judgements", slog.Int("count", recovered))
	}
	return j.resetNoConnection(ctx)
}

func (j *VersionJudge) resetNoConnection(ctx context.Context) error {
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

// HandleJob judges the claimed pair and records the result, which ends the job or schedules its retry. An error goes
// back to the scheduler, which records it as a failed judgement, or as a snooze for errJudgingTurnedOff.
func (j *VersionJudge) HandleJob(ctx context.Context, job domain.Job) error {
	keys := [2]string{job.ContentKey, job.ContentKeyB}
	input, err := j.store.JudgementInput(ctx, keys)
	if err != nil {
		// Judging turned off first discards this failure as it would any other result.
		if off := j.requireOn(ctx, keys); errors.Is(off, errJudgingTurnedOff) {
			return off
		}
		return err
	}
	result := j.judge(ctx, input)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return j.record(ctx, input.Keys, result)
}

func (j *VersionJudge) judge(ctx context.Context, job domain.VersionJudgeJob) domain.VersionJudgeResult {
	rule := domain.JudgeVersionFrames(job.Fingerprints[0], job.Fingerprints[1])
	properties := domain.CompareVersionProperties(job.Properties[0], job.Properties[1])
	leaning := domain.LeanVersionProperties(properties)
	result, probability := j.decide(ctx, job, rule, leaning)
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
	return result
}

// record applies the result under the settings lock only while judging is on; otherwise the pair snoozes, so no
// bundle or dismissal commits after SaveEnabled(false) returns.
func (j *VersionJudge) record(ctx context.Context, keys [2]string, result domain.VersionJudgeResult) error {
	j.saveMu.Lock()
	defer j.saveMu.Unlock()
	if err := j.requireOn(ctx, keys); err != nil {
		return err
	}
	if result.Verdict == domain.VersionNoConnection {
		saved, err := j.settings.OllamaConnectionSaved(ctx)
		if err != nil {
			return err
		}
		if saved {
			return errConnectionSavedMidway
		}
	}
	_, err := j.store.ApplyJudgement(ctx, keys, result)
	return err
}

// requireOn returns errJudgingTurnedOff while judging is off, so the claimed pair snoozes instead of recording a result.
func (j *VersionJudge) requireOn(ctx context.Context, keys [2]string) error {
	on, err := j.Enabled(ctx)
	if err != nil {
		return err
	}
	if !on {
		j.logger.Info("discarded a duplicate judgement because judging was turned off",
			slog.String("key_a", keys[0]), slog.String("key_b", keys[1]))
		return errJudgingTurnedOff
	}
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
