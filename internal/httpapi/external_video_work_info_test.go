package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/extgen"
)

// 外部連携 API の作品情報と人物・団体を、本物の
// 認証・保存層と Bearer の境界で確かめる。

// assertExternalField は誤りの応答の field が want であることを確かめる。
func assertExternalField(t *testing.T, label string, rec *httptest.ResponseRecorder, want string) {
	t.Helper()
	var got extgen.Error
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("%s: 本文を読めない: %v: %s", label, err, rec.Body)
	}
	if got.Field == nil || *got.Field != want {
		t.Errorf("%s: field = %v, want %q", label, got.Field, want)
	}
}

func (f externalOverridesFixture) get(t *testing.T, target string) *httptest.ResponseRecorder {
	t.Helper()
	return f.env.serve(authRequest{method: http.MethodGet, target: target, header: bearer(f.secret)})
}

// createCredit は保存層で人物か団体を作り、その id を返す。
func (f externalOverridesFixture) createCredit(t *testing.T, kind domain.CreditKind, input domain.CreditInput) int64 {
	t.Helper()
	credit, err := f.env.db.Credits().Create(context.Background(), kind, input)
	if err != nil {
		t.Fatal(err)
	}
	return credit.ID
}

// 受け入れ条件 8（サーバー側）: 2 項目の POST が 200 で両方の作品情報を返し、lookup が未設定の code を
// null にしてそれらを返す。同じ要求を繰り返しても 200 で updatedAt が変わらない。
func TestExternalVideoWorkInfoAppliesAndRepeats(t *testing.T) {
	f := newExternalOverridesFixture(t)
	performer := f.createCredit(t, domain.CreditPerson, domain.CreditInput{Name: "出演者"})
	maker := f.createCredit(t, domain.CreditOrganization, domain.CreditInput{Name: "メーカー"})

	body := map[string]any{"items": []any{
		map[string]any{"video": map[string]any{"id": f.ids["a"]}, "workInfo": map[string]any{
			"code":          " ABC-123 ",
			"dates":         []any{map[string]any{"kind": "release", "date": "2024-06"}},
			"people":        []any{map[string]any{"role": "performer", "person": map[string]any{"id": performer}}},
			"organizations": []any{map[string]any{"role": "maker", "organization": map[string]any{"id": maker}}},
			"urls":          []string{"https://example.com/abc-123"},
			"externalIds":   []any{map[string]any{"source": "example", "value": "abc123"}},
		}},
		map[string]any{"video": map[string]any{"contentKey": "content-key-b"}, "workInfo": map[string]any{
			"description": "説明だけ",
		}},
	}}
	rec := f.post(t, "/api/v1/video-work-info", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("設定: status = %d: %s", rec.Code, rec.Body)
	}
	out := decode[extgen.VideoWorkInfoResponse](t, rec)
	code, description := "ABC-123", "説明だけ"
	wantA := extgen.WorkInfo{
		Code:          &code,
		Dates:         []extgen.WorkDate{{Kind: extgen.Release, Date: "2024-06"}},
		People:        []extgen.WorkPerson{{Role: extgen.Performer, Person: extgen.CreditRef{Id: performer, Name: "出演者"}}},
		Organizations: []extgen.WorkOrganization{{Role: extgen.Maker, Organization: extgen.CreditRef{Id: maker, Name: "メーカー"}}},
		Urls:          []string{"https://example.com/abc-123"},
		ExternalIds:   []extgen.ExternalId{{Source: "example", Value: "abc123"}},
	}
	wantB := extgen.WorkInfo{
		Description: &description, Dates: []extgen.WorkDate{}, People: []extgen.WorkPerson{},
		Organizations: []extgen.WorkOrganization{}, Urls: []string{}, ExternalIds: []extgen.ExternalId{},
	}
	if len(out.Items) != 2 || out.Items[0].Video.Id != f.ids["a"] || out.Items[1].Video.ContentKey != "content-key-b" {
		t.Fatalf("items = %+v", out.Items)
	}
	if !reflect.DeepEqual(out.Items[0].WorkInfo, wantA) || !reflect.DeepEqual(out.Items[1].WorkInfo, wantB) {
		t.Errorf("応答の workInfo = %+v / %+v", out.Items[0].WorkInfo, out.Items[1].WorkInfo)
	}

	lookedB := f.get(t, fmt.Sprintf("/api/v1/videos/lookup?id=%d", f.ids["b"]))
	var raw struct {
		WorkInfo map[string]json.RawMessage `json:"workInfo"`
	}
	if err := json.Unmarshal(lookedB.Body.Bytes(), &raw); err != nil || lookedB.Code != http.StatusOK {
		t.Fatalf("lookup: %d %s", lookedB.Code, lookedB.Body)
	}
	if got := string(raw.WorkInfo["code"]); got != "null" {
		t.Errorf("lookup の未設定の code = %s, want null", got)
	}
	if got := string(raw.WorkInfo["people"]); got != "[]" {
		t.Errorf("lookup の空の people = %s, want []", got)
	}
	before := f.lookup(t, "a")
	if !reflect.DeepEqual(before.WorkInfo, wantA) {
		t.Errorf("lookup の workInfo = %+v", before.WorkInfo)
	}
	// 作品情報の無い動画も workInfo を持つ。
	page := decode[extgen.ExternalVideoPage](t, f.get(t, "/api/v1/videos?limit=200"))
	for _, item := range page.Items {
		if item.WorkInfo.People == nil || item.WorkInfo.Urls == nil {
			t.Errorf("一覧の %d の workInfo = %+v", item.Id, item.WorkInfo)
		}
	}

	// 同じ要求を繰り返すと 200 で、updatedAt は変わらない。
	if again := f.post(t, "/api/v1/video-work-info", body); again.Code != http.StatusOK {
		t.Fatalf("繰り返し: status = %d: %s", again.Code, again.Body)
	}
	if after := f.lookup(t, "a"); !after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Errorf("繰り返しで updatedAt が %v から %v に変わった", before.UpdatedAt, after.UpdatedAt)
	}

	// {} はすべてを解除する。
	cleared := f.post(t, "/api/v1/video-work-info", map[string]any{"items": []any{
		map[string]any{"video": map[string]any{"id": f.ids["a"]}, "workInfo": map[string]any{}},
	}})
	if cleared.Code != http.StatusOK {
		t.Fatalf("解除: status = %d: %s", cleared.Code, cleared.Body)
	}
	if got := f.lookup(t, "a").WorkInfo; got.Code != nil || len(got.People) != 0 || got.People == nil {
		t.Errorf("解除の後の workInfo = %+v", got)
	}
}

