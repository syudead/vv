package domain

import (
	"strings"
	"testing"
)

func TestGroupNameFromTitle(t *testing.T) {
	for name, c := range map[string]struct {
		title string
		want  string
	}{
		"短い題名はそのまま":          {title: "Episode 1", want: "Episode 1"},
		"前後の空白を削る":           {title: "  Episode 1　", want: "Episode 1"},
		"100 文字に切る":          {title: strings.Repeat("あ", 120), want: strings.Repeat("あ", 100)},
		"長い空白で始まる題名は文字を残す":   {title: strings.Repeat(" ", 120) + "Episode 1", want: "Episode 1"},
		"空白を除いてから 100 文字に切る": {title: "　" + strings.Repeat("あ", 120), want: strings.Repeat("あ", 100)},
	} {
		got, err := GroupNameFromTitle(c.title)
		if err != nil {
			t.Errorf("%s: error = %v", name, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s: got %q, want %q", name, got, c.want)
		}
	}
}
