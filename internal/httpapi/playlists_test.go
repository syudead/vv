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

// プレイリストの管理の経路（specs/044-playlists/contracts/screen-api.md）を、本物の認証・保存層・
// 経路をつないで確かめる。中身は library_test.go の libraryFixture と同じ。

func (f *libraryFixture) playlistRequest(method, target, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	return f.env.serve(authRequest{method: method, target: target, body: body, cookies: cookies})
}

func playlistPath(id int64, suffix string) string {
	return "/api/playlists/" + strconv.FormatInt(id, 10) + suffix
}

func playlistItemPath(id, itemID int64, suffix string) string {
	return playlistPath(id, "/items/"+strconv.FormatInt(itemID, 10)+suffix)
}

func playlistNameBody(name string) string {
	out, err := json.Marshal(map[string]string{"name": name})
	if err != nil {
		panic(err)
	}
	return string(out)
}

// playlistAddBody は POST /api/playlists/{id}/items の本文を作る。
func playlistAddBody(videoIDs []int64, groupIDs ...int64) string {
	body := map[string]any{}
	if videoIDs != nil {
		body["videoIds"] = videoIDs
	}
	if groupIDs != nil {
		body["groupIds"] = groupIDs
	}
	out, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	return string(out)
}

func playlistMoveBody(after *int64) string {
	if after == nil {
		return `{"afterItemId":null}`
	}
	return fmt.Sprintf(`{"afterItemId":%d}`, *after)
}

// createPlaylist は所有者としてプレイリストを作り、その要約を返す。
func (f *libraryFixture) createPlaylist(t *testing.T, name string) gen.PlaylistSummary {
	t.Helper()
	rec := f.playlistRequest(http.MethodPost, "/api/playlists", playlistNameBody(name), f.owner)
	if rec.Code != http.StatusCreated {
		t.Fatalf("作成: status = %d: %s", rec.Code, rec.Body)
	}
	return decode[gen.PlaylistSummary](t, rec)
}

