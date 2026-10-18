package httpapi

import (
	"net/http"
	"testing"

	"github.com/syudead/vv/internal/httpapi/gen"
)

func (f *creditFixture) favoritePeople(t *testing.T, favorite bool, ids ...int64) {
	t.Helper()
	rec := f.owned(http.MethodPut, "/api/favorites", jsonBody(map[string]any{"personIds": ids, "favorite": favorite}))
	if rec.Code != http.StatusOK {
		t.Fatalf("人物のお気に入り (%v): status = %d: %s", favorite, rec.Code, rec.Body)
	}
}

func (f *creditFixture) personList(t *testing.T, target string) gen.PersonList {
	t.Helper()
	rec := f.env.get(target, f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s: status = %d: %s", target, rec.Code, rec.Body)
	}
	return decode[gen.PersonList](t, rec)
}

func TestPersonFavoritesSetFilterAndClear(t *testing.T) {
	f := newCreditFixture(t)
	liked := f.createPerson(t, map[string]any{"name": "好き"})
	f.createPerson(t, map[string]any{"name": "ほか"})
	if liked.Favorite {
		t.Fatal("作ったばかりの人物の favorite = true")
	}

	f.favoritePeople(t, true, liked.Id, 999_999)
	rec := f.env.get(creditPath("/api/people", liked.Id, ""), f.owner)
	if got := decode[gen.Person](t, rec); !got.Favorite {
		t.Error("付けたあとの favorite = false")
	}
	page := f.personList(t, "/api/people?favorite=true")
	if len(page.Items) != 1 || page.Items[0].Id != liked.Id || page.Total != 1 || page.TotalAll != 2 {
		t.Errorf("favorite=true の一覧 = %+v", page)
	}
	if all := f.personList(t, "/api/people?favorite=false"); all.Total != 2 {
		t.Errorf("favorite=false の一覧 total = %d, want 2", all.Total)
	}

	f.favoritePeople(t, false, liked.Id)
	if page := f.personList(t, "/api/people?favorite=true"); len(page.Items) != 0 || page.Total != 0 {
		t.Errorf("外したあとの favorite=true の一覧 = %+v", page)
	}
}

func TestPersonFavoritesRefusals(t *testing.T) {
	f := newCreditFixture(t)
	person := f.createPerson(t, map[string]any{"name": "人"})
	for _, target := range []string{"/api/organizations?favorite=true", "/api/organizations?favorite=false"} {
		assertFieldError(t, target, f.env.get(target, f.owner),
			wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest}, "")
	}
	assertUnauthenticated(t, "ゲストの favorite=true", f.env.get("/api/people?favorite=true"))
	assertUnauthenticated(t, "ゲストの personIds",
		f.request(http.MethodPut, "/api/favorites", jsonBody(map[string]any{"personIds": []int64{person.Id}, "favorite": true})))
	if page := f.personList(t, "/api/people?favorite=true"); len(page.Items) != 0 {
		t.Errorf("ゲストの要求のあとの favorite=true の一覧 = %+v", page)
	}
}
