package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/httpapi/gen"
)

// グループの一覧と編集の経路（specs/048-groups/contracts/screen-api.md「Statuses」）を、本物の
// 認証・保存層・経路をつないで確かめる。中身は library_test.go の libraryFixture で、show と pair が
// 自動グループ、solo がグループに入らない動画である。

func groupWritePath(id int64, suffix string) string {
	return "/api/groups/" + strconv.FormatInt(id, 10) + suffix
}

func groupJSON(body map[string]any) string {
	out, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	return string(out)
}

func (f *libraryFixture) groupRequest(method, target, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	return f.env.serve(authRequest{method: method, target: target, body: body, cookies: cookies})
}

// createGroup は所有者としてグループを作り、その値を返す。
func (f *libraryFixture) createGroup(t *testing.T, name string, videoIDs, groupIDs []int64) gen.LibraryGroup {
	t.Helper()
	rec := f.groupRequest(http.MethodPost, "/api/groups",
		groupJSON(map[string]any{"name": name, "videoIds": videoIDs, "groupIds": groupIDs}), f.owner)
	if rec.Code != http.StatusCreated {
		t.Fatalf("作成 %s: status = %d: %s", name, rec.Code, rec.Body)
	}
	return decode[gen.LibraryGroup](t, rec)
}

// groupItemIDs はグループ id の項目の id と名前（動画の題名か「group:名前」）を位置の順に返す。
func (f *libraryFixture) groupItemIDs(t *testing.T, id int64) ([]int64, []string) {
	t.Helper()
	rec := f.env.get(groupWritePath(id, ""), f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("読み出し: status = %d: %s", rec.Code, rec.Body)
	}
	detail := decode[gen.GroupDetail](t, rec)
	var ids []int64
	var names []string
	for _, item := range detail.Items {
		ids = append(ids, item.Id)
		if item.Group != nil {
			names = append(names, "group:"+item.Group.Name)
		} else {
			names = append(names, item.Video.Title)
		}
	}
	return ids, names
}

func (f *libraryFixture) assertGroupMissing(t *testing.T, label string, id int64) {
	t.Helper()
	if rec := f.env.get(groupWritePath(id, ""), f.owner); rec.Code != http.StatusNotFound {
		t.Errorf("%s: GET status = %d, want 404", label, rec.Code)
	}
}

func assertGroupError(t *testing.T, label string, rec *httptest.ResponseRecorder, want wantError) {
	t.Helper()
	assertErrorBody(t, label, rec.Code, rec.Body.Bytes(), want)
}

var (
	wantGroupNotFound       = wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound}
	wantGroupTargetNotFound = wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: reasonGroupTargetNotFound}
	wantGroupInvalid        = wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest}
)

// POST /api/groups: 201 で前後を削った名前の LibraryGroup。名前が空か長すぎるか、メンバーが無ければ
// 400、存在しない動画かグループは 404 group_target_not_found で何も作らない。
func TestCreateGroupStatuses(t *testing.T) {
	f := newLibraryFixture(t)
	pair := f.groupID(t, "pair")
	created := f.createGroup(t, "  Mix  ", []int64{f.ids["solo"]}, []int64{pair})
	if created.Name != "Mix" || created.Folder != nil || created.VideoCount != 3 ||
		!slices.Equal(created.VideoIds, []int64{f.ids["solo"], f.ids["p1"], f.ids["p2"]}) {
		t.Errorf("作成の応答 = %+v", created)
	}
	if created.WatchState == nil || created.Favorite == nil {
		t.Errorf("所有者の値が無い: %+v", created)
	}
	if _, names := f.groupItemIDs(t, created.Id); !slices.Equal(names, []string{"solo", "group:pair"}) {
		t.Errorf("項目 = %v", names)
	}

	before := decode[gen.GroupListPage](t, f.env.get("/api/groups", f.owner))
	for _, tc := range []struct {
		label string
		body  map[string]any
		want  wantError
	}{
		{"空の名前", map[string]any{"name": "   ", "videoIds": []int64{f.ids["solo"]}}, wantGroupInvalid},
		{"101 文字", map[string]any{"name": strings.Repeat("あ", 101), "videoIds": []int64{f.ids["solo"]}}, wantGroupInvalid},
		{"メンバーなし", map[string]any{"name": "Empty"}, wantGroupInvalid},
		{"空の配列", map[string]any{"name": "Empty", "videoIds": []int64{}, "groupIds": []int64{}}, wantGroupInvalid},
		{"無い動画", map[string]any{"name": "Gone", "videoIds": []int64{f.ids["solo"], 999999}}, wantGroupTargetNotFound},
		{"無いグループ", map[string]any{"name": "Gone", "groupIds": []int64{pair, 999999}}, wantGroupTargetNotFound},
	} {
		assertGroupError(t, tc.label, f.groupRequest(http.MethodPost, "/api/groups", groupJSON(tc.body), f.owner), tc.want)
	}
	after := decode[gen.GroupListPage](t, f.env.get("/api/groups", f.owner))
	if len(after.Groups) != len(before.Groups) {
		t.Errorf("失敗した作成のあとのグループ数 = %d, want %d", len(after.Groups), len(before.Groups))
	}
}

