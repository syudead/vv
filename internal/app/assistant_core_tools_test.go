package app

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// toolResult decodes the tool message that answered the call-th tool call into out.
func toolResult(t *testing.T, chat *scriptedChat, call int, out any) string {
	t.Helper()
	var results []string
	for _, m := range chat.requests[len(chat.requests)-1].Messages {
		if m.Role == domain.ChatRoleTool {
			results = append(results, m.Content)
		}
	}
	if call >= len(results) {
		t.Fatalf("tool results = %v, want at least %d", results, call+1)
	}
	if out != nil {
		if err := json.Unmarshal([]byte(results[call]), out); err != nil {
			t.Fatalf("result %d = %s: %v", call, results[call], err)
		}
	}
	return results[call]
}

// registryRunner offers its own registry in place of the loop tests' one.
type registryRunner struct {
	*fakeToolRunner
	tools []domain.AssistantTool
}

func (r registryRunner) Tools() []domain.AssistantTool { return r.tools }

func TestScreenContextAnswersTheSelectedVideosAndTheFilter(t *testing.T) {
	f := newLoopFixture(t, calls(toolCall("screen_context", `{}`)), text("Clip 7 と Clip 8 を選んでいます。"))
	f.a.videos = &fakeLibraryVideos{videos: map[int64]domain.Video{
		7: {ID: 7, Title: "Clip 7"}, 8: {ID: 8, Title: "Clip 8"},
	}}
	f.a.vocabulary = fakeVocabulary{entities: map[domain.AssistantLabelKind]map[int64]string{
		domain.AssistantLabelTag: {3: "Karaoke"},
	}}
	filter := domain.SmartRule{Tags: []int64{3}, VideoConditions: domain.VideoConditions{WatchState: domain.WatchStateUnwatched}}
	in := domain.AssistantTurnInput{Text: "選んだ動画は?", Context: domain.AssistantTurnContext{
		Route: domain.AssistantRouteLibrary, SelectedVideoIDs: []int64{7, 8}, Filter: &filter,
	}}

	turn := f.run(t, in)

	var got screenResult
	toolResult(t, f.chat, 0, &got)
	if !slices.Equal(got.SelectedVideos, []screenEntity{{ID: 7, Name: "Clip 7"}, {ID: 8, Name: "Clip 8"}}) ||
		got.SelectedVideoCount != 2 {
		t.Errorf("selected videos = %+v (%d)", got.SelectedVideos, got.SelectedVideoCount)
	}
	if got.Filter == nil || !slices.Equal(got.Filter.Rule.Tags, []int64{3}) ||
		got.Filter.Rule.WatchState != domain.WatchStateUnwatched ||
		!slices.Equal(got.Filter.Labels, []domain.AssistantLabel{{Kind: domain.AssistantLabelTag, ID: 3, Name: "Karaoke"}}) {
		t.Errorf("filter = %+v", got.Filter)
	}
	if turn.Status != domain.AssistantTurnDone {
		t.Errorf("turn = %+v", turn)
	}
}

func TestAppHelpAnswersTheFavoriteOperationsAndTheLibraryPage(t *testing.T) {
	f := newLoopFixture(t,
		calls(toolCall("app_help", `{"query":"お気に入り"}`)),
		calls(toolCall("app_help", `{"query":"how to use favorites"}`)),
		text("ライブラリで動画を選んで Add to favorites を押します。"))
	registry := append(slices.Clone(loopRegistry), domain.AssistantTool{
		Name: "updateFavorites", Kind: domain.AssistantToolWrite, Operation: "updateFavorites",
		Summary: "Set or clear favorites on videos and groups",
	})
	in := domain.AssistantTurnInput{Text: "お気に入りの使い方を教えて", Context: domain.AssistantTurnContext{Route: domain.AssistantRouteLibrary}}

	f.a.runTurn(context.Background(), registryRunner{f.runner, registry}, domain.AssistantTurn{
		ID: 1, Status: domain.AssistantTurnRunning, Request: in.Text, Context: in.Context, StartedAt: f.now,
	})

	var unmatched, help helpResult
	toolResult(t, f.chat, 0, &unmatched)
	if unmatched.Note == "" || !slices.ContainsFunc(unmatched.AllPages, func(p helpPage) bool { return p.Title == "Library" }) {
		t.Errorf("a query that matches nothing = %+v, want the page titles to search again with", unmatched)
	}
	toolResult(t, f.chat, 1, &help)
	if len(help.Pages) == 0 || help.Pages[0].Route != "/" || !strings.Contains(help.Pages[0].Purpose, "Add to favorites") {
		t.Errorf("pages = %+v, want the library first", help.Pages)
	}
	if !slices.ContainsFunc(help.Operations, func(op helpOperation) bool { return op.Name == "updateFavorites" }) {
		t.Errorf("operations = %+v, want updateFavorites", help.Operations)
	}
	if turn := f.turns.finished; turn.Status != domain.AssistantTurnDone || turn.ProposalID != 0 || turn.Directive != nil {
		t.Errorf("turn = %+v, want an answer and nothing else", turn)
	}
}

