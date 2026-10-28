package domain

import (
	"errors"
	"time"
)

// MaxJobAttempts is the default number of rounds before a job fails, so one corrupt file cannot hold work forever.
const MaxJobAttempts = 3

// ErrFailureRepeats marks a failure every retry would reproduce, such as an input too slow to process or one FFmpeg
// reports as invalid data; the job fails once its round ends, without the remaining rounds.
var ErrFailureRepeats = errors.New("retrying would repeat the failure")

// ErrJobSnoozed marks a job that cannot run now for a reason outside the job, such as an unreadable source root; it
// is requeued at once without counting an attempt.
var ErrJobSnoozed = errors.New("the job cannot run now")

// FailureRepeats marks err as a failure every retry would reproduce, keeping err's message.
func FailureRepeats(err error) error {
	return repeatingFailure{err}
}

type repeatingFailure struct{ error }

func (e repeatingFailure) Unwrap() []error { return []error{e.error, ErrFailureRepeats} }

// Default delay rule: 30 s × 4^(round − 1), spread by ±10% so jobs failed together do not retry together.
const (
	jobRetryBaseDelay = 30 * time.Second
	jobRetryJitter    = 0.1
	// Bounds the exponent for a job recovered past its limit; the delay stops growing after this round.
	jobRetryMaxRound = 10
)

// JobRetryPolicy is how a kind retries a failed job. A kind may override the default, as a River or Oban worker can.
type JobRetryPolicy struct {
	// MaxAttempts fails the job after this many failed rounds; zero retries without limit.
	MaxAttempts int
	// Delay is the wait after a failed round ends; nil uses JobRetryDelay.
	Delay func(round int) time.Duration
}

// JobRetryPolicyFor returns the kind's policy. Duplicate judging keeps its own delays and never gives up, since a
// classifier that is down for a day must not leave pairs failed for good.
func JobRetryPolicyFor(kind JobKind) JobRetryPolicy {
	if kind == JobVersionJudge {
		return JobRetryPolicy{Delay: VersionJudgeRetryDelay}
	}
	return JobRetryPolicy{MaxAttempts: MaxJobAttempts}
}

// JobRetryDelay is the default wait after round round fails; unit in [0, 1) picks the point within ±10%.
func JobRetryDelay(round int, unit float64) time.Duration {
	round = min(max(round, 1), jobRetryMaxRound)
	base := jobRetryBaseDelay << (2 * (round - 1))
	return time.Duration(float64(base) * (1 - jobRetryJitter + 2*jobRetryJitter*unit))
}

// JobFailure is the claimed job's state after a failed run, and the wait before it may be claimed again.
type JobFailure struct {
	State JobState
	Delay time.Duration
	// Uncounted reports a snooze, whose claim must not count as an attempt.
	Uncounted bool
}

// AfterFailure decides a failed claim's outcome. attempts counts rounds including this claim; roundEnds reports a
// claim of the last current location, whose failure ends the round. A failure inside a round requeues at once so the
// next location is tried; unit in [0, 1) spreads the default delay.
func (p JobRetryPolicy) AfterFailure(attempts int, cause error, roundEnds bool, unit float64) JobFailure {
	switch {
	case errors.Is(cause, ErrJobSnoozed):
		return JobFailure{State: JobQueued, Uncounted: true}
	case !roundEnds:
		return JobFailure{State: JobQueued}
	case errors.Is(cause, ErrFailureRepeats), p.MaxAttempts > 0 && attempts >= p.MaxAttempts:
		return JobFailure{State: JobFailed}
	case p.Delay != nil:
		return JobFailure{State: JobQueued, Delay: p.Delay(attempts)}
	default:
		return JobFailure{State: JobQueued, Delay: JobRetryDelay(attempts, unit)}
	}
}