// PATCH /api/groups/{id}: 200 で前後を削った名前。無ければ 404 not_found、空の名前は 400。
// DELETE /api/groups/{id}: 204 で解散し、子グループと動画は残る。無ければ 404 not_found。
func TestRenameAndDissolveGroupStatuses(t *testing.T) {
	f := newLibraryFixture(t)
	pair := f.groupID(t, "pair")
	mix := f.createGroup(t, "Mix", []int64{f.ids["solo"]}, []int64{pair})

	rec := f.groupRequest(http.MethodPatch, groupWritePath(mix.Id, ""), groupJSON(map[string]any{"name": " Renamed "}), f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("改名: status = %d: %s", rec.Code, rec.Body)
	}
	if got := decode[gen.LibraryGroup](t, rec); got.Name != "Renamed" || got.Id != mix.Id || got.VideoCount != 3 {
		t.Errorf("改名の応答 = %+v", got)
	}
	assertGroupError(t, "空の名前", f.groupRequest(http.MethodPatch, groupWritePath(mix.Id, ""), groupJSON(map[string]any{"name": " "}), f.owner), wantGroupInvalid)
	assertGroupError(t, "無いグループの改名", f.groupRequest(http.MethodPatch, groupWritePath(999999, ""), groupJSON(map[string]any{"name": "x"}), f.owner), wantGroupNotFound)

	rec = f.groupRequest(http.MethodDelete, groupWritePath(mix.Id, ""), "", f.owner)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("解散: status = %d: %s", rec.Code, rec.Body)
	}
	f.assertGroupMissing(t, "解散したグループ", mix.Id)
	if rec := f.env.get(groupWritePath(pair, ""), f.owner); rec.Code != http.StatusOK {
		t.Errorf("子グループ pair: status = %d, want 200", rec.Code)
	}
	if rec := f.env.get("/api/videos/"+strconv.FormatInt(f.ids["solo"], 10), f.owner); rec.Code != http.StatusOK {
		t.Errorf("動画 solo: status = %d, want 200", rec.Code)
	}
	assertGroupError(t, "解散済みの解散", f.groupRequest(http.MethodDelete, groupWritePath(mix.Id, ""), "", f.owner), wantGroupNotFound)
}

