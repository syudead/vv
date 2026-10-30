package app

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// writeRunner adds write tools to the loop's registry, splits schema rows' arguments as the registry does, and
// counts the calls that would change something.
type writeRunner struct {
	*fakeToolRunner
	tools []domain.AssistantTool
	runs  []domain.AssistantCall
}

func (r *writeRunner) Tools() []domain.AssistantTool { return r.tools }

func (r *writeRunner) Run(_ context.Context, call domain.AssistantCall) (domain.AssistantCallResult, error) {
	r.runs = append(r.runs, call)
	return domain.AssistantCallResult{}, nil
}

func (r *writeRunner) WriteCall(actionID string, raw json.RawMessage) (domain.AssistantCall, error) {
	var args map[string]any
	if err := json.Unmarshal(raw, &args); err != nil {
		return domain.AssistantCall{}, fmt.Errorf("%w: %w", domain.ErrAssistantInvalidArguments, err)
	}
	call := domain.AssistantCall{ActionID: actionID, Path: map[string]string{}}
	call.Body, _ = args["body"].(map[string]any)
	for name, value := range args {
		if id, ok := value.(float64); ok {
			call.Path[name] = strconv.FormatInt(int64(id), 10)
		}
	}
	return call, nil
}

func writeTool(id string) domain.AssistantTool {
	row, _ := domain.LookupAssistantAction(id)
	return domain.AssistantTool{Name: id, Kind: domain.AssistantToolWrite, Operation: row.Operation, Summary: id, Row: &row}
}

func writeRegistry() []domain.AssistantTool {
	move := writeTool("moveFaceAppearances")
	move.IDs = []domain.AssistantIDField{
		{In: "path", Field: "id", Kind: "person"},
		{In: "body", Field: "/appearanceIds", Kind: "faceAppearance"},
		{In: "body", Field: "/targetId", Kind: "person"},
	}
	details := writeTool("setVideoDetails")
	details.IDs = []domain.AssistantIDField{
		{In: "path", Field: "id", Kind: "video"}, {In: "body", Field: "/workInfo/people/*/person/id", Kind: "person"},
	}
	return append(slices.Clone(loopRegistry), details,
		domain.AssistantTool{Name: "fetchWorkInfo", Kind: domain.AssistantToolRead, Operation: "fetchWorkInfo", Summary: "fetch"},
		writeTool("createTag"), writeTool("updateVideoTags.add"), writeTool("mergeTag"),
		writeTool("applyWorkInfoFetch"), move)
}

var (
	liveTag    = domain.AssistantCandidate{Kind: domain.AssistantLabelTag, ID: 3, Name: "ライブ", Count: 5}
	concertTag = domain.AssistantCandidate{Kind: domain.AssistantLabelTag, ID: 4, Name: "コンサート", Count: 9}
	catTags    = []domain.AssistantCandidate{
		{Kind: domain.AssistantLabelTag, ID: 5, Name: "猫", Count: 2}, {Kind: domain.AssistantLabelTag, ID: 6, Name: "ネコ", Count: 1},
	}
)

type writeFixture struct {
	chat   *scriptedChat
	turns  *fakeTurnStore
	store  *fakeAssistantStore
	runner *writeRunner
	a      *Assistant
}

// newWriteFixture scripts the replies after a find_tools call that loads every write tool.
func newWriteFixture(t *testing.T, replies ...domain.ChatReply) *writeFixture {
	t.Helper()
	now := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	f := &writeFixture{
		chat: &scriptedChat{t: t, replies: append([]domain.ChatReply{
			calls(toolCall("find_tools", `{"query":"tag fetch move details"}`)),
		}, replies...)},
		turns: &fakeTurnStore{turns: map[int64]domain.AssistantTurn{}}, store: &fakeAssistantStore{},
		runner: &writeRunner{fakeToolRunner: &fakeToolRunner{bodies: map[string]string{}}, tools: writeRegistry()},
	}
	f.a = NewAssistant(AssistantOptions{
		Turns: f.turns, Chat: f.chat, Store: f.store,
		Vocabulary: fakeVocabulary{
			names: map[domain.AssistantLabelKind]map[string][]domain.AssistantCandidate{domain.AssistantLabelTag: {
				"ライブ": {liveTag}, "コンサート": {concertTag}, "猫": catTags,
			}},
			entities: map[domain.AssistantLabelKind]map[int64]string{
				domain.AssistantLabelPerson:         {11: "Aさん", 12: "Bさん"},
				domain.AssistantLabelFaceAppearance: {101: "Clip 7", 102: "Clip 8"},
			},
		},
		Videos: &fakeLibraryVideos{videos: map[int64]domain.Video{7: {ID: 7, UserKey: "key-7", Title: "Clip 7"}}},
		Now:    func() time.Time { return now },
	})
	return f
}

