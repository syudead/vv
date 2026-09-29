package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeAPITokenName(t *testing.T) {
	cases := []struct {
		input   string
		want    string
		problem APITokenNameProblem
	}{
		{input: "Claude Code", want: "Claude Code"},
		{input: "  tagger　", want: "tagger"},
		{input: "a  b", want: "a  b"},
		{input: strings.Repeat("あ", APITokenNameMaxLength), want: strings.Repeat("あ", APITokenNameMaxLength)},
		{input: "", problem: APITokenNameEmpty},
		{input: " 　 ", problem: APITokenNameEmpty},
		{input: "tagger\n", problem: APITokenNameControlCharacters},
		{input: "\ttagger", problem: APITokenNameControlCharacters},
		{input: "a\u0000b", problem: APITokenNameControlCharacters},
		{input: strings.Repeat("a", APITokenNameMaxLength+1), problem: APITokenNameTooLong},
	}
	for _, tc := range cases {
		got, err := NormalizeAPITokenName(tc.input)
		if tc.problem == 0 {
			if err != nil || got != tc.want {
				t.Errorf("NormalizeAPITokenName(%q) = %q, %v, want %q", tc.input, got, err, tc.want)
			}
			continue
		}
		var invalid *InvalidAPITokenNameError
		if !errors.As(err, &invalid) || invalid.Problem != tc.problem {
			t.Errorf("NormalizeAPITokenName(%q) err = %v, want problem %d", tc.input, err, tc.problem)
			continue
		}
		if !errors.Is(err, ErrInvalidAPITokenName) {
			t.Errorf("NormalizeAPITokenName(%q) err が ErrInvalidAPITokenName を包まない", tc.input)
		}
		if err.Error() == "" {
			t.Errorf("NormalizeAPITokenName(%q) の誤りの文が空", tc.input)
		}
	}
}
