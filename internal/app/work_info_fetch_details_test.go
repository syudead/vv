package app

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// 取得した作品の詳細の取得と反映の受け渡し（specs/052-plugin-work-info-fields/contracts/screen-api.md、
// research.md R-3・R-4、#1159）。照合と併合そのものは internal/store と internal/domain のテストが確かめる。

// fakeCreditMatcher は名前ごとに決めた答えを返し、受けた件を記録する。決めていない名前は new である。
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
	result, err := e.plugins.FetchWorkInfo(context.Background(), fetchVideo, 1, "https://example.com/works/1")
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

// 受け入れ: 取得の結果は人物・団体の各件を照合の答えつきで持ち、日付は種類の順、人物は役割の順に並ぶ。
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

	// 照合に失敗すれば結果を持たない。
	e.matcher.err = errors.New("database is locked")
	e.runtime.result = domain.WorkInfoRunResult{WorkInfo: detailedWork()}
	if _, err := e.plugins.FetchWorkInfo(context.Background(), fetchVideo, 1, "https://example.com/"); err == nil {
		t.Fatal("a failed match was not reported")
	}
}

// 受け入れ: 題名とタグだけのプラグインの取得は詳細の一覧が空で品番を持たず、照合を読まない。
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

// 反映は題名とタグ以外の選んだ項目を比較の行の順に渡し、保存先が飛ばした値を返す。
func TestApplyWorkInfoFetchPassesDetailFieldsAndReportsSkipped(t *testing.T) {
	e := newFetchEnv()
	result := e.fetchDetails(t, detailedWork())
	e.applier.skipped = []domain.SkippedWorkInfoValue{
		{Field: domain.DateField(domain.DateRelease), Value: "2024-03-01", Reason: domain.SkippedWorkInfoInvalid},
	}
	performer := domain.PersonField(domain.PersonPerformer)
	release := domain.DateField(domain.DateRelease)
	applied, err := e.plugins.ApplyWorkInfoFetch(context.Background(), fetchVideo, result.ID,
		[]domain.WorkInfoField{performer, domain.WorkInfoFieldTitle, release, domain.WorkInfoFieldCode})
	if err != nil {
		t.Fatal(err)
	}
	call := e.applier.calls[0]
	if want := []domain.WorkInfoField{domain.WorkInfoFieldCode, release, performer}; !slices.Equal(call.DetailFields, want) {
		t.Fatalf("detail fields = %v; want %v", call.DetailFields, want)
	}
	if !reflect.DeepEqual(call.Details, result.Details) || call.Title == nil || call.Tags != nil {
		t.Fatalf("apply = %+v; want the fetched details and the title", call)
	}
	if !reflect.DeepEqual(applied.Skipped, e.applier.skipped) || applied.SkippedTags == nil {
		t.Fatalf("applied = %+v", applied)
	}

	// 題名とタグだけの反映は作品情報の項目を渡さず、飛ばした値は空の一覧である。
	e.applier.skipped = nil
	again := e.fetchDetails(t, detailedWork())
	applied, err = e.plugins.ApplyWorkInfoFetch(context.Background(), fetchVideo, again.ID,
		[]domain.WorkInfoField{domain.WorkInfoFieldTags})
	if err != nil || e.applier.calls[1].DetailFields != nil || applied.Skipped == nil || len(applied.Skipped) != 0 {
		t.Fatalf("tags-only apply = %+v, %v (detail fields %v)", applied, err, e.applier.calls[1].DetailFields)
	}
}

// 受け入れ: 結果に値の無い項目を選ぶと ErrWorkInfoApplyInvalid で、何も書かず結果を残す。
func TestApplyWorkInfoFetchRefusesADetailWithoutAValue(t *testing.T) {
	e := newFetchEnv()
	work := detailedWork()
	work.Dates = []domain.WorkDate{{Kind: domain.DateRecorded, Date: "2024"}}
	result := e.fetchDetails(t, work)
	for _, field := range []domain.WorkInfoField{
		domain.DateField(domain.DateRelease),
		domain.PersonField(domain.PersonStreamer),
		domain.OrganizationField(domain.OrganizationLabel),
		"dates.unknown",
	} {
		_, err := e.plugins.ApplyWorkInfoFetch(context.Background(), fetchVideo, result.ID,
			[]domain.WorkInfoField{domain.WorkInfoFieldCode, field})
		if !errors.Is(err, domain.ErrWorkInfoApplyInvalid) {
			t.Fatalf("%s: err = %v; want ErrWorkInfoApplyInvalid", field, err)
		}
	}
	if len(e.applier.calls) != 0 {
		t.Fatal("a refused apply reached the store")
	}
	every := []domain.WorkInfoField{
		domain.WorkInfoFieldCode, domain.WorkInfoFieldDescription, domain.DateField(domain.DateRecorded),
		domain.PersonField(domain.PersonPerformer), domain.PersonField(domain.PersonDirector),
		domain.OrganizationField(domain.OrganizationMaker), domain.WorkInfoFieldURLs, domain.WorkInfoFieldExternalIDs,
	}
	if _, err := e.plugins.ApplyWorkInfoFetch(context.Background(), fetchVideo, result.ID, every); err != nil {
		t.Fatalf("the kept result could not be applied: %v", err)
	}
}
