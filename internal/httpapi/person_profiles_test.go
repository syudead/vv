package httpapi

import (
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"github.com/syudead/vv/internal/httpapi/extgen"
	"github.com/syudead/vv/internal/httpapi/gen"
)

func TestPersonProfileRoundTripsOnTheScreenAPI(t *testing.T) {
	f := newCreditFixture(t)
	person := f.createPerson(t, map[string]any{"name": "山田花子", "profile": map[string]any{
		"gender": "female", "birthDate": "1990-05", "heightCm": 160, "cup": "c", "bio": "一行目\n二行目",
		"links": []string{"https://a.example"},
	}})
	created := `{"bio":"一行目\n二行目","birthDate":"1990-05","cup":"C","gender":"female","heightCm":160,"links":["https://a.example"]}`
	if got := jsonBody(person.Profile); got != created {
		t.Errorf("作成の profile = %s, want %s", got, created)
	}

	rec := f.owned(http.MethodPatch, creditPath("/api/people", person.Id, ""), `{"profile":{"heightCm":null,"Cup":"e","links":[]}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("profile の書き換え: status = %d: %s", rec.Code, rec.Body)
	}
	patched := `{"bio":"一行目\n二行目","birthDate":"1990-05","cup":"E","gender":"female"}`
	if got := jsonBody(decode[gen.Person](t, rec).Profile); got != patched {
		t.Errorf("書き換えた profile = %s, want %s", got, patched)
	}
	if got := jsonBody(decode[gen.Person](t, f.env.get(creditPath("/api/people", person.Id, ""), f.owner)).Profile); got != patched {
		t.Errorf("読み直した profile = %s, want %s", got, patched)
	}

	var bare map[string]json.RawMessage
	if err := json.Unmarshal(f.owned(http.MethodPost, "/api/people", `{"name":"空"}`).Body.Bytes(), &bare); err != nil ||
		string(bare["profile"]) != "{}" {
		t.Errorf("profile の無い人物の profile = %s, %v", bare["profile"], err)
	}
}

func TestPersonProfileErrorsOnTheScreenAPI(t *testing.T) {
	f := newCreditFixture(t)
	person := f.createPerson(t, map[string]any{"name": "山田"})
	organization := f.createOrganization(t, map[string]any{"name": "会社"})
	invalidRequest := wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest}
	cases := []struct {
		label, method, target, body string
		want                        wantError
		field                       string
	}{
		{"団体の作成に profile", http.MethodPost, "/api/organizations", `{"name":"団","profile":{}}`, invalidRequest, ""},
		{"団体の書き換えに profile", http.MethodPatch, creditPath("/api/organizations", organization.Id, ""), `{"profile":{"bio":null}}`, invalidRequest, ""},
		{"profile が null", http.MethodPatch, creditPath("/api/people", person.Id, ""), `{"profile":null}`, invalidRequest, ""},
		{
			"範囲外の身長", http.MethodPatch, creditPath("/api/people", person.Id, ""), `{"profile":{"heightCm":10}}`,
			wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: gen.ErrorReasonWorkInfoOutOfRange},
			"profile.heightCm",
		},
	}
	for _, tc := range cases {
		assertFieldError(t, tc.label, f.owned(tc.method, tc.target, tc.body), tc.want, tc.field)
	}
}

func TestPersonProfileRoundTripsOnTheExternalAPI(t *testing.T) {
	f := newExternalOverridesFixture(t)
	created := f.post(t, "/api/v1/people", map[string]any{"name": "山田花子", "profile": map[string]any{
		"bloodType": "ab", "careerStartYear": 2010, "bio": "紹介",
	}})
	if created.Code != http.StatusOK {
		t.Fatalf("作成: status = %d: %s", created.Code, created.Body)
	}
	person := decode[extgen.Person](t, created)
	start := 2010
	if want := (extgen.PersonProfile{
		BloodType: new(extgen.AB), CareerStartYear: &start, Bio: new("紹介"),
	}); !reflect.DeepEqual(person.Profile, want) {
		t.Errorf("作成の profile = %s, want %s", jsonBody(person.Profile), jsonBody(want))
	}

	updated := f.post(t, "/api/v1/people/update", map[string]any{
		"id": person.Id, "profile": map[string]any{"bio": nil, "links": []string{"https://a.example"}},
	})
	if updated.Code != http.StatusOK {
		t.Fatalf("書き換え: status = %d: %s", updated.Code, updated.Body)
	}
	links := []string{"https://a.example"}
	if want := (extgen.PersonProfile{
		BloodType: new(extgen.AB), CareerStartYear: &start, Links: &links,
	}); !reflect.DeepEqual(decode[extgen.Person](t, updated).Profile, want) {
		t.Errorf("書き換えた profile = %s, want %s", updated.Body, jsonBody(want))
	}

	organization := f.post(t, "/api/v1/organizations", map[string]any{"name": "会社", "profile": map[string]any{}})
	assertExternalError(t, "団体の profile", organization, wantExternalError{
		status: http.StatusBadRequest, code: extgen.ErrorCodeInvalidRequest, index: -1,
	})
}
