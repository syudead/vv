package domain

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestAssistantUngroundedListsNumbersAndNamesWithoutEvidence(t *testing.T) {
	evidence := `{"total":1234,"items":[{"name":"Karaoke"}]} request 2026-10-30`
	got := AssistantUngrounded("There are 1,234 Karaoke videos, 56 of them live, and 7 new, since 2026-10-30; Jazz too.",
		evidence, []string{"karaoke", "jazz"})
	if want := []string{"56", "jazz"}; !slices.Equal(got, want) {
		t.Fatalf("ungrounded = %v, want %v", got, want)
	}
}

func TestAssistantUngroundedCountsANameOnlyAsItsOwnToken(t *testing.T) {
	tests := []struct {
		name, answer string
		names, want  []string
	}{
		{"katakana inside a longer katakana word", "ライブラリにメディアフォルダーを追加します。", []string{"ライブ"}, nil},
		{"katakana before kanji", "ライブの動画はありません。", []string{"ライブ"}, []string{"ライブ"}},
		{"half-width katakana before the long-vowel mark", "ﾗｲﾌﾞｰﾙを開きます。", []string{"ライブ"}, nil},
		{"kanji inside a longer kanji word", "東京都庁の動画です。", []string{"東京都"}, nil},
		{"latin inside longer words", "Open the Library for lively clips.", []string{"lib", "live"}, nil},
		{"latin words", "Lib and LIVE clips.", []string{"lib", "live"}, []string{"lib", "live"}},
		{"invented full-width count", "ライブラリには５６本あります。", []string{"ライブ"}, []string{"56"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AssistantUngrounded(tt.answer, "request 2026-10-30", tt.names); !slices.Equal(got, tt.want) {
				t.Fatalf("ungrounded = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTrimAssistantToolResultMarksEachCut(t *testing.T) {
	items := make([]int, AssistantResultItemMax+5)
	body, err := json.Marshal(map[string]any{"items": items, "title": strings.Repeat("あ", AssistantResultTextMax+1)})
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Items []any  `json:"items"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal([]byte(TrimAssistantToolResult(body)), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != AssistantResultItemMax+1 || got.Items[AssistantResultItemMax] != "[cut: 55 items in all]" {
		t.Fatalf("items = %d, last = %v", len(got.Items), got.Items[len(got.Items)-1])
	}
	if !strings.HasSuffix(got.Title, "…[cut: 501 characters in all]") {
		t.Fatalf("title ends %q", got.Title[len(got.Title)-40:])
	}
}

func TestTrimAssistantToolResultCutsTheWholeResult(t *testing.T) {
	items := slices.Repeat([]string{strings.Repeat("x", 480)}, 50)
	body, err := json.Marshal(map[string]any{"a": items, "b": items})
	if err != nil {
		t.Fatal(err)
	}
	got := TrimAssistantToolResult(body)
	if !strings.HasSuffix(got, "bytes in all]") || len(got) > AssistantResultBytesMax+40 {
		t.Fatalf("result of %d bytes ends %q", len(got), got[len(got)-30:])
	}
}
