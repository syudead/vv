package httpapi

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/store"
)

// ホームの経路（docs/design-docs/home.md）を、本物の認証・保存層・経路をつないで
// 確かめる。中身は library_test.go の libraryFixture で、公開は ep1・ep2・p1・solo である。

// homeFixture は libraryFixture に、所有者が並べた棚を持たせたものである。
type homeFixture struct {
	*libraryFixture
	// all は条件の無い棚、manual はタグ「手」（ep1）の棚、hidden は非公開の p2 だけに一致する棚、
	// child は manual の子フォルダ（棚ではない）である。
	all, manual, hidden, child int64
}

// newHomeFixture は棚を all・manual・hidden の順に並べる。
func newHomeFixture(t *testing.T) *homeFixture {
	t.Helper()
	f := &homeFixture{libraryFixture: newLibraryFixture(t)}
	private := f.smartFolderTag(t, "非公開", "p2")
	f.all = f.createSmartFolder(t, map[string]any{"name": "すべて", "rule": map[string]any{}}).Id
	f.manual = f.createSmartFolder(t, map[string]any{"name": "手", "rule": map[string]any{"tags": []int64{f.libraryFixture.manual}}}).Id
	f.hidden = f.createSmartFolder(t, map[string]any{"name": "隠れ", "rule": map[string]any{"tags": []int64{private}}}).Id
	f.child = f.createSmartFolder(t, map[string]any{"name": "子", "rule": map[string]any{}, "parentId": f.manual}).Id
	f.setShelves(t, f.all, f.manual, f.hidden)
	return f
}

// setShelves は所有者として棚を ids にする。
func (f *homeFixture) setShelves(t *testing.T, ids ...int64) {
	t.Helper()
	rec := f.groupRequest(http.MethodPut, "/api/home/shelves", groupJSON(map[string]any{"folderIds": ids}), f.owner)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("PUT /api/home/shelves %v: status = %d: %s", ids, rec.Code, rec.Body)
	}
}

func (f *homeFixture) home(t *testing.T, cookies ...*http.Cookie) gen.Home {
	t.Helper()
	rec := f.env.get("/api/home", cookies...)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/home status = %d: %s", rec.Code, rec.Body)
	}
	return decode[gen.Home](t, rec)
}

func (f *homeFixture) shelfItems(t *testing.T, keys string, cookies ...*http.Cookie) map[string]gen.HomeShelfItems {
	t.Helper()
	rec := f.env.get("/api/home/shelves?keys="+keys, cookies...)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/home/shelves?keys=%s status = %d: %s", keys, rec.Code, rec.Body)
	}
	shelves := map[string]gen.HomeShelfItems{}
	for _, shelf := range decode[gen.HomeShelfItemsList](t, rec).Shelves {
		shelves[shelf.Key] = shelf
	}
	return shelves
}

func folderKey(id int64) string { return "folder:" + strconv.FormatInt(id, 10) }

func homeShelfKeys(home gen.Home) []string {
	keys := make([]string, 0, len(home.Shelves))
	for _, shelf := range home.Shelves {
		keys = append(keys, shelf.Key)
	}
	return keys
}

// Saved shelf order is the complete home summary for the owner.
func TestHomeListsOnlySavedShelvesInTheirOrder(t *testing.T) {
	f := newHomeFixture(t)
	f.setShelves(t, f.hidden, f.all, f.manual)
	home := f.home(t, f.owner)
	want := []string{folderKey(f.hidden), folderKey(f.all), folderKey(f.manual)}
	if got := homeShelfKeys(home); !slices.Equal(got, want) {
		t.Fatalf("棚 = %v, want %v", got, want)
	}
	first := home.Shelves[0]
	if first.Name != "隠れ" || first.FolderId != f.hidden {
		t.Errorf("フォルダの棚 = %+v", first)
	}
	if home.Empty != nil {
		t.Errorf("empty = %v, want none", *home.Empty)
	}
}

// 受け入れ条件 8: ゲストには見せる動画の無いフォルダの棚は出ない。
// その棚の項目も読めない。
func TestHomeHidesOwnerStateAndEmptyShelvesFromGuests(t *testing.T) {
	f := newHomeFixture(t)
	home := f.home(t)
	want := []string{folderKey(f.all), folderKey(f.manual)}
	if got := homeShelfKeys(home); !slices.Equal(got, want) {
		t.Fatalf("ゲストの棚 = %v, want %v", got, want)
	}
	keys := folderKey(f.hidden) + "," + folderKey(f.manual)
	shelves := f.shelfItems(t, keys)
	if _, ok := shelves[folderKey(f.hidden)]; ok || len(shelves) != 1 {
		t.Errorf("ゲストの棚の項目 = %v, want only %s", shelves, folderKey(f.manual))
	}
	if owner := f.shelfItems(t, keys, f.owner); owner[folderKey(f.hidden)].Total != 1 {
		t.Errorf("所有者の隠れの棚 = %+v, want total 1", owner[folderKey(f.hidden)])
	}
}

// 見る人にライブラリの動画が 1 本も見えなければ empty は noVideos である。
func TestHomeReportsNoVideos(t *testing.T) {
	env := newAuthEnvWith(t, t.TempDir(), func(db *store.DB) Options { return Options{Home: db.Home()} })
	owner := env.setup()
	rec := env.get("/api/home", owner)
	home := decode[gen.Home](t, rec)
	if rec.Code != http.StatusOK || home.Empty == nil || *home.Empty != gen.NoVideos {
		t.Fatalf("動画の無いホーム = %d %+v", rec.Code, home)
	}
	if got := homeShelfKeys(home); len(got) != 0 {
		t.Errorf("棚 = %v", got)
	}
}

