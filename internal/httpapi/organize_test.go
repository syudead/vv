package httpapi

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/httpapi/gen"
)

// ドラッグの操作と取り消しの経路（specs/050-drag-organize/contracts/screen-api.md「Errors」）を、
// 本物の認証・保存層・internal/app の Organizer をつないで確かめる。中身は library_test.go の
// libraryFixture である。

func (f *libraryFixture) organize(route string, body map[string]any, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	return f.groupRequest(http.MethodPost, "/api/organize/"+route, groupJSON(body), cookies...)
}

// reorder は所有者として並べ替え、取り消しのトークンを返す。
func (f *libraryFixture) reorder(t *testing.T, body map[string]any) string {
	t.Helper()
	rec := f.organize("reorder", body, f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("並べ替え: status = %d: %s", rec.Code, rec.Body)
	}
	result := decode[gen.OrganizeResult](t, rec)
	if result.UndoToken == "" || result.GroupId != nil {
		t.Fatalf("並べ替えの応答 = %+v", result)
	}
	return result.UndoToken
}

// mixGroup は 5 本の動画（solo・p1・p2・ep1・ep2）を順に持つグループを作り、その id と項目の id を返す。
func (f *libraryFixture) mixGroup(t *testing.T) (int64, []int64) {
	t.Helper()
	mix := f.createGroup(t, "Mix", []int64{f.ids["solo"], f.ids["p1"], f.ids["p2"], f.ids["ep1"], f.ids["ep2"]}, nil)
	ids, _ := f.groupItemIDs(t, mix.Id)
	return mix.Id, ids
}

func (f *libraryFixture) assertGroupNames(t *testing.T, label string, id int64, want ...string) {
	t.Helper()
	if _, names := f.groupItemIDs(t, id); !slices.Equal(names, want) {
		t.Errorf("%s: 項目 = %v, want %v", label, names, want)
	}
}

// 200 で並べ替えてトークンを返し、取り消しは 204 で元の順に戻す。同じトークンの 2 回目と知らない
// トークンは 410 undo_expired、後から並べ替えたグループの取り消しは 409 undo_conflict で何も変えない。
func TestOrganizeReorderAndUndoStatuses(t *testing.T) {
	f := newLibraryFixture(t)
	id, items := f.mixGroup(t)
	expired := wantError{status: http.StatusGone, code: gen.ErrorCodeNotFound, reason: reasonUndoExpired}
	conflict := wantError{status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: reasonUndoConflict}

	token := f.reorder(t, map[string]any{"groupId": id, "itemIds": []int64{items[2], items[4]}, "after": items[0]})
	f.assertGroupNames(t, "並べ替えのあと", id, "solo", "p2", "ep2", "p1", "ep1")
	if rec := f.organize("undo", map[string]any{"token": token}, f.owner); rec.Code != http.StatusNoContent {
		t.Fatalf("取り消し: status = %d: %s", rec.Code, rec.Body)
	}
	f.assertGroupNames(t, "取り消しのあと", id, "solo", "p1", "p2", "ep1", "ep2")
	assertGroupError(t, "2 回目", f.organize("undo", map[string]any{"token": token}, f.owner), expired)
	assertGroupError(t, "知らないトークン", f.organize("undo", map[string]any{"token": "unknown"}, f.owner), expired)

	first := f.reorder(t, map[string]any{"groupId": id, "itemIds": []int64{items[4]}})
	f.reorder(t, map[string]any{"groupId": id, "itemIds": []int64{items[0]}, "after": items[3]})
	f.assertGroupNames(t, "2 回の並べ替えのあと", id, "ep2", "p1", "p2", "ep1", "solo")
	assertGroupError(t, "変わったあと", f.organize("undo", map[string]any{"token": first}, f.owner), conflict)
	f.assertGroupNames(t, "拒まれた取り消しのあと", id, "ep2", "p1", "p2", "ep1", "solo")
}