func (f *writeFixture) run(t *testing.T, previous int64) domain.AssistantTurn {
	t.Helper()
	f.a.runTurn(context.Background(), f.runner, domain.AssistantTurn{
		ID: 1, Status: domain.AssistantTurnRunning, Request: "タグを作って選択中の動画に付けて", PreviousTurnID: previous,
		Context:   domain.AssistantTurnContext{Route: domain.AssistantRouteLibrary, SelectedVideoIDs: []int64{7}},
		StartedAt: time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC),
	})
	if f.turns.finished == nil {
		t.Fatal("the turn was not finished")
	}
	return *f.turns.finished
}

// modelFunctionName is the function name rule providers share.
var modelFunctionName = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

func TestWriteCallsBecomeOneProposalWithTheCreatedTagFromStepOne(t *testing.T) {
	f := newWriteFixture(t,
		calls(toolCall("createTag", `{"name":"新曲"}`),
			toolCall("updateVideoTags__add", `{"target":{"kind":"selection"},"tagNames":["新曲"]}`)),
		text("提案しました。"))

	turn := f.run(t, 0)

	if turn.Status != domain.AssistantTurnDone || turn.ProposalID != 1 || len(f.store.plans) != 2 {
		t.Fatalf("turn = %+v, plans = %+v; want one proposal of two steps", turn, f.store.plans)
	}
	offered := f.chat.toolNames(1)
	if !slices.Contains(offered, "updateVideoTags__add") {
		t.Fatalf("offered tools = %v, want updateVideoTags.add as updateVideoTags__add", offered)
	}
	for _, name := range offered {
		if !modelFunctionName.MatchString(name) {
			t.Errorf("offered tool %q is not a valid function name", name)
		}
	}
	attach := f.store.plans[1]
	if attach.ActionID != "updateVideoTags.add" || turn.Progress[2].Tool != "updateVideoTags.add" {
		t.Fatalf("step action = %q, progress tool = %q; want the row id", attach.ActionID, turn.Progress[2].Tool)
	}
	if !slices.Equal(attach.CreatedBy, []domain.AssistantCreatedBy{{Name: "新曲", Step: 0}}) || len(attach.Created) != 0 {
		t.Fatalf("second step createdBy = %+v, created = %v; want created by step 1", attach.CreatedBy, attach.Created)
	}
	if tag, _ := attach.Call.Body["tag"].(map[string]any); tag["name"] != "新曲" || !slices.Equal(attach.Targets.VideoKeys, []string{"key-7"}) {
		t.Fatalf("second step call = %+v, targets = %+v", attach.Call, attach.Targets)
	}
	if len(f.runner.runs) != 0 {
		t.Fatalf("write calls reached the runner before approval: %+v", f.runner.runs)
	}
}

// A created name that ends in the tag word is the earlier step's tag, not a new tag without the word.
func TestWriteCallKeepsTheTagWordOfANameAnEarlierStepCreates(t *testing.T) {
	f := newWriteFixture(t,
		calls(toolCall("createTag", `{"name":"新曲タグ"}`),
			toolCall("updateVideoTags__add", `{"target":{"kind":"selection"},"tagNames":["新曲タグ"]}`)),
		text("提案しました。"))

	f.run(t, 0)

	attach := f.store.plans[1]
	if !slices.Equal(attach.CreatedBy, []domain.AssistantCreatedBy{{Name: "新曲タグ", Step: 0}}) || len(attach.Created) != 0 {
		t.Fatalf("second step createdBy = %+v, created = %v; want created by step 1", attach.CreatedBy, attach.Created)
	}
}

func TestWriteCallWithANameOfTwoTagsEndsTheTurnAsClarify(t *testing.T) {
	f := newWriteFixture(t,
		calls(toolCall("createTag", `{"name":"新曲"}`),
			toolCall("updateVideoTags.add", `{"target":{"kind":"selection"},"tagNames":["猫"]}`)))

	turn := f.run(t, 0)

	want := domain.AssistantTurnClarification{Name: "猫", Kind: domain.AssistantLabelTag, Candidates: catTags}
	if turn.Status != domain.AssistantTurnClarify || turn.Clarify == nil || turn.Clarify.Name != want.Name ||
		turn.Clarify.Kind != want.Kind || !slices.Equal(turn.Clarify.Candidates, want.Candidates) {
		t.Fatalf("turn = %+v, clarify = %+v; want clarify on both tags", turn, turn.Clarify)
	}
	if turn.ProposalID != 0 || f.store.plans != nil {
		t.Fatalf("a clarified turn proposed %+v", f.store.plans)
	}
}

