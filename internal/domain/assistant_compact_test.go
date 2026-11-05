package domain

import "testing"

func TestAssistantCompactResultKeepsTheFieldsTheModelUses(t *testing.T) {
	cases := []struct {
		name, operation, body, want string
	}{
		{
			name:      "video page",
			operation: "listVideos",
			body: `{"total":305,"nextCursor":"c2","items":[{"id":7,"title":"Clip","durationMs":20000,` +
				`"thumbnailUrl":"/t/7","videoCodec":"h264","tags":[{"id":3,"name":"カラオケ","manual":true}],` +
				`"cardCredits":{"performers":[{"id":1,"name":"Aki","role":"cast"}]},"workInfo":null}]}`,
			want: `{"items":[{"cardCredits":{"performers":["Aki"]},"durationMs":20000,"id":7,"tags":["カラオケ"],` +
				`"title":"Clip"}],"nextCursor":"c2","total":305}`,
		},
		{
			name:      "other operation",
			operation: "getVideo",
			body:      `{"id":7,"thumbnailUrl":"/t/7"}`,
			want:      `{"id":7,"thumbnailUrl":"/t/7"}`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := string(AssistantCompactResult(c.operation, []byte(c.body))); got != c.want {
				t.Fatalf("got %s, want %s", got, c.want)
			}
		})
	}
}
