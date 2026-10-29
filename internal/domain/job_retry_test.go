package domain

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestJobRetryDelayGrowsFourfoldWithinTenPercent(t *testing.T) {
	for round, base := range map[int]time.Duration{1: 30 * time.Second, 2: 2 * time.Minute, 3: 8 * time.Minute} {
		low, high := JobRetryDelay(round, 0), JobRetryDelay(round, 0.999999)
		if low != base*9/10 || high > base*11/10 || high < base*109/100 {
			t.Errorf("round %d: delay in [%v, %v], want [%v, %v]", round, low, high, base*9/10, base*11/10)
		}
	}
}

func TestJobFailureRequeuesInsideARoundAndWaitsAtItsEnd(t *testing.T) {
	policy := JobRetryPolicyFor(JobProbe)
	plain := errors.New("ffmpeg failed")
	if got := policy.AfterFailure(1, plain, false, 0.5); got != (JobFailure{State: JobQueued}) {
		t.Errorf("inside a round = %+v, want queued at once", got)
	}
	if got := policy.AfterFailure(1, plain, true, 0.5); got != (JobFailure{State: JobQueued, Delay: 30 * time.Second}) {
		t.Errorf("first round ends = %+v, want queued after 30s", got)
	}
	if got := policy.AfterFailure(MaxJobAttempts, plain, true, 0.5); got.State != JobFailed {
		t.Errorf("last round ends = %+v, want failed", got)
	}
}

func TestJobFailureRepeatingFailsWhenItsRoundEnds(t *testing.T) {
	policy := JobRetryPolicyFor(JobProbe)
	repeats := FailureRepeats(errors.New("Invalid data found when processing input"))
	if got := policy.AfterFailure(1, repeats, false, 0); got.State != JobQueued {
		t.Errorf("inside a round = %+v, want queued so the next location is tried", got)
	}
	if got := policy.AfterFailure(1, fmt.Errorf("probe: %w", repeats), true, 0); got.State != JobFailed {
		t.Errorf("first round ends = %+v, want failed", got)
	}
	if got := repeats.Error(); got != "Invalid data found when processing input" {
		t.Errorf("message = %q, want the cause's message", got)
	}
	unlimited := JobRetryPolicy{Delay: func(int) time.Duration { return time.Hour }}
	if got := unlimited.AfterFailure(1, repeats, true, 0); got.State != JobFailed {
		t.Errorf("without an attempt limit = %+v, want failed", got)
	}
}

func TestJobFailureOverriddenPolicyReplacesDelayAndLimit(t *testing.T) {
	policy := JobRetryPolicy{Delay: func(round int) time.Duration { return time.Duration(round) * time.Hour }}
	got := policy.AfterFailure(10, errors.New("unanswered"), true, 0)
	if got != (JobFailure{State: JobQueued, Delay: 10 * time.Hour}) {
		t.Errorf("round 10 without a limit = %+v, want queued after 10h", got)
	}
}

func TestJobFailureSnoozeRequeuesUncounted(t *testing.T) {
	snoozed := fmt.Errorf("root unreadable: %w", ErrJobSnoozed)
	got := JobRetryPolicyFor(JobProbe).AfterFailure(MaxJobAttempts, snoozed, true, 0)
	if got != (JobFailure{State: JobQueued, Uncounted: true}) {
		t.Errorf("snooze = %+v, want queued at once and uncounted", got)
	}
}
