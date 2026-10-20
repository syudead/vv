package app

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// Test fetch/apply wiring; store and domain tests cover matching and merging.

// Record matches and return configured answers; unknown names default to new.
type fakeCreditMatcher struct {
	answers map[string]domain.CreditMatch
	calls   []domain.CreditKind
	names   [][]string
	err     error
}

func (m *fakeCreditMatcher) Match(
	_ context.Context, kind domain.CreditKind, entries []domain.CreditMatchEntry,
) ([]domain.CreditMatch, error) {
	m.calls = append(m.calls, kind)
	names := make([]string, 0, len(entries))
	out := make([]domain.CreditMatch, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name)
		answer, ok := m.answers[entry.Name]
		if !ok {
			answer = domain.CreditMatch{State: domain.CreditMatchNew}
		}
		out = append(out, answer)
	}
	m.names = append(m.names, names)
	return out, m.err
}

func (e *fetchEnv) fetchDetails(t *testing.T, info domain.FetchedWorkInfo) domain.WorkInfoFetchResult {
	t.Helper()
	e.runtime.result = domain.WorkInfoRunResult{WorkInfo: info}
	e.runtime.err = nil
	result, err := e.plugins.FetchWorkInfo(context.Background(), fetchVideo, 1, "W-1")
	if err != nil {
		t.Fatalf("FetchWorkInfo: %v", err)
	}
	return result
}

func detailedWork() domain.FetchedWorkInfo {
	return domain.FetchedWorkInfo{
		Title: "Title", Tags: []string{"drama"}, Code: "ABC-123", Description: "About",
		Dates: []domain.WorkDate{
			{Kind: domain.DatePublished, Date: "2024-04"},
			{Kind: domain.DateRelease, Date: "2024-03-01"},
		},
		People: []domain.FetchedPerson{
			{Role: domain.PersonDirector, Name: "Director"},
			{Role: domain.PersonPerformer, Name: "Aoi", ExternalIDs: []domain.ExternalID{{Source: "fanza", Value: "a-1"}}},
			{Role: domain.PersonPerformer, Name: "Twins"},
			{Role: domain.PersonPerformer, Name: "Mika"},
		},
		Organizations: []domain.FetchedOrganization{{Role: domain.OrganizationMaker, Name: "Maker"}},
		URLs:          []string{"https://example.com/works/1"},
		ExternalIDs:   []domain.ExternalID{{Source: "fanza", Value: "abc123"}},
	}
}

func TestFetchWorkInfoCarriesDetailsWithMatches(t *testing.T) {
	e := newFetchEnv()
	aoi := domain.CreditRef{ID: 11, Name: "Aoi Sora"}
	e.matcher.answers = map[string]domain.CreditMatch{
		"Aoi":   {State: domain.CreditMatchExisting, Credit: &aoi},
		"Twins": {State: domain.CreditMatchAmbiguous},
	}
	result := e.fetchDetails(t, detailedWork())

	details := result.Details
	if details.Code != "ABC-123" || details.Description != "About" || details.Title != "" || details.Tags != nil {
		t.Fatalf("details = %+v; want the code and description without the title and tags", details)
	}
	if kinds := []domain.DateKind{details.Dates[0].Kind, details.Dates[1].Kind}; !slices.Equal(kinds,
		[]domain.DateKind{domain.DateRelease, domain.DatePublished}) {
		t.Fatalf("dates = %+v; want kind order", details.Dates)
	}
	var names []string
	for _, p := range details.People {
		names = append(names, p.Name)
	}
	if !slices.Equal(names, []string{"Aoi", "Twins", "Mika", "Director"}) {
		t.Fatalf("people = %v; want role order, fetched order inside a role", names)
	}
	wantPeople := []domain.CreditMatch{
		{State: domain.CreditMatchExisting, Credit: &aoi},
		{State: domain.CreditMatchAmbiguous},
		{State: domain.CreditMatchNew},
		{State: domain.CreditMatchNew},
	}
	if !reflect.DeepEqual(result.PersonMatches, wantPeople) {
		t.Fatalf("person matches = %+v; want %+v", result.PersonMatches, wantPeople)
	}
	if len(result.OrganizationMatches) != 1 || result.OrganizationMatches[0].State != domain.CreditMatchNew {
		t.Fatalf("organization matches = %+v", result.OrganizationMatches)
	}
	if !slices.Equal(e.matcher.calls, []domain.CreditKind{domain.CreditPerson, domain.CreditOrganization}) ||
		!slices.Equal(e.matcher.names[0], names) {
		t.Fatalf("matcher calls = %v %v; want people in the shown order, then organizations", e.matcher.calls, e.matcher.names)
	}

	e.matcher.err = errors.New("database is locked")
	e.runtime.result = domain.WorkInfoRunResult{WorkInfo: detailedWork()}
	if _, err := e.plugins.FetchWorkInfo(context.Background(), fetchVideo, 1, "W-1"); err == nil {
		t.Fatal("a failed match was not reported")
	}
}