// 本文の誤りは 400 invalid_request、無いグループ・ほかのグループの項目・無い目印は 404
// group_target_not_found で、どれも何も変えない。
func TestOrganizeReorderErrors(t *testing.T) {
	f := newLibraryFixture(t)
	id, items := f.mixGroup(t)
	other, otherItems := f.mixGroup(t)
	tooMany := make([]int64, maxVideoTagsIDs+1)
	for _, tc := range []struct {
		label string
		body  map[string]any
		want  wantError
	}{
		{"itemIds なし", map[string]any{"groupId": id}, wantGroupInvalid},
		{"空の itemIds", map[string]any{"groupId": id, "itemIds": []int64{}}, wantGroupInvalid},
		{"20001 項目", map[string]any{"groupId": id, "itemIds": tooMany}, wantGroupInvalid},
		{"目印が itemIds の中", map[string]any{"groupId": id, "itemIds": []int64{items[1], items[2]}, "after": items[2]}, wantGroupInvalid},
		{"知らない欄", map[string]any{"groupId": id, "itemIds": []int64{items[1]}, "extra": 1}, wantGroupInvalid},
		{"無いグループ", map[string]any{"groupId": 999999, "itemIds": []int64{items[1]}}, wantGroupTargetNotFound},
		{"ほかのグループの項目", map[string]any{"groupId": id, "itemIds": []int64{items[1], otherItems[0]}}, wantGroupTargetNotFound},
		{"無い目印", map[string]any{"groupId": id, "itemIds": []int64{items[1]}, "after": 999999}, wantGroupTargetNotFound},
		{"ほかのグループの目印", map[string]any{"groupId": id, "itemIds": []int64{items[1]}, "after": otherItems[0]}, wantGroupTargetNotFound},
	} {
		assertGroupError(t, tc.label, f.organize("reorder", tc.body, f.owner), tc.want)
	}
	f.assertGroupNames(t, "拒まれた並べ替えのあと", id, "solo", "p1", "p2", "ep1", "ep2")
	f.assertGroupNames(t, "ほかのグループ", other, "solo", "p1", "p2", "ep1", "ep2")

	for _, tc := range []struct {
		label string
		body  map[string]any
	}{
		{"token なし", map[string]any{}},
		{"空の token", map[string]any{"token": ""}},
	} {
		assertGroupError(t, tc.label, f.organize("undo", tc.body, f.owner), wantGroupInvalid)
	}
}

// ゲストはどちらの経路も 401 で、何も変わらず、トークンも使われない。
func TestOrganizeOwnerOnly(t *testing.T) {
	f := newLibraryFixture(t)
	id, items := f.mixGroup(t)
	token := f.reorder(t, map[string]any{"groupId": id, "itemIds": []int64{items[4]}})
	assertUnauthenticated(t, "reorder", f.organize("reorder", map[string]any{"groupId": id, "itemIds": []int64{items[0]}, "after": items[1]}))
	assertUnauthenticated(t, "undo", f.organize("undo", map[string]any{"token": token}))
	f.assertGroupNames(t, "ゲストの要求のあと", id, "ep2", "solo", "p1", "p2", "ep1")
	if rec := f.organize("undo", map[string]any{"token": token}, f.owner); rec.Code != http.StatusNoContent {
		t.Fatalf("所有者の取り消し: status = %d: %s", rec.Code, rec.Body)
	}
}

// moved は所有者としてカードを動かし、取り消しのトークンを返す。
func (f *libraryFixture) moved(t *testing.T, body map[string]any) string {
	t.Helper()
	rec := f.organize("move", body, f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("移動: status = %d: %s", rec.Code, rec.Body)
	}
	result := decode[gen.OrganizeResult](t, rec)
	if result.UndoToken == "" || result.GroupId != nil {
		t.Fatalf("移動の応答 = %+v", result)
	}
	return result.UndoToken
}

func (f *libraryFixture) undone(t *testing.T, label, token string) {
	t.Helper()
	if rec := f.organize("undo", map[string]any{"token": token}, f.owner); rec.Code != http.StatusNoContent {
		t.Fatalf("%s: status = %d: %s", label, rec.Code, rec.Body)
	}
}

