package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/store"
)

// ホームの経路（specs/057-home/contracts/screen-api.md）を、本物の認証・保存層・経路をつないで
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

// 棚は並べた順に出て、決まった棚（所有者には continue、recent、unshelved）がそのあとに来る。所有者には
// 整理役の状態と byOrganizer が入る。再生もお気に入りも無いので recommended は出ない。
func TestHomeListsShelvesInTheirOrderThenTheFixedShelves(t *testing.T) {
	f := newHomeFixture(t)
	f.setShelves(t, f.hidden, f.all, f.manual)
	home := f.home(t, f.owner)
	want := []string{folderKey(f.hidden), folderKey(f.all), folderKey(f.manual), "continue", "recent", "unshelved"}
	if got := homeShelfKeys(home); !slices.Equal(got, want) {
		t.Fatalf("棚 = %v, want %v", got, want)
	}
	first := home.Shelves[0]
	if first.Kind != gen.HomeShelfKindFolder || first.Name != "隠れ" || first.FolderId == nil || *first.FolderId != f.hidden ||
		first.ByOrganizer == nil || *first.ByOrganizer {
		t.Errorf("フォルダの棚 = %+v", first)
	}
	if fixed := home.Shelves[4]; fixed.Kind != gen.HomeShelfKindRecent || fixed.Name != "" || fixed.FolderId != nil || fixed.ByOrganizer != nil {
		t.Errorf("決まった棚 = %+v", fixed)
	}
	if home.Organizer == nil || home.Organizer.Running || home.Organizer.Pending != nil || home.Organizer.Last != nil {
		t.Errorf("整理役の状態 = %+v", home.Organizer)
	}
	if home.Empty != nil {
		t.Errorf("empty = %v, want none", *home.Empty)
	}
}

// 所有者の応答の整理役の状態は、待っている実行と最後の実行を settings から読む。
func TestHomeShowsTheOrganizerStateToTheOwner(t *testing.T) {
	f := newHomeFixture(t)
	ctx := context.Background()
	if err := f.env.db.Home().SavePendingOrganize(ctx, domain.HomeExtend); err != nil {
		t.Fatal(err)
	}
	last := domain.HomeOrganizeLast{Run: domain.HomeRebuild, Outcome: domain.HomeOrganizeDone, FinishedAt: 1700, Added: 3, Removed: 1}
	if err := f.env.db.Home().SaveLastOrganize(ctx, last); err != nil {
		t.Fatal(err)
	}
	organizer := f.home(t, f.owner).Organizer
	if organizer == nil || organizer.Pending == nil || *organizer.Pending != gen.HomeOrganizerPendingExtend {
		t.Fatalf("整理役の状態 = %+v, want pending extend", organizer)
	}
	if got := organizer.Last; got == nil || got.Run != gen.HomeOrganizeRunRunRebuild || got.Outcome != gen.HomeOrganizeRunOutcomeDone ||
		got.FinishedAt != 1700 || got.Added != 3 || got.Removed != 1 {
		t.Errorf("最後の実行 = %+v", got)
	}
}

