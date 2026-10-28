package domain

import "context"

// JobResources counts slots of each resource the job scheduler grants: the demand of one job, or slots lent to it.
// See docs/design-docs/job-scheduling.md.
type JobResources struct {
	// Read slots allow reading one source file each, on the disk of the job's location.
	Read int
	// CPU slots allow one ffmpeg decode or encode each.
	CPU int
	// Classifier slots allow one request to the classifier each.
	Classifier int
}

// JobCPUSlots leaves CPU for streaming, since ffmpeg already spreads one decode or encode over several threads.
func JobCPUSlots(numCPU int) int {
	return numCPU/4 + 1
}

// JobClassifierSlots is 1 because a CPU-only classifier provider cannot serve parallel requests.
const JobClassifierSlots = 1

// JobDemand is the fixed demand a kind declares; a kind with no demand always fits.
func JobDemand(kind JobKind) JobResources {
	switch kind {
	case JobProbe:
		// ffprobe reads the container header.
		return JobResources{Read: 1}
	case JobThumbnail, JobSeekThumbnail, JobPreview:
		return JobResources{Read: 1, CPU: 1}
	case JobKeyframes, JobMediaInfo:
		// ffprobe reads the packet index or the container.
		return JobResources{Read: 1}
	case JobFingerprint:
		// Reads sprite frames from the data directory, not the source file.
		return JobResources{CPU: 1}
	case JobAutoTag:
		// Sends generated thumbnails and seek frames, never the source file.
		return JobResources{Classifier: 1}
	case JobVersionJudge:
		// Compares stored fingerprints and sends frames cut from the seek previews.
		return JobResources{Classifier: 1}
	default:
		return JobResources{}
	}
}

// DiskType is what is known about a disk's seek cost.
type DiskType int

const (
	// DiskUnknown is a disk whose type could not be read, such as a network share or a Btrfs volume.
	DiskUnknown DiskType = iota
	DiskRotational
	DiskSolidState
)

func (t DiskType) String() string {
	switch t {
	case DiskRotational:
		return "rotational"
	case DiskSolidState:
		return "solid-state"
	default:
		return "unknown"
	}
}

// MaxDiskReadSlots is the read capacity of a solid-state disk, and the most extractions one job runs at once.
const MaxDiskReadSlots = 4

// Disk identifies the device a media folder is stored on; folders with the same key share its read slots.
type Disk struct {
	Key  string
	Type DiskType
}

// ReadSlots is 1 for a rotational or unknown disk, because parallel sequential reads make a rotational disk seek.
func (d Disk) ReadSlots() int {
	if d.Type == DiskSolidState {
		return MaxDiskReadSlots
	}
	return 1
}

// JobSlots lends a running job further slots on its own disk, one extraction at a time, without waiting.
type JobSlots interface {
	// TryAcquire takes demand when it fits now and reports whether it did.
	TryAcquire(demand JobResources) bool
	// Release returns demand taken by TryAcquire.
	Release(demand JobResources)
}

type jobSlotsKey struct{}

// WithJobSlots hands the job's lender to the code that splits its work.
func WithJobSlots(ctx context.Context, slots JobSlots) context.Context {
	return context.WithValue(ctx, jobSlotsKey{}, slots)
}

// JobSlotsFrom returns the running job's lender; outside the scheduler every request fits.
func JobSlotsFrom(ctx context.Context) JobSlots {
	if slots, ok := ctx.Value(jobSlotsKey{}).(JobSlots); ok {
		return slots
	}
	return unlimitedSlots{}
}

type unlimitedSlots struct{}

func (unlimitedSlots) TryAcquire(JobResources) bool { return true }
func (unlimitedSlots) Release(JobResources)         {}