// 規則に合わない項目・無い人物・引けない動画・形の誤りはどれも何も反映せず、index と field を返す。
func TestExternalVideoWorkInfoRejectsAndChangesNothing(t *testing.T) {
	f := newExternalOverridesFixture(t)
	first := map[string]any{"video": map[string]any{"id": f.ids["a"]}, "workInfo": map[string]any{"code": "KEEP-1"}}
	people := make([]any, 0, 101)
	for i := range 101 {
		id := f.createCredit(t, domain.CreditPerson, domain.CreditInput{Name: fmt.Sprintf("人物 %d", i)})
		people = append(people, map[string]any{"role": "performer", "person": map[string]any{"id": id}})
	}

	tooMany := f.post(t, "/api/v1/video-work-info", map[string]any{"items": []any{first,
		map[string]any{"video": map[string]any{"id": f.ids["b"]}, "workInfo": map[string]any{"people": people}},
	}})
	assertExternalError(t, "101 人", tooMany, wantExternalError{
		status: http.StatusBadRequest, code: extgen.ErrorCodeInvalidRequest, reason: extgen.WorkInfoTooMany, index: 1, limit: 100,
	})
	assertExternalField(t, "101 人", tooMany, "people")

	missing := f.post(t, "/api/v1/video-work-info", map[string]any{"items": []any{first,
		map[string]any{"video": map[string]any{"id": f.ids["b"]}, "workInfo": map[string]any{
			"people": []any{map[string]any{"role": "director", "person": map[string]any{"id": 999999}}},
		}},
	}})
	assertExternalError(t, "無い人物", missing, wantExternalError{
		status: http.StatusNotFound, code: extgen.ErrorCodeNotFound, reason: extgen.PersonNotFound, index: 1,
	})
	assertExternalField(t, "無い人物", missing, "people[0].person.id")

	noVideo := f.post(t, "/api/v1/video-work-info", map[string]any{"items": []any{first,
		map[string]any{"video": map[string]any{"path": "/nowhere.mp4"}, "workInfo": map[string]any{}},
	}})
	assertExternalError(t, "無い動画", noVideo, wantExternalError{
		status: http.StatusNotFound, code: extgen.ErrorCodeNotFound, reason: extgen.VideoNotFound, index: 1,
	})

	for label, item := range map[string]any{
		"workInfo が無い": map[string]any{"video": map[string]any{"id": f.ids["b"]}},
		"知らないプロパティ":    map[string]any{"video": map[string]any{"id": f.ids["b"]}, "workInfo": map[string]any{"title": "x"}},
		"日付の date が無い": map[string]any{"video": map[string]any{"id": f.ids["b"]}, "workInfo": map[string]any{"dates": []any{map[string]any{"kind": "release"}}}},
		"動画の指定が 2 つ":   map[string]any{"video": map[string]any{"id": f.ids["b"], "path": "/x"}, "workInfo": map[string]any{}},
		"知らない役割":       map[string]any{"video": map[string]any{"id": f.ids["b"]}, "workInfo": map[string]any{"people": []any{map[string]any{"role": "x", "person": map[string]any{"id": 1}}}}},
	} {
		rec := f.post(t, "/api/v1/video-work-info", map[string]any{"items": []any{first, item}})
		assertExternalError(t, label, rec, wantExternalError{status: http.StatusBadRequest, code: extgen.ErrorCodeInvalidRequest, index: 1})
	}
	empty := f.post(t, "/api/v1/video-work-info", map[string]any{"items": []any{}})
	assertExternalError(t, "0 件", empty, wantExternalError{
		status: http.StatusBadRequest, code: extgen.ErrorCodeInvalidRequest, reason: extgen.TooManyVideos, index: -1, limit: 20000,
	})

	if got := f.lookup(t, "a").WorkInfo.Code; got != nil {
		t.Errorf("誤りの後に 1 つ目の項目が反映された: code = %q", *got)
	}
}

