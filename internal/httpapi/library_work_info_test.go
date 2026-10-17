package httpapi

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// 一覧の person と organization。

// creditFilterFixture は newLibraryFixture に、花子を ep1 の出演者と solo の監督に、メーカー A を
// ep1 と p1 に結んだものである。show の当たったメンバーは ep1 だけなので、show は動画の項目 ep1 になる。
func creditFilterFixture(t *testing.T) (f *libraryFixture, person, organization string) {
	t.Helper()
	f = newLibraryFixture(t)
	ctx := context.Background()
	db := f.env.db
	hanako, err := db.Credits().Create(ctx, domain.CreditPerson, domain.CreditInput{Name: "山田花子"})
	if err != nil {
		t.Fatal(err)
	}
	maker, err := db.Credits().Create(ctx, domain.CreditOrganization, domain.CreditInput{Name: "メーカーA"})
	if err != nil {
		t.Fatal(err)
	}
	performer := domain.WorkPerson{Role: domain.PersonPerformer, Person: domain.CreditRef{ID: hanako.ID}}
	director := domain.WorkPerson{Role: domain.PersonDirector, Person: domain.CreditRef{ID: hanako.ID}}
	makerLink := domain.WorkOrganization{Role: domain.OrganizationMaker, Organization: domain.CreditRef{ID: maker.ID}}
	for name, info := range map[string]domain.WorkInfo{
		"ep1":  {People: []domain.WorkPerson{performer}, Organizations: []domain.WorkOrganization{makerLink}},
		"solo": {People: []domain.WorkPerson{director}},
		"p1":   {Organizations: []domain.WorkOrganization{makerLink}},
	} {
		if _, err := db.WorkInfo().Set(ctx, f.ids[name], info); err != nil {
			t.Fatal(err)
		}
	}
	return f, strconv.FormatInt(hanako.ID, 10), strconv.FormatInt(maker.ID, 10)
}

func TestListLibraryFiltersByPersonAndOrganization(t *testing.T) {
	f, person, organization := creditFilterFixture(t)
	cases := []struct {
		query string
		want  []string
	}{
		{"person=" + person, []string{"ep1", "solo"}},
		{"organization=" + organization, []string{"ep1", "p1"}},
		{"person=" + person + "&organization=" + organization, []string{"ep1"}},
		{"person=" + person + "&query=solo", []string{"solo"}},
		{"person=999999", []string{}},
		{"organization=999999", []string{}},
	}
	for _, tc := range cases {
		rec := f.env.get("/api/library?"+tc.query, f.owner)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status = %d: %s", tc.query, rec.Code, rec.Body)
			continue
		}
		page := decode[gen.LibraryPage](t, rec)
		names := libraryItemNames(page)
		slices.Sort(names)
		if !slices.Equal(names, tc.want) || page.Total != len(tc.want) {
			t.Errorf("%s: 項目 = %v・total %d, want %v", tc.query, names, page.Total, tc.want)
		}
		if page.MissingTagIds != nil {
			t.Errorf("%s: missingTagIds = %v, want なし", tc.query, *page.MissingTagIds)
		}
	}
}

// GET /api/videos と GET /api/library/ids も同じ動画に絞る。
func TestListVideosAndLibraryIdsFilterByPerson(t *testing.T) {
	f, person, _ := creditFilterFixture(t)
	want := []int64{f.ids["ep1"], f.ids["solo"]}
	slices.Sort(want)

	rec := f.env.get("/api/videos?person="+person, f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("/api/videos: status = %d: %s", rec.Code, rec.Body)
	}
	videos := decode[gen.VideoPage](t, rec)
	var listed []int64
	for _, video := range videos.Items {
		listed = append(listed, video.Id)
	}
	slices.Sort(listed)
	if !slices.Equal(listed, want) || videos.Total != len(want) {
		t.Errorf("/api/videos = %v・total %d, want %v", listed, videos.Total, want)
	}

	rec = f.env.get("/api/library/ids?person="+person, f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("/api/library/ids: status = %d: %s", rec.Code, rec.Body)
	}
	ids := decode[gen.VideoIdsResponse](t, rec)
	slices.Sort(ids.Ids)
	if !slices.Equal(ids.Ids, want) {
		t.Errorf("/api/library/ids = %v, want %v", ids.Ids, want)
	}
}

// 正の整数でない値は 400 invalid_request、ゲストからは 400 guest_filter_not_allowed である。
func TestCreditFiltersRejectInvalidValuesAndGuests(t *testing.T) {
	f, person, organization := creditFilterFixture(t)
	invalid := wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest}
	for _, path := range []string{"/api/videos", "/api/library", "/api/library/ids"} {
		for _, query := range []string{"person=0", "person=-1", "person=abc", "organization=0", "organization=abc"} {
			rec := f.env.get(path+"?"+query, f.owner)
			assertErrorBody(t, path+"?"+query, rec.Code, rec.Body.Bytes(), invalid)
		}
	}
	guest := wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonGuestFilterNotAllowed}
	for _, path := range []string{"/api/videos", "/api/library"} {
		for _, query := range []string{"person=" + person, "organization=" + organization} {
			rec := f.env.get(path + "?" + query)
			assertErrorBody(t, "ゲストの "+path+"?"+query, rec.Code, rec.Body.Bytes(), guest)
		}
	}
}