// The call also writes an argument it leaves out as null and a boolean and an integer as strings, as chat models do.
func TestShowInLibraryPutsTheResolvedFilterOnTheTurn(t *testing.T) {
	f := newLoopFixture(t, calls(toolCall("show_in_library",
		`{"minDurationMs":"1800000","addedFrom":"2026-09-01","addedTo":"2026-09-30","watchState":"unwatched","q":null,`+
			`"favorite":"true"}`)),
		text("先月追加した30分以上の未視聴は 4 本です。"))
	videos := &fakeLibraryVideos{total: 4}
	f.a.videos = videos

	turn := f.run(t, request)

	want := domain.VideoConditions{MinDurationMs: new(int64(1800000)), AddedFrom: "2026-09-01", AddedTo: "2026-09-30",
		WatchState: domain.WatchStateUnwatched}
	d := turn.Directive
	if d == nil || d.Kind != "filter" || d.Criteria == nil || !sameConditions(d.Criteria.Rule.VideoConditions, want) ||
		!d.Criteria.Rule.Favorite {
		t.Fatalf("directive = %+v, want the filter with %+v", d, want)
	}
	if len(videos.queries) != 1 || !sameConditions(videos.queries[0].VideoConditions, want) {
		t.Errorf("count queries = %+v", videos.queries)
	}
	var got struct {
		Total int `json:"total"`
	}
	if toolResult(t, f.chat, 0, &got); got.Total != 4 {
		t.Errorf("total = %d, want the library's 4", got.Total)
	}
	if turn.Status != domain.AssistantTurnDone {
		t.Errorf("turn = %+v", turn)
	}
}

func sameConditions(a, b domain.VideoConditions) bool {
	bound := func(p *int64) int64 {
		if p == nil {
			return -1
		}
		return *p
	}
	return bound(a.MinDurationMs) == bound(b.MinDurationMs) && bound(a.MaxDurationMs) == bound(b.MaxDurationMs) &&
		a.AddedFrom == b.AddedFrom && a.AddedTo == b.AddedTo && a.WatchState == b.WatchState
}

func TestBuiltinToolRefusesArgumentsOutsideItsSchema(t *testing.T) {
	f := newLoopFixture(t, calls(toolCall("show_in_library", `{"watchState":"never"}`)), text("できませんでした。"))

	turn := f.run(t, request)

	if result := toolResult(t, f.chat, 0, nil); !strings.Contains(result, `"error"`) || !strings.Contains(result, "watchState") {
		t.Errorf("result = %s, want an error naming watchState", result)
	}
	if turn.Directive != nil || turn.Progress[0].Error == "" {
		t.Errorf("turn = %+v, want no directive and a failed step", turn)
	}
}

