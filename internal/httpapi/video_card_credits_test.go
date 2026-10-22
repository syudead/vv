package httpapi

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

type fakeCardWorkInfo struct {
	WorkInfo
	infos map[string]domain.WorkInfo
	keys  []string
}

func (f *fakeCardWorkInfo) ByUserKeys(_ context.Context, keys []string) (map[string]domain.WorkInfo, error) {
	f.keys = append(f.keys, keys...)
	return f.infos, nil
}

func TestToAPICardCreditsKeepsPerformersAndPrefersLabel(t *testing.T) {
	info := domain.WorkInfo{
		Code: "ABC-123",
		People: []domain.WorkPerson{
			{Role: domain.PersonDirector, Person: domain.CreditRef{ID: 1, Name: "監督"}},
			{Role: domain.PersonPerformer, Person: domain.CreditRef{ID: 2, Name: "出演者"}, ImageRevision: 7},
		},
		Organizations: []domain.WorkOrganization{
			{Role: domain.OrganizationMaker, Organization: domain.CreditRef{ID: 10, Name: "メーカー"}},
			{Role: domain.OrganizationLabel, Organization: domain.CreditRef{ID: 11, Name: "レーベル"}},
		},
	}
	got := toAPICardCredits(info)
	if got == nil || got.Code == nil || *got.Code != "ABC-123" {
		t.Fatalf("code = %+v, want ABC-123", got)
	}
	if len(got.Performers) != 1 || got.Performers[0].Person.Id != 2 ||
		got.Performers[0].ImageRevision == nil || *got.Performers[0].ImageRevision != 7 {
		t.Errorf("performers = %+v, want only person 2 with image revision 7", got.Performers)
	}
	if got.Organization == nil || got.Organization.Id != 11 {
		t.Errorf("organization = %+v, want the label 11", got.Organization)
	}
}

func TestToAPICardCreditsFallsBackToMaker(t *testing.T) {
	got := toAPICardCredits(domain.WorkInfo{Organizations: []domain.WorkOrganization{
		{Role: domain.OrganizationMaker, Organization: domain.CreditRef{ID: 10, Name: "メーカー"}},
	}})
	if got == nil || got.Organization == nil || got.Organization.Id != 10 {
		t.Fatalf("organization = %+v, want the maker 10", got)
	}
	if got.Performers == nil || len(got.Performers) != 0 {
		t.Errorf("performers = %v, want []", got.Performers)
	}
}

func TestToAPICardCreditsOmitsWorksWithNothingToShow(t *testing.T) {
	info := domain.WorkInfo{
		Description: "説明",
		People:      []domain.WorkPerson{{Role: domain.PersonDirector, Person: domain.CreditRef{ID: 1}}},
	}
	if got := toAPICardCredits(info); got != nil {
		t.Errorf("card credits = %+v, want nil", got)
	}
}

func TestListVideosIncludesCardCredits(t *testing.T) {
	credited := sampleVideo(1, "a")
	plain := sampleVideo(2, "b")
	plain.UserKey = "other-key"
	work := &fakeCardWorkInfo{infos: map[string]domain.WorkInfo{credited.UserKey: {Code: "ABC-123"}}}
	handler := newTestServer(t, Options{
		Videos:   &fakeLibrary{page: domain.VideoPage{Items: []domain.Video{credited, plain}}},
		WorkInfo: work,
	})

	rec := do(t, handler, http.MethodGet, "/api/videos")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	page := decode[gen.VideoPage](t, rec)
	if got := page.Items[0].CardCredits; got == nil || got.Code == nil || *got.Code != "ABC-123" {
		t.Errorf("first cardCredits = %+v, want code ABC-123", got)
	}
	if got := page.Items[1].CardCredits; got != nil {
		t.Errorf("second cardCredits = %+v, want omitted", got)
	}
	if want := []string{credited.UserKey, plain.UserKey}; !slices.Equal(work.keys, want) {
		t.Errorf("read keys = %v, want %v", work.keys, want)
	}
}

func TestForAudienceDropsCardCreditsForGuests(t *testing.T) {
	code := "ABC-123"
	video := gen.Video{CardCredits: &gen.VideoCardCredits{Code: &code, Performers: []gen.WorkPerson{}}}
	if got := forAudience(domain.AudienceGuest, video).CardCredits; got != nil {
		t.Errorf("guest cardCredits = %+v, want omitted", got)
	}
}

// Lists replace a changed item with the detail, so the detail must keep the card's credits.
func TestGetVideoIncludesCardCredits(t *testing.T) {
	video := sampleVideo(7, "a")
	video.WorkInfo = &domain.WorkInfo{Code: "ABC-123"}
	handler := newTestServer(t, Options{Videos: &fakeLibrary{videos: map[int64]domain.Video{7: video}}})

	rec := do(t, handler, http.MethodGet, "/api/videos/7")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}
	if got := decode[gen.Video](t, rec).CardCredits; got == nil || got.Code == nil || *got.Code != "ABC-123" {
		t.Errorf("cardCredits = %+v, want code ABC-123", got)
	}
}
