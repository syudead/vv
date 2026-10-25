package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// fakeStructuredGenerator answers one fixed outcome, or an error, and keeps the request.
type fakeStructuredGenerator struct {
	answer string
	err    error
	got    domain.GenerationRequest
}

func (g *fakeStructuredGenerator) Generate(_ context.Context, req domain.GenerationRequest) (json.RawMessage, error) {
	g.got = req
	return json.RawMessage(g.answer), g.err
}

// fakeVocabulary matches names exactly, per kind.
type fakeVocabulary struct {
	names map[domain.AssistantLabelKind]map[string][]domain.AssistantCandidate
}

func (v fakeVocabulary) FindNames(_ context.Context, kind domain.AssistantLabelKind, name string) ([]domain.AssistantCandidate, error) {
	return v.names[kind][name], nil
}

func (v fakeVocabulary) EntityNames(context.Context, domain.AssistantLabelKind, []int64) (map[int64]string, error) {
	return map[int64]string{}, nil
}

// fakeLibraryVideos returns its videos by id and answers every listing with total.
type fakeLibraryVideos struct {
	videos  map[int64]domain.Video
	total   int
	queries []domain.VideoQuery
}

func (f *fakeLibraryVideos) ListVideos(_ context.Context, _ domain.Audience, q domain.VideoQuery) (domain.VideoPage, error) {
	f.queries = append(f.queries, q)
	return domain.VideoPage{Total: f.total}, nil
}

func (f *fakeLibraryVideos) VideosByIDs(_ context.Context, _ domain.Audience, ids []int64) ([]domain.Video, error) {
	var out []domain.Video
	for _, id := range ids {
		if v, ok := f.videos[id]; ok {
			out = append(out, v)
		}
	}
	return out, nil
}

func (f *fakeAssistantStore) DiscardProposal(_ context.Context, id int64, _ time.Time) error {
	f.discarded = append(f.discarded, id)
	return nil
}

type turnFixture struct {
	store  *fakeAssistantStore
	gen    *fakeStructuredGenerator
	videos *fakeLibraryVideos
	a      *Assistant
}

func newTurnFixture(answer string, tags map[string][]domain.AssistantCandidate) turnFixture {
	f := turnFixture{
		store: &fakeAssistantStore{}, gen: &fakeStructuredGenerator{answer: answer},
		videos: &fakeLibraryVideos{videos: map[int64]domain.Video{
			7: {ID: 7, UserKey: "key-7", Title: "Clip 7"}, 8: {ID: 8, UserKey: "key-8", Title: "Clip 8"},
		}},
	}
	f.a = NewAssistant(AssistantOptions{
		Store: f.store, Generator: f.gen, Videos: f.videos,
		Vocabulary: fakeVocabulary{names: map[domain.AssistantLabelKind]map[string][]domain.AssistantCandidate{
			domain.AssistantLabelTag: tags,
		}},
		Now: func() time.Time { return time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC) },
	})
	return f
}

func selection(ids ...int64) domain.AssistantTurnInput {
	return domain.AssistantTurnInput{Text: "request", Context: domain.AssistantTurnContext{
		Route: domain.AssistantRouteLibrary, SelectedVideoIDs: ids,
	}}
}

func tagAction(operation, tag string) string {
	return fmt.Sprintf(`{"kind":"action","action":{"operation":%q,"target":{"kind":"selection"},`+
		`"arguments":{"tagNames":[%q]}}}`, operation, tag)
}

var catTag = domain.AssistantCandidate{Kind: domain.AssistantLabelTag, ID: 3, Name: "Cat", Count: 12}

