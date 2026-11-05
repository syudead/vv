package httpapi

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/syudead/vv/internal/httpapi/gen"
)

func TestOrganizationProfilePatchReplacesAndClearsSentFieldsOnly(t *testing.T) {
	f := newCreditFixture(t)
	org := f.createOrganization(t, map[string]any{"name": "Studio", "organizationProfile": map[string]any{
		"foundedDate": "1990", "location": "Tokyo", "links": []string{"https://a.example"},
	}})
	rec := f.owned(http.MethodPatch, creditPath("/api/organizations", org.Id, ""),
		`{"organizationProfile": {"location": null, "bio": "b"}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH: status = %d: %s", rec.Code, rec.Body)
	}
	got := decode[gen.Organization](t, rec).Profile
	want := gen.OrganizationProfile{FoundedDate: new("1990"), Bio: new("b"), Links: &[]string{"https://a.example"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("profile = %+v, want %+v", got, want)
	}

	rec = f.owned(http.MethodPatch, creditPath("/api/organizations", org.Id, ""),
		`{"organizationProfile": {"dissolvedDate": "1980"}}`)
	assertFieldError(t, "解散が設立より前", rec, wantError{
		status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: gen.ErrorReasonWorkInfoOutOfRange,
	},
		"organizationProfile.dissolvedDate")
}

func TestOrganizationProfileIsRefusedForAPerson(t *testing.T) {
	f := newCreditFixture(t)
	person := f.createPerson(t, map[string]any{"name": "人"})
	rec := f.owned(http.MethodPatch, creditPath("/api/people", person.Id, ""), `{"organizationProfile": {"bio": "b"}}`)
	assertFieldError(t, "人物への organizationProfile", rec,
		wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest}, "")
	rec = f.owned(http.MethodPost, "/api/people", `{"name": "別人", "organizationProfile": {}}`)
	assertFieldError(t, "人物の作成に organizationProfile", rec,
		wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest}, "")
}

func TestOrganizationFavoritesFilterTheList(t *testing.T) {
	f := newCreditFixture(t)
	liked := f.createOrganization(t, map[string]any{"name": "Liked"})
	f.createOrganization(t, map[string]any{"name": "Other"})
	rec := f.owned(http.MethodPut, "/api/favorites", `{"organizationIds": [`+itoa(liked.Id)+`], "favorite": true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT /api/favorites: status = %d: %s", rec.Code, rec.Body)
	}
	rec = f.env.get("/api/organizations?favorite=true", f.owner)
	page := decode[gen.OrganizationList](t, rec)
	if len(page.Items) != 1 || page.Items[0].Id != liked.Id || !page.Items[0].Favorite || page.Total != 1 {
		t.Errorf("favorite=true の一覧 = %+v", page)
	}
}

func TestOrganizationLogoUploadAndRemoval(t *testing.T) {
	f := newPersonImageFixture(t)
	org := f.createOrganization(t, map[string]any{"name": "Studio"})
	rec := f.putCreditImage(t, "/api/organizations", org.Id)
	if rec.Code != http.StatusOK || decode[gen.Organization](t, rec).ImageRevision == nil {
		t.Fatalf("PUT logo: status = %d: %s", rec.Code, rec.Body)
	}
	if rec := f.env.get(creditPath("/api/organizations", org.Id, "/image"), f.owner); rec.Code != http.StatusOK ||
		rec.Header().Get("Content-Type") != "image/jpeg" {
		t.Errorf("GET logo: status = %d, type %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	rec = f.owned(http.MethodDelete, creditPath("/api/organizations", org.Id, "/image"), "")
	if rec.Code != http.StatusOK || decode[gen.Organization](t, rec).ImageRevision != nil {
		t.Fatalf("DELETE logo: status = %d: %s", rec.Code, rec.Body)
	}
	assertFieldError(t, "ロゴの無い団体", f.env.get(creditPath("/api/organizations", org.Id, "/image"), f.owner),
		wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: gen.ErrorReasonPersonImageNotFound}, "")
	assertFieldError(t, "無い団体", f.env.get(creditPath("/api/organizations", 999_999, "/image"), f.owner),
		wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: gen.ErrorReasonOrganizationNotFound}, "")
}
