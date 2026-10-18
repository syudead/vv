package domain

import "slices"

// A video's five import stages share one activity entry.

type ImportJobState struct {
	Kind  JobKind
	State JobState
}

const ImportStageCount = 5

// ImportActivityProgress selects the running stage or earliest queued stage and counts completed stages up to Total; it returns nil without queued or running jobs.
func ImportActivityProgress(jobs []ImportJobState) *ActivityImportProgress {
	stage, ok := firstImportStage(jobs, JobRunning)
	if !ok {
		stage, ok = firstImportStage(jobs, JobQueued)
	}
	if !ok {
		return nil
	}
	done := 0
	for _, kind := range JobKinds {
		if slices.Contains(jobs, ImportJobState{Kind: kind, State: JobDone}) {
			done++
		}
	}
	return &ActivityImportProgress{Stage: stage, Done: done, Total: ImportStageCount}
}

func firstImportStage(jobs []ImportJobState, state JobState) (JobKind, bool) {
	for _, kind := range JobKinds {
		if slices.Contains(jobs, ImportJobState{Kind: kind, State: state}) {
			return kind, true
		}
	}
	return "", false
}
