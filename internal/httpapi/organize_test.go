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

func organizeCard(kind string, id int64) map[string]any {
	return map[string]any{"kind": kind, "id": id}
}

// マージは 200 でトークンを返し、マージ先が自分の項目、続いて動画、ドラッグしたグループの項目の順に
// なり、ドラッグしたグループは消える。取り消しは 204 で両方のグループを元に戻す。
func TestOrganizeMergeAndUndo(t *testing.T) {
	f := newLibraryFixture(t)
	into := f.createGroup(t, "Into", []int64{f.ids["solo"]}, nil)
	dragged := f.createGroup(t, "Dragged", []int64{f.ids["ep1"], f.ids["ep2"]}, nil)

	rec := f.organize("merge", map[string]any{
		"items": []any{organizeCard("video", f.ids["p1"]), organizeCard("group", dragged.Id)},
		"into":  into.Id,
	}, f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("マージ: status = %d: %s", rec.Code, rec.Body)
	}
	result := decode[gen.OrganizeResult](t, rec)
	if result.UndoToken == "" || result.GroupId != nil {
		t.Fatalf("マージの応答 = %+v", result)
	}
	f.assertGroupNames(t, "マージのあと", into.Id, "solo", "p1", "ep1", "ep2")
	assertGroupError(t, "消えたグループ", f.env.get(groupWritePath(dragged.Id, ""), f.owner), wantGroupNotFound)

	if rec := f.organize("undo", map[string]any{"token": result.UndoToken}, f.owner); rec.Code != http.StatusNoContent {
		t.Fatalf("取り消し: status = %d: %s", rec.Code, rec.Body)
	}
	f.assertGroupNames(t, "取り消しのあと", into.Id, "solo")
	f.assertGroupNames(t, "戻ったグループ", dragged.Id, "ep1", "ep2")
}

// 本文の誤りとグループの無い items は 400 invalid_request、無いマージ先・グループ・動画は 404
// group_target_not_found、マージ先そのものか祖先のグループは 409 group_cycle で、どれも何も変えない。
// ゲストは 401。
func TestOrganizeMergeErrors(t *testing.T) {
	f := newLibraryFixture(t)
	into := f.createGroup(t, "Into", []int64{f.ids["solo"]}, nil)
	parent := f.createGroup(t, "Parent", nil, []int64{into.Id})
	other := f.createGroup(t, "Other", []int64{f.ids["p1"]}, nil)
	cycle := wantError{status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: reasonGroupCycle}
	tooMany := make([]any, maxVideoTagsIDs+1)
	for i := range tooMany {
		tooMany[i] = organizeCard("group", other.Id)
	}
	for _, tc := range []struct {
		label string
		body  map[string]any
		want  wantError
	}{
		{"items なし", map[string]any{"into": into.Id}, wantGroupInvalid},
		{"空の items", map[string]any{"items": []any{}, "into": into.Id}, wantGroupInvalid},
		{"20001 枚", map[string]any{"items": tooMany, "into": into.Id}, wantGroupInvalid},
		{"動画だけ", map[string]any{"items": []any{organizeCard("video", f.ids["p2"])}, "into": into.Id}, wantGroupInvalid},
		{"知らない種類", map[string]any{"items": []any{organizeCard("tag", 1), organizeCard("group", other.Id)}, "into": into.Id}, wantGroupInvalid},
		{"知らない欄", map[string]any{"items": []any{organizeCard("group", other.Id)}, "into": into.Id, "extra": 1}, wantGroupInvalid},
		{"無いマージ先", map[string]any{"items": []any{organizeCard("group", other.Id)}, "into": 999999}, wantGroupTargetNotFound},
		{"無いグループ", map[string]any{"items": []any{organizeCard("group", 999999)}, "into": into.Id}, wantGroupTargetNotFound},
		{"無い動画", map[string]any{"items": []any{organizeCard("video", 999999), organizeCard("group", other.Id)}, "into": into.Id}, wantGroupTargetNotFound},
		{"自分自身", map[string]any{"items": []any{organizeCard("group", into.Id)}, "into": into.Id}, cycle},
		{"祖先", map[string]any{"items": []any{organizeCard("group", parent.Id)}, "into": into.Id}, cycle},
	} {
		assertGroupError(t, tc.label, f.organize("merge", tc.body, f.owner), tc.want)
	}
	assertUnauthenticated(t, "merge", f.organize("merge", map[string]any{"items": []any{organizeCard("group", other.Id)}, "into": into.Id}))
	f.assertGroupNames(t, "拒まれたマージのあと", into.Id, "solo")
	f.assertGroupNames(t, "ドラッグしたグループ", other.Id, "p1")
}
