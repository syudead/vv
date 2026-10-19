package domain

import "testing"

func TestPipelineState(t *testing.T) {
	waiting := ActivityState{Status: ActivityRunning, Reason: ActivityWaiting}
	processing := ActivityState{Status: ActivityRunning, Reason: ActivityProcessing}
	failed := ActivityState{Status: ActivityAttention, Reason: ActivityFailed}
	noConnection := ActivityState{Status: ActivityAttention, Reason: ActivityNoConnection}
	completed := ActivityState{Status: ActivityDone, Outcome: ActivityCompleted}
	gone := ActivityState{Status: ActivityDone, Outcome: ActivityGone}
	superseded := ActivityState{Status: ActivityDone, Outcome: ActivitySuperseded}
	for _, tc := range []struct {
		name    string
		members []PipelineMember
		want    ActivityState
	}{
		{"running and processing", []PipelineMember{
			{ID: 1, Kind: ActivityImport, State: completed}, {ID: 2, Kind: ActivityAutoTag, State: waiting},
			{ID: 3, Kind: ActivityVersionJudgement, State: processing},
		}, processing},
		{"running and only waiting", []PipelineMember{
			{ID: 1, Kind: ActivityImport, State: failed}, {ID: 2, Kind: ActivityAutoTag, State: waiting},
		}, waiting},
		{"attention takes the oldest reason", []PipelineMember{
			{ID: 1, Kind: ActivityImport, State: completed}, {ID: 2, Kind: ActivityAutoTag, State: noConnection},
			{ID: 3, Kind: ActivityVersionJudgement, State: failed},
		}, noConnection},
		{"import gone", []PipelineMember{
			{ID: 1, Kind: ActivityImport, State: gone}, {ID: 2, Kind: ActivityAutoTag, State: completed},
		}, gone},
		{"import superseded from outside", []PipelineMember{
			{ID: 1, Kind: ActivityImport, State: superseded, SupersededBy: 9},
		}, superseded},
		{"all done", []PipelineMember{
			{ID: 1, Kind: ActivityImport, State: completed}, {ID: 2, Kind: ActivityAutoTag, State: superseded, SupersededBy: 9},
		}, completed},
		{"a replaced member is not read", []PipelineMember{
			{ID: 1, Kind: ActivityImport, State: superseded, SupersededBy: 2}, {ID: 2, Kind: ActivityImport, State: completed},
		}, completed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := PipelineState(tc.members); got != tc.want {
				t.Errorf("PipelineState = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestPipelineIsNeitherRetriedNorUndoneNorSuperseded(t *testing.T) {
	pipeline := ActivityEntry{Kind: ActivityPipeline, Status: ActivityAttention, Reason: ActivityFailed, Decider: ActivityByRule}
	if pipeline.RetryOffered() {
		t.Error("a pipeline needing attention offers retry")
	}
	done := ActivityEntry{Kind: ActivityPipeline, Status: ActivityDone, Outcome: ActivityCompleted, Decider: ActivityByRule}
	if done.UndoOffered() {
		t.Error("a done pipeline offers undo")
	}
	if pipeline.Supersedes(pipeline) {
		t.Error("a new pipeline supersedes an older one of the same video")
	}
	if feature, ok := ActivityPipeline.Feature(); !ok || feature != ActivityFeatureImport {
		t.Errorf("pipeline feature = %q, %v; want import", feature, ok)
	}
}