// POST /api/groups/{id}/items: 200 で足した後の値。存在しない動画かグループは 404
// group_target_not_found、自分か祖先は 409 group_cycle で、どちらも何も足さない。
func TestAddGroupItemsStatuses(t *testing.T) {
	f := newLibraryFixture(t)
	pair := f.groupID(t, "pair")
	show := f.groupID(t, "show")
	outer := f.createGroup(t, "Outer", nil, []int64{pair})

	rec := f.groupRequest(http.MethodPost, groupWritePath(outer.Id, "/items"),
		groupJSON(map[string]any{"videoIds": []int64{f.ids["solo"]}, "groupIds": []int64{show, pair}}), f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("追加: status = %d: %s", rec.Code, rec.Body)
	}
	if got := decode[gen.LibraryGroup](t, rec); got.VideoCount != 6 {
		t.Errorf("追加の応答の本数 = %d, want 6", got.VideoCount)
	}
	_, want := f.groupItemIDs(t, outer.Id)
	if !slices.Equal(want, []string{"group:pair", "solo", "group:show"}) {
		t.Fatalf("項目 = %v", want)
	}

	cycle := wantError{status: http.StatusConflict, code: gen.ErrorCodeConflict, reason: reasonGroupCycle}
	for _, tc := range []struct {
		label string
		id    int64
		body  map[string]any
		want  wantError
	}{
		{"無い動画", outer.Id, map[string]any{"videoIds": []int64{999999}}, wantGroupTargetNotFound},
		{"無いグループ", outer.Id, map[string]any{"groupIds": []int64{999999}}, wantGroupTargetNotFound},
		{"自分自身", outer.Id, map[string]any{"groupIds": []int64{outer.Id}}, cycle},
		{"祖先", pair, map[string]any{"groupIds": []int64{outer.Id}}, cycle},
		{"空の本文", outer.Id, map[string]any{}, wantGroupInvalid},
		{"無いグループへ", 999999, map[string]any{"videoIds": []int64{f.ids["solo"]}}, wantGroupNotFound},
	} {
		assertGroupError(t, tc.label, f.groupRequest(http.MethodPost, groupWritePath(tc.id, "/items"), groupJSON(tc.body), f.owner), tc.want)
	}
	if _, got := f.groupItemIDs(t, outer.Id); !slices.Equal(got, want) {
		t.Errorf("失敗した追加のあとの項目 = %v, want %v", got, want)
	}
	if _, got := f.groupItemIDs(t, pair); !slices.Equal(got, []string{"p1", "p2"}) {
		t.Errorf("循環を拒んだあとの pair = %v", got)
	}
}

// POST /api/groups/{id}/items/remove: 200 で外した後の値。グループに無い項目は 404
// group_target_not_found で何も外さない。全部外したグループは解散され、本文の無い 204 で、その後の
// GET は 404。外した動画は残る。
func TestRemoveGroupItemsStatuses(t *testing.T) {
	f := newLibraryFixture(t)
	pair := f.groupID(t, "pair")
	mix := f.createGroup(t, "Mix", []int64{f.ids["solo"], f.ids["ep1"]}, []int64{pair})
	ids, _ := f.groupItemIDs(t, mix.Id)
	remove := func(id int64, itemIDs []int64) *httptest.ResponseRecorder {
		return f.groupRequest(http.MethodPost, groupWritePath(id, "/items/remove"), groupJSON(map[string]any{"itemIds": itemIDs}), f.owner)
	}

	assertGroupError(t, "グループに無い項目", remove(mix.Id, []int64{ids[0], 999999}), wantGroupTargetNotFound)
	if got, _ := f.groupItemIDs(t, mix.Id); !slices.Equal(got, ids) {
		t.Errorf("失敗した除去のあとの項目 = %v, want %v", got, ids)
	}
	assertGroupError(t, "空の itemIds", remove(mix.Id, []int64{}), wantGroupInvalid)
	assertGroupError(t, "無いグループ", remove(999999, []int64{ids[0]}), wantGroupNotFound)

	rec := remove(mix.Id, []int64{ids[2]})
	if rec.Code != http.StatusOK {
		t.Fatalf("除去: status = %d: %s", rec.Code, rec.Body)
	}
	if got := decode[gen.LibraryGroup](t, rec); !slices.Equal(got.VideoIds, []int64{f.ids["solo"], f.ids["ep1"]}) {
		t.Errorf("除去の応答 = %v", got.VideoIds)
	}
	rec = remove(mix.Id, ids[:2])
	if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
		t.Fatalf("全部の除去: status = %d: %s", rec.Code, rec.Body)
	}
	f.assertGroupMissing(t, "空にしたグループ", mix.Id)
	if rec := f.env.get(groupWritePath(pair, ""), f.owner); rec.Code != http.StatusOK {
		t.Errorf("外した子グループ pair: status = %d, want 200", rec.Code)
	}
	if rec := f.env.get("/api/videos/"+strconv.FormatInt(f.ids["solo"], 10), f.owner); rec.Code != http.StatusOK {
		t.Errorf("外した動画 solo: status = %d, want 200", rec.Code)
	}
}