// Shelf pages contain videos and groups without child-folder items.
func TestHomeShelfItemsHoldVideosAndGroupsOnly(t *testing.T) {
	f := newHomeFixture(t)
	shelves := f.shelfItems(t, folderKey(f.manual)+","+folderKey(f.all)+","+folderKey(f.manual), f.owner)
	manual := shelves[folderKey(f.manual)]
	if manual.Total != 1 || len(manual.Items) != 1 || manual.Items[0].Kind != gen.HomeShelfItemKindVideo ||
		manual.Items[0].Video.Id != f.ids["ep1"] {
		t.Fatalf("手の棚 = %+v, want ep1 only and no child folder", manual)
	}
	all := shelves[folderKey(f.all)]
	names := make([]string, 0, len(all.Items))
	for _, item := range all.Items {
		switch item.Kind {
		case gen.HomeShelfItemKindGroup:
			names = append(names, "group:"+item.Group.Name)
		case gen.HomeShelfItemKindVideo:
			names = append(names, item.Video.Title)
		default:
			t.Errorf("all shelf: %s の項目がある", item.Kind)
		}
	}
	if want := []string{"solo", "group:pair", "group:show"}; !slices.Equal(names, want) || all.Total != 3 {
		t.Errorf("all = %v (total %d), want %v", names, all.Total, want)
	}
	f.setShelves(t, f.manual, f.hidden)
	// Removed shelves and child folders are omitted.
	if got := f.shelfItems(t, folderKey(f.all)+","+folderKey(f.child), f.owner); len(got) != 0 {
		t.Errorf("棚でない鍵の項目 = %v", got)
	}
}

// keys が無いか 9 件以上か形が誤っていれば 400 invalid_request である。
func TestHomeShelfItemsRejectsBadKeys(t *testing.T) {
	f := newHomeFixture(t)
	nine := strings.TrimSuffix(strings.Repeat("folder:1,", 9), ",")
	for _, target := range []string{
		"/api/home/shelves", "/api/home/shelves?keys=", "/api/home/shelves?keys=" + nine,
		"/api/home/shelves?keys=folder:x", "/api/home/shelves?keys=folder:0", "/api/home/shelves?keys=folder:01",
		"/api/home/shelves?keys=later",
	} {
		rec := f.env.get(target, f.owner)
		if rec.Code != http.StatusBadRequest || decode[gen.Error](t, rec).Code != codeInvalidRequest {
			t.Errorf("%s: status = %d: %s", target, rec.Code, rec.Body)
		}
	}
	eight := strings.TrimSuffix(strings.Repeat("folder:1,", 8), ",")
	if rec := f.env.get("/api/home/shelves?keys="+eight, f.owner); rec.Code != http.StatusOK {
		t.Errorf("8 件の keys: status = %d", rec.Code)
	}
}

// PUT /api/home/shelves: 知らない id は 404 not_found、51 件以上と同じ id の 2 度は 400 invalid_request
// で、どれも並びを変えない。セッションの無い要求は 401 である。
func TestSetHomeShelvesStatuses(t *testing.T) {
	f := newHomeFixture(t)
	many := make([]int64, 51)
	for i := range many {
		many[i] = int64(i + 1)
	}
	for _, tc := range []struct {
		label string
		body  string
		want  wantError
	}{
		{"知らない id", fmt.Sprintf(`{"folderIds": [%d, 999999]}`, f.all), wantError{status: http.StatusNotFound, code: codeNotFound}},
		{"同じ id", fmt.Sprintf(`{"folderIds": [%d, %d]}`, f.all, f.all), wantGroupInvalid},
		{"51 件", groupJSON(map[string]any{"folderIds": many}), wantGroupInvalid},
		{"folderIds なし", `{}`, wantGroupInvalid},
	} {
		rec := f.groupRequest(http.MethodPut, "/api/home/shelves", tc.body, f.owner)
		assertErrorBody(t, tc.label, rec.Code, rec.Body.Bytes(), tc.want)
	}
	if rec := f.groupRequest(http.MethodPut, "/api/home/shelves", `{"folderIds": []}`); rec.Code != http.StatusUnauthorized {
		t.Errorf("ゲストの PUT: status = %d, want 401", rec.Code)
	}
	want := []string{folderKey(f.all), folderKey(f.manual), folderKey(f.hidden)}
	if got := homeShelfKeys(f.home(t, f.owner)); !slices.Equal(got, want) {
		t.Errorf("失敗した PUT のあとの棚 = %v, want %v", got, want)
	}
}

// GET /api/smart-folders/{id} の onHome は所有者の応答にだけ入る。
func TestSmartFolderOnHome(t *testing.T) {
	f := newHomeFixture(t)
	for _, tc := range []struct {
		id   int64
		want bool
	}{{f.manual, true}, {f.child, false}} {
		detail := decode[gen.SmartFolderDetail](t, f.env.get(smartFolderPath(tc.id, ""), f.owner))
		if detail.OnHome == nil || *detail.OnHome != tc.want {
			t.Errorf("フォルダ %d の onHome = %v, want %v", tc.id, detail.OnHome, tc.want)
		}
	}
	if guest := decode[gen.SmartFolderDetail](t, f.env.get(smartFolderPath(f.manual, ""))); guest.OnHome != nil {
		t.Errorf("ゲストの onHome = %v", *guest.OnHome)
	}
}