func (f *libraryFixture) getPlaylist(t *testing.T, id int64) gen.Playlist {
	t.Helper()
	rec := f.env.get(playlistPath(id, ""), f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("読み出し: status = %d: %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Cache-Control"); got != cacheNoStore {
		t.Errorf("Cache-Control = %q", got)
	}
	return decode[gen.Playlist](t, rec)
}

// playlistItemNames は項目を「group:名前」か動画の題名にして位置の順に返す。
func playlistItemNames(playlist gen.Playlist) []string {
	names := make([]string, 0, len(playlist.Items))
	for _, item := range playlist.Items {
		switch item.Kind {
		case gen.PlaylistItemKindGroup:
			names = append(names, "group:"+item.Group.Name)
		default:
			names = append(names, item.Video.Title)
		}
	}
	return names
}

// 作成は 201 でトリムした名前、空の名前と 101 文字は 400 invalid_request。名前の変更は 200 で
// トリムした名前を返し、一覧は更新日時の新しい順に並ぶ。
func TestPlaylistCreateRenameAndList(t *testing.T) {
	f := newLibraryFixture(t)

	created := f.createPlaylist(t, "  朝のまとめ  ")
	if created.Name != "朝のまとめ" || created.ItemCount != 0 || created.Id < 1 {
		t.Errorf("作成の応答 = %+v", created)
	}
	for label, name := range map[string]string{
		"空":      "",
		"空白だけ":   " \t　 ",
		"101 文字": strings.Repeat("あ", 101),
	} {
		rec := f.playlistRequest(http.MethodPost, "/api/playlists", playlistNameBody(name), f.owner)
		assertErrorBody(t, "作成 "+label, rec.Code, rec.Body.Bytes(), wantError{
			status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest,
		})
	}
	other := f.createPlaylist(t, strings.Repeat("あ", 100))

	rec := f.playlistRequest(http.MethodPatch, playlistPath(created.Id, ""), playlistNameBody(" 夜のまとめ "), f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("名前の変更: status = %d: %s", rec.Code, rec.Body)
	}
	if renamed := decode[gen.PlaylistSummary](t, rec); renamed.Id != created.Id || renamed.Name != "夜のまとめ" {
		t.Errorf("名前の変更の応答 = %+v", renamed)
	}
	rec = f.playlistRequest(http.MethodPatch, playlistPath(created.Id, ""), playlistNameBody("  "), f.owner)
	assertErrorBody(t, "空の名前への変更", rec.Code, rec.Body.Bytes(), wantError{
		status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest,
	})

	rec = f.env.get("/api/playlists", f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("一覧: status = %d: %s", rec.Code, rec.Body)
	}
	list := decode[gen.PlaylistList](t, rec)
	ids := make([]int64, 0, len(list.Items))
	for _, item := range list.Items {
		ids = append(ids, item.Id)
	}
	// 並び順（更新日時、同じミリ秒なら id）は保存層の試験が確かめる。ここでは両方が並ぶことと、
	// 名前が変わったことを確かめる。
	if len(ids) != 2 || !slices.Contains(ids, created.Id) || !slices.Contains(ids, other.Id) {
		t.Fatalf("一覧の id = %v", ids)
	}
	for _, item := range list.Items {
		if item.Id == created.Id && item.Name != "夜のまとめ" {
			t.Errorf("一覧の名前 = %q", item.Name)
		}
	}
	if list.Total != 2 || list.NextCursor != nil {
		t.Errorf("1 ページに収まる一覧の total = %d, nextCursor = %v", list.Total, list.NextCursor)
	}
}

// 一覧は limit 件ずつ返し、続きがあるときだけ nextCursor を入れ、total はどのページでも全部の数、
// version はどのページでも同じで、プレイリストが変わると変わる。limit の範囲外は 400
// invalid_request、読めない cursor は 400 invalid_cursor
// （specs/044-playlists/contracts/screen-api.md「GET /api/playlists」）。
func TestPlaylistListPaging(t *testing.T) {
	f := newLibraryFixture(t)
	var want []int64
	for _, name := range []string{"a", "b", "c"} {
		want = append([]int64{f.createPlaylist(t, name).Id}, want...)
	}

	var got []int64
	var version string
	query := "?limit=2"
	for pages := 0; ; pages++ {
		rec := f.env.get("/api/playlists"+query, f.owner)
		if rec.Code != http.StatusOK {
			t.Fatalf("ページ %d: status = %d: %s", pages, rec.Code, rec.Body)
		}
		list := decode[gen.PlaylistList](t, rec)
		if list.Total != 3 {
			t.Errorf("ページ %d の total = %d, want 3", pages, list.Total)
		}
		if list.Version == "" || (pages > 0 && list.Version != version) {
			t.Errorf("ページ %d の version = %q, 1 ページ目 = %q", pages, list.Version, version)
		}
		version = list.Version
		for _, item := range list.Items {
			if item.Covers == nil {
				t.Errorf("ページ %d の %q に covers が無い", pages, item.Name)
			}
			got = append(got, item.Id)
		}
		if list.NextCursor == nil {
			if len(list.Items) != 1 {
				t.Errorf("最後のページ = %d 件, want 1", len(list.Items))
			}
			break
		}
		if len(list.Items) != 2 {
			t.Errorf("ページ %d = %d 件, want 2", pages, len(list.Items))
		}
		query = "?limit=2&cursor=" + url.QueryEscape(*list.NextCursor)
		if pages > 3 {
			t.Fatal("ページが終わらない")
		}
	}
	if !slices.Equal(got, want) {
		t.Errorf("ページを結んだ id = %v, want %v", got, want)
	}
	f.createPlaylist(t, "d")
	if changed := decode[gen.PlaylistList](t, f.env.get("/api/playlists?limit=2", f.owner)); changed.Version == version {
		t.Errorf("作ったあとも version が同じ: %q", changed.Version)
	}

	for label, query := range map[string]string{"limit 0": "?limit=0", "limit 201": "?limit=201"} {
		rec := f.env.get("/api/playlists"+query, f.owner)
		assertErrorBody(t, label, rec.Code, rec.Body.Bytes(), wantError{
			status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest,
		})
	}
	rec := f.env.get("/api/playlists?cursor=not-a-cursor", f.owner)
	assertErrorBody(t, "読めない cursor", rec.Code, rec.Body.Bytes(), wantError{
		status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonInvalidCursor,
	})
}

// 追加は 200 で added と existing を返し、要求の順（動画、次にグループ）で末尾に足す。ライブラリに
// 無い動画の id と無いグループの id はどちらにも数えない。読み出しの項目は所有者の値を持つ。
func TestPlaylistAddAndRead(t *testing.T) {
	f := newLibraryFixture(t)
	playlist := f.createPlaylist(t, "まとめ")
	show := f.groupID(t, "show")

	rec := f.playlistRequest(http.MethodPost, playlistPath(playlist.Id, "/items"), playlistAddBody(
		[]int64{f.ids["solo"], f.ids["ep2"], 99999, f.ids["solo"]}, show, 99999, show,
	), f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("追加: status = %d: %s", rec.Code, rec.Body)
	}
	if got := decode[gen.PlaylistItemsAdded](t, rec); got.Added != 3 || got.Existing != 0 {
		t.Errorf("追加の応答 = %+v, want 3・0", got)
	}
	rec = f.playlistRequest(http.MethodPost, playlistPath(playlist.Id, "/items"),
		playlistAddBody([]int64{f.ids["solo"], f.ids["p1"]}, show), f.owner)
	if got := decode[gen.PlaylistItemsAdded](t, rec); rec.Code != http.StatusOK || got.Added != 1 || got.Existing != 2 {
		t.Errorf("2 度目の追加: status = %d・応答 = %+v, want 1・2", rec.Code, got)
	}

	read := f.getPlaylist(t, playlist.Id)
	if names := playlistItemNames(read); !slices.Equal(names, []string{"solo", "ep2", "group:show", "p1"}) {
		t.Fatalf("項目 = %v", names)
	}
	if read.ItemCount != 4 || read.Name != "まとめ" {
		t.Errorf("要約 = %d・%q", read.ItemCount, read.Name)
	}
	ep2 := read.Items[1].Video
	if ep2.Progress == nil || ep2.Progress.PositionMs != 30_000 || ep2.Favorite == nil {
		t.Errorf("ep2 の所有者の値 = progress %+v・favorite %v", ep2.Progress, ep2.Favorite)
	}
	group := read.Items[2].Group
	if read.Items[2].Video != nil || group.WatchState == nil || group.WatchedCount == nil || *group.WatchedCount != 1 ||
		group.Favorite == nil || group.OpenVideoId != f.ids["ep2"] {
		t.Errorf("グループの項目 = %+v", read.Items[2])
	}
}

// 並べ替えは 204 で、null は先頭、自分の後ろは 400、プレイリストに無い項目は 404。項目を外すと
// 204 で、もう一度外すと 404。外しても動画とその再生位置は変わらない。
func TestPlaylistMoveAndRemove(t *testing.T) {
	f := newLibraryFixture(t)
	playlist := f.createPlaylist(t, "まとめ")
	rec := f.playlistRequest(http.MethodPost, playlistPath(playlist.Id, "/items"),
		playlistAddBody([]int64{f.ids["solo"], f.ids["ep2"], f.ids["p1"]}), f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("追加: status = %d: %s", rec.Code, rec.Body)
	}
	items := f.getPlaylist(t, playlist.Id).Items
	solo, ep2, p1 := items[0].Id, items[1].Id, items[2].Id

	move := func(itemID int64, body string) *httptest.ResponseRecorder {
		return f.playlistRequest(http.MethodPost, playlistItemPath(playlist.Id, itemID, "/move"), body, f.owner)
	}
	assertStatus(t, "先頭へ", move(p1, playlistMoveBody(nil)), http.StatusNoContent)
	if names := playlistItemNames(f.getPlaylist(t, playlist.Id)); !slices.Equal(names, []string{"p1", "solo", "ep2"}) {
		t.Errorf("先頭へ移したあと = %v", names)
	}
	assertStatus(t, "ep2 の後ろへ", move(solo, playlistMoveBody(&ep2)), http.StatusNoContent)
	if names := playlistItemNames(f.getPlaylist(t, playlist.Id)); !slices.Equal(names, []string{"p1", "ep2", "solo"}) {
		t.Errorf("ep2 の後ろへ移したあと = %v", names)
	}

	rec = move(solo, playlistMoveBody(&solo))
	assertErrorBody(t, "自分の後ろへ", rec.Code, rec.Body.Bytes(), wantError{
		status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest,
	})
	for label, body := range map[string]string{
		"afterItemId が無い": `{}`,
		"整数でない":           `{"afterItemId":"1"}`,
		"知らない欄":           `{"afterItemId":null,"extra":1}`,
	} {
		assertStatus(t, label, move(solo, body), http.StatusBadRequest)
	}
	missing := int64(99999)
	for label, rec := range map[string]*httptest.ResponseRecorder{
		"無い項目":           move(missing, playlistMoveBody(nil)),
		"無い afterItemId": move(solo, playlistMoveBody(&missing)),
	} {
		assertErrorBody(t, label, rec.Code, rec.Body.Bytes(), wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound})
	}

	remove := func(itemID int64) *httptest.ResponseRecorder {
		return f.playlistRequest(http.MethodDelete, playlistItemPath(playlist.Id, itemID, ""), "", f.owner)
	}
	assertStatus(t, "外す", remove(ep2), http.StatusNoContent)
	rec = remove(ep2)
	assertErrorBody(t, "もう一度外す", rec.Code, rec.Body.Bytes(), wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound})
	if names := playlistItemNames(f.getPlaylist(t, playlist.Id)); !slices.Equal(names, []string{"p1", "solo"}) {
		t.Errorf("外したあと = %v", names)
	}
	video := decode[gen.Video](t, f.env.get("/api/videos/"+strconv.FormatInt(f.ids["ep2"], 10), f.owner))
	if video.Progress == nil || video.Progress.PositionMs != 30_000 {
		t.Errorf("外した ep2 の再生位置 = %+v", video.Progress)
	}
}