func TestTurnAttachesAnExistingTagByID(t *testing.T) {
	f := newTurnFixture(tagAction("updateVideoTags.add", "cat"), map[string][]domain.AssistantCandidate{"cat": {catTag}})
	result, err := f.a.Turn(context.Background(), selection(7, 8))
	if err != nil {
		t.Fatalf("Turn() error = %v", err)
	}
	if result.Kind != domain.AssistantTurnProposal {
		t.Fatalf("kind = %q, clarify = %+v", result.Kind, result.Clarify)
	}
	plan := f.store.created
	if !slices.Equal(plan.Targets.VideoKeys, []string{"key-7", "key-8"}) || plan.Call.VideoField != "videoIds" {
		t.Fatalf("targets = %+v, call = %+v", plan.Targets, plan.Call)
	}
	if tag, _ := plan.Call.Body["tag"].(map[string]any); tag["id"] != int64(3) || len(plan.Created) != 0 {
		t.Fatalf("body = %+v, created = %v", plan.Call.Body, plan.Created)
	}
	if !json.Valid(f.gen.got.Schema) || len(f.gen.got.Schema) == 0 {
		t.Fatal("the generator did not receive the outcome schema")
	}
}

func TestTurnAsksWhenATagNameMatchesTwoTags(t *testing.T) {
	other := domain.AssistantCandidate{Kind: domain.AssistantLabelTag, ID: 4, Name: "CAT", Count: 2}
	f := newTurnFixture(tagAction("updateVideoTags.add", "cat"), map[string][]domain.AssistantCandidate{"cat": {catTag, other}})
	result, err := f.a.Turn(context.Background(), selection(7))
	if err != nil {
		t.Fatalf("Turn() error = %v", err)
	}
	if result.Kind != domain.AssistantTurnClarify || !slices.Equal(result.Clarify.Candidates, []domain.AssistantCandidate{catTag, other}) {
		t.Fatalf("result = %+v", result)
	}
	if f.store.created.ActionID != "" {
		t.Fatal("a clarification stored a proposal")
	}
}

func TestTurnPinnedCandidateSettlesAnAliasThatMatchedTwoTags(t *testing.T) {
	other := domain.AssistantCandidate{Kind: domain.AssistantLabelTag, ID: 4, Name: "Kitten", Count: 2}
	f := newTurnFixture(tagAction("updateVideoTags.add", "neko"), map[string][]domain.AssistantCandidate{"neko": {catTag, other}})
	in := selection(7)
	in.Context.Pinned = []domain.AssistantPin{{Kind: domain.AssistantLabelTag, ID: catTag.ID, Name: catTag.Name}}
	result, err := f.a.Turn(context.Background(), in)
	if err != nil || result.Kind != domain.AssistantTurnProposal {
		t.Fatalf("Turn() = %+v, %v", result, err)
	}
	if tag, _ := f.store.created.Call.Body["tag"].(map[string]any); tag["id"] != catTag.ID {
		t.Fatalf("body = %+v", f.store.created.Call.Body)
	}
}

func TestTurnPinnedPersonSettlesANameATagAlsoHolds(t *testing.T) {
	person := domain.AssistantCandidate{Kind: domain.AssistantLabelPerson, ID: 5, Name: "Mika", Count: 3}
	tag := domain.AssistantCandidate{Kind: domain.AssistantLabelTag, ID: 6, Name: "Mika", Count: 1}
	f := newTurnFixture(`{"kind":"answer","answer":{"question":"appearances","entity":{"kind":"any","name":"mika"}}}`, nil)
	f.a.vocabulary = fakeVocabulary{names: map[domain.AssistantLabelKind]map[string][]domain.AssistantCandidate{
		domain.AssistantLabelPerson: {"mika": {person}},
		domain.AssistantLabelTag:    {"mika": {tag}},
	}}
	in := selection()
	in.Context.Pinned = []domain.AssistantPin{{Kind: domain.AssistantLabelPerson, ID: person.ID, Name: person.Name}}
	result, err := f.a.Turn(context.Background(), in)
	if err != nil || result.Kind != domain.AssistantTurnAnswer {
		t.Fatalf("Turn() = %+v, %v", result, err)
	}
	if q := f.videos.queries[0]; q.Person != person.ID || len(q.TagIDs) != 0 {
		t.Fatalf("query = %+v", q)
	}
}

func TestTurnMarksAMissingTagToAttachAsCreated(t *testing.T) {
	f := newTurnFixture(tagAction("updateVideoTags.add", "sunset"), nil)
	result, err := f.a.Turn(context.Background(), selection(7))
	if err != nil || result.Kind != domain.AssistantTurnProposal {
		t.Fatalf("Turn() = %+v, %v", result, err)
	}
	plan := f.store.created
	if tag, _ := plan.Call.Body["tag"].(map[string]any); tag["name"] != "sunset" || !slices.Equal(plan.Created, []string{"sunset"}) {
		t.Fatalf("body = %+v, created = %v", plan.Call.Body, plan.Created)
	}
}

