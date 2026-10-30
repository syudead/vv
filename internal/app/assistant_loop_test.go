package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// scriptedChat answers its replies in order and keeps every request; a call past the script fails the test.
type scriptedChat struct {
	t        *testing.T
	replies  []domain.ChatReply
	err      error
	requests []domain.ChatRequest
}

func (c *scriptedChat) Chat(_ context.Context, req domain.ChatRequest) (domain.ChatReply, error) {
	c.requests = append(c.requests, domain.ChatRequest{Messages: slices.Clone(req.Messages), Tools: slices.Clone(req.Tools)})
	if c.err != nil {
		return domain.ChatReply{}, c.err
	}
	if len(c.requests) > len(c.replies) {
		c.t.Fatalf("model call %d is past the script", len(c.requests))
	}
	return c.replies[len(c.requests)-1], nil
}

func (c *scriptedChat) toolNames(call int) []string {
	var names []string
	for _, tool := range c.requests[call].Tools {
		names = append(names, tool.Name)
	}
	return names
}

// fakeTurnStore keeps turns in memory and counts the progress commits.
type fakeTurnStore struct {
	AssistantTurnStore
	turns    map[int64]domain.AssistantTurn
	commits  [][]domain.AssistantProgressItem
	finished *domain.AssistantTurn
}

func (s *fakeTurnStore) Turn(_ context.Context, id int64) (domain.AssistantTurn, error) {
	if t, ok := s.turns[id]; ok {
		return t, nil
	}
	return domain.AssistantTurn{}, domain.ErrNotFound
}

func (s *fakeTurnStore) RecordTurnProgress(_ context.Context, _ int64, progress []domain.AssistantProgressItem) error {
	s.commits = append(s.commits, slices.Clone(progress))
	return nil
}

func (s *fakeTurnStore) FinishTurn(_ context.Context, turn domain.AssistantTurn, _ time.Time) error {
	s.finished = &turn
	return nil
}

// fakeToolRunner offers a small registry and answers reads from bodies by operation.
type fakeToolRunner struct {
	ActionRunner
	bodies map[string]string
	reads  []domain.AssistantRead
	during func()
}