// 外部 ID での検索・作成・書き換え・統合・削除が画面と同じ保存先の操作を通る。
func TestExternalCreditsLookupCreateUpdateMergeDelete(t *testing.T) {
	f := newExternalOverridesFixture(t)
	created := f.post(t, "/api/v1/people", map[string]any{
		"name": "山田 花子", "aliases": []any{map[string]any{"name": "Hanako", "language": "en"}},
		"externalIds": []any{map[string]any{"source": "example", "value": "42"}},
	})
	if created.Code != http.StatusOK {
		t.Fatalf("作成: status = %d: %s", created.Code, created.Body)
	}
	person := decode[extgen.Person](t, created)

	found := decode[extgen.PersonList](t, f.get(t, "/api/v1/people?source=example&externalId=42"))
	if len(found.Items) != 1 || found.Items[0].Id != person.Id || found.Total != 1 || found.TotalAll != 1 {
		t.Errorf("外部 ID の検索 = %+v", found)
	}
	// 保存と同じく前後の空白を落として照らす。
	if spaced := decode[extgen.PersonList](t, f.get(t, "/api/v1/people?source=%20example&externalId=42%20")); len(spaced.Items) != 1 {
		t.Errorf("空白付きの検索 = %+v", spaced)
	}
	none := f.get(t, "/api/v1/people?source=example&externalId=43")
	var noneRaw map[string]json.RawMessage
	if err := json.Unmarshal(none.Body.Bytes(), &noneRaw); err != nil || none.Code != http.StatusOK || string(noneRaw["items"]) != "[]" {
		t.Errorf("無い外部 ID の検索: %d %s", none.Code, none.Body)
	}
	half := f.get(t, "/api/v1/people?source=example")
	assertExternalError(t, "source だけ", half, wantExternalError{status: http.StatusBadRequest, code: extgen.ErrorCodeInvalidRequest, index: -1})

	taken := f.post(t, "/api/v1/people", map[string]any{
		"name": "別人", "externalIds": []any{map[string]any{"source": "example", "value": "42"}},
	})
	assertExternalError(t, "使われた外部 ID", taken, wantExternalError{
		status: http.StatusConflict, code: extgen.ErrorCodeConflict, reason: extgen.ExternalIdTaken, index: -1,
	})
	assertExternalField(t, "使われた外部 ID", taken, "externalIds[0]")
	withParent := f.post(t, "/api/v1/people", map[string]any{"name": "親つき", "parentId": nil})
	assertExternalError(t, "人物の parentId", withParent, wantExternalError{status: http.StatusBadRequest, code: extgen.ErrorCodeInvalidRequest, index: -1})

	// 書き換え: aliases の null は一覧を空にし、省いた externalIds は残る。
	updated := f.post(t, "/api/v1/people/update", map[string]any{"id": person.Id, "name": "山田 花子（新）", "aliases": nil})
	if updated.Code != http.StatusOK {
		t.Fatalf("書き換え: status = %d: %s", updated.Code, updated.Body)
	}
	if got := decode[extgen.Person](t, updated); got.Name != "山田 花子（新）" || len(got.Aliases) != 0 || got.Aliases == nil ||
		len(got.ExternalIds) != 1 {
		t.Errorf("書き換えの結果 = %+v", got)
	}
	missingUpdate := f.post(t, "/api/v1/people/update", map[string]any{"id": 999999, "name": "x"})
	assertExternalError(t, "無い人物の書き換え", missingUpdate, wantExternalError{
		status: http.StatusNotFound, code: extgen.ErrorCodeNotFound, reason: extgen.PersonNotFound, index: -1,
	})
	emptyName := f.post(t, "/api/v1/people/update", map[string]any{"id": person.Id, "name": " "})
	assertExternalError(t, "空の名前", emptyName, wantExternalError{
		status: http.StatusBadRequest, code: extgen.ErrorCodeInvalidRequest, reason: extgen.WorkInfoEmpty, index: -1,
	})
	assertExternalField(t, "空の名前", emptyName, "name")

	// 統合: 統合元の名前は別名になり、無い統合元は notFoundIds に出る。
	source := f.createCredit(t, domain.CreditPerson, domain.CreditInput{Name: "旧名"})
	merged := f.post(t, "/api/v1/people/merge", map[string]any{"targetId": person.Id, "sourceIds": []int64{source, 888888}})
	if merged.Code != http.StatusOK {
		t.Fatalf("統合: status = %d: %s", merged.Code, merged.Body)
	}
	if got := decode[extgen.PersonMergeResponse](t, merged); len(got.Person.Aliases) != 1 ||
		got.Person.Aliases[0].Name != "旧名" || !slices.Equal(got.NotFoundIds, []int64{888888}) {
		t.Errorf("統合の結果 = %+v", got)
	}
	same := f.post(t, "/api/v1/people/merge", map[string]any{"targetId": person.Id, "sourceIds": []int64{person.Id}})
	assertExternalError(t, "自分への統合", same, wantExternalError{
		status: http.StatusBadRequest, code: extgen.ErrorCodeInvalidRequest, reason: extgen.MergeSamePerson, index: -1,
	})
	noSources := f.post(t, "/api/v1/people/merge", map[string]any{"targetId": person.Id, "sourceIds": []int64{}})
	assertExternalError(t, "統合元が空", noSources, wantExternalError{
		status: http.StatusBadRequest, code: extgen.ErrorCodeInvalidRequest, reason: extgen.WorkInfoTooMany, index: -1, limit: domain.MaxTagBatch,
	})
	assertExternalField(t, "統合元が空", noSources, "sourceIds")

	// 削除: 消した id と無かった id を ids の順で重複なく返す。
	other := f.createCredit(t, domain.CreditPerson, domain.CreditInput{Name: "消す人"})
	deleted := f.post(t, "/api/v1/people/delete", map[string]any{"ids": []int64{777777, other, person.Id, other}})
	if deleted.Code != http.StatusOK {
		t.Fatalf("削除: status = %d: %s", deleted.Code, deleted.Body)
	}
	if got := decode[extgen.CreditDeleteResponse](t, deleted); !slices.Equal(got.DeletedIds, []int64{other, person.Id}) ||
		!slices.Equal(got.NotFoundIds, []int64{777777}) {
		t.Errorf("削除の結果 = %+v", got)
	}
	if after := decode[extgen.PersonList](t, f.get(t, "/api/v1/people")); len(after.Items) != 0 || after.TotalAll != 0 {
		t.Errorf("削除の後の一覧 = %+v", after)
	}
}