func TestTurnAsksWhenATagToRemoveIsMissing(t *testing.T) {
	f := newTurnFixture(tagAction("updateVideoTags.remove", "sunset"), nil)
	result, err := f.a.Turn(context.Background(), selection(7))
	if err != nil {
		t.Fatalf("Turn() error = %v", err)
	}
	if result.Kind != domain.AssistantTurnClarify || !slices.Equal(result.Clarify.Names, []string{"sunset"}) {
		t.Fatalf("result = %+v", result)
	}
}

func TestTurnAsksForTheTargetWhenNothingIsSelected(t *testing.T) {
	f := newTurnFixture(tagAction("updateVideoTags.add", "cat"), map[string][]domain.AssistantCandidate{"cat": {catTag}})
	result, err := f.a.Turn(context.Background(), selection())
	if err != nil {
		t.Fatalf("Turn() error = %v", err)
	}
	if result.Kind != domain.AssistantTurnClarify || result.Clarify.About != "target" {
		t.Fatalf("result = %+v", result)
	}
}

func TestTurnAnswersACountWithTheLibraryTotal(t *testing.T) {
	f := newTurnFixture(`{"kind":"answer","answer":{"question":"count","filter":{"tags":["cat"],"maxDurationMs":600000}}}`,
		map[string][]domain.AssistantCandidate{"cat": {catTag}})
	f.videos.total = 42
	result, err := f.a.Turn(context.Background(), selection())
	if err != nil {
		t.Fatalf("Turn() error = %v", err)
	}
	if result.Kind != domain.AssistantTurnAnswer || result.Answer.Total == nil || *result.Answer.Total != 42 {
		t.Fatalf("result = %+v", result)
	}
	q := f.videos.queries[0]
	if !slices.Equal(q.TagIDs, []int64{3}) || q.MaxDurationMs == nil || *q.MaxDurationMs != 600000 {
		t.Fatalf("query = %+v", q)
	}
	if !slices.Equal(result.Answer.Criteria.Rule.Tags, []int64{3}) {
		t.Fatalf("link criteria = %+v", result.Answer.Criteria)
	}
}

func TestTurnFollowUpDiscardsThePreviousProposal(t *testing.T) {
	f := newTurnFixture(tagAction("updateVideoTags.add", "cat"), map[string][]domain.AssistantCandidate{"cat": {catTag}})
	in := selection(7)
	in.Previous, in.PreviousProposalID = json.RawMessage(`{"kind":"proposal"}`), 5
	if _, err := f.a.Turn(context.Background(), in); err != nil {
		t.Fatalf("Turn() error = %v", err)
	}
	if !slices.Equal(f.store.discarded, []int64{5}) {
		t.Fatalf("discarded = %v", f.store.discarded)
	}
}

func TestTurnStoresNothingWhenTheModelTimesOut(t *testing.T) {
	f := newTurnFixture("", nil)
	f.gen.err = fmt.Errorf("model: %w", domain.ErrOllamaTimeout)
	in := selection(7)
	in.PreviousProposalID = 5
	if _, err := f.a.Turn(context.Background(), in); !errors.Is(err, domain.ErrOllamaTimeout) {
		t.Fatalf("Turn() error = %v, want ErrOllamaTimeout", err)
	}
	if f.store.created.ActionID != "" || len(f.store.discarded) > 0 {
		t.Fatalf("stored %+v, discarded %v", f.store.created, f.store.discarded)
	}
}

func TestTurnAnswersUnreadableForJSONOutsideTheSchema(t *testing.T) {
	f := newTurnFixture(`{"kind":"answer","text":"You have 3 videos."}`, nil)
	result, err := f.a.Turn(context.Background(), selection())
	if err != nil || result.Kind != domain.AssistantTurnUnreadable {
		t.Fatalf("Turn() = %+v, %v", result, err)
	}
}
