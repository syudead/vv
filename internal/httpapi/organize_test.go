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

func videoRefs(ids ...int64) []map[string]any {
	refs := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		refs = append(refs, map[string]any{"kind": "video", "id": id})
	}
	return refs
}

// makeGroup は所有者として「グループにする」を行い、結果を返す。
func (f *libraryFixture) makeGroup(t *testing.T, body map[string]any) gen.OrganizeResult {
	t.Helper()
	rec := f.organize("group", body, f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("グループにする: status = %d: %s", rec.Code, rec.Body)
	}
	result := decode[gen.OrganizeResult](t, rec)
	if result.UndoToken == "" || result.GroupId == nil {
		t.Fatalf("グループにするの応答 = %+v", result)
	}
	return result
}

// 200 で新しいグループの id とトークンを返す。from があればグループが落とした先の位置を取る。
// 取り消しは 204 でグループを消し、from を元に戻す。
func TestOrganizeGroupAndUndoStatuses(t *testing.T) {
	f := newLibraryFixture(t)
	id, _ := f.mixGroup(t)

	result := f.makeGroup(t, map[string]any{"items": videoRefs(f.ids["ep2"]), "target": f.ids["p1"], "from": id})
	f.assertGroupNames(t, "新しいグループ", *result.GroupId, "p1", "ep2")
	f.assertGroupNames(t, "from", id, "solo", "group:p1", "p2", "ep1")
	if rec := f.organize("undo", map[string]any{"token": result.UndoToken}, f.owner); rec.Code != http.StatusNoContent {
		t.Fatalf("取り消し: status = %d: %s", rec.Code, rec.Body)
	}
	f.assertGroupMissing(t, "取り消しのあと", *result.GroupId)
	f.assertGroupNames(t, "取り消しのあとの from", id, "solo", "p1", "p2", "ep1", "ep2")

	loose := f.makeGroup(t, map[string]any{"items": videoRefs(f.ids["solo"]), "target": f.ids["ep1"]})
	f.assertGroupNames(t, "from の無いグループ", *loose.GroupId, "ep1", "solo")
}

// 本文の誤りとグループを含む items は 400 invalid_request、無い動画・無い from・from に無い動画は
// 404 group_target_not_found で、どれも何も変えない。ゲストは 401。
func TestOrganizeGroupErrors(t *testing.T) {
	f := newLibraryFixture(t)
	id, _ := f.mixGroup(t)
	pair := f.groupID(t, "pair")
	for _, tc := range []struct {
		label string
		body  map[string]any
		want  wantError
	}{
		{"items なし", map[string]any{"target": f.ids["p1"]}, wantGroupInvalid},
		{"空の items", map[string]any{"items": []any{}, "target": f.ids["p1"]}, wantGroupInvalid},
		{"グループを含む", map[string]any{"items": []map[string]any{{"kind": "group", "id": pair}}, "target": f.ids["p1"]}, wantGroupInvalid},
		{"知らない種類", map[string]any{"items": []map[string]any{{"kind": "tag", "id": 1}}, "target": f.ids["p1"]}, wantGroupInvalid},
		{"無い動画", map[string]any{"items": videoRefs(999999), "target": f.ids["p1"]}, wantGroupTargetNotFound},
		{"無い落とした先", map[string]any{"items": videoRefs(f.ids["p1"]), "target": 999999}, wantGroupTargetNotFound},
		{"無い from", map[string]any{"items": videoRefs(f.ids["p1"]), "target": f.ids["p2"], "from": 999999}, wantGroupTargetNotFound},
		{"from に無い動画", map[string]any{"items": videoRefs(f.ids["solo"]), "target": f.ids["p2"], "from": pair}, wantGroupTargetNotFound},
	} {
		assertGroupError(t, tc.label, f.organize("group", tc.body, f.owner), tc.want)
	}
	assertUnauthenticated(t, "ゲスト", f.organize("group", map[string]any{"items": videoRefs(f.ids["ep2"]), "target": f.ids["p1"], "from": id}))
	f.assertGroupNames(t, "拒まれたあと", id, "solo", "p1", "p2", "ep1", "ep2")
	f.assertGroupNames(t, "拒まれたあとの pair", pair, "p1", "p2")
}

// undoToken なしの PATCH /api/groups/{id} は 048 と同じに答え、ありでも同じに答える。そのグループを
// 作ったトークンなら、取り消しは名前を変えたグループを消す。知らないトークンや別の操作のトークンは
// 名前変更を失敗させない。
func TestRenameGroupWithAnUndoToken(t *testing.T) {
	f := newLibraryFixture(t)
	id, items := f.mixGroup(t)
	rename := func(t *testing.T, groupID int64, body map[string]any) gen.LibraryGroup {
		t.Helper()
		rec := f.groupRequest(http.MethodPatch, groupWritePath(groupID, ""), groupJSON(body), f.owner)
		if rec.Code != http.StatusOK {
			t.Fatalf("改名 %v: status = %d: %s", body, rec.Code, rec.Body)
		}
		return decode[gen.LibraryGroup](t, rec)
	}
	result := f.makeGroup(t, map[string]any{"items": videoRefs(f.ids["ep2"]), "target": f.ids["p1"], "from": id})
	reorder := f.reorder(t, map[string]any{"groupId": id, "itemIds": []int64{items[0]}, "after": items[3]})
	plain := rename(t, *result.GroupId, map[string]any{"name": " Plain "})
	for _, token := range []string{"unknown", reorder} {
		got := rename(t, *result.GroupId, map[string]any{"name": " Plain ", "undoToken": token})
		if got.Name != "Plain" || got.Id != plain.Id || got.VideoCount != plain.VideoCount {
			t.Errorf("トークン %q の改名 = %+v, want %+v", token, got, plain)
		}
	}
	assertGroupError(t, "トークン付きの空の名前", f.groupRequest(http.MethodPatch, groupWritePath(*result.GroupId, ""),
		groupJSON(map[string]any{"name": " ", "undoToken": result.UndoToken}), f.owner), wantGroupInvalid)
	assertGroupError(t, "トークン付きの無いグループ", f.groupRequest(http.MethodPatch, groupWritePath(999999, ""),
		groupJSON(map[string]any{"name": "x", "undoToken": result.UndoToken}), f.owner), wantGroupNotFound)

	// トークンなしで名前を変えたので、取り消しは 409。新しい操作で、トークン付きの改名を確かめる。
	conflict := wantError{status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: reasonUndoConflict}
	assertGroupError(t, "トークンなしの改名のあと", f.organize("undo", map[string]any{"token": result.UndoToken}, f.owner), conflict)

	second := f.makeGroup(t, map[string]any{"items": videoRefs(f.ids["solo"]), "target": f.ids["ep1"]})
	if got := rename(t, *second.GroupId, map[string]any{"name": "Named", "undoToken": second.UndoToken}); got.Name != "Named" {
		t.Errorf("トークン付きの改名 = %+v", got)
	}
	if rec := f.organize("undo", map[string]any{"token": second.UndoToken}, f.owner); rec.Code != http.StatusNoContent {
		t.Fatalf("トークン付きの改名のあとの取り消し: status = %d: %s", rec.Code, rec.Body)
	}
	f.assertGroupMissing(t, "取り消しのあと", *second.GroupId)
}
