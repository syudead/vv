package app

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

var titleAndTags = []domain.WorkInfoField{domain.WorkInfoFieldTitle, domain.WorkInfoFieldTags}

func TestApplyWorkInfoFetchAttachesOnlyTheSelectedTags(t *testing.T) {
	e := newFetchEnv()
	result := e.fetch(t, "Title", "drama", "comedy", "action")
	applied, err := e.plugins.ApplyWorkInfoFetch(context.Background(), fetchVideo, result.ID, titleAndTags,
		[]string{"action", "drama"})
	if err != nil {
		t.Fatalf("ApplyWorkInfoFetch: %v", err)
	}
	if got := e.applier.tags[fetchVideo]; !slices.Equal(got, []string{"drama", "action"}) {
		t.Fatalf("tags = %v; want drama and action in the result's order", got)
	}
	if len(applied.SkippedTags) != 0 {
		t.Fatalf("skipped = %+v; want none", applied.SkippedTags)
	}
}

// Report skipped tags only within the selected subset.
func TestApplyWorkInfoFetchReportsOnlySelectedSkippedTags(t *testing.T) {
	e := newFetchEnv()
	e.applier.rejected = []string{"spoiler", "gore"}
	result := e.fetch(t, "Title", "drama", "spoiler", "gore", "bad\x01name", "worse\x01name")
	applied, err := e.plugins.ApplyWorkInfoFetch(context.Background(), fetchVideo, result.ID, titleAndTags,
		[]string{"drama", "gore", "worse\x01name"})
	if err != nil {
		t.Fatalf("ApplyWorkInfoFetch: %v", err)
	}
	want := []domain.SkippedTag{{Name: "gore", Reason: domain.SkippedTagRejected}, {Name: "worse\x01name", Reason: domain.SkippedTagInvalid}}
	if !slices.Equal(applied.SkippedTags, want) {
		t.Fatalf("skipped = %+v; want %+v", applied.SkippedTags, want)
	}
	if got := e.applier.tags[fetchVideo]; !slices.Equal(got, []string{"drama"}) {
		t.Fatalf("tags = %v; want drama", got)
	}
}

// Invalid selections preserve the result without writes; omitted tags selects every fetched tag.
func TestApplyWorkInfoFetchRefusesABadTagSelection(t *testing.T) {
	for _, tc := range []struct {
		name   string
		fields []domain.WorkInfoField
		tags   []string
	}{
		{"empty", titleAndTags, []string{}},
		{"a name the result does not have", titleAndTags, []string{"drama", "horror"}},
		{"a name spelled otherwise", titleAndTags, []string{" drama "}},
		{"tags without the tags field", []domain.WorkInfoField{domain.WorkInfoFieldTitle}, []string{"drama"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newFetchEnv()
			result := e.fetch(t, "Title", "drama", "comedy")
			_, err := e.plugins.ApplyWorkInfoFetch(context.Background(), fetchVideo, result.ID, tc.fields, tc.tags)
			if !errors.Is(err, domain.ErrWorkInfoApplyInvalid) {
				t.Fatalf("err = %v; want ErrWorkInfoApplyInvalid", err)
			}
			if len(e.applier.calls) != 0 {
				t.Fatal("a refused apply reached the store")
			}
			if _, err := e.plugins.ApplyWorkInfoFetch(context.Background(), fetchVideo, result.ID, titleAndTags, nil); err != nil {
				t.Fatalf("the result was not kept: %v", err)
			}
			if got := e.applier.tags[fetchVideo]; !slices.Equal(got, []string{"drama", "comedy"}) {
				t.Fatalf("an apply without tags attached %v; want every fetched tag", got)
			}
		})
	}
}
