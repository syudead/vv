package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/httpapi/gen"
)

// お気に入りの付け外し（specs/035-favorites/contracts/screen-api.md §0・§1）を、本物の認証・
// 保存層・経路をつないで確かめる。中身は library_test.go の libraryFixture と同じ。

func newFavoritesFixture(t *testing.T) *libraryFixture {
	t.Helper()
	return newLibraryFixture(t)
}

// favoritesBody は PUT /api/favorites の本文を作る。folders は登録フォルダの id と相対パスの組。
func favoritesBody(favorite bool, videoIDs []int64, folders ...gen.VideoFolder) string {
	body := map[string]any{"favorite": favorite}
	if videoIDs != nil {
		body["videoIds"] = videoIDs
	}
	if folders != nil {
		body["folders"] = folders
	}
	out, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	return string(out)
}

func (f *libraryFixture) putFavorites(body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	return f.env.serve(authRequest{method: http.MethodPut, target: "/api/favorites", body: body, cookies: cookies})
}

func (f *libraryFixture) videoFavorite(t *testing.T, name string) bool {
	t.Helper()
	rec := f.env.get("/api/videos/"+strconv.FormatInt(f.ids[name], 10), f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s: status = %d: %s", name, rec.Code, rec.Body)
	}
	video := decode[gen.Video](t, rec)
	if video.Favorite == nil {
		t.Fatalf("所有者の %s に favorite が無い: %s", name, rec.Body)
	}
	return *video.Favorite
}

func (f *libraryFixture) groupFavorite(t *testing.T, rel string) bool {
	t.Helper()
	rec := f.env.get(f.groupPath(rel), f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("グループ %s: status = %d: %s", rel, rec.Code, rec.Body)
	}
	group := decode[gen.LibraryGroup](t, rec)
	if group.Favorite == nil {
		t.Fatalf("所有者のグループ %s に favorite が無い: %s", rel, rec.Body)
	}
	return *group.Favorite
}

// 動画 2 本とグループ 1 つをお気に入りにすると、それぞれの応答の favorite が true になり、
// グループのメンバーは変わらない（受け入れ条件 7）。外すと false に戻る。
func TestUpdateFavoritesSetsVideosAndGroups(t *testing.T) {
	f := newFavoritesFixture(t)
	show := gen.VideoFolder{RootId: f.rootID, Path: "show"}

	// 重複は 1 つに数え、既に同じ状態のものも誤りにしない。
	for round := range 2 {
		rec := f.putFavorites(favoritesBody(true, []int64{f.ids["p1"], f.ids["solo"], f.ids["p1"]}, show, show), f.owner)
		if rec.Code != http.StatusOK {
			t.Fatalf("付ける %d 回目: status = %d: %s", round, rec.Code, rec.Body)
		}
		if got := decode[gen.FavoritesResponse](t, rec); got.AppliedVideos != 2 || got.AppliedFolders != 1 {
			t.Errorf("付ける %d 回目: 応答 = %+v, want 2・1", round, got)
		}
		if got := rec.Header().Get("Cache-Control"); got != cacheNoStore {
			t.Errorf("Cache-Control = %q", got)
		}
	}
	for name, want := range map[string]bool{"p1": true, "solo": true, "ep1": false, "ep2": false, "ep10": false, "p2": false} {
		if got := f.videoFavorite(t, name); got != want {
			t.Errorf("%s の favorite = %v, want %v", name, got, want)
		}
	}
	if !f.groupFavorite(t, "show") {
		t.Error("show の favorite が false")
	}
	if f.groupFavorite(t, "pair") {
		t.Error("付けていない pair の favorite が true")
	}

	rec := f.putFavorites(favoritesBody(false, []int64{f.ids["solo"]}, show), f.owner)
	if got := decode[gen.FavoritesResponse](t, rec); rec.Code != http.StatusOK || got.AppliedVideos != 1 || got.AppliedFolders != 1 {
		t.Fatalf("外す: status = %d・応答 = %+v", rec.Code, got)
	}
	if f.videoFavorite(t, "solo") || f.groupFavorite(t, "show") {
		t.Error("外した solo か show の favorite が true")
	}
	if !f.videoFavorite(t, "p1") {
		t.Error("外していない p1 の favorite が false")
	}
}

