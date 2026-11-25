package domain

import "fmt"

// ErrJobPaused is the cause a pause of background work cancels a running job with; the job is requeued as a snooze,
// so it keeps its attempts and runs again from the start after the resume. See docs/design-docs/background-pause.md.
var ErrJobPaused = fmt.Errorf("background work is paused: %w", ErrJobSnoozed)

// BackgroundWork is the owner's switch over scans and scheduled jobs, the work running now and the work left.
type BackgroundWork struct {
	Paused bool
	// Running is oldest first.
	Running []BackgroundActivity
	// Groups holds one entry per BackgroundGroups, in that order.
	Groups []BackgroundGroupStatus
}

// BackgroundGroup is what the owner sees background work as: a scan, or the jobs of one feature.
type BackgroundGroup string

const (
	BackgroundScan         BackgroundGroup = "scan"
	BackgroundImport       BackgroundGroup = "import"
	BackgroundAutoTag      BackgroundGroup = "auto_tag"
	BackgroundFace         BackgroundGroup = "face"
	BackgroundVersionJudge BackgroundGroup = "version_judge"
	BackgroundBackfill     BackgroundGroup = "backfill"
)

// BackgroundGroups are every group in the order the status answers them.
var BackgroundGroups = []BackgroundGroup{
	BackgroundScan, BackgroundImport, BackgroundAutoTag, BackgroundFace, BackgroundVersionJudge, BackgroundBackfill,
}

// BackgroundRegistering is the kind of the running entry of a scan registering a file.
const BackgroundRegistering = "registering"

// JobGroup is the group a scheduled kind's jobs are counted and listed in; it follows what the job does, not its
// priority class. Every kind of ScheduledJobKinds has one.
func JobGroup(kind JobKind) (BackgroundGroup, bool) {
	switch kind {
	case JobProbe, JobThumbnail, JobSeekThumbnail, JobPreview, JobFingerprint:
		return BackgroundImport, true
	case JobAutoTag:
		return BackgroundAutoTag, true
	case JobFace, JobFaceImage, JobFaceCast, JobQueueFaceImages:
		return BackgroundFace, true
	case JobVersionJudge:
		return BackgroundVersionJudge, true
	case JobKeyframes, JobMediaInfo, JobFrameIndex, JobQueueKeyframes, JobQueueMediaInfo, JobQueueFrameIndex,
		JobVirtualThumbnails:
		return BackgroundBackfill, true
	}
	return "", false
}

// GroupJobKinds are the scheduled kinds of group, in ScheduledJobKinds order.
func GroupJobKinds(group BackgroundGroup) []JobKind {
	var kinds []JobKind
	for _, kind := range ScheduledJobKinds {
		if g, _ := JobGroup(kind); g == group {
			kinds = append(kinds, kind)
		}
	}
	return kinds
}

// BackgroundActivity is one piece of work running now.
type BackgroundActivity struct {
	Group BackgroundGroup
	// Kind is BackgroundRegistering for a scan, otherwise the job kind.
	Kind string
	// Video is nil when the work names no video or the viewer's audience does not list it.
	Video *BackgroundVideo
	// FileName is empty when the work has no location.
	FileName string
}

type BackgroundVideo struct {
	ID    int64
	Title string
}

// BackgroundGroupStatus is the work left in one group.
type BackgroundGroupStatus struct {
	Group BackgroundGroup
	// Remaining counts files for scan, videos for import and jobs otherwise.
	Remaining int
	// Waiting is set for scan only: a paused scan or held watch directories wait for the resume.
	Waiting bool
	// FeatureOff is set when the group's feature holds its kinds back, whether or not background work is paused.
	FeatureOff bool
}

// EmptyBackgroundGroups is every group with nothing left.
func EmptyBackgroundGroups() []BackgroundGroupStatus {
	groups := make([]BackgroundGroupStatus, len(BackgroundGroups))
	for i, group := range BackgroundGroups {
		groups[i] = BackgroundGroupStatus{Group: group}
	}
	return groups
}

// BackgroundChanged reports that the switch, the running work or a remaining count may have changed. The owner's
// screens receive it at most once a second; see docs/design-docs/background-pause.md.
type BackgroundChanged struct{}

func (BackgroundChanged) event() {}
