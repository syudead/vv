package ollama

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestStatusMessageTruncatesAtARuneBoundary(t *testing.T) {
	got := statusMessage(500, "a"+strings.Repeat("猫", 200))
	want := "ollama answered 500: a" + strings.Repeat("猫", 99)
	if !utf8.ValidString(got) || got != want {
		t.Errorf("statusMessage = %q, want %q", got, want)
	}
}