// POST /api/groups/{id}/items/{itemId}/move: 204。after が自分なら 400、項目か after がグループに
// 無ければ 404 group_target_not_found。after が無ければ先頭へ移す。
func TestMoveGroupItemStatuses(t *testing.T) {
	f := newLibraryFixture(t)
	mix := f.createGroup(t, "Mix", []int64{f.ids["solo"], f.ids["ep1"], f.ids["p1"]}, nil)
	ids, _ := f.groupItemIDs(t, mix.Id)
	move := func(id, itemID int64, body string) *httptest.ResponseRecorder {
		return f.groupRequest(http.MethodPost, groupWritePath(id, fmt.Sprintf("/items/%d/move", itemID)), body, f.owner)
	}

	if rec := move(mix.Id, ids[0], fmt.Sprintf(`{"after":%d}`, ids[2])); rec.Code != http.StatusNoContent {
		t.Fatalf("移動: status = %d: %s", rec.Code, rec.Body)
	}
	if _, names := f.groupItemIDs(t, mix.Id); !slices.Equal(names, []string{"ep1", "p1", "solo"}) {
		t.Errorf("移動のあと = %v", names)
	}
	if rec := move(mix.Id, ids[0], `{}`); rec.Code != http.StatusNoContent {
		t.Fatalf("先頭へ: status = %d: %s", rec.Code, rec.Body)
	}
	if _, names := f.groupItemIDs(t, mix.Id); !slices.Equal(names, []string{"solo", "ep1", "p1"}) {
		t.Errorf("先頭へ移したあと = %v", names)
	}
	assertGroupError(t, "自分の後ろ", move(mix.Id, ids[1], fmt.Sprintf(`{"after":%d}`, ids[1])), wantGroupInvalid)
	assertGroupError(t, "無い項目", move(mix.Id, 999999, `{}`), wantGroupTargetNotFound)
	assertGroupError(t, "無い after", move(mix.Id, ids[1], `{"after":999999}`), wantGroupTargetNotFound)
	assertGroupError(t, "無いグループ", move(999999, ids[1], `{}`), wantGroupNotFound)
	if _, names := f.groupItemIDs(t, mix.Id); !slices.Equal(names, []string{"solo", "ep1", "p1"}) {
		t.Errorf("失敗した移動のあと = %v", names)
	}
}

// POST /api/groups/{id}/tag: 200 で { tag, created }、グループは解散する。名前がタグ名の規則に
// 反すれば 400 で何も変えない。無ければ 404 not_found。
func TestTagGroupStatuses(t *testing.T) {
	f := newLibraryFixture(t)
	mix := f.createGroup(t, "Mixed", []int64{f.ids["solo"]}, nil)
	rec := f.groupRequest(http.MethodPost, groupWritePath(mix.Id, "/tag"), "", f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("タグ化: status = %d: %s", rec.Code, rec.Body)
	}
	if got := decode[gen.GroupTagResult](t, rec); got.Tag.Name != "Mixed" || !got.Created {
		t.Errorf("タグ化の応答 = %+v", got)
	}
	f.assertGroupMissing(t, "タグにしたグループ", mix.Id)

	again := f.createGroup(t, "Mixed", []int64{f.ids["p1"]}, nil)
	rec = f.groupRequest(http.MethodPost, groupWritePath(again.Id, "/tag"), "", f.owner)
	if got := decode[gen.GroupTagResult](t, rec); rec.Code != http.StatusOK || got.Created {
		t.Errorf("既存のタグ: status = %d・%+v", rec.Code, got)
	}

	bad := f.createGroup(t, "a\u0001b", []int64{f.ids["solo"]}, nil)
	assertGroupError(t, "タグ名に使えない名前", f.groupRequest(http.MethodPost, groupWritePath(bad.Id, "/tag"), "", f.owner),
		wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonTagNameControlCharacters})
	if rec := f.env.get(groupWritePath(bad.Id, ""), f.owner); rec.Code != http.StatusOK {
		t.Errorf("断ったあとのグループ: status = %d, want 200", rec.Code)
	}
	assertGroupError(t, "無いグループ", f.groupRequest(http.MethodPost, groupWritePath(999999, "/tag"), "", f.owner), wantGroupNotFound)
}

