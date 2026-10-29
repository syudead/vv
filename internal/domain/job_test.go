package domain

import "testing"

func TestClaimAttemptsCountsOnlyNewRounds(t *testing.T) {
	if got := ClaimAttempts(0, true); got != 1 {
		t.Errorf("first claim = %d, want 1", got)
	}
	if got := ClaimAttempts(1, false); got != 1 {
		t.Errorf("next location in the same round = %d, want 1", got)
	}
	if got := ClaimAttempts(1, true); got != 2 {
		t.Errorf("wrapped round = %d, want 2", got)
	}
}

func TestClaimConditionWaitsForProbeOnlyForThumbnails(t *testing.T) {
	for _, kind := range JobKinds {
		c := ClaimConditionFor(kind)
		if c.Allows(false, ProbeStateDone, false, SeekThumbnailDone) {
			t.Errorf("%s: allowed without a registered location", kind)
		}
		for _, probe := range []ProbeState{ProbeStateDone, ProbeStateFailed} {
			if !c.Allows(true, probe, false, SeekThumbnailDone) {
				t.Errorf("%s: not allowed after probe %s", kind, probe)
			}
		}
		wantPending := kind != JobThumbnail && kind != JobSeekThumbnail
		if got := c.Allows(true, ProbeStatePending, false, SeekThumbnailDone); got != wantPending {
			t.Errorf("%s: allowed while probe pending = %v, want %v", kind, got, wantPending)
		}
	}
}

func TestClaimConditionSeekThumbnailWaitsForClaimableThumbnails(t *testing.T) {
	for _, kind := range JobKinds {
		got := ClaimConditionFor(kind).Allows(true, ProbeStateDone, true, SeekThumbnailDone)
		want := kind != JobSeekThumbnail
		if got != want {
			t.Errorf("%s: allowed while a thumbnail job is claimable = %v, want %v", kind, got, want)
		}
	}
}

func TestClaimConditionFingerprintWaitsForSeekThumbnail(t *testing.T) {
	for _, kind := range JobKinds {
		for _, seek := range []SeekThumbnailState{SeekThumbnailPending, SeekThumbnailFailed} {
			got := ClaimConditionFor(kind).Allows(true, ProbeStateDone, false, seek)
			want := kind != JobFingerprint
			if got != want {
				t.Errorf("%s: allowed while seek thumbnails are %s = %v, want %v", kind, seek, got, want)
			}
		}
		if !ClaimConditionFor(kind).Allows(true, ProbeStateDone, false, SeekThumbnailDone) {
			t.Errorf("%s: not allowed after seek thumbnails are done", kind)
		}
	}
	if JobKinds[len(JobKinds)-1] != JobFingerprint {
		t.Fatalf("JobKinds = %v, want fingerprint last", JobKinds)
	}
}
