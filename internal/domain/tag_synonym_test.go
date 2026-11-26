package domain

import (
	"errors"
	"testing"
)

func TestCheckSynonymOfAcceptsOnlyTheTagsOwnSynonym(t *testing.T) {
	cases := []struct {
		name   string
		holder *TagNameHolder
		want   error
	}{
		{"synonym of the tag", &TagNameHolder{TagID: 1}, nil},
		{"canonical name of the tag", &TagNameHolder{TagID: 1, Canonical: true}, ErrTagSynonymNotFound},
		{"synonym of another tag", &TagNameHolder{TagID: 2}, ErrTagSynonymNotFound},
		{"held by no tag", nil, ErrTagSynonymNotFound},
	}
	for _, tc := range cases {
		if got := CheckSynonymOf(1, tc.holder); !errors.Is(got, tc.want) || (tc.want == nil && got != nil) {
			t.Errorf("%s: CheckSynonymOf() = %v, want %v", tc.name, got, tc.want)
		}
	}
}
