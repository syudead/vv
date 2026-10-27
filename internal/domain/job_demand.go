package domain

// JobResources counts slots of each resource the job scheduler grants: the demand of one job, the slots in use, or
// the capacity. See docs/design-docs/job-scheduling.md.
type JobResources struct {
	// Read slots allow reading one source file each.
	Read int
	// CPU slots allow one ffmpeg decode or encode each.
	CPU int
}

// SharedReadSlots is the read capacity shared by every disk.
const SharedReadSlots = 4

// JobCapacity leaves CPU for streaming, since ffmpeg already spreads one decode or encode over several threads.
func JobCapacity(numCPU int) JobResources {
	return JobResources{Read: SharedReadSlots, CPU: numCPU/4 + 1}
}

// JobDemand is the fixed demand a kind declares; a kind with no demand always fits.
func JobDemand(kind JobKind) JobResources {
	switch kind {
	case JobProbe:
		// ffprobe reads the container header.
		return JobResources{Read: 1}
	case JobThumbnail, JobSeekThumbnail, JobPreview:
		return JobResources{Read: 1, CPU: 1}
	case JobFingerprint:
		// Reads sprite frames from the data directory, not the source file.
		return JobResources{CPU: 1}
	default:
		return JobResources{}
	}
}

func (r JobResources) Plus(other JobResources) JobResources {
	return JobResources{Read: r.Read + other.Read, CPU: r.CPU + other.CPU}
}

func (r JobResources) Minus(other JobResources) JobResources {
	return JobResources{Read: r.Read - other.Read, CPU: r.CPU - other.CPU}
}

// Within reports whether every resource of r is at most capacity's.
func (r JobResources) Within(capacity JobResources) bool {
	return r.Read <= capacity.Read && r.CPU <= capacity.CPU
}