func TestOpenPageReturnsAnAmbiguousNameToTheModel(t *testing.T) {
	f := newLoopFixture(t,
		calls(toolCall("open_page", `{"to":"person","name":"Aoi"}`)),
		calls(toolCall("open_page", `{"to":"person","name":"Aoi Sato"}`)),
		text("Aoi Sato さんのページを開きます。"))
	person := domain.AssistantLabelPerson
	f.a.vocabulary = fakeVocabulary{
		names: map[domain.AssistantLabelKind]map[string][]domain.AssistantCandidate{person: {
			"Aoi":      {{Kind: person, ID: 1, Name: "Aoi Sato"}, {Kind: person, ID: 2, Name: "Aoi Mori"}},
			"Aoi Sato": {{Kind: person, ID: 1, Name: "Aoi Sato"}},
		}},
		entities: map[domain.AssistantLabelKind]map[int64]string{person: {1: "Aoi Sato"}},
	}

	turn := f.run(t, request)

	if ambiguous := toolResult(t, f.chat, 0, nil); !strings.Contains(ambiguous, "Aoi Sato") || !strings.Contains(ambiguous, "Aoi Mori") {
		t.Errorf("first result = %s, want both candidates", ambiguous)
	}
	d := turn.Directive
	if d == nil || d.Kind != "navigate" || d.Destination == nil || *d.Destination != (domain.AssistantDestination{Kind: "person", ID: 1}) {
		t.Fatalf("directive = %+v, want the person 1", d)
	}
	if labels := turn.Progress[1].Labels; len(labels) != 1 || labels[0].Name != "Aoi Sato" {
		t.Errorf("progress labels = %+v", labels)
	}
}

func TestStartReviewStartsOneRun(t *testing.T) {
	f := newLoopFixture(t, calls(toolCall("start_review", `{"kind":"tags"}`)), text("タグのレビューを始めました。"))
	reviews := &fakeAssistantReviews{}
	f.a.reviews = reviews
	in := domain.AssistantTurnInput{Text: "タグを整理して", Context: domain.AssistantTurnContext{Route: domain.AssistantRouteLibrary}}

	turn := f.run(t, in)

	if len(reviews.started) != 1 || reviews.started[0].Kind != domain.AssistantReviewTags ||
		reviews.started[0].Request != "タグを整理して" {
		t.Fatalf("started = %+v, want one tags run for the request", reviews.started)
	}
	var got struct {
		ID int64 `json:"id"`
	}
	if toolResult(t, f.chat, 0, &got); got.ID != 5 || turn.Progress[0].ReviewRunID != 5 {
		t.Errorf("result id = %d, progress = %+v; want the run 5", got.ID, turn.Progress[0])
	}
}

func TestTurnLoopAnswersAnExcludedRequestWithItsPage(t *testing.T) {
	cases := []struct {
		name    string
		replies []domain.ChatReply
		want    *domain.AssistantExcludedRef
	}{
		{"nothing else ran", []domain.ChatReply{
			calls(toolCall("find_tools", `{"query":"create api token"}`)), text("API トークンは設定で作れます。"),
		}, &domain.AssistantExcludedRef{Operation: "createApiToken", Reason: "out of scope", Page: "/settings/api"}},
		{"a read ran after it", []domain.ChatReply{
			calls(toolCall("find_tools", `{"query":"create api token"}`)), calls(toolCall("listTags", `{}`)),
			text("タグはありません。"),
		}, nil},
		// createApiToken and listPlaylists both match "playlists" once, so neither is what the request asked for.
		{"the excluded tool only ties", []domain.ChatReply{
			calls(toolCall("find_tools", `{"query":"playlists"}`)), text("API トークンは作れません。"),
		}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newLoopFixture(t, tc.replies...)

			turn := f.run(t, domain.AssistantTurnInput{Text: "API トークンを作って"})

			var found struct {
				Tools []foundTool `json:"tools"`
			}
			if toolResult(t, f.chat, 0, &found); len(found.Tools) == 0 || found.Tools[0].Name != "createApiToken" ||
				found.Tools[0].Page != "/settings/api" || found.Tools[0].Reason == "" {
				t.Fatalf("found = %+v, want createApiToken excluded with its reason and page", found.Tools)
			}
			if tc.want == nil {
				if turn.Status != domain.AssistantTurnDone || turn.Stop != nil {
					t.Errorf("turn = %+v, want the answer", turn)
				}
				return
			}
			if turn.Status != domain.AssistantTurnStopped || turn.Stop == nil || turn.Stop.Excluded == nil ||
				*turn.Stop.Excluded != *tc.want || turn.Answer != "" {
				t.Errorf("turn = %+v, want stopped with %+v", turn, tc.want)
			}
		})
	}
}
