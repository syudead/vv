package domain

import (
	"slices"
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

// A result keeps <, > and & as written; a body that is not JSON stays as it is.
func TestAssistantToolResultTextKeepsTheText(t *testing.T) {
	if got := AssistantToolResultText([]byte(`{"title":"A <B> & C"}`)); got != `{"title":"A <B> & C"}` {
		t.Fatalf("result = %s", got)
	}
	if got := AssistantToolResultText([]byte("not json")); got != "not json" {
		t.Fatalf("result = %s", got)
	}
}