// followUpStore has no earlier proposal to describe, so only the discard is observed.
type followUpStore struct{ *fakeAssistantStore }

func (followUpStore) Proposal(context.Context, int64) (domain.AssistantProposal, error) {
	return domain.AssistantProposal{}, domain.ErrNotFound
}

func TestFollowUpProposalDiscardsThePreviousPendingOne(t *testing.T) {
	f := newWriteFixture(t, calls(toolCall("createTag", `{"name":"新曲"}`)), text("提案しました。"))
	f.a.store = followUpStore{f.store}
	f.turns.turns[4] = domain.AssistantTurn{ID: 4, Request: "猫のタグを作って", ProposalID: 9}

	turn := f.run(t, 4)

	if turn.ProposalID != 1 || !slices.Equal(f.store.discarded, []int64{9}) {
		t.Fatalf("proposal = %d, discarded = %v; want the new proposal and the previous one discarded", turn.ProposalID, f.store.discarded)
	}
}

func TestMergeTagStepIsIrreversibleWithItsImpact(t *testing.T) {
	f := newWriteFixture(t,
		calls(toolCall("mergeTag", `{"target":{"kind":"named","names":["コンサート"]},"names":["ライブ"]}`)),
		text("提案しました。"))

	f.run(t, 0)

	if len(f.store.plans) != 1 {
		t.Fatalf("plans = %+v", f.store.plans)
	}
	step := domain.NewAssistantSteps(f.store.plans)[0]
	if !step.Irreversible() || step.Plan.Impact == nil || step.Plan.Impact.Videos != 42 ||
		!slices.Equal(f.store.impactIDs, []int64{liveTag.ID}) || step.Plan.Call.Path["id"] != "4" {
		t.Fatalf("merge step = %+v, impact ids = %v", step.Plan, f.store.impactIDs)
	}
}

func TestApplyWorkInfoFetchStepListsTheFetchedFieldsItSets(t *testing.T) {
	f := newWriteFixture(t,
		calls(toolCall("fetchWorkInfo", `{"id":7,"body":{"pluginId":1,"id":"ABC-1"}}`)),
		calls(toolCall("applyWorkInfoFetch",
			`{"target":{"kind":"selection"},"fetchId":"f1","fields":["title","tags","dates","people.performer"]}`)),
		text("提案しました。"))
	f.runner.bodies["fetchWorkInfo"] = `{"fetchId":"f1","title":"New title","tags":["a","b"],` +
		`"dates":[{"kind":"release","date":"2024-01-02"}],"people":[{"role":"director","name":"D"},` +
		`{"role":"performer","name":"P"}],"organizations":[],"urls":[],"externalIds":[],"covers":[]}`

	f.run(t, 0)

	if len(f.store.plans) != 1 {
		t.Fatalf("plans = %+v", f.store.plans)
	}
	plan := f.store.plans[0]
	fields, _ := plan.Call.Body["fields"].([]string)
	if !slices.Equal(fields, []string{"title", "tags", "dates.release"}) || plan.Call.Path["fetchId"] != "f1" ||
		!slices.Equal(plan.Call.Body["people"].([]int), []int{1}) {
		t.Fatalf("call = %+v", plan.Call)
	}
	var shown []string
	for _, change := range plan.Change {
		shown = append(shown, string(change.Argument)+"="+string(change.Value))
	}
	want := []string{`title="New title"`, `tags=["a","b"]`, `dates.release="2024-01-02"`, `people=["P"]`}
	if !slices.Equal(shown, want) {
		t.Fatalf("card = %v, want %v", shown, want)
	}
}

