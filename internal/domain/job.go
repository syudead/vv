package domain

import "errors"

// Only expensive processing is queued; scans themselves are not jobs.
type JobKind string

const (
	JobProbe         JobKind = "probe"
	JobThumbnail     JobKind = "thumbnail"
	JobSeekThumbnail JobKind = "seek_thumbnail"
	JobPreview       JobKind = "preview"
	// JobFingerprint reads sprite frames without ffmpeg or the source file.
	JobFingerprint JobKind = "fingerprint"
)

// Bound retries so one corrupt file cannot monopolize a single worker forever.
const MaxJobAttempts = 3

var ErrNoJob = errors.New("no job to process")

// ErrFailureRepeats marks a failure every retry would reproduce, such as an input too slow to process; the job fails
// once every current location has failed, without spending its remaining attempts.
var ErrFailureRepeats = errors.New("retrying would repeat the failure")

// AttemptsAfterFailure counts a repeating failure as the last allowed attempt.
func AttemptsAfterFailure(attempts int, cause error) int {
	if errors.Is(cause, ErrFailureRepeats) {
		return max(attempts, MaxJobAttempts)
	}
	return attempts
}

type Job struct {
	ID              int64
	Kind            JobKind
	VideoID         int64
	ContentKey      string
	LocationID      int64
	LocationVersion int64
	LocationPath    string
	// LocationGeneration prevents completing or permanently failing against stale location membership after a claim.
	LocationGeneration int64
	// LastLocation refers to the registered location set at claim time.
	LastLocation bool
	// Attempts includes this claim.
	Attempts int
	// Force regenerates the artifact whatever the stage state is, and its seek stage queues no follow-up process.
	Force bool
}

// JobKinds defines stage order and worker creation; omitted kinds are never processed.
var JobKinds = []JobKind{JobProbe, JobThumbnail, JobSeekThumbnail, JobPreview, JobFingerprint}

type JobState string

const (
	JobQueued  JobState = "queued"
	JobRunning JobState = "running"
	JobDone    JobState = "done"
	JobFailed  JobState = "failed"
)

// ClaimAttempts counts full path-ordered rounds across registered locations, not individual claims; startsRound marks initial claims or wraparound.
func ClaimAttempts(attempts int, startsRound bool) int {
	if startsRound {
		return attempts + 1
	}
	return attempts
}

// JobStateAfterFailure requeues stale claims without counting them and fails only after MaxJobAttempts on the last location, ensuring every current location is tried.
func JobStateAfterFailure(attempts int, lastLocation, claimCurrent bool) JobState {
	if claimCurrent && lastLocation && attempts >= MaxJobAttempts {
		return JobFailed
	}
	return JobQueued
}

type JobClaimCondition struct {
	// RegisteredLocation prevents workers from receiving unregistered paths while preserving jobs for resumption after registration.
	RegisteredLocation bool
	ProbeFinished      bool
	// NoClaimableThumbnail counts queued/running registered thumbnails, including those still waiting for probe.
	NoClaimableThumbnail  bool
	SeekThumbnailFinished bool
}

// ClaimConditionFor waits for probe before thumbnails, prioritizes representative thumbnails over seek sprites, and requires completed sprites for fingerprints; it never interrupts running work.
func ClaimConditionFor(kind JobKind) JobClaimCondition {
	return JobClaimCondition{
		RegisteredLocation:    true,
		ProbeFinished:         kind == JobThumbnail || kind == JobSeekThumbnail,
		NoClaimableThumbnail:  kind == JobSeekThumbnail,
		SeekThumbnailFinished: kind == JobFingerprint,
	}
}

func (c JobClaimCondition) Allows(
	hasRegisteredLocation bool, probe ProbeState, claimableThumbnail bool, seekThumbnail SeekThumbnailState,
) bool {
	if c.RegisteredLocation && !hasRegisteredLocation {
		return false
	}
	if c.ProbeFinished && probe == ProbeStatePending {
		return false
	}
	if c.NoClaimableThumbnail && claimableThumbnail {
		return false
	}
	if c.SeekThumbnailFinished && seekThumbnail != SeekThumbnailDone {
		return false
	}
	return true
}
