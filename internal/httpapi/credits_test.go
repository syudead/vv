package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/store"
)

// 人物と団体の管理の経路を、
// 本物の認証・保存層・経路をつないで確かめる。

type creditFixture struct {
	env   *authEnv
	owner *http.Cookie
}

func newCreditFixture(t *testing.T) *creditFixture {
	t.Helper()
	env := newAuthEnvWith(t, t.TempDir(), func(db *store.DB) Options { return Options{Credits: db.Credits()} })
	return &creditFixture{env: env, owner: env.setup()}
}

func (f *creditFixture) request(method, target, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	return f.env.serve(authRequest{method: method, target: target, body: body, cookies: cookies})
}

func (f *creditFixture) owned(method, target, body string) *httptest.ResponseRecorder {
	return f.request(method, target, body, f.owner)
}

func creditPath(prefix string, id int64, suffix string) string {
	return prefix + "/" + strconv.FormatInt(id, 10) + suffix
}

func jsonBody(value any) string {
	out, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(out)
}

// createPerson は所有者として人物を作り、その人物を返す。
func (f *creditFixture) createPerson(t *testing.T, body any) gen.Person {
	t.Helper()
	rec := f.owned(http.MethodPost, "/api/people", jsonBody(body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("人物の作成: status = %d: %s", rec.Code, rec.Body)
	}
	return decode[gen.Person](t, rec)
}

// createOrganization は所有者として団体を作り、その団体を返す。
func (f *creditFixture) createOrganization(t *testing.T, body any) gen.Organization {
	t.Helper()
	rec := f.owned(http.MethodPost, "/api/organizations", jsonBody(body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("団体の作成: status = %d: %s", rec.Code, rec.Body)
	}
	return decode[gen.Organization](t, rec)
}

// assertFieldError は assertErrorBody に加えて、field が want であること（空なら無いこと）を確かめる。
func assertFieldError(t *testing.T, label string, rec *httptest.ResponseRecorder, want wantError, field string) {
	t.Helper()
	assertErrorBody(t, label, rec.Code, rec.Body.Bytes(), want)
	var got gen.Error
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		return
	}
	switch {
	case field == "" && got.Field != nil:
		t.Errorf("%s: field = %q, want なし", label, *got.Field)
	case field != "" && (got.Field == nil || *got.Field != field):
		t.Errorf("%s: field = %v, want %q", label, got.Field, field)
	}
}

func TestCreatePersonAndReadIt(t *testing.T) {
	f := newCreditFixture(t)
	created := f.createPerson(t, map[string]any{
		"name":        " 山田花子 ",
		"aliases":     []map[string]string{{"name": "Hanako Yamada", "language": "en"}},
		"externalIds": []map[string]string{{"source": "db", "value": "42"}},
	})
	if created.Name != "山田花子" || len(created.Aliases) != 1 || created.Aliases[0].Name != "Hanako Yamada" ||
		created.Aliases[0].Language == nil || *created.Aliases[0].Language != "en" ||
		len(created.ExternalIds) != 1 || created.ExternalIds[0] != (gen.ExternalId{Source: "db", Value: "42"}) {
		t.Fatalf("作成の応答 = %+v", created)
	}

	rec := f.env.get(creditPath("/api/people", created.Id, ""), f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("読み出し: status = %d: %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Cache-Control"); got != cacheNoStore {
		t.Errorf("Cache-Control = %q", got)
	}
	read := decode[gen.Person](t, rec)
	if read.Id != created.Id || read.Name != created.Name || len(read.Aliases) != 1 || read.VideoCount != 0 {
		t.Errorf("読み出し = %+v, want %+v", read, created)
	}
}

func TestPatchPersonAliasesKeepsName(t *testing.T) {
	f := newCreditFixture(t)
	person := f.createPerson(t, map[string]any{"name": "山田", "aliases": []map[string]string{{"name": "古い"}}})

	rec := f.owned(http.MethodPatch, creditPath("/api/people", person.Id, ""),
		jsonBody(map[string]any{"aliases": []map[string]string{{"name": "新しい"}, {"name": "Yamada", "language": "en"}}}))
	if rec.Code != http.StatusOK {
		t.Fatalf("書き換え: status = %d: %s", rec.Code, rec.Body)
	}
	got := decode[gen.Person](t, rec)
	names := []string{}
	for _, alias := range got.Aliases {
		names = append(names, alias.Name)
	}
	if got.Name != "山田" || fmt.Sprint(names) != "[Yamada 新しい]" {
		t.Errorf("書き換えの後 = %q・%v", got.Name, names)
	}

	// null は一覧を空にし、省いた名前は残る。
	rec = f.owned(http.MethodPatch, creditPath("/api/people", person.Id, ""), `{"aliases":null}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("null の書き換え: status = %d: %s", rec.Code, rec.Body)
	}
	if got := decode[gen.Person](t, rec); got.Name != "山田" || len(got.Aliases) != 0 {
		t.Errorf("null の書き換えの後 = %+v", got)
	}

	// encoding/json と同じく、プロパティ名の大文字小文字は区別しない（Aliases も黙って捨てない）。
	rec = f.owned(http.MethodPatch, creditPath("/api/people", person.Id, ""), `{"Aliases":[{"name":"大文字"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("Aliases の書き換え: status = %d: %s", rec.Code, rec.Body)
	}
	if got := decode[gen.Person](t, rec); len(got.Aliases) != 1 || got.Aliases[0].Name != "大文字" {
		t.Errorf("Aliases の書き換えの後 = %+v", got.Aliases)
	}
}

func TestCreditFieldErrors(t *testing.T) {
	f := newCreditFixture(t)
	holder := f.createPerson(t, map[string]any{"name": "持ち主", "externalIds": []map[string]string{{"source": "db", "value": "1"}}})
	parent := f.createOrganization(t, map[string]any{"name": "親"})
	child := f.createOrganization(t, map[string]any{"name": "子", "parentId": parent.Id})
	if child.Parent == nil || child.Parent.Id != parent.Id || child.Parent.Name != "親" {
		t.Fatalf("子の親 = %+v", child.Parent)
	}

	invalid := func(reason gen.ErrorReason) wantError {
		return wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reason}
	}
	cases := []struct {
		label, method, target, body string
		want                        wantError
		field                       string
	}{
		{
			"別名の language が x", http.MethodPost, "/api/people",
			jsonBody(map[string]any{"name": "名", "aliases": []map[string]string{{"name": "別", "language": "x"}}}),
			invalid(gen.ErrorReasonWorkInfoInvalidLanguage), "aliases[0].language",
		},
		{
			"空の名前", http.MethodPost, "/api/people", `{"name":"  "}`,
			invalid(gen.ErrorReasonWorkInfoEmpty), "name",
		},
		{
			"制御文字の別名", http.MethodPost, "/api/people", jsonBody(map[string]any{"name": "名", "aliases": []map[string]string{{"name": "a\tb"}}}),
			invalid(gen.ErrorReasonWorkInfoControlCharacters), "aliases[0].name",
		},
		{
			"他の人物の外部 ID", http.MethodPost, "/api/people",
			jsonBody(map[string]any{"name": "別人", "externalIds": []map[string]string{{"source": "db", "value": "1"}}}),
			wantError{status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: gen.ErrorReasonExternalIdTaken}, "externalIds[0]",
		},
		{
			"子を親にする", http.MethodPatch, creditPath("/api/organizations", parent.Id, ""),
			jsonBody(map[string]any{"parentId": child.Id}),
			invalid(gen.ErrorReasonOrganizationParentCycle), "",
		},
		{
			"無い親", http.MethodPost, "/api/organizations", `{"name":"孤","parentId":999999}`,
			wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: gen.ErrorReasonOrganizationNotFound}, "parentId",
		},
		{
			"人物に親", http.MethodPost, "/api/people", `{"name":"名","parentId":1}`,
			wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest}, "",
		},
		{
			"人物に null の親", http.MethodPost, "/api/people", `{"name":"名","parentId":null}`,
			wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest}, "",
		},
		{
			"人物の書き換えに大文字の親", http.MethodPatch, creditPath("/api/people", holder.Id, ""), `{"ParentId":null}`,
			wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest}, "",
		},
		{
			"人物の書き換えに親", http.MethodPatch, creditPath("/api/people", holder.Id, ""), `{"parentId":null}`,
			wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest}, "",
		},
		{
			"名前の null", http.MethodPatch, creditPath("/api/people", holder.Id, ""), `{"name":null}`,
			wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest}, "",
		},
		{
			"無い人物の書き換え", http.MethodPatch, creditPath("/api/people", 999999, ""), `{"name":"名"}`,
			wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: gen.ErrorReasonPersonNotFound}, "",
		},
		{
			"統合元が空", http.MethodPost, creditPath("/api/people", holder.Id, "/merge"), `{"sourceIds":[]}`,
			wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: gen.ErrorReasonWorkInfoTooMany, limit: 20000}, "sourceIds",
		},
		{
			"自分への統合", http.MethodPost, creditPath("/api/people", holder.Id, "/merge"), jsonBody(map[string]any{"sourceIds": []int64{holder.Id}}),
			invalid(gen.ErrorReasonMergeSamePerson), "",
		},
		{
			"祖先を統合する", http.MethodPost, creditPath("/api/organizations", child.Id, "/merge"), jsonBody(map[string]any{"sourceIds": []int64{parent.Id}}),
			invalid(gen.ErrorReasonOrganizationParentCycle), "",
		},
	}
	for _, tc := range cases {
		assertFieldError(t, tc.label, f.owned(tc.method, tc.target, tc.body), tc.want, tc.field)
	}

	// 上限を超える名前は limit を添える。
	long := make([]byte, 201)
	for i := range long {
		long[i] = 'a'
	}
	assertFieldError(t, "長すぎる名前", f.owned(http.MethodPost, "/api/people", jsonBody(map[string]string{"name": string(long)})),
		wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: gen.ErrorReasonWorkInfoTooLong, limit: 200}, "name")
}