func TestMoveFaceAppearancesStepLabelsBothPeopleAndCountsTheAppearances(t *testing.T) {
	f := newWriteFixture(t,
		calls(toolCall("moveFaceAppearances", `{"id":11,"body":{"targetId":12,"appearanceIds":[101,102]}}`)),
		text("提案しました。"))

	f.run(t, 0)

	if len(f.store.plans) != 1 {
		t.Fatalf("plans = %+v", f.store.plans)
	}
	plan := f.store.plans[0]
	if !slices.Equal(plan.Targets.Labels, []domain.AssistantLabel{{Kind: domain.AssistantLabelPerson, ID: 11, Name: "Aさん"}}) {
		t.Fatalf("targets = %+v, want the person the path id names", plan.Targets.Labels)
	}
	byArgument := map[domain.AssistantArgument]domain.AssistantChange{}
	for _, change := range plan.Change {
		byArgument[change.Argument] = change
	}
	target, appearances := byArgument["targetId"], byArgument["appearanceIds"]
	if !slices.Equal(target.Labels, []domain.AssistantLabel{{Kind: domain.AssistantLabelPerson, ID: 12, Name: "Bさん"}}) {
		t.Fatalf("targetId = %+v, want the person it names", target)
	}
	if string(appearances.Value) != "2" || len(appearances.Labels) != 1 || appearances.Labels[0].ID != 101 {
		t.Fatalf("appearanceIds = %+v, want the count and the first appearance", appearances)
	}
	if len(plan.Change) != 2 || !domain.NewAssistantSteps(f.store.plans)[0].Irreversible() {
		t.Fatalf("change = %+v; want only the two id fields, marked irreversible", plan.Change)
	}
}

func TestVideoSchemaStepSendsTheIDItsUserKeyHasAtExecution(t *testing.T) {
	f := newWriteFixture(t,
		calls(toolCall("setVideoDetails", `{"id":7,"body":{"title":"New"}}`)), text("提案しました。"))
	f.run(t, 0)
	if len(f.store.plans) != 1 {
		t.Fatalf("plans = %+v", f.store.plans)
	}
	plan := f.store.plans[0]

	t.Run("a rescan gave the video a new id", func(t *testing.T) {
		store := &fakeAssistantStore{current: domain.AssistantCurrent{Videos: map[string]int64{"key-7": 70}}}
		runner := &fakeRunner{}
		newTestAssistant(store).execute(context.Background(), runner, runningProposal(plan))
		if len(runner.calls) != 1 || runner.calls[0].Path["id"] != "70" || runner.calls[0].Body["title"] != "New" {
			t.Fatalf("calls = %+v, want one call on the new id 70", runner.calls)
		}
	})
	t.Run("the video is gone", func(t *testing.T) {
		store := &fakeAssistantStore{current: domain.AssistantCurrent{Videos: map[string]int64{}}}
		runner := &fakeRunner{}
		newTestAssistant(store).execute(context.Background(), runner, runningProposal(plan))
		got := finishedStatuses(t, store)
		if len(runner.calls) != 0 || !slices.Equal(got, []domain.AssistantStepStatus{domain.AssistantStepStale}) {
			t.Fatalf("calls = %+v, steps = %v; want no call and a stale step", runner.calls, got)
		}
	})
}

func TestBindCreatedKeepsANullBodyField(t *testing.T) {
	plan, failure, _ := bindCreated(domain.AssistantPlan{
		ActionID: "setVideoThumbnailPosition", Call: domain.AssistantPlanCall{Body: map[string]any{"positionMs": nil}},
	})
	if value, ok := plan.Call.Body["positionMs"]; failure != "" || !ok || value != nil {
		t.Fatalf("body = %+v, failure = %q; want positionMs kept as null", plan.Call.Body, failure)
	}
}

func TestVideoDetailsStepShowsTheWholeObjectANestedIDSitsIn(t *testing.T) {
	f := newWriteFixture(t, calls(toolCall("setVideoDetails",
		`{"id":7,"body":{"workInfo":{"description":"rewritten","people":[{"role":"performer","person":{"id":11}}]}}}`)),
		text("提案しました。"))

	f.run(t, 0)

	if len(f.store.plans) != 1 {
		t.Fatalf("plans = %+v", f.store.plans)
	}
	byArgument := map[domain.AssistantArgument]domain.AssistantChange{}
	for _, change := range f.store.plans[0].Change {
		byArgument[change.Argument] = change
	}
	person := byArgument["workInfo/people/*/person/id"]
	if len(person.Labels) != 1 || person.Labels[0].Name != "Aさん" {
		t.Fatalf("change = %+v, want the person the nested id names", f.store.plans[0].Change)
	}
	var shown map[string]any
	if err := json.Unmarshal(byArgument["workInfo"].Value, &shown); err != nil || shown["description"] != "rewritten" {
		t.Fatalf("workInfo on the card = %s, want the whole object with its description", byArgument["workInfo"].Value)
	}
}