var loopRegistry = []domain.AssistantTool{
	{Name: "listVideos", Kind: domain.AssistantToolRead, Operation: "listVideos", Summary: "List videos",
		Parameters: json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}}}`)},
	{Name: "getVideo", Kind: domain.AssistantToolRead, Operation: "getVideo", Summary: "Read a video",
		IDs: []domain.AssistantIDField{{In: "path", Field: "id", Kind: "video"}}},
	{Name: "listTags", Kind: domain.AssistantToolRead, Operation: "listTags", Summary: "List tags"},
	{Name: "listPeople", Kind: domain.AssistantToolRead, Operation: "listPeople", Summary: "List people"},
	{Name: "listOrganizations", Kind: domain.AssistantToolRead, Operation: "listOrganizations", Summary: "List organizations"},
	{Name: "listPlaylists", Kind: domain.AssistantToolRead, Operation: "listPlaylists", Summary: "List playlists"},
	{Name: "createApiToken", Kind: domain.AssistantToolExcluded, Operation: "createApiToken", Summary: "Create an API token for playlists",
		Exclusion: &domain.AssistantExclusion{Operation: "createApiToken", Reason: "out of scope", Page: "/settings/api"}},
}

func (r *fakeToolRunner) Tools() []domain.AssistantTool { return loopRegistry }

func (r *fakeToolRunner) Read(_ context.Context, read domain.AssistantRead) (domain.AssistantReadResult, error) {
	r.reads = append(r.reads, read)
	if r.during != nil {
		r.during()
	}
	if strings.Contains(string(read.Arguments), "bad") {
		return domain.AssistantReadResult{}, fmt.Errorf("%w: q must be a string", domain.ErrAssistantInvalidArguments)
	}
	return domain.AssistantReadResult{Body: json.RawMessage(r.bodies[read.Operation])}, nil
}

type loopFixture struct {
	chat   *scriptedChat
	turns  *fakeTurnStore
	runner *fakeToolRunner
	now    time.Time
	a      *Assistant
}

func newLoopFixture(t *testing.T, replies ...domain.ChatReply) *loopFixture {
	t.Helper()
	f := &loopFixture{
		chat: &scriptedChat{t: t, replies: replies}, turns: &fakeTurnStore{turns: map[int64]domain.AssistantTurn{}},
		runner: &fakeToolRunner{bodies: map[string]string{}},
		now:    time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC),
	}
	f.a = NewAssistant(AssistantOptions{
		Turns: f.turns, Chat: f.chat, Store: &fakeAssistantStore{},
		Vocabulary: fakeVocabulary{entities: map[domain.AssistantLabelKind]map[int64]string{
			domain.AssistantLabelVideo: {7: "Clip 7"},
		}},
		Now: func() time.Time { return f.now },
	})
	return f
}

// run runs one turn of the request to its end and answers the turn as stored.
func (f *loopFixture) run(t *testing.T, in domain.AssistantTurnInput) domain.AssistantTurn {
	t.Helper()
	f.a.runTurn(context.Background(), f.runner, domain.AssistantTurn{
		ID: 1, Status: domain.AssistantTurnRunning, Request: in.Text, Context: in.Context,
		PreviousTurnID: in.PreviousTurnID, StartedAt: f.now,
	})
	if f.turns.finished == nil {
		t.Fatal("the turn was not finished")
	}
	return *f.turns.finished
}

func calls(calls ...domain.ToolCall) domain.ChatReply { return domain.ChatReply{ToolCalls: calls} }

func toolCall(name, args string) domain.ToolCall {
	return domain.ToolCall{Name: name, Arguments: json.RawMessage(args)}
}

func text(answer string) domain.ChatReply { return domain.ChatReply{Text: answer} }

var request = domain.AssistantTurnInput{Text: "カラオケの動画は何本?", Context: domain.AssistantTurnContext{Route: domain.AssistantRouteLibrary}}

func TestTurnLoopAnswersFromAReadResult(t *testing.T) {
	f := newLoopFixture(t,
		calls(toolCall("listVideos", `{"q":"karaoke"}`), toolCall("getVideo", `{"id":7}`)),
		text("カラオケの動画は 12 本です。"))
	f.runner.bodies["listVideos"] = `{"items":[],"total":12}`
	f.runner.bodies["getVideo"] = `{"id":7,"title":"Clip 7"}`

	turn := f.run(t, request)

	if turn.Status != domain.AssistantTurnDone || turn.Answer != "カラオケの動画は 12 本です。" {
		t.Fatalf("turn = %+v", turn)
	}
	second := f.chat.requests[1].Messages
	result := second[len(second)-2]
	if result.Role != domain.ChatRoleTool || result.ToolName != "listVideos" || result.Content != `{"items":[],"total":12}` {
		t.Fatalf("read result message = %+v", result)
	}
	if got := f.runner.reads[0]; got.Operation != "listVideos" || string(got.Arguments) != `{"q":"karaoke"}` {
		t.Fatalf("read = %+v", got)
	}
	if len(turn.Progress) != 2 || turn.Progress[0].Tool != "listVideos" || turn.Progress[1].Tool != "getVideo" ||
		turn.Progress[0].Kind != domain.AssistantProgressRead {
		t.Fatalf("progress = %+v", turn.Progress)
	}
	if labels := turn.Progress[1].Labels; len(labels) != 1 || labels[0] != (domain.AssistantLabel{
		Kind: domain.AssistantLabelVideo, ID: 7, Name: "Clip 7",
	}) {
		t.Fatalf("getVideo labels = %+v", labels)
	}
	if len(f.turns.commits) != 2 || len(f.turns.commits[0]) != 1 {
		t.Fatalf("progress commits = %d, want one per call as it starts", len(f.turns.commits))
	}
}

func TestTurnLoopOffersWhatFindToolsLoads(t *testing.T) {
	f := newLoopFixture(t, calls(toolCall("find_tools", `{"query":"playlists"}`)), text("No playlists."))

	f.run(t, request)

	first, second := f.chat.toolNames(0), f.chat.toolNames(1)
	if slices.Contains(first, "listPlaylists") || !slices.Contains(second, "listPlaylists") {
		t.Fatalf("tools = %v then %v, want listPlaylists offered after find_tools", first, second)
	}
	if slices.Contains(second, "createApiToken") {
		t.Fatalf("the excluded tool was offered: %v", second)
	}
	var found struct {
		Tools []foundTool `json:"tools"`
	}
	messages := f.chat.requests[1].Messages
	if err := json.Unmarshal([]byte(messages[len(messages)-1].Content), &found); err != nil {
		t.Fatal(err)
	}
	excluded := slices.IndexFunc(found.Tools, func(tool foundTool) bool { return tool.Name == "createApiToken" })
	if excluded < 0 || found.Tools[excluded].Kind != domain.AssistantToolExcluded || found.Tools[excluded].Page != "/settings/api" {
		t.Fatalf("found = %+v, want createApiToken as excluded with its page", found.Tools)
	}
}

func TestTurnLoopAnswersBadCallsAsErrorResults(t *testing.T) {
	f := newLoopFixture(t,
		calls(toolCall("searchEverything", `{}`), toolCall("listVideos", `{"q":"bad"}`)), text("I could not search."))

	turn := f.run(t, request)

	messages := f.chat.requests[1].Messages
	for i, want := range []string{"there is no tool named searchEverything", "q must be a string"} {
		got := messages[len(messages)-2+i]
		if got.Role != domain.ChatRoleTool || !strings.Contains(got.Content, want) || !strings.Contains(got.Content, `"error"`) {
			t.Errorf("result %d = %+v, want an error with %q", i, got, want)
		}
	}
	if turn.Status != domain.AssistantTurnDone || len(turn.Progress) != 2 || turn.Progress[0].Error == "" ||
		turn.Progress[1].Error == "" {
		t.Fatalf("turn = %+v, want both calls counted as failed steps", turn)
	}
}

func TestTurnLoopStopsBeforeAThirteenthModelCall(t *testing.T) {
	var replies []domain.ChatReply
	for range domain.AssistantModelCallMax {
		replies = append(replies, calls(toolCall("listTags", `{}`)))
	}
	f := newLoopFixture(t, replies...)

	turn := f.run(t, request)

	if len(f.chat.requests) != domain.AssistantModelCallMax {
		t.Fatalf("model calls = %d, want %d", len(f.chat.requests), domain.AssistantModelCallMax)
	}
	if turn.Status != domain.AssistantTurnStopped || turn.Stop.Reason != domain.AssistantStopStepLimit {
		t.Fatalf("turn = %+v", turn)
	}
}

func TestTurnLoopStopsWhenTheBudgetPassesDuringATool(t *testing.T) {
	f := newLoopFixture(t, calls(toolCall("listTags", `{}`), toolCall("listPeople", `{}`)))
	f.runner.during = func() { f.now = f.now.Add(domain.AssistantTurnBudget) }

	turn := f.run(t, request)

	if turn.Status != domain.AssistantTurnStopped || turn.Stop.Reason != domain.AssistantStopTimeout {
		t.Fatalf("turn = %+v", turn)
	}
	if len(f.runner.reads) != 1 || len(f.chat.requests) != 1 {
		t.Fatalf("reads = %d, model calls = %d; want nothing after the budget", len(f.runner.reads), len(f.chat.requests))
	}
}

func TestTurnLoopCorrectsAnUngroundedAnswerOnce(t *testing.T) {
	f := newLoopFixture(t,
		calls(toolCall("listVideos", `{}`)), text("There are 42 videos."), text("There are 43 videos."))
	f.runner.bodies["listVideos"] = `{"items":[],"total":12}`

	turn := f.run(t, request)

	correction := f.chat.requests[2].Messages
	if last := correction[len(correction)-1]; last.Role != domain.ChatRoleUser || !strings.Contains(last.Content, "42") {
		t.Fatalf("correction = %+v", last)
	}
	if turn.Status != domain.AssistantTurnStopped || turn.Stop.Reason != domain.AssistantStopUngrounded || turn.Answer != "" {
		t.Fatalf("turn = %+v, want ungrounded with no answer", turn)
	}
}

func TestTurnLoopChecksLibraryNamesInTheAnswer(t *testing.T) {
	f := newLoopFixture(t, calls(toolCall("listTags", `{}`)), text("The tag Karaoke is used."), text("The tag Live is used."))
	f.a.vocabulary = fakeVocabulary{known: []string{"karaoke", "live"}}
	f.runner.bodies["listTags"] = `{"items":[{"name":"Live"}]}`

	turn := f.run(t, request)

	if turn.Status != domain.AssistantTurnDone || turn.Answer != "The tag Live is used." {
		t.Fatalf("turn = %+v, want the corrected answer that names a listed tag", turn)
	}
}

// proposalReader answers one stored proposal.
type proposalReader struct {
	AssistantStore
	proposal domain.AssistantProposal
}

func (p proposalReader) Proposal(context.Context, int64) (domain.AssistantProposal, error) {
	return p.proposal, nil
}

func TestTurnLoopFollowUpCarriesThePreviousRequestAndSteps(t *testing.T) {
	f := newLoopFixture(t, text("Done."))
	f.turns.turns[4] = domain.AssistantTurn{ID: 4, Request: "猫のタグを付けて", ProposalID: 9}
	f.a.store = proposalReader{proposal: domain.AssistantProposal{ID: 9, Steps: []domain.AssistantStep{
		{Plan: domain.AssistantPlan{ActionID: "createTag", Created: []string{"Cat"}}},
		{Plan: domain.AssistantPlan{ActionID: "updateVideoTags.add", Targets: domain.AssistantPlanTargets{
			Labels: []domain.AssistantLabel{{Kind: domain.AssistantLabelVideo, Name: "Clip 7"}},
		}}},
	}}}
	in := request
	in.Text, in.PreviousTurnID = "やっぱり犬にして", 4

	f.run(t, in)

	system := f.chat.requests[0].Messages[0].Content
	for _, want := range []string{"<previous-request>猫のタグを付けて</previous-request>", "1. createTag; creating Cat",
		"2. updateVideoTags.add; on Clip 7", "Today is 2026-10-30."} {
		if !strings.Contains(system, want) {
			t.Errorf("system message lacks %q:\n%s", want, system)
		}
	}
}

func TestTurnLoopStopsWithTheModelFailure(t *testing.T) {
	cases := map[error]domain.AssistantStopReason{
		domain.ErrOllamaUnreachable: domain.AssistantStopConnection,
		domain.ErrOllamaModel:       domain.AssistantStopModel,
		domain.ErrOllamaTimeout:     domain.AssistantStopTimeout,
	}
	for cause, reason := range cases {
		f := newLoopFixture(t)
		f.chat.err = fmt.Errorf("chat: %w", cause)
		if turn := f.run(t, request); turn.Status != domain.AssistantTurnStopped || turn.Stop.Reason != reason {
			t.Errorf("%v: turn = %+v, want stopped with %s", cause, turn, reason)
		}
	}
}

// busyTurns refuses every turn while turn 3 runs.
type busyTurns struct{ AssistantTurnStore }

func (busyTurns) CreateTurn(context.Context, domain.AssistantTurnInput, time.Time) (domain.AssistantTurn, error) {
	return domain.AssistantTurn{ID: 3, Status: domain.AssistantTurnRunning}, domain.ErrAssistantBusy
}

func TestStartTurnAnswersTheRunningTurnWhenBusy(t *testing.T) {
	a := NewAssistant(AssistantOptions{Turns: busyTurns{}, Chat: &scriptedChat{t: t}, Vocabulary: fakeVocabulary{}})
	turn, err := a.StartTurn(context.Background(), request)
	if !errors.Is(err, domain.ErrAssistantBusy) || turn.ID != 3 {
		t.Fatalf("StartTurn() = %+v, %v", turn, err)
	}
}