func groupListNames(page gen.GroupListPage) []string {
	names := make([]string, 0, len(page.Groups))
	for _, group := range page.Groups {
		names = append(names, group.Name)
	}
	return names
}

// GET /api/groups: 見る人に見せるグループをどの深さでも自然な名前順に、q は大文字小文字を無視して
// 名前に含むもの。ゲストは公開の動画のあるグループだけ。読めないカーソルは 400 invalid_cursor。
func TestListGroupsStatuses(t *testing.T) {
	f := newLibraryFixture(t)
	pair := f.groupID(t, "pair")
	f.createGroup(t, "Season 10", nil, []int64{pair})
	f.createGroup(t, "Season 2", []int64{f.ids["solo"]}, nil)
	f.createGroup(t, "Private", []int64{f.ids["p2"]}, nil)
	f.createGroup(t, "Emptied", []int64{f.ids["ep10"]}, nil)
	emptied := decode[gen.GroupListPage](t, f.env.get("/api/groups?q=emptied", f.owner)).Groups[0].Id
	ids, _ := f.groupItemIDs(t, emptied)
	if rec := f.groupRequest(http.MethodPost, groupWritePath(emptied, "/items/remove"), groupJSON(map[string]any{"itemIds": ids}), f.owner); rec.Code != http.StatusNoContent {
		t.Fatalf("空にする（解散）: status = %d: %s", rec.Code, rec.Body)
	}

	rec := f.env.get("/api/groups", f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("一覧: status = %d: %s", rec.Code, rec.Body)
	}
	if got := groupListNames(decode[gen.GroupListPage](t, rec)); !slices.Equal(got, []string{"pair", "Private", "Season 2", "Season 10", "show"}) {
		t.Errorf("所有者の一覧 = %v", got)
	}
	if got := groupListNames(decode[gen.GroupListPage](t, f.env.get("/api/groups?q="+url.QueryEscape("  SEASON "), f.owner))); !slices.Equal(got, []string{"Season 2", "Season 10"}) {
		t.Errorf("q=SEASON = %v", got)
	}

	var paged []string
	cursor := ""
	for range 10 {
		page := decode[gen.GroupListPage](t, f.env.get("/api/groups?limit=2&cursor="+url.QueryEscape(cursor), f.owner))
		paged = append(paged, groupListNames(page)...)
		if page.NextCursor == nil {
			break
		}
		cursor = *page.NextCursor
	}
	if !slices.Equal(paged, []string{"pair", "Private", "Season 2", "Season 10", "show"}) {
		t.Errorf("ページを辿った一覧 = %v", paged)
	}

	guest := f.env.get("/api/groups")
	if guest.Code != http.StatusOK {
		t.Fatalf("ゲストの一覧: status = %d: %s", guest.Code, guest.Body)
	}
	guestPage := decode[gen.GroupListPage](t, guest)
	if got := groupListNames(guestPage); !slices.Equal(got, []string{"pair", "Season 2", "Season 10", "show"}) {
		t.Errorf("ゲストの一覧 = %v", got)
	}
	for _, group := range guestPage.Groups {
		if group.WatchState != nil || group.Favorite != nil || len(group.Tags) != 0 {
			t.Errorf("ゲストに所有者の値: %+v", group)
		}
	}

	rec = f.env.get("/api/groups?cursor=broken", f.owner)
	assertGroupError(t, "読めないカーソル", rec, wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonInvalidCursor})
	for _, query := range []string{"limit=0", "limit=201"} {
		assertGroupError(t, query, f.env.get("/api/groups?"+query, f.owner), wantGroupInvalid)
	}
}

