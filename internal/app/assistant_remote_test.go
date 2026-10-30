package app

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// peerClip is the peer's video the screen shows; the runner forwards its read as the screens do.
var peerClip = domain.ToRemoteID(42)

// newPeerVideoFixture answers getVideo for the peer's video and offers reprobeVideo, which the screens do not offer
// for it.
func newPeerVideoFixture(t *testing.T, replies ...domain.ChatReply) *writeFixture {
	t.Helper()
	f := newWriteFixture(t, replies...)
	f.runner.bodies["getVideo"] = `{"id":1099511627818,"title":"Peer clip"}`
	f.runner.tools = append(f.runner.tools, writeTool("reprobeVideo"))
	return f
}

// runOnPeerVideo runs the turn on the peer video's page.
func (f *writeFixture) runOnPeerVideo(t *testing.T) domain.AssistantTurn {
	t.Helper()
	return f.runIn(t, 0, "この動画にライブタグを付けて",
		domain.AssistantTurnContext{Route: domain.AssistantRouteVideo, RouteID: peerClip})
}

func TestWriteCallOnThePeerVideoBecomesAStepKeyedByItsRemoteID(t *testing.T) {
	for name, target := range map[string]string{
		"the screen's video":      `{"kind":"current"}`,
		"its title and remote id": `{"kind":"named","names":[{"name":"Peer clip","id":1099511627818}]}`,
		"its title alone":         `{"kind":"named","names":["Peer clip"]}`,
	} {
		t.Run(name, func(t *testing.T) {
			f := newPeerVideoFixture(t,
				calls(toolCall("updateVideoTags__add", `{"target":`+target+`,"tagNames":["ライブ"]}`)), text("提案しました。"))

			turn := f.runOnPeerVideo(t)

			key := domain.AssistantRemoteVideoKey(peerClip)
			if turn.Status != domain.AssistantTurnDone || turn.Clarify != nil || len(f.store.plans) != 1 {
				t.Fatalf("turn = %+v, clarify = %+v, plans = %+v; want one step", turn, turn.Clarify, f.store.plans)
			}
			plan := f.store.plans[0]
			if !slices.Equal(plan.Targets.VideoKeys, []string{key}) || plan.Call.VideoField != "videoIds" ||
				len(plan.Targets.Labels) != 1 || plan.Targets.Labels[0].Name != "Peer clip" {
				t.Fatalf("targets = %+v, call = %+v; want the peer's video by its remote key", plan.Targets, plan.Call)
			}
		})
	}
}

func TestWriteCallOnThePeerVideoAnswersAnOperationTheScreensDoNotOffer(t *testing.T) {
	f := newPeerVideoFixture(t, calls(toolCall("reprobeVideo", `{"target":{"kind":"current"}}`)), text("できません。"))

	turn := f.runOnPeerVideo(t)

	failure := turn.Progress[len(turn.Progress)-1].Error
	if turn.Status != domain.AssistantTurnDone || turn.Clarify != nil || len(f.store.plans) != 0 ||
		!strings.Contains(failure, "not available for a video of the other VVMDM") {
		t.Fatalf("turn = %+v, failure = %q; want the refusal as the call's result", turn, failure)
	}
}

// A blank name used to end the turn as a clarification asking about "".
func TestWriteCallWithABlankVideoNameNeverAsksAboutAnEmptyName(t *testing.T) {
	f := newPeerVideoFixture(t,
		calls(toolCall("updateVideoTags__add", `{"target":{"kind":"named","names":[" "]},"tagNames":[" "]}`)),
		text("できません。"))

	turn := f.runOnPeerVideo(t)

	failure := turn.Progress[len(turn.Progress)-1].Error
	if turn.Status == domain.AssistantTurnClarify || !strings.Contains(failure, "name in the call is empty") {
		t.Fatalf("turn = %+v, clarify = %+v, failure = %q; want a failure for the model", turn, turn.Clarify, failure)
	}
}

func TestVideoSchemaStepOnThePeerVideoCallsItsRemoteID(t *testing.T) {
	f := newPeerVideoFixture(t,
		calls(toolCall("setVideoDetails", `{"id":1099511627818,"body":{"title":"New"}}`)), text("提案しました。"))
	f.runOnPeerVideo(t)
	if len(f.store.plans) != 1 {
		t.Fatalf("plans = %+v", f.store.plans)
	}

	store := &fakeAssistantStore{current: domain.AssistantCurrent{Videos: map[string]int64{}}}
	runner := &fakeRunner{}
	newTestAssistant(store).execute(context.Background(), runner, runningProposal(f.store.plans[0]))

	if len(runner.calls) != 1 || runner.calls[0].Path["id"] != "1099511627818" {
		t.Fatalf("calls = %+v, want one call on the remote id", runner.calls)
	}
}

func TestWorkerSendsThePeerVideosOfABatchByTheirRemoteIDs(t *testing.T) {
	plan := domain.AssistantPlan{
		ActionID: "updateVideoTags.add",
		Targets: domain.AssistantPlanTargets{
			Kind: domain.AssistantTargetVideos, VideoKeys: []string{"key-7", domain.AssistantRemoteVideoKey(peerClip)},
		},
		Call: domain.AssistantPlanCall{VideoField: "videoIds", Body: map[string]any{"tag": map[string]any{"id": 3}}},
	}
	store := &fakeAssistantStore{current: domain.AssistantCurrent{Videos: map[string]int64{"key-7": 7}}}
	runner := &fakeRunner{}

	newTestAssistant(store).execute(context.Background(), runner, runningProposal(plan))

	if len(runner.calls) != 1 {
		t.Fatalf("calls = %+v, want one batch", runner.calls)
	}
	if ids, _ := runner.calls[0].Body["videoIds"].([]int64); !slices.Equal(ids, []int64{7, peerClip}) {
		t.Fatalf("videoIds = %v, want the local id and the remote id", runner.calls[0].Body["videoIds"])
	}
}
