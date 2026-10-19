package app

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// usePersonPlugin makes plugin 1 an approved person plugin with two sources.
func (e *fetchEnv) usePersonPlugin() {
	e.store.plugins[1].Manifest.Kind = domain.WorkInfoPluginKindPerson
	e.store.plugins[1].Manifest.Sources = []string{"site", "wiki"}
}

func TestSearchPeopleRunsAPersonPluginsSearch(t *testing.T) {
	e := newFetchEnv()
	e.usePersonPlugin()
	e.runtime.result = domain.WorkInfoRunResult{PersonCandidates: []domain.PersonCandidate{
		{Source: "wiki", ID: "p-1", Name: "Person", URL: "https://e.test/p/1"},
	}}
	candidates, err := e.plugins.SearchPeople(context.Background(), 1, "  person  ")
	if err != nil || len(candidates) != 1 || candidates[0].Source != "wiki" || candidates[0].Name != "Person" {
		t.Fatalf("SearchPeople = %+v, %v", candidates, err)
	}
	req := e.runtime.requests[0]
	if req.Operation != domain.WorkInfoSearch || req.Query != "person" || req.Kind != domain.WorkInfoPluginKindPerson ||
		!slices.Equal(req.Sources, []string{"site", "wiki"}) {
		t.Fatalf("request = %+v; want a trimmed person search that checks candidates against the sources", req)
	}
}

func TestRunRefusesAPluginOfTheOtherKind(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name   string
		person bool
		call   func(p *WorkInfoPlugins) error
	}{
		{"work search on a person plugin", true, func(p *WorkInfoPlugins) error {
			_, err := p.SearchWorkInfo(ctx, 1, "work")
			return err
		}},
		{"work fetch on a person plugin", true, func(p *WorkInfoPlugins) error {
			_, err := p.FetchWorkInfo(ctx, fetchVideo, 1, "W-1")
			return err
		}},
		{"person search on a work-info plugin", false, func(p *WorkInfoPlugins) error {
			_, err := p.SearchPeople(ctx, 1, "person")
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newFetchEnv()
			if tc.person {
				e.usePersonPlugin()
			}
			if err := tc.call(e.plugins); !errors.Is(err, domain.ErrWorkInfoPluginKindMismatch) {
				t.Fatalf("err = %v; want ErrWorkInfoPluginKindMismatch", err)
			}
			if len(e.runtime.requests) != 0 || len(e.applier.fetches) != 0 {
				t.Fatal("the module ran or a fetch was recorded")
			}
		})
	}
}