// ゲストはどの書き込みでも 401 で、何も変わらない。
func TestGroupWritesOwnerOnly(t *testing.T) {
	f := newLibraryFixture(t)
	mix := f.createGroup(t, "Mix", []int64{f.ids["solo"], f.ids["p1"]}, nil)
	ids, _ := f.groupItemIDs(t, mix.Id)
	for _, req := range []struct{ method, target, body string }{
		{http.MethodPost, "/api/groups", groupJSON(map[string]any{"name": "Guest", "videoIds": []int64{f.ids["solo"]}})},
		{http.MethodPatch, groupWritePath(mix.Id, ""), groupJSON(map[string]any{"name": "Guest"})},
		{http.MethodDelete, groupWritePath(mix.Id, ""), ""},
		{http.MethodPost, groupWritePath(mix.Id, "/items"), groupJSON(map[string]any{"videoIds": []int64{f.ids["ep1"]}})},
		{http.MethodPost, groupWritePath(mix.Id, "/items/remove"), groupJSON(map[string]any{"itemIds": ids[:1]})},
		{http.MethodPost, groupWritePath(mix.Id, fmt.Sprintf("/items/%d/move", ids[1])), `{}`},
		{http.MethodPost, groupWritePath(mix.Id, "/tag"), ""},
	} {
		assertUnauthenticated(t, req.method+" "+req.target, f.groupRequest(req.method, req.target, req.body))
	}
	if gotIDs, names := f.groupItemIDs(t, mix.Id); !slices.Equal(gotIDs, ids) || !slices.Equal(names, []string{"solo", "p1"}) {
		t.Errorf("ゲストの要求のあと = %v", names)
	}
	if got := groupListNames(decode[gen.GroupListPage](t, f.env.get("/api/groups?q=guest", f.owner))); len(got) != 0 {
		t.Errorf("ゲストが作ったグループ = %v", got)
	}
}

// videoIds・groupIds・itemIds の上限 20000 は、契約の maxItems どおり重複を除く前の配列の長さで
// 数える。同じ id を 20001 個並べた本文も 400 too_many_videos である。
func TestGroupWritesCountLimitBeforeDeduplicating(t *testing.T) {
	f := newLibraryFixture(t)
	mix := f.createGroup(t, "Mix", []int64{f.ids["solo"]}, nil)
	ids, _ := f.groupItemIDs(t, mix.Id)
	tooMany := wantError{
		status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonTooManyVideos, limit: maxVideoTagsIDs,
	}
	repeat := func(id int64) []int64 { return slices.Repeat([]int64{id}, maxVideoTagsIDs+1) }
	for _, tc := range []struct {
		label, target string
		body          map[string]any
	}{
		{"作成の videoIds", "/api/groups", map[string]any{"name": "Dup", "videoIds": repeat(f.ids["solo"])}},
		{"追加の groupIds", groupWritePath(mix.Id, "/items"), map[string]any{"groupIds": repeat(f.groupID(t, "pair"))}},
		{"除去の itemIds", groupWritePath(mix.Id, "/items/remove"), map[string]any{"itemIds": repeat(ids[0])}},
	} {
		assertGroupError(t, tc.label, f.groupRequest(http.MethodPost, tc.target, groupJSON(tc.body), f.owner), tooMany)
	}
	if got, _ := f.groupItemIDs(t, mix.Id); !slices.Equal(got, ids) {
		t.Errorf("拒んだあとの項目 = %v, want %v", got, ids)
	}
}

// お気に入りの自動グループから項目を全部外すと、そのグループは解散されて（204）お気に入りからも
// 消え、フォルダの grouping は ungroup になる（specs/048-groups/data-model.md「Edits」の Remove）。
func TestRemovingEveryItemDissolvesGroup(t *testing.T) {
	f := newLibraryFixture(t)
	pair := f.groupID(t, "pair")
	rec := f.putFavorites(favoritesBody(true, nil, pair), f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("お気に入り: status = %d: %s", rec.Code, rec.Body)
	}
	ids, _ := f.groupItemIDs(t, pair)
	rec = f.groupRequest(http.MethodPost, groupWritePath(pair, "/items/remove"), groupJSON(map[string]any{"itemIds": ids}), f.owner)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("全部の除去: status = %d: %s", rec.Code, rec.Body)
	}
	f.assertGroupMissing(t, "空にしたお気に入りのグループ", pair)
	if got := libraryItemNames(f.libraryPage(t, "favorite=true")); len(got) != 0 {
		t.Errorf("お気に入りに残った = %+v", got)
	}
	assertGrouping(t, "空にしたフォルダ", f.ownerFolder(t, "pair").Folder.Grouping, gen.FolderGroupingModeUngroup, false)
}