// 団体の版は parent（根では null）と parentId を持ち、親の輪を断る。一覧は画面と同じ引数で読む。
func TestExternalOrganizationsCarryParent(t *testing.T) {
	f := newExternalOverridesFixture(t)
	root := decode[extgen.Organization](t, f.post(t, "/api/v1/organizations", map[string]any{"name": "親会社"}))
	child := f.post(t, "/api/v1/organizations", map[string]any{"name": "レーベル", "parentId": root.Id})
	if child.Code != http.StatusOK {
		t.Fatalf("子の作成: status = %d: %s", child.Code, child.Body)
	}
	label := decode[extgen.Organization](t, child)
	if label.Parent == nil || label.Parent.Id != root.Id || label.Parent.Name != "親会社" {
		t.Errorf("子の parent = %+v", label.Parent)
	}
	var rootRaw map[string]json.RawMessage
	rec := f.get(t, "/api/v1/organizations?q=親会社")
	if err := json.Unmarshal(rec.Body.Bytes(), &rootRaw); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("一覧: %d %s", rec.Code, rec.Body)
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(rootRaw["items"], &items); err != nil || len(items) != 1 || string(items[0]["parent"]) != "null" {
		t.Errorf("根の団体の parent = %s", rootRaw["items"])
	}

	cycle := f.post(t, "/api/v1/organizations/update", map[string]any{"id": root.Id, "parentId": label.Id})
	assertExternalError(t, "親の輪", cycle, wantExternalError{
		status: http.StatusBadRequest, code: extgen.ErrorCodeInvalidRequest, reason: extgen.OrganizationParentCycle, index: -1,
	})
	detached := f.post(t, "/api/v1/organizations/update", map[string]any{"id": label.Id, "parentId": nil})
	if got := decode[extgen.Organization](t, detached); detached.Code != http.StatusOK || got.Parent != nil {
		t.Errorf("親を外す: %d %+v", detached.Code, got)
	}
	noParent := f.post(t, "/api/v1/organizations", map[string]any{"name": "孤児", "parentId": 999999})
	assertExternalError(t, "無い親", noParent, wantExternalError{
		status: http.StatusNotFound, code: extgen.ErrorCodeNotFound, reason: extgen.OrganizationNotFound, index: -1,
	})
	assertExternalField(t, "無い親", noParent, "parentId")

	page := decode[extgen.OrganizationList](t, f.get(t, "/api/v1/organizations?sort=name&limit=1"))
	if len(page.Items) != 1 || page.NextCursor == nil || page.TotalAll != 2 {
		t.Errorf("1 ページ目 = %+v", page)
	}
	badSort := f.get(t, "/api/v1/organizations?sort=unknown")
	assertExternalError(t, "知らない並び順", badSort, wantExternalError{status: http.StatusBadRequest, code: extgen.ErrorCodeInvalidRequest, index: -1})
}