// 消したプレイリストは 204 のあと、どの経路でも 404 not_found。
func TestPlaylistDelete(t *testing.T) {
	f := newLibraryFixture(t)
	playlist := f.createPlaylist(t, "まとめ")
	rec := f.playlistRequest(http.MethodPost, playlistPath(playlist.Id, "/items"), playlistAddBody([]int64{f.ids["solo"]}), f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("追加: status = %d: %s", rec.Code, rec.Body)
	}
	itemID := f.getPlaylist(t, playlist.Id).Items[0].Id

	assertStatus(t, "削除", f.playlistRequest(http.MethodDelete, playlistPath(playlist.Id, ""), "", f.owner), http.StatusNoContent)
	for label, rec := range map[string]*httptest.ResponseRecorder{
		"読み出し":   f.env.get(playlistPath(playlist.Id, ""), f.owner),
		"名前の変更":  f.playlistRequest(http.MethodPatch, playlistPath(playlist.Id, ""), playlistNameBody("x"), f.owner),
		"もう一度削除": f.playlistRequest(http.MethodDelete, playlistPath(playlist.Id, ""), "", f.owner),
		"追加": f.playlistRequest(http.MethodPost, playlistPath(playlist.Id, "/items"),
			playlistAddBody([]int64{f.ids["solo"]}), f.owner),
		"並べ替え": f.playlistRequest(http.MethodPost, playlistItemPath(playlist.Id, itemID, "/move"),
			playlistMoveBody(nil), f.owner),
		"項目を外す": f.playlistRequest(http.MethodDelete, playlistItemPath(playlist.Id, itemID, ""), "", f.owner),
	} {
		assertErrorBody(t, label, rec.Code, rec.Body.Bytes(), wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound})
	}
	if list := decode[gen.PlaylistList](t, f.env.get("/api/playlists", f.owner)); len(list.Items) != 0 {
		t.Errorf("削除のあとの一覧 = %+v", list.Items)
	}
}

