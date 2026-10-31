package app

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// fakeVocabulary matches names exactly, per kind, offers similar names from its own table, and reads the current
// names of entities.
type fakeVocabulary struct {
	names    map[domain.AssistantLabelKind]map[string][]domain.AssistantCandidate
	similar  map[domain.AssistantLabelKind]map[string][]domain.AssistantCandidate
	entities map[domain.AssistantLabelKind]map[int64]string
	known    []string
}

func (v fakeVocabulary) FindNames(_ context.Context, kind domain.AssistantLabelKind, name string) ([]domain.AssistantCandidate, error) {
	return v.names[kind][name], nil
}

func (v fakeVocabulary) FindSimilarNames(_ context.Context, kind domain.AssistantLabelKind, name string) ([]domain.AssistantCandidate, error) {
	return v.similar[kind][name], nil
}

func (v fakeVocabulary) EntityNames(_ context.Context, kind domain.AssistantLabelKind, ids []int64) (map[int64]string, error) {
	out := map[int64]string{}
	for _, id := range ids {
		if name, ok := v.entities[kind][id]; ok {
			out[id] = name
		}
	}
	return out, nil
}

// NamesIn finds the library names, kept folded in known, that the folded text contains.
func (v fakeVocabulary) NamesIn(_ context.Context, text string) ([]string, error) {
	var out []string
	for _, name := range v.known {
		if strings.Contains(domain.FoldForMatch(text), name) {
			out = append(out, name)
		}
	}
	return out, nil
}

// fakeGroups returns the member videos of each group.
type fakeGroups map[int64][]int64

func (g fakeGroups) GroupVideoIDs(_ context.Context, ids []int64) ([]int64, error) {
	var out []int64
	for _, id := range ids {
		out = append(out, g[id]...)
	}
	return out, nil
}

// fakeLibraryVideos returns its videos by id and answers every listing with items and total.
type fakeLibraryVideos struct {
	videos  map[int64]domain.Video
	items   []domain.Video
	total   int
	queries []domain.VideoQuery
}

func (f *fakeLibraryVideos) ListVideos(_ context.Context, _ domain.Audience, q domain.VideoQuery) (domain.VideoPage, error) {
	f.queries = append(f.queries, q)
	return domain.VideoPage{Items: f.items, Total: f.total}, nil
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
	store *fakeAssistantStore
	// outcome is what the tool would have resolved, as {"kind": ..., plus the member of that kind}.
	outcome string
	videos  *fakeLibraryVideos
	a       *Assistant
}

