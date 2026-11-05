package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// video_stats reads every page of the screen's filter narrowed by the call, and answers the counts and the top groups
// in one result.
func TestVideoStatsMeasuresEveryPageOfTheScreen(t *testing.T) {
	f := newLoopFixture(t,
		calls(toolCall("video_stats", `{"scope":"screen","minDurationMs":10000,"groupBy":"tag","top":1}`)),
		text("3本です。"))
	f.runner.pages = map[string]string{
		"": `{"nextCursor":"c2","items":[` +
			`{"id":1,"title":"A","durationMs":20000,"favorite":true,"tags":[{"name":"歌"},{"name":"夏"}]},` +
			`{"id":2,"title":"B","durationMs":10000,"tags":[{"name":"歌"}]}]}`,
		"c2": `{"items":[{"id":3,"title":"C","tags":[{"name":"夏"},{"name":"歌"}]}]}`,
	}
	in := request
	in.Context.Filter = &domain.SmartRule{Q: "動画", Favorite: true}

	f.run(t, in)

	wantArgs := []string{
		`{"favorite":true,"limit":200,"minDurationMs":10000,"query":"動画"}`,
		`{"cursor":"c2","favorite":true,"limit":200,"minDurationMs":10000,"query":"動画"}`,
	}
	if len(f.runner.reads) != len(wantArgs) {
		t.Fatalf("reads = %+v, want both pages", f.runner.reads)
	}
	for i, want := range wantArgs {
		if got := string(f.runner.reads[i].Arguments); got != want {
			t.Errorf("read %d = %s, want %s", i, got, want)
		}
	}
	messages := f.chat.requests[1].Messages
	want := `{"conditions":{"favorite":true,"minDurationMs":10000,"query":"動画"},"count":3,"favorites":1,"groupCount":2,"groups":[{"count":3,"name":"歌"}],` +
		`"longest":{"duration":"0:00:20","durationMs":20000,"id":1,"title":"A"},` +
		`"shortest":{"duration":"0:00:10","durationMs":10000,"id":2,"title":"B"},` +
		`"totalDuration":"0:00:30","totalDurationMs":30000,"withDuration":2}`
	if got := messages[len(messages)-1].Content; got != want {
		t.Fatalf("result = %s\nwant     %s", got, want)
	}
}

func TestVideoStatsRefusesGroupsOfASelection(t *testing.T) {
	l := &turnLoop{turn: domain.AssistantTurn{Context: domain.AssistantTurnContext{SelectedVideoIDs: []int64{1}}}}

	out, err := l.videoStats(t.Context(), json.RawMessage(`{"scope":"screen","groupBy":"tag"}`))

	if err != nil || !strings.Contains(out.failure, "takes neither conditions nor groupBy") {
		t.Fatalf("outcome = %+v, %v; want the refusal", out, err)
	}
}

// A selection is measured from the selected videos themselves.
func TestVideoStatsMeasuresTheSelection(t *testing.T) {
	long, short := int64(90000), int64(30000)
	videos := &fakeLibraryVideos{videos: map[int64]domain.Video{
		1: {ID: 1, Title: "A", DurationMs: &long, Favorite: true},
		2: {ID: 2, Title: "B", DurationMs: &short},
		3: {ID: 3, Title: "C"},
	}}
	l := &turnLoop{a: NewAssistant(AssistantOptions{Videos: videos}),
		turn: domain.AssistantTurn{Context: domain.AssistantTurnContext{SelectedVideoIDs: []int64{1, 2, 3}}}}

	out, err := l.videoStats(t.Context(), json.RawMessage(`{"scope":"screen"}`))

	want := `{"count":3,"favorites":1,"longest":{"duration":"0:01:30","durationMs":90000,"id":1,"title":"A"},` +
		`"shortest":{"duration":"0:00:30","durationMs":30000,"id":2,"title":"B"},` +
		`"totalDuration":"0:02:00","totalDurationMs":120000,"withDuration":2}`
	if err != nil || out.content != want {
		t.Fatalf("outcome = %+v, %v\nwant content %s", out, err, want)
	}
}

// On a person's page the screen scope counts that person's videos, and groupBy groups by the cards' credits.
func TestVideoStatsGroupsThePagesVideosByCredits(t *testing.T) {
	runner := &fakeToolRunner{pages: map[string]string{"": `{"items":[` +
		`{"id":1,"title":"A","cardCredits":{"performers":[{"name":"Aki"},{"name":"Mio"}],"organization":{"name":"S1"}}},` +
		`{"id":2,"title":"B","cardCredits":{"performers":[{"name":"Aki"}]}}]}`}}
	l := &turnLoop{a: NewAssistant(AssistantOptions{}), runner: runner, turn: domain.AssistantTurn{Context: domain.AssistantTurnContext{
		Route: domain.AssistantRoutePerson, RouteID: 5, Filter: &domain.SmartRule{Q: "live"},
	}}}
	cases := map[string]string{"person": `[{"count":2,"name":"Aki"},{"count":1,"name":"Mio"}]`, "organization": `[{"count":1,"name":"S1"}]`}
	for groupBy, want := range cases {
		runner.reads = nil

		out, err := l.videoStats(t.Context(), json.RawMessage(`{"scope":"screen","groupBy":"`+groupBy+`"}`))

		var got struct {
			Groups json.RawMessage `json:"groups"`
		}
		if err != nil || json.Unmarshal([]byte(out.content), &got) != nil || string(got.Groups) != want {
			t.Errorf("%s: outcome = %+v, %v; want groups %s", groupBy, out, err, want)
		}
		if args := string(runner.reads[0].Arguments); args != `{"limit":200,"person":5,"query":"live"}` {
			t.Errorf("%s: read %s, want the page's person and the screen's filter", groupBy, args)
		}
	}
}