// 200 でカードを A から B へ動かしてトークンを返し、取り消しは 204 で元に戻す。to が無ければ外す
// だけで、最後の項目を外した B は解散し、取り消すと同じ id で戻る。
func TestOrganizeMoveAndUndo(t *testing.T) {
	f := newLibraryFixture(t)
	a := f.createGroup(t, "A", []int64{f.ids["solo"], f.ids["p1"]}, nil)
	b := f.createGroup(t, "B", []int64{f.ids["ep1"]}, nil)
	video := func(name string) map[string]any { return map[string]any{"kind": "video", "id": f.ids[name]} }

	token := f.moved(t, map[string]any{"items": []any{video("p1")}, "from": a.Id, "to": b.Id})
	f.assertGroupNames(t, "移動のあと A", a.Id, "solo")
	f.assertGroupNames(t, "移動のあと B", b.Id, "ep1", "p1")
	f.undone(t, "取り消し", token)
	f.assertGroupNames(t, "取り消しのあと A", a.Id, "solo", "p1")
	f.assertGroupNames(t, "取り消しのあと B", b.Id, "ep1")

	token = f.moved(t, map[string]any{"items": []any{video("ep1")}, "from": b.Id})
	f.assertGroupMissing(t, "外したあと", b.Id)
	f.undone(t, "解散の取り消し", token)
	f.assertGroupNames(t, "解散の取り消しのあと", b.Id, "ep1")
}

// 本文の誤りは 400 invalid_request、無いグループ（明示した 0 以下の id も）・from に無いカードは
// 404 group_target_not_found、自分の子孫へ動かすグループは 409 group_cycle で、どれも何も変えない。
// ゲストは 401 である。
func TestOrganizeMoveErrors(t *testing.T) {
	f := newLibraryFixture(t)
	inner := f.createGroup(t, "Inner", []int64{f.ids["solo"]}, nil)
	outer := f.createGroup(t, "Outer", []int64{f.ids["p1"]}, []int64{inner.Id})
	cycle := wantError{status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: reasonGroupCycle}
	solo := map[string]any{"kind": "video", "id": f.ids["solo"]}
	tooMany := make([]any, maxVideoTagsIDs+1)
	for i := range tooMany {
		tooMany[i] = solo
	}
	for _, tc := range []struct {
		label string
		body  map[string]any
		want  wantError
	}{
		{"items なし", map[string]any{"to": outer.Id}, wantGroupInvalid},
		{"空の items", map[string]any{"items": []any{}, "to": outer.Id}, wantGroupInvalid},
		{"20001 枚", map[string]any{"items": tooMany, "to": outer.Id}, wantGroupInvalid},
		{"知らない種類", map[string]any{"items": []any{map[string]any{"kind": "tag", "id": 1}}, "to": outer.Id}, wantGroupInvalid},
		{"from も to も無い", map[string]any{"items": []any{solo}}, wantGroupInvalid},
		{"from と to が同じ", map[string]any{"items": []any{solo}, "from": inner.Id, "to": inner.Id}, wantGroupInvalid},
		{"無い to", map[string]any{"items": []any{solo}, "to": 999999}, wantGroupTargetNotFound},
		{"0 の to", map[string]any{"items": []any{solo}, "from": inner.Id, "to": 0}, wantGroupTargetNotFound},
		{"0 の from", map[string]any{"items": []any{solo}, "from": 0, "to": outer.Id}, wantGroupTargetNotFound},
		{"負の to", map[string]any{"items": []any{solo}, "from": inner.Id, "to": -1}, wantGroupTargetNotFound},
		{"from に無い", map[string]any{"items": []any{solo}, "from": outer.Id, "to": inner.Id}, wantGroupTargetNotFound},
		{"子孫へ", map[string]any{"items": []any{map[string]any{"kind": "group", "id": outer.Id}}, "to": inner.Id}, cycle},
	} {
		assertGroupError(t, tc.label, f.organize("move", tc.body, f.owner), tc.want)
	}
	f.assertGroupNames(t, "拒まれた移動のあと Outer", outer.Id, "p1", "group:Inner")
	f.assertGroupNames(t, "拒まれた移動のあと Inner", inner.Id, "solo")
	assertUnauthenticated(t, "ゲスト", f.organize("move", map[string]any{"items": []any{solo}, "to": outer.Id}))
	f.assertGroupNames(t, "ゲストの要求のあと", outer.Id, "p1", "group:Inner")
}
