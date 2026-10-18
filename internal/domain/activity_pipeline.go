package domain

import (
	"slices"
	"time"
)

// A pipeline entry groups one video's import run with the follow-up processes that run started; its state is
// rolled up from its members.

// PipelineMember is the part of a member entry the roll-up reads.
type PipelineMember struct {
	ID    int64
	Kind  ActivityKind
	State ActivityState
	// SupersededBy is the entry that closed this one as superseded, or zero.
	SupersededBy int64
}

// PipelineState rolls up members ordered oldest first, ignoring a member replaced by another member of the same
// pipeline. Running work wins, then the oldest member needing attention, then the import member's outcome.
func PipelineState(members []PipelineMember) ActivityState {
	current := currentPipelineMembers(members)
	if state, ok := runningPipelineState(current); ok {
		return state
	}
	for _, member := range current {
		if member.State.Status == ActivityAttention {
			return ActivityState{Status: ActivityAttention, Reason: member.State.Reason}
		}
	}
	return ActivityState{Status: ActivityDone, Outcome: pipelineOutcome(current)}
}

func currentPipelineMembers(members []PipelineMember) []PipelineMember {
	ids := make(map[int64]bool, len(members))
	for _, member := range members {
		ids[member.ID] = true
	}
	return slices.DeleteFunc(slices.Clone(members), func(member PipelineMember) bool {
		return member.SupersededBy != 0 && ids[member.SupersededBy]
	})
}

// A pipeline with no member yet is waiting for the member written in the same transaction.
func runningPipelineState(members []PipelineMember) (ActivityState, bool) {
	if len(members) == 0 {
		return ActivityState{Status: ActivityRunning, Reason: ActivityWaiting}, true
	}
	running := false
	for _, member := range members {
		if member.State.Status != ActivityRunning {
			continue
		}
		if member.State.Reason == ActivityProcessing {
			return ActivityState{Status: ActivityRunning, Reason: ActivityProcessing}, true
		}
		running = true
	}
	return ActivityState{Status: ActivityRunning, Reason: ActivityWaiting}, running
}

func pipelineOutcome(members []PipelineMember) ActivityOutcome {
	outcome := ActivityCompleted
	for _, member := range members {
		if member.Kind != ActivityImport {
			continue
		}
		switch member.State.Outcome {
		case ActivityGone:
			return ActivityGone
		case ActivitySuperseded:
			outcome = ActivitySuperseded
		}
	}
	return outcome
}

// ActivityStageState is the state of one ingest stage inside an import entry.
type ActivityStageState string

const (
	ActivityStageWaiting    ActivityStageState = "waiting"
	ActivityStageProcessing ActivityStageState = "processing"
	ActivityStageDone       ActivityStageState = "done"
	ActivityStageFailed     ActivityStageState = "failed"
	// ActivityStageGone marks a stage left unfinished because its video was deleted or lost every location.
	ActivityStageGone ActivityStageState = "gone"
)

// ActivityStage is one stage row of an import entry; Detail is the stage's last error message, untranslated.
type ActivityStage struct {
	Stage     JobKind
	State     ActivityStageState
	Detail    string
	UpdatedAt time.Time
}

var importStageStates = map[JobState]ActivityStageState{
	JobQueued:  ActivityStageWaiting,
	JobRunning: ActivityStageProcessing,
	JobDone:    ActivityStageDone,
	JobFailed:  ActivityStageFailed,
}

// ImportStageState maps a job state to the state of its stage row.
func ImportStageState(state JobState) ActivityStageState {
	return importStageStates[state]
}
