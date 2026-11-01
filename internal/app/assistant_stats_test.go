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
	want := `{"count":3,"favorites":1,"groupCount":2,"groups":[{"count":3,"name":"歌"}],` +
		`"longest":{"durationMs":20000,"id":1,"title":"A"},"shortest":{"durationMs":10000,"id":2,"title":"B"},` +
		`"totalDurationMs":30000,"withDuration":2}`
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
