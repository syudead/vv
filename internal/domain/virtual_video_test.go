package domain

import (
	"errors"
	"strings"
	"testing"
)

// An empty title is ErrInvalidVirtualTitle even when it holds only control whitespace; otherwise the display-name
// rules apply.
func TestNormalizeVirtualTitle(t *testing.T) {
	for _, tc := range []struct {
		name, input, want string
		err               error
	}{
		{"trimmed", "  Moonlight ", "Moonlight", nil},
		{"whitespace only", " \t\n", "", ErrInvalidVirtualTitle},
		{"control character", "Moon\x01light", "", ErrInvalidDisplayName},
		{"too long", strings.Repeat("a", DisplayNameMaxLength+1), "", ErrInvalidDisplayName},
	} {
		got, err := NormalizeVirtualTitle(tc.input)
		if got != tc.want || !errors.Is(err, tc.err) {
			t.Errorf("%s: NormalizeVirtualTitle = %q, %v, want %q, %v", tc.name, got, err, tc.want, tc.err)
		}
	}
}