// 必須のプロパティ（作成の name・統合の targetId・書き換えの id）が無いか null の本文は、項目の規則の誤りや
// 無い実体ではなく形の誤り（400 invalid_request、reason なし）である。外部 ID での検索は limit を読まない
// ので、整数でない limit でも引け、一覧では limit が整数でなければ 400 である。
func TestExternalCreditsRejectMalformedBodiesAndIgnoreLookupLimit(t *testing.T) {
	f := newExternalOverridesFixture(t)
	id := f.createCredit(t, domain.CreditPerson, domain.CreditInput{
		Name: "山田 花子", ExternalIDs: []domain.ExternalID{{Source: "example", Value: "42"}},
	})
	malformed := wantExternalError{status: http.StatusBadRequest, code: extgen.ErrorCodeInvalidRequest, index: -1}
	for label, request := range map[string]struct {
		target string
		body   any
	}{
		"作成の name が null":     {"/api/v1/people", map[string]any{"name": nil}},
		"作成の name が無い":        {"/api/v1/organizations", map[string]any{"aliases": []any{}}},
		"統合の targetId が null": {"/api/v1/people/merge", map[string]any{"targetId": nil, "sourceIds": []int64{id}}},
		"統合の targetId が無い":    {"/api/v1/organizations/merge", map[string]any{"sourceIds": []int64{id}}},
		"書き換えの id が null":     {"/api/v1/people/update", map[string]any{"id": nil, "name": "x"}},
	} {
		assertExternalError(t, label, f.post(t, request.target, request.body), malformed)
	}
	if after := decode[extgen.PersonList](t, f.get(t, "/api/v1/people")); after.TotalAll != 1 {
		t.Errorf("断った本文の後の人物の数 = %d, want 1", after.TotalAll)
	}

	found := f.get(t, "/api/v1/people?source=example&externalId=42&limit=abc")
	if found.Code != http.StatusOK {
		t.Fatalf("limit=abc の検索: status = %d: %s", found.Code, found.Body)
	}
	if got := decode[extgen.PersonList](t, found); len(got.Items) != 1 || got.Items[0].Id != id {
		t.Errorf("limit=abc の検索 = %+v", got)
	}
	assertExternalError(t, "一覧の limit=abc", f.get(t, "/api/v1/organizations?limit=abc"), malformed)
	if page := decode[extgen.PersonList](t, f.get(t, "/api/v1/people?limit=1")); len(page.Items) != 1 {
		t.Errorf("limit=1 の一覧 = %+v", page)
	}
}