// Title/tag-only plugins must not trigger entity matching.
func TestFetchWorkInfoTitleAndTagsOnlyHasNoDetails(t *testing.T) {
	e := newFetchEnv()
	result := e.fetch(t, "Title", "drama")
	if !reflect.DeepEqual(result.Details, domain.FetchedWorkInfo{}) || result.PersonMatches != nil ||
		result.OrganizationMatches != nil {
		t.Fatalf("details = %+v; want none", result)
	}
	if len(e.matcher.calls) != 0 {
		t.Fatal("a fetch without people or organizations read the matches")
	}
}

// Pass the details narrowed to the selection with their fields in comparison order, and propagate storage skips.
func TestApplyWorkInfoFetchPassesTheChosenDetailsAndReportsSkipped(t *testing.T) {
	e := newFetchEnv()
	result := e.fetchDetails(t, detailedWork())
	e.applier.skipped = []domain.SkippedWorkInfoValue{
		{Field: domain.DateField(domain.DateRelease), Value: "2024-03-01", Reason: domain.SkippedWorkInfoInvalid},
	}
	// Both chosen entries are linked to one person.
	e.applier.personIDs = []int64{9, 9}
	release := domain.DateField(domain.DateRelease)
	applied, err := e.plugins.ApplyWorkInfoFetch(context.Background(), fetchVideo, result.ID, domain.WorkInfoSelection{
		Fields: []domain.WorkInfoField{domain.WorkInfoFieldTitle, release, domain.WorkInfoFieldCode},
		People: []int{2, 0}, URLs: result.Details.URLs,
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	call := e.applier.calls[0]
	want := []domain.WorkInfoField{
		domain.WorkInfoFieldCode, release, domain.PersonField(domain.PersonPerformer), domain.WorkInfoFieldURLs,
	}
	if !slices.Equal(call.DetailFields, want) {
		t.Fatalf("detail fields = %v; want %v", call.DetailFields, want)
	}
	chosen := result.Details
	chosen.People = []domain.FetchedPerson{chosen.People[0], chosen.People[2]}
	chosen.Organizations, chosen.ExternalIDs = nil, nil
	if !reflect.DeepEqual(call.Details, chosen) || call.Title == nil || call.Tags != nil {
		t.Fatalf("apply = %+v; want the chosen details and the title", call)
	}
	if !reflect.DeepEqual(applied.Skipped, e.applier.skipped) || applied.SkippedTags == nil ||
		!slices.Equal(applied.LinkedPeople, []int64{9}) {
		t.Fatalf("applied = %+v; want the store's skips and person 9 linked once", applied)
	}

	e.applier.skipped, e.applier.personIDs = nil, nil
	again := e.fetchDetails(t, detailedWork())
	applied, err = e.plugins.ApplyWorkInfoFetch(context.Background(), fetchVideo, again.ID,
		domain.WorkInfoSelection{Fields: []domain.WorkInfoField{domain.WorkInfoFieldTags}}, false)
	if err != nil || e.applier.calls[1].DetailFields != nil || applied.Skipped == nil || len(applied.Skipped) != 0 ||
		applied.LinkedPeople != nil {
		t.Fatalf("tags-only apply = %+v, %v (detail fields %v)", applied, err, e.applier.calls[1].DetailFields)
	}
}

// Selections the result cannot give preserve it without writes.
func TestApplyWorkInfoFetchRefusesASelectionTheResultCannotGive(t *testing.T) {
	e := newFetchEnv()
	work := detailedWork()
	work.Dates = []domain.WorkDate{{Kind: domain.DateRecorded, Date: "2024"}}
	result := e.fetchDetails(t, work)
	code := []domain.WorkInfoField{domain.WorkInfoFieldCode}
	for name, selection := range map[string]domain.WorkInfoSelection{
		"a date without a value":  {Fields: append(code, domain.DateField(domain.DateRelease))},
		"an unknown field":        {Fields: append(code, "dates.unknown")},
		"a role as a whole field": {Fields: append(code, domain.PersonField(domain.PersonPerformer))},
		"a person past the end":   {Fields: code, People: []int{len(work.People)}},
		"an organization twice":   {Fields: code, Organizations: []int{0, 0}},
		"a URL not fetched":       {Fields: code, URLs: []string{"https://example.com/other"}},
	} {
		if _, err := e.plugins.ApplyWorkInfoFetch(context.Background(), fetchVideo, result.ID, selection, false); !errors.Is(err, domain.ErrWorkInfoApplyInvalid) {
			t.Fatalf("%s: err = %v; want ErrWorkInfoApplyInvalid", name, err)
		}
	}
	if len(e.applier.calls) != 0 {
		t.Fatal("a refused apply reached the store")
	}
	every := domain.WorkInfoSelection{
		Fields: []domain.WorkInfoField{
			domain.WorkInfoFieldCode, domain.WorkInfoFieldDescription, domain.DateField(domain.DateRecorded),
		},
		People: []int{0, 1, 2, 3}, Organizations: []int{0}, URLs: work.URLs, ExternalIDs: work.ExternalIDs,
	}
	if _, err := e.plugins.ApplyWorkInfoFetch(context.Background(), fetchVideo, result.ID, every, false); err != nil {
		t.Fatalf("the kept result could not be applied: %v", err)
	}
}
