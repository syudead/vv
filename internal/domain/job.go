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

// JobPriority is a job's class; claims serve a lower class strictly first. See docs/design-docs/job-scheduling.md.
type JobPriority int

const (
	// JobPriorityOwner is work the owner asked for on one video: a force rescan or a retry.
	JobPriorityOwner JobPriority = 0
	// JobPriorityImport is everything an import, a scan or an after-import rule queues.
	JobPriorityImport JobPriority = 1
	// JobPriorityBackfill is work that only refreshes stored results.
	JobPriorityBackfill JobPriority = 2
)

var ErrNoJob = errors.New("no job to process")

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
	// StartsRound reports that this claim began a round and so added one to Attempts.
	StartsRound bool
	// Force regenerates the artifact whatever the stage state is, and its seek stage queues no follow-up process.
	Force bool
	// Priority is the job's class; the stages a successful owner-class job queues take the owner class too.
	Priority JobPriority
	// RootID is the media folder holding the claimed location.
	RootID int64
}

// JobClaim selects the job one claim may take: a kind, its class, and where its next location may be.
type JobClaim struct {
	Kind JobKind
	// MaxPriority admits this class and every more urgent one.
	MaxPriority JobPriority
	// Roots admits only jobs whose next location is under one of these media folders, the folders of one disk; nil
	// admits any folder.
	Roots []int64
	// Unavailable media folders' locations are passed over, so a job stored only under them stays queued.
	Unavailable []int64
}

// AnyJobClaim admits every queued job of kind, as a claim outside the scheduler does.
func AnyJobClaim(kind JobKind) JobClaim {
	return JobClaim{Kind: kind, MaxPriority: JobPriorityBackfill}
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