// 無い登録フォルダ・今グループでないフォルダ・無い id は誤りにせず数えない。
func TestUpdateFavoritesSkipsMissingTargets(t *testing.T) {
	f := newFavoritesFixture(t)
	rec := f.putFavorites(favoritesBody(true, []int64{99999},
		gen.VideoFolder{RootId: 999, Path: "show"},
		gen.VideoFolder{RootId: f.rootID, Path: "none"},
		gen.VideoFolder{RootId: f.rootID, Path: ""},
	), f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if got := decode[gen.FavoritesResponse](t, rec); got.AppliedVideos != 0 || got.AppliedFolders != 0 {
		t.Errorf("応答 = %+v, want 0・0", got)
	}
}

// 合計が 0 か 20000 を超えれば too_many_videos、不正なパスは invalid_folder_path で、何も変えない。
func TestUpdateFavoritesRejectsInvalidRequests(t *testing.T) {
	f := newFavoritesFixture(t)
	show := gen.VideoFolder{RootId: f.rootID, Path: "show"}

	ids := make([]int64, maxVideoTagsIDs)
	for i := range ids {
		// ライブラリの id と重ならない値にする（上限に収まる要求は反映される）。
		ids[i] = int64(1_000_000 + i)
	}
	for label, body := range map[string]string{
		"どちらも無い":         `{"favorite":true}`,
		"どちらも空":          `{"videoIds":[],"folders":[],"favorite":true}`,
		"動画だけで 20001":    favoritesBody(true, append(append([]int64{}, ids...), 2_000_000)),
		"動画とフォルダで 20001": favoritesBody(true, ids, show),
	} {
		rec := f.putFavorites(body, f.owner)
		assertErrorBody(t, label, rec.Code, rec.Body.Bytes(), wantError{
			status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonTooManyVideos, limit: maxVideoTagsIDs,
		})
	}
	// 重複を 1 つに数えれば 20000 で、上限に収まる。
	if rec := f.putFavorites(favoritesBody(true, append(append([]int64{}, ids...), ids[0])), f.owner); rec.Code != http.StatusOK {
		t.Errorf("重複を含む 20000: status = %d: %s", rec.Code, rec.Body)
	}

	for _, path := range []string{"../show", "/show", "show/", "a//b", "."} {
		rec := f.putFavorites(favoritesBody(true, []int64{f.ids["p1"]}, gen.VideoFolder{RootId: f.rootID, Path: path}), f.owner)
		assertErrorBody(t, "パス "+path, rec.Code, rec.Body.Bytes(), wantError{
			status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonInvalidFolderPath,
		})
	}
	if f.videoFavorite(t, "p1") {
		t.Error("誤りの要求で p1 がお気に入りになった")
	}

	for label, body := range map[string]string{
		"favorite が無い": fmt.Sprintf(`{"videoIds":[%d]}`, f.ids["p1"]),
		"知らない欄":        fmt.Sprintf(`{"videoIds":[%d],"favorite":true,"extra":1}`, f.ids["p1"]),
	} {
		assertStatus(t, label, f.putFavorites(body, f.owner), http.StatusBadRequest)
	}
}

// ゲストは付け外しできず（401）、応答に favorite が無い（受け入れ条件 10）。
func TestFavoritesHiddenFromGuests(t *testing.T) {
	f := newFavoritesFixture(t)
	show := gen.VideoFolder{RootId: f.rootID, Path: "show"}

	assertUnauthenticated(t, "ゲストの PUT", f.putFavorites(favoritesBody(true, []int64{f.ids["solo"]})))
	if f.videoFavorite(t, "solo") {
		t.Error("ゲストの PUT で solo がお気に入りになった")
	}

	// 公開の動画と、公開のメンバーが 2 本あるグループをお気に入りにしても、ゲストには出さない。
	if rec := f.putFavorites(favoritesBody(true, []int64{f.ids["ep1"], f.ids["solo"]}, show), f.owner); rec.Code != http.StatusOK {
		t.Fatalf("付ける: status = %d: %s", rec.Code, rec.Body)
	}

	rec := f.env.get("/api/videos/" + strconv.FormatInt(f.ids["solo"], 10))
	if rec.Code != http.StatusOK {
		t.Fatalf("ゲストの詳細: status = %d: %s", rec.Code, rec.Body)
	}
	var detail map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	assertGuestVideo(t, "ゲストの詳細", detail)

	rec = f.env.get(f.groupPath("show"))
	if rec.Code != http.StatusOK {
		t.Fatalf("ゲストのグループ: status = %d: %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), `"favorite"`) {
		t.Errorf("ゲストのグループに favorite がある: %s", rec.Body)
	}

	rec = f.env.get("/api/library")
	if rec.Code != http.StatusOK {
		t.Fatalf("ゲストのライブラリ: status = %d: %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), `"favorite"`) {
		t.Errorf("ゲストのライブラリに favorite がある: %s", rec.Body)
	}
}

// 所有者には動画を返すどの応答にも favorite が入る。
func TestFavoriteInOwnerVideoResponses(t *testing.T) {
	f := newFavoritesFixture(t)
	if rec := f.putFavorites(favoritesBody(true, []int64{f.ids["solo"]}), f.owner); rec.Code != http.StatusOK {
		t.Fatalf("付ける: status = %d: %s", rec.Code, rec.Body)
	}

	solo := strconv.FormatInt(f.ids["solo"], 10)
	for _, target := range []string{
		"/api/videos",
		"/api/folders/" + strconv.FormatInt(f.rootID, 10) + "/videos",
		"/api/videos/" + solo + "/versions",
	} {
		rec := f.env.get(target, f.owner)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d: %s", target, rec.Code, rec.Body)
		}
		items := rawItems(t, rec.Body.Bytes(), "items")
		if len(items) == 0 {
			t.Fatalf("%s: 項目が無い: %s", target, rec.Body)
		}
		for _, item := range items {
			value, ok := item["favorite"]
			if !ok {
				t.Errorf("%s: favorite が無い: %v", target, item)
				continue
			}
			var id int64
			if err := json.Unmarshal(item["id"], &id); err != nil {
				t.Fatal(err)
			}
			if want := strconv.FormatBool(id == f.ids["solo"]); string(value) != want {
				t.Errorf("%s: id %d の favorite = %s, want %s", target, id, value, want)
			}
		}
	}

	rec := f.env.get("/api/library", f.owner)
	for _, item := range rawItems(t, rec.Body.Bytes(), "items") {
		field := "video"
		if string(item["kind"]) == `"group"` {
			field = "group"
		}
		var payload map[string]json.RawMessage
		if err := json.Unmarshal(item[field], &payload); err != nil {
			t.Fatal(err)
		}
		if _, ok := payload["favorite"]; !ok {
			t.Errorf("所有者のライブラリの %s に favorite が無い: %s", field, item[field])
		}
	}
}

// 関連動画の応答にも所有者には favorite が入り、ゲストには入らない。
func TestFavoriteInRelatedVideos(t *testing.T) {
	f := newGuestFixture(t, true)
	rec := f.env.serve(authRequest{
		method: http.MethodPut, target: "/api/favorites",
		body: favoritesBody(true, []int64{f.ids["d"]}), cookies: []*http.Cookie{f.owner},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("付ける: status = %d: %s", rec.Code, rec.Body)
	}

	rec = f.env.get(f.videoPath("a", "/related"), f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("所有者の関連: status = %d: %s", rec.Code, rec.Body)
	}
	items := rawItems(t, rec.Body.Bytes(), "items")
	if len(items) == 0 {
		t.Fatalf("所有者の関連に項目が無い: %s", rec.Body)
	}
	for _, item := range items {
		var id int64
		if err := json.Unmarshal(item["id"], &id); err != nil {
			t.Fatal(err)
		}
		if want := strconv.FormatBool(id == f.ids["d"]); string(item["favorite"]) != want {
			t.Errorf("所有者の関連の id %d の favorite = %s, want %s", id, item["favorite"], want)
		}
	}

	rec = f.env.get(f.videoPath("a", "/related"))
	if rec.Code != http.StatusOK {
		t.Fatalf("ゲストの関連: status = %d: %s", rec.Code, rec.Body)
	}
	for _, item := range rawItems(t, rec.Body.Bytes(), "items") {
		assertGuestVideo(t, "ゲストの関連の項目", item)
	}
}

// folders の上限 20000 件を深いパスで送っても、本文の大きさで断らない。rootName が違うだけの
// 同じフォルダは 1 つに数える（rootId と path の組が同一性）。
func TestUpdateFavoritesAcceptsFullFolderBatch(t *testing.T) {
	f := newFavoritesFixture(t)
	rootName := "Library"
	folders := make([]gen.VideoFolder, 0, maxVideoTagsIDs+1)
	for i := range maxVideoTagsIDs {
		folders = append(folders, gen.VideoFolder{RootId: f.rootID, Path: fmt.Sprintf("season/collection/episode-%05d", i)})
	}
	folders[0].Path = "show"
	// 上限ちょうどの 20000 件に、rootName だけが違う重複を足す。
	folders = append(folders, gen.VideoFolder{RootId: f.rootID, Path: "show", RootName: &rootName})
	body := favoritesBody(true, nil, folders...)
	if len(body) <= 1<<20 {
		t.Fatalf("本文 %d バイトが既定の上限 1 MiB に収まり、確かめにならない", len(body))
	}
	rec := f.putFavorites(body, f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if got := decode[gen.FavoritesResponse](t, rec); got.AppliedVideos != 0 || got.AppliedFolders != 1 {
		t.Errorf("応答 = %+v, want 0・1", got)
	}
	if !f.groupFavorite(t, "show") {
		t.Error("show の favorite が false")
	}
}

// 上限を超えた本文は、上限を示して断る。
func TestUpdateFavoritesRejectsOversizedBody(t *testing.T) {
	f := newFavoritesFixture(t)
	body := fmt.Sprintf(`{"videoIds":[%d],"favorite":true,"folders":[{"rootId":%d,"path":"%s"}]}`,
		f.ids["p1"], f.rootID, strings.Repeat("a", favoritesBodyLimit))
	rec := f.putFavorites(body, f.owner)
	assertStatus(t, "大きすぎる本文", rec, http.StatusBadRequest)
	if !strings.Contains(rec.Body.String(), strconv.Itoa(favoritesBodyLimit)) {
		t.Errorf("上限を示していない: %s", rec.Body)
	}
	if f.videoFavorite(t, "p1") {
		t.Error("断った要求で p1 がお気に入りになった")
	}
}