func TestMergePeopleReturnsTargetAndNotFound(t *testing.T) {
	f := newCreditFixture(t)
	target := f.createPerson(t, map[string]any{"name": "統合先"})
	source := f.createPerson(t, map[string]any{"name": "統合元", "externalIds": []map[string]string{{"source": "db", "value": "7"}}})

	rec := f.owned(http.MethodPost, creditPath("/api/people", target.Id, "/merge"),
		jsonBody(map[string]any{"sourceIds": []int64{source.Id, 999999}}))
	if rec.Code != http.StatusOK {
		t.Fatalf("統合: status = %d: %s", rec.Code, rec.Body)
	}
	got := decode[gen.PersonMergeResponse](t, rec)
	if got.Person.Id != target.Id || len(got.Person.Aliases) != 1 || got.Person.Aliases[0].Name != "統合元" ||
		len(got.Person.ExternalIds) != 1 || fmt.Sprint(got.NotFoundIds) != "[999999]" {
		t.Errorf("統合の応答 = %+v", got)
	}
	assertFieldError(t, "統合元の読み出し", f.env.get(creditPath("/api/people", source.Id, ""), f.owner),
		wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: gen.ErrorReasonPersonNotFound}, "")

	org := f.createOrganization(t, map[string]any{"name": "団体"})
	rec = f.owned(http.MethodPost, creditPath("/api/organizations", org.Id, "/merge"), `{"sourceIds":[999999]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("団体の統合: status = %d: %s", rec.Code, rec.Body)
	}
	if got := decode[gen.OrganizationMergeResponse](t, rec); got.Organization.Id != org.Id || fmt.Sprint(got.NotFoundIds) != "[999999]" {
		t.Errorf("団体の統合の応答 = %+v", got)
	}
}

func TestDeletePersonThenNotFound(t *testing.T) {
	f := newCreditFixture(t)
	person := f.createPerson(t, map[string]any{"name": "消す"})
	rec := f.owned(http.MethodDelete, creditPath("/api/people", person.Id, ""), "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("削除: status = %d: %s", rec.Code, rec.Body)
	}
	notFound := wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: gen.ErrorReasonPersonNotFound}
	assertFieldError(t, "削除の後の読み出し", f.env.get(creditPath("/api/people", person.Id, ""), f.owner), notFound, "")
	assertFieldError(t, "2 回目の削除", f.owned(http.MethodDelete, creditPath("/api/people", person.Id, ""), ""), notFound, "")
}

func TestListPeoplePages(t *testing.T) {
	f := newCreditFixture(t)
	for _, name := range []string{"A", "B", "C"} {
		f.createPerson(t, map[string]any{"name": name})
	}
	rec := f.env.get("/api/people?limit=2", f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("一覧: status = %d: %s", rec.Code, rec.Body)
	}
	page := decode[gen.PersonList](t, rec)
	if len(page.Items) != 2 || page.Total != 3 || page.TotalAll != 3 || page.NextCursor == nil {
		t.Fatalf("1 ページ目 = %+v", page)
	}
	rec = f.env.get("/api/people?limit=2&cursor="+*page.NextCursor, f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("2 ページ目: status = %d: %s", rec.Code, rec.Body)
	}
	if next := decode[gen.PersonList](t, rec); len(next.Items) != 1 || next.Items[0].Name != "C" || next.NextCursor != nil {
		t.Errorf("2 ページ目 = %+v", next)
	}

	rec = f.env.get("/api/people?q=b", f.owner)
	if all := decode[gen.PersonList](t, rec); len(all.Items) != 1 || all.Total != 1 || all.TotalAll != 3 {
		t.Errorf("q の一覧 = %+v", all)
	}
	assertFieldError(t, "壊れたカーソル", f.env.get("/api/people?limit=2&cursor=broken", f.owner),
		wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: gen.ErrorReasonInvalidCursor}, "")
	assertFieldError(t, "範囲の外の limit", f.env.get("/api/organizations?limit=0", f.owner),
		wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest}, "")
	assertFieldError(t, "知らない sort", f.env.get("/api/organizations?sort=random", f.owner),
		wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest}, "")
}

// ゲストはどの経路でも 401 で、何も変わらない。
func TestCreditsOwnerOnly(t *testing.T) {
	f := newCreditFixture(t)
	person := f.createPerson(t, map[string]any{"name": "人"})
	org := f.createOrganization(t, map[string]any{"name": "団"})
	for _, kind := range []struct {
		prefix string
		id     int64
	}{{"/api/people", person.Id}, {"/api/organizations", org.Id}} {
		for _, req := range []struct{ method, target, body string }{
			{http.MethodGet, kind.prefix, ""},
			{http.MethodPost, kind.prefix, `{"name":"ゲスト"}`},
			{http.MethodGet, creditPath(kind.prefix, kind.id, ""), ""},
			{http.MethodPatch, creditPath(kind.prefix, kind.id, ""), `{"name":"ゲスト"}`},
			{http.MethodDelete, creditPath(kind.prefix, kind.id, ""), ""},
			{http.MethodPost, creditPath(kind.prefix, kind.id, "/merge"), `{"sourceIds":[999999]}`},
		} {
			assertUnauthenticated(t, req.method+" "+req.target, f.request(req.method, req.target, req.body))
		}
	}
	rec := f.env.get("/api/people", f.owner)
	if list := decode[gen.PersonList](t, rec); len(list.Items) != 1 || list.Items[0].Name != "人" {
		t.Errorf("ゲストの要求のあと = %+v", list)
	}
}