// 追加の要求の誤り: どちらも空は invalid_request、重複を除いて 20000 を超えれば too_many_videos、
// 廃止した folders は知らない欄として 400 で、何も足さない。
func TestPlaylistAddRejectsInvalidRequests(t *testing.T) {
	f := newLibraryFixture(t)
	playlist := f.createPlaylist(t, "まとめ")
	add := func(body string) *httptest.ResponseRecorder {
		return f.playlistRequest(http.MethodPost, playlistPath(playlist.Id, "/items"), body, f.owner)
	}

	for label, body := range map[string]string{
		"どちらも無い": `{}`,
		"どちらも空":  `{"videoIds":[],"groupIds":[]}`,
	} {
		rec := add(body)
		assertErrorBody(t, label, rec.Code, rec.Body.Bytes(), wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest})
	}

	ids := make([]int64, maxVideoTagsIDs)
	for i := range ids {
		ids[i] = int64(1_000_000 + i)
	}
	show := f.groupID(t, "show")
	for label, body := range map[string]string{
		"動画だけで 20001":    playlistAddBody(append(append([]int64{}, ids...), 2_000_000)),
		"動画とグループで 20001": playlistAddBody(ids, show),
	} {
		rec := add(body)
		assertErrorBody(t, label, rec.Code, rec.Body.Bytes(), wantError{
			status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonTooManyVideos, limit: maxVideoTagsIDs,
		})
	}
	if rec := add(fmt.Sprintf(`{"videoIds":[%d],"folders":[{"rootId":%d,"path":"show"}]}`, f.ids["p1"], f.rootID)); rec.Code != http.StatusBadRequest {
		t.Errorf("廃止した folders: status = %d, want 400", rec.Code)
	}
	if read := f.getPlaylist(t, playlist.Id); len(read.Items) != 0 {
		t.Errorf("誤りの要求で項目が足された: %v", playlistItemNames(read))
	}

	// 重複を 1 つに数えれば 20000 で、上限に収まる。
	if rec := add(playlistAddBody(append(append([]int64{}, ids...), ids[0]))); rec.Code != http.StatusOK {
		t.Errorf("重複を含む 20000: status = %d: %s", rec.Code, rec.Body)
	}
}

