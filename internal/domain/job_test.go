package domain

import "testing"

func TestClaimAttemptsCountsOnlyNewRounds(t *testing.T) {
	if got := ClaimAttempts(0, true); got != 1 {
		t.Errorf("first claim = %d, want 1", got)
	}
	// 同じ巡回の次の所在を試すときは数えない。
	if got := ClaimAttempts(1, false); got != 1 {
		t.Errorf("next location in the same round = %d, want 1", got)
	}
	// 最後の所在から先頭へ戻ると、新しい巡回として数える。
	if got := ClaimAttempts(1, true); got != 2 {
		t.Errorf("wrapped round = %d, want 2", got)
	}
}

func TestJobStateAfterFailureStopsAtLimitOnLastLocation(t *testing.T) {
	if MaxJobAttempts != 3 {
		t.Fatalf("MaxJobAttempts = %d, want 3", MaxJobAttempts)
	}
	tests := []struct {
		name         string
		attempts     int
		lastLocation bool
		claimCurrent bool
		want         JobState
	}{
		{"below limit", MaxJobAttempts - 1, true, true, JobQueued},
		{"at limit", MaxJobAttempts, true, true, JobFailed},
		{"over limit", MaxJobAttempts + 1, true, true, JobFailed},
		{"untried location remains", MaxJobAttempts, false, true, JobQueued},
		{"claim no longer current", MaxJobAttempts, true, false, JobQueued},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := JobStateAfterFailure(tt.attempts, tt.lastLocation, tt.claimCurrent); got != tt.want {
				t.Fatalf("JobStateAfterFailure(%d, %v, %v) = %q, want %q",
					tt.attempts, tt.lastLocation, tt.claimCurrent, got, tt.want)
			}
		})
	}
}

// 一連の専有と失敗で、MaxJobAttempts 巡目の最後の所在で失敗したときに初めて
// failed になることを確かめる。
func TestJobRetriesUntilLimitAcrossLocations(t *testing.T) {
	const locations = 2
	attempts := 0
	var state JobState
	claims := 0
	for state != JobFailed {
		for i := range locations {
			attempts = ClaimAttempts(attempts, i == 0)
			claims++
			state = JobStateAfterFailure(attempts, i == locations-1, true)
			if state == JobFailed {
				break
			}
		}
		if claims > locations*(MaxJobAttempts+1) {
			t.Fatal("job never failed")
		}
	}
	if attempts != MaxJobAttempts || claims != locations*MaxJobAttempts {
		t.Fatalf("failed after attempts=%d claims=%d, want %d and %d", attempts, claims, MaxJobAttempts, locations*MaxJobAttempts)
	}
}

func TestClaimConditionWaitsForProbeOnlyForThumbnail(t *testing.T) {
	for _, kind := range JobKinds {
		c := ClaimConditionFor(kind)
		if c.Allows(false, ProbeStateDone) {
			t.Errorf("%s: allowed without a registered location", kind)
		}
		for _, probe := range []ProbeState{ProbeStateDone, ProbeStateFailed} {
			if !c.Allows(true, probe) {
				t.Errorf("%s: not allowed after probe %s", kind, probe)
			}
		}
		wantPending := kind != JobThumbnail
		if got := c.Allows(true, ProbeStatePending); got != wantPending {
			t.Errorf("%s: allowed while probe pending = %v, want %v", kind, got, wantPending)
		}
	}
}