// 受け入れ条件 8: ゲストには整理役の状態と byOrganizer が出ず、見せる動画の無いフォルダの棚も出ない。
// その棚の項目も読めない。
func TestHomeHidesOwnerStateAndEmptyShelvesFromGuests(t *testing.T) {
	f := newHomeFixture(t)
	home := f.home(t)
	want := []string{folderKey(f.all), folderKey(f.manual), "recent", "unshelved"}
	if got := homeShelfKeys(home); !slices.Equal(got, want) {
		t.Fatalf("ゲストの棚 = %v, want %v", got, want)
	}
	if home.Organizer != nil {
		t.Errorf("ゲストに整理役の状態 = %+v", home.Organizer)
	}
	for _, shelf := range home.Shelves {
		if shelf.ByOrganizer != nil {
			t.Errorf("ゲストの棚 %s に byOrganizer がある", shelf.Key)
		}
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
	if got := homeShelfKeys(home); !slices.Equal(got, []string{"continue", "recent", "unshelved"}) {
		t.Errorf("棚 = %v", got)
	}
}

// フォルダの棚の項目に子フォルダは入らない（research.md R-14）。recent は新しい順で、unshelved は
// どの棚にも一致しない項目である。項目の数はライブラリの項目と同じに数える。
func TestHomeShelfItemsHoldVideosAndGroupsOnly(t *testing.T) {
	f := newHomeFixture(t)
	shelves := f.shelfItems(t, folderKey(f.manual)+",recent,unshelved,"+folderKey(f.manual), f.owner)
	manual := shelves[folderKey(f.manual)]
	if manual.Total != 1 || len(manual.Items) != 1 || manual.Items[0].Kind != gen.LibraryItemKindVideo ||
		manual.Items[0].Video.Id != f.ids["ep1"] {
		t.Fatalf("手の棚 = %+v, want ep1 only and no child folder", manual)
	}
	recent := shelves["recent"]
	names := make([]string, 0, len(recent.Items))
	for _, item := range recent.Items {
		switch item.Kind {
		case gen.LibraryItemKindGroup:
			names = append(names, "group:"+item.Group.Name)
		case gen.LibraryItemKindVideo:
			names = append(names, item.Video.Title)
		default:
			t.Errorf("recent に %s の項目がある", item.Kind)
		}
	}
	if want := []string{"solo", "group:pair", "group:show"}; !slices.Equal(names, want) || recent.Total != 3 {
		t.Errorf("recent = %v (total %d), want %v", names, recent.Total, want)
	}
	if unshelved := shelves["unshelved"]; unshelved.Total != 0 || len(unshelved.Items) != 0 {
		t.Errorf("すべての棚があるときの unshelved = %+v", unshelved)
	}

	f.setShelves(t, f.manual, f.hidden)
	if unshelved := f.shelfItems(t, "unshelved", f.owner)["unshelved"]; unshelved.Total != 4 {
		t.Errorf("unshelved = %+v, want ep2, ep10, p1 and solo", unshelved)
	}
	// 棚から外したフォルダの鍵と、種の無いおすすめの鍵は落とす。
	if got := f.shelfItems(t, folderKey(f.all)+","+folderKey(f.child)+",recommended", f.owner); len(got) != 0 {
		t.Errorf("棚でない鍵の項目 = %v", got)
	}
}

// keys が無いか 9 件以上か形が誤っていれば 400 invalid_request である。
func TestHomeShelfItemsRejectsBadKeys(t *testing.T) {
	f := newHomeFixture(t)
	nine := strings.TrimSuffix(strings.Repeat("recent,", 9), ",")
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
	eight := strings.TrimSuffix(strings.Repeat("recent,", 8), ",")
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
	want := []string{folderKey(f.all), folderKey(f.manual), folderKey(f.hidden), "continue", "recent", "unshelved"}
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

// runningOrganizer は走っている整理役の状態を返す HomeOrganizer である。
type runningOrganizer struct{}

func (runningOrganizer) Reorganize(context.Context) (domain.HomeOrganizerState, error) {
	return domain.HomeOrganizerState{Running: true, Pending: domain.HomeRebuild}, nil
}

func (runningOrganizer) State(context.Context) (domain.HomeOrganizerState, error) {
	return domain.HomeOrganizerState{Running: true}, nil
}

// 整理役があれば、所有者の応答の整理役の状態は、走っているかを知る整理役から読む。
func TestHomeShowsARunningOrganizerToTheOwner(t *testing.T) {
	f := &homeFixture{libraryFixture: newLibraryFixtureWith(t, func(options Options) Options {
		options.HomeOrganizer = runningOrganizer{}
		return options
	})}
	if organizer := f.home(t, f.owner).Organizer; organizer == nil || !organizer.Running {
		t.Fatalf("整理役の状態 = %+v, want running", organizer)
	}
	if organizer := f.home(t).Organizer; organizer != nil {
		t.Errorf("ゲストの整理役の状態 = %+v, want none", organizer)
	}
}

// 続きから見るとおすすめは所有者だけの棚である（research.md R-13）。お気に入り（ep10）が種になると
// おすすめが最後の棚に出て、種と見終わった動画（ep1）を除き、種と同じフォルダの ep2 が先頭に来る。
// ゲストのホームと棚の項目にはどちらも出ない。
func TestHomeShowsContinueAndRecommendedToTheOwnerOnly(t *testing.T) {
	f := newHomeFixture(t)
	if _, err := f.env.db.Favorites().SetFavorites(context.Background(),
		domain.FavoriteChange{VideoIDs: []int64{f.ids["ep10"]}, Favorite: true}); err != nil {
		t.Fatal(err)
	}
	want := []string{folderKey(f.all), folderKey(f.manual), folderKey(f.hidden), "continue", "recent", "unshelved", "recommended"}
	if got := homeShelfKeys(f.home(t, f.owner)); !slices.Equal(got, want) {
		t.Fatalf("所有者の棚 = %v, want %v", got, want)
	}
	shelves := f.shelfItems(t, "recommended,continue,recent", f.owner)
	recommended := shelves["recommended"]
	var ids []int64
	for _, item := range recommended.Items {
		if item.Kind != gen.LibraryItemKindVideo {
			t.Fatalf("おすすめに %s の項目がある", item.Kind)
		}
		ids = append(ids, item.Video.Id)
	}
	if len(ids) == 0 || ids[0] != f.ids["ep2"] || slices.Contains(ids, f.ids["ep10"]) || slices.Contains(ids, f.ids["ep1"]) ||
		recommended.Total != len(ids) {
		t.Errorf("おすすめ = %v (total %d), want ep2 first and neither ep10 nor ep1", ids, recommended.Total)
	}
	// ep2 は途中まで見たので、そのグループ show が続きから見るに出る。
	if cont := shelves["continue"]; cont.Total != 1 || cont.Items[0].Kind != gen.LibraryItemKindGroup || cont.Items[0].Group.Name != "show" {
		t.Errorf("続きから見る = %+v, want the group show", cont)
	}

	if got := homeShelfKeys(f.home(t)); slices.Contains(got, "continue") || slices.Contains(got, "recommended") {
		t.Errorf("ゲストの棚 = %v, want neither continue nor recommended", got)
	}
	if got := f.shelfItems(t, "continue,recommended"); len(got) != 0 {
		t.Errorf("ゲストの続きから見るとおすすめの項目 = %v", got)
	}
}