func newTurnFixture(outcome string, tags map[string][]domain.AssistantCandidate) turnFixture {
	f := turnFixture{
		store: &fakeAssistantStore{}, outcome: outcome,
		videos: &fakeLibraryVideos{videos: map[int64]domain.Video{
			7: {ID: 7, UserKey: "key-7", Title: "Clip 7"}, 8: {ID: 8, UserKey: "key-8", Title: "Clip 8"},
		}},
	}
	f.a = NewAssistant(AssistantOptions{
		Store: f.store, Videos: f.videos,
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

func TestResolveAttachesAnExistingTagByID(t *testing.T) {
	f := newTurnFixture(tagAction("updateVideoTags.add", "cat"), map[string][]domain.AssistantCandidate{"cat": {catTag}})
	result, err := f.resolve(context.Background(), selection(7, 8))
	if err != nil {
		t.Fatalf("resolve() error = %v", err)
	}
	if result.Kind != domain.AssistantResolvedProposal {
		t.Fatalf("kind = %q, clarify = %+v", result.Kind, result.Clarify)
	}
	plan := f.store.created
	if !slices.Equal(plan.Targets.VideoKeys, []string{"key-7", "key-8"}) || plan.Call.VideoField != "videoIds" {
		t.Fatalf("targets = %+v, call = %+v", plan.Targets, plan.Call)
	}
	if tag, _ := plan.Call.Body["tag"].(map[string]any); tag["id"] != int64(3) || len(plan.Created) != 0 {
		t.Fatalf("body = %+v, created = %v", plan.Call.Body, plan.Created)
	}
}

func TestResolveAsksWhenATagNameMatchesTwoTags(t *testing.T) {
	other := domain.AssistantCandidate{Kind: domain.AssistantLabelTag, ID: 4, Name: "CAT", Count: 2}
	f := newTurnFixture(tagAction("updateVideoTags.add", "cat"), map[string][]domain.AssistantCandidate{"cat": {catTag, other}})
	result, err := f.resolve(context.Background(), selection(7))
	if err != nil {
		t.Fatalf("resolve() error = %v", err)
	}
	if result.Kind != domain.AssistantResolvedClarify || !slices.Equal(result.Clarify.Candidates, []domain.AssistantCandidate{catTag, other}) {
		t.Fatalf("result = %+v", result)
	}
	if f.store.created.ActionID != "" {
		t.Fatal("a clarification stored a proposal")
	}
}

func TestResolvePinnedCandidateSettlesAnAliasThatMatchedTwoTags(t *testing.T) {
	other := domain.AssistantCandidate{Kind: domain.AssistantLabelTag, ID: 4, Name: "Kitten", Count: 2}
	f := newTurnFixture(tagAction("updateVideoTags.add", "neko"), map[string][]domain.AssistantCandidate{"neko": {catTag, other}})
	in := selection(7)
	in.Context.Pinned = []domain.AssistantPin{{Kind: domain.AssistantLabelTag, ID: catTag.ID, Name: catTag.Name}}
	result, err := f.resolve(context.Background(), in)
	if err != nil || result.Kind != domain.AssistantResolvedProposal {
		t.Fatalf("resolve() = %+v, %v", result, err)
	}
	if tag, _ := f.store.created.Call.Body["tag"].(map[string]any); tag["id"] != catTag.ID {
		t.Fatalf("body = %+v", f.store.created.Call.Body)
	}
}

func TestResolveMarksAMissingTagToAttachAsCreated(t *testing.T) {
	f := newTurnFixture(tagAction("updateVideoTags.add", "sunset"), nil)
	result, err := f.resolve(context.Background(), selection(7))
	if err != nil || result.Kind != domain.AssistantResolvedProposal {
		t.Fatalf("resolve() = %+v, %v", result, err)
	}
	plan := f.store.created
	if tag, _ := plan.Call.Body["tag"].(map[string]any); tag["name"] != "sunset" || !slices.Equal(plan.Created, []string{"sunset"}) {
		t.Fatalf("body = %+v, created = %v", plan.Call.Body, plan.Created)
	}
}

// A name that matches nothing exactly offers the entities it is part of, so the owner chooses instead of retyping.
func TestResolveOffersSimilarNamesWhenATargetMatchesNothing(t *testing.T) {
	similar := []domain.AssistantCandidate{
		{Kind: domain.AssistantLabelTag, ID: 4, Name: "コスプレ衣装", Count: 1},
		{Kind: domain.AssistantLabelTag, ID: 5, Name: "コスプレ衣裳", Count: 1},
	}
	f := newTurnFixture(`{"kind":"action","action":{"operation":"deleteTag","target":{"kind":"named","names":["コスプレ"]}}}`, nil)
	f.a.vocabulary = fakeVocabulary{similar: map[domain.AssistantLabelKind]map[string][]domain.AssistantCandidate{
		domain.AssistantLabelTag: {"コスプレ": similar},
	}}
	result, err := f.resolve(context.Background(), domain.AssistantTurnInput{Text: "コスプレのタグを消して"})
	if err != nil {
		t.Fatalf("resolve() error = %v", err)
	}
	if result.Kind != domain.AssistantResolvedClarify || result.Clarify.About != "target" ||
		!slices.Equal(result.Clarify.Candidates, similar) || len(result.Clarify.Names) != 0 {
		t.Fatalf("result = %+v, clarify = %+v", result, result.Clarify)
	}
	if f.store.created.ActionID != "" {
		t.Fatalf("a proposal was stored: %+v", f.store.created)
	}
}

// Choosing a similar candidate re-sends the request with the candidate pinned; the pin settles it even though
// the candidate's name is not the request's name.
func TestResolvePinnedSimilarCandidateSettlesTheTarget(t *testing.T) {
	similar := []domain.AssistantCandidate{
		{Kind: domain.AssistantLabelTag, ID: 4, Name: "コスプレ衣装", Count: 1},
		{Kind: domain.AssistantLabelTag, ID: 5, Name: "コスプレ衣裳", Count: 1},
	}
	f := newTurnFixture(`{"kind":"action","action":{"operation":"deleteTag","target":{"kind":"named","names":["コスプレ"]}}}`, nil)
	f.a.vocabulary = fakeVocabulary{similar: map[domain.AssistantLabelKind]map[string][]domain.AssistantCandidate{
		domain.AssistantLabelTag: {"コスプレ": similar},
	}}
	in := domain.AssistantTurnInput{Text: "コスプレのタグを消して", Context: domain.AssistantTurnContext{
		Pinned: []domain.AssistantPin{{Kind: domain.AssistantLabelTag, Name: "コスプレ衣裳", ID: 5}},
	}}
	result, err := f.resolve(context.Background(), in)
	if err != nil {
		t.Fatalf("resolve() error = %v", err)
	}
	if result.Kind != domain.AssistantResolvedProposal {
		t.Fatalf("kind = %q, clarify = %+v", result.Kind, result.Clarify)
	}
	if labels := f.store.created.Targets.Labels; len(labels) != 1 || labels[0].ID != 5 {
		t.Fatalf("targets = %+v", labels)
	}
}

// A model copies the word "tag" into the name, as in "ライブタグ"; the tag is still found without it.
func TestResolveFindsATagNamedWithTheWordTag(t *testing.T) {
	f := newTurnFixture(tagAction("updateVideoTags.remove", "Catタグ"), map[string][]domain.AssistantCandidate{"Cat": {catTag}})
	result, err := f.resolve(context.Background(), selection(7))
	if err != nil {
		t.Fatalf("resolve() error = %v", err)
	}
	if result.Kind != domain.AssistantResolvedProposal {
		t.Fatalf("kind = %q, clarify = %+v", result.Kind, result.Clarify)
	}
	if change := f.store.created.Change; len(change) != 1 || len(change[0].Labels) != 1 || change[0].Labels[0].ID != 3 {
		t.Fatalf("change = %+v", change)
	}
}

func TestResolveAsksWhenATagToRemoveIsMissing(t *testing.T) {
	f := newTurnFixture(tagAction("updateVideoTags.remove", "sunset"), nil)
	result, err := f.resolve(context.Background(), selection(7))
	if err != nil {
		t.Fatalf("resolve() error = %v", err)
	}
	if result.Kind != domain.AssistantResolvedClarify || !slices.Equal(result.Clarify.Names, []string{"sunset"}) {
		t.Fatalf("result = %+v", result)
	}
}

func TestResolveAsksForTheTargetWhenNothingIsSelected(t *testing.T) {
	f := newTurnFixture(tagAction("updateVideoTags.add", "cat"), map[string][]domain.AssistantCandidate{"cat": {catTag}})
	result, err := f.resolve(context.Background(), selection())
	if err != nil {
		t.Fatalf("resolve() error = %v", err)
	}
	if result.Kind != domain.AssistantResolvedClarify || result.Clarify.About != "target" {
		t.Fatalf("result = %+v", result)
	}
}

func TestResolvePinnedVideoSettlesATitleThatMatchedTwoVideos(t *testing.T) {
	f := newTurnFixture(`{"kind":"navigate","navigate":{"to":"video","name":"clip"}}`, nil)
	f.videos.items, f.videos.total = []domain.Video{f.videos.videos[7], f.videos.videos[8]}, 2
	in := selection()
	in.Context.Pinned = []domain.AssistantPin{{Kind: domain.AssistantLabelVideo, ID: 8, Name: "Clip 8"}}
	result, err := f.resolve(context.Background(), in)
	if err != nil || result.Kind != domain.AssistantResolvedNavigate {
		t.Fatalf("resolve() = %+v, %v", result, err)
	}
	if result.Destination.ID != 8 {
		t.Fatalf("destination = %+v, want video 8", result.Destination)
	}
}

func favoritesFixture() turnFixture {
	f := newTurnFixture(`{"kind":"action","action":{"operation":"updateFavorites","target":{"kind":"selection"},`+
		`"arguments":{"enabled":true}}}`, nil)
	f.a.vocabulary = fakeVocabulary{entities: map[domain.AssistantLabelKind]map[int64]string{
		domain.AssistantLabelGroup: {20: "Pair"}, domain.AssistantLabelSmartFolder: {30: "Cats"},
	}}
	f.a.groups = fakeGroups{20: {8}}
	return f
}

// The library favorites a selected group without its members and a selected smart folder as itself; no smart
// folder port is configured, so expanding the folder would fail the turn.
func TestResolveFavoritesSelectedCollectionsAsThemselves(t *testing.T) {
	f := favoritesFixture()
	in := selection(7, 8)
	in.Context.SelectedGroupIDs, in.Context.SelectedSmartFolderIDs = []int64{20}, []int64{30}
	result, err := f.resolve(context.Background(), in)
	if err != nil || result.Kind != domain.AssistantResolvedProposal {
		t.Fatalf("resolve() = %+v, %v", result, err)
	}
	plan := f.store.created
	if !slices.Equal(plan.Targets.VideoKeys, []string{"key-7"}) {
		t.Fatalf("video keys = %v, want only the standalone video", plan.Targets.VideoKeys)
	}
	groups, _ := plan.Call.Body["groupIds"].([]int64)
	folders, _ := plan.Call.Body["smartFolderIds"].([]int64)
	if !slices.Equal(groups, []int64{20}) || !slices.Equal(folders, []int64{30}) {
		t.Fatalf("body = %+v", plan.Call.Body)
	}
	folder := domain.AssistantLabel{Kind: domain.AssistantLabelSmartFolder, ID: 30, Name: "Cats"}
	if !slices.Contains(plan.Targets.Labels, folder) {
		t.Fatalf("labels = %+v, want %+v", plan.Targets.Labels, folder)
	}
}

func TestResolveFavoritesTheOpenSmartFolderAsItself(t *testing.T) {
	f := favoritesFixture()
	f.outcome = `{"kind":"action","action":{"operation":"updateFavorites","target":{"kind":"current"},` +
		`"arguments":{"enabled":true}}}`
	in := domain.AssistantTurnInput{Text: "request", Context: domain.AssistantTurnContext{
		Route: domain.AssistantRouteSmartFolder, RouteID: 30,
	}}
	result, err := f.resolve(context.Background(), in)
	if err != nil || result.Kind != domain.AssistantResolvedProposal {
		t.Fatalf("resolve() = %+v, %v", result, err)
	}
	plan := f.store.created
	folders, _ := plan.Call.Body["smartFolderIds"].([]int64)
	if len(plan.Targets.VideoKeys) != 0 || !slices.Equal(folders, []int64{30}) {
		t.Fatalf("targets = %+v, body = %+v", plan.Targets, plan.Call.Body)
	}
}