// ゲストはどの経路でも 401 で、何も変わらない（research.md R-8）。
func TestPlaylistsOwnerOnly(t *testing.T) {
	f := newLibraryFixture(t)
	playlist := f.createPlaylist(t, "まとめ")
	rec := f.playlistRequest(http.MethodPost, playlistPath(playlist.Id, "/items"), playlistAddBody([]int64{f.ids["solo"]}), f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("追加: status = %d: %s", rec.Code, rec.Body)
	}
	itemID := f.getPlaylist(t, playlist.Id).Items[0].Id

	for _, req := range []struct{ method, target, body string }{
		{http.MethodGet, "/api/playlists", ""},
		{http.MethodPost, "/api/playlists", playlistNameBody("ゲスト")},
		{http.MethodGet, playlistPath(playlist.Id, ""), ""},
		{http.MethodPatch, playlistPath(playlist.Id, ""), playlistNameBody("ゲスト")},
		{http.MethodDelete, playlistPath(playlist.Id, ""), ""},
		{http.MethodPost, playlistPath(playlist.Id, "/items"), playlistAddBody([]int64{f.ids["p1"]})},
		{http.MethodPost, playlistItemPath(playlist.Id, itemID, "/move"), playlistMoveBody(nil)},
		{http.MethodDelete, playlistItemPath(playlist.Id, itemID, ""), ""},
	} {
		assertUnauthenticated(t, req.method+" "+req.target, f.playlistRequest(req.method, req.target, req.body))
	}
	read := f.getPlaylist(t, playlist.Id)
	if read.Name != "まとめ" || !slices.Equal(playlistItemNames(read), []string{"solo"}) {
		t.Errorf("ゲストの要求のあと = %q・%v", read.Name, playlistItemNames(read))
	}
}

// 一覧の要約は covers に、見せる項目の順で、動画の項目はその動画、グループの項目はそのカードの絵柄の
// 先頭と同じメンバーのサムネイルを、サムネイル生成済みのものだけ持つ。空のプレイリストは空の配列で、
// 作成の応答は covers を省く。
func TestPlaylistListCovers(t *testing.T) {
	f := newLibraryFixture(t)
	empty := f.createPlaylist(t, "空")
	if empty.Covers != nil {
		t.Errorf("作成の応答の covers = %v, want 省く", *empty.Covers)
	}
	playlist := f.createPlaylist(t, "絵柄")
	rec := f.playlistRequest(http.MethodPost, playlistPath(playlist.Id, "/items"), playlistAddBody(
		[]int64{f.ids["solo"]}, f.groupID(t, "show"),
	), f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("追加: status = %d: %s", rec.Code, rec.Body)
	}
	rec = f.playlistRequest(http.MethodPost, playlistPath(playlist.Id, "/items"), playlistAddBody([]int64{f.ids["ep10"]}), f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("追加: status = %d: %s", rec.Code, rec.Body)
	}
	read := f.getPlaylist(t, playlist.Id)
	group := read.Items[1].Group
	if group == nil || len(group.Previews) == 0 {
		t.Fatalf("グループの項目 = %+v", read.Items[1])
	}

	rec = f.env.get("/api/playlists", f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("一覧: status = %d: %s", rec.Code, rec.Body)
	}
	covers := map[int64][]gen.FolderPreview{}
	for _, item := range decode[gen.PlaylistList](t, rec).Items {
		if item.Covers == nil {
			t.Fatalf("一覧の %q の covers が無い", item.Name)
		}
		covers[item.Id] = *item.Covers
	}
	if got := covers[empty.Id]; len(got) != 0 {
		t.Errorf("空のプレイリストの covers = %v", got)
	}
	// solo にはサムネイルが無いので飛ばす。グループはカードの絵柄の先頭、ep10 はその動画。
	ep10 := read.Items[2].Video
	want := []gen.FolderPreview{group.Previews[0], {VideoId: ep10.Id, ThumbnailUrl: *ep10.ThumbnailUrl}}
	if got := covers[playlist.Id]; len(got) != 2 || !samePreview(got[0], want[0]) || !samePreview(got[1], want[1]) {
		t.Errorf("covers = %+v, want %+v", got, want)
	}
}

// samePreview は差し込むサムネイルの動画と URL が同じかを返す（previewUrl はファイルの有無で変わるので見ない）。
func samePreview(a, b gen.FolderPreview) bool {
	return a.VideoId == b.VideoId && a.ThumbnailUrl == b.ThumbnailUrl
}
