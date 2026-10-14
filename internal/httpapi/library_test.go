package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/store"
)

// ライブラリの項目（specs/017-folder-groups/contracts/library-api.md）を、本物の認証・
// 保存層・経路をつないで確かめる。
//
//	root = <media>
//	  show/ep1.mp4   公開。完了。手で付けたタグ「手」
//	  show/ep2.mp4   公開。途中まで
//	  show/ep10.mp4  非公開
//	  pair/p1.mp4    公開
//	  pair/p2.mp4    非公開
//	  solo.mp4       公開
//
// show と pair はグループになる。フォルダ名 show と同じ名前のタグがあり、show の
// メンバーにはフォルダ由来で付く。
type libraryFixture struct {
	env      *authEnv
	owner    *http.Cookie
	ids      map[string]int64
	rootID   int64
	mediaDir string
	manual   int64
	folder   int64
}

func newLibraryFixture(t *testing.T) *libraryFixture {
	t.Helper()
	return newLibraryFixtureWith(t, func(options Options) Options { return options })
}

// newLibraryFixtureWith は newLibraryFixture と同じ中身を、経路の問い合わせ先を
// adjust で差し替えて作る。
func newLibraryFixtureWith(t *testing.T, adjust func(Options) Options) *libraryFixture {
	t.Helper()
	ctx := context.Background()
	mediaDir := t.TempDir()
	f := &libraryFixture{ids: map[string]int64{}, mediaDir: mediaDir}
	f.env = newAuthEnvWith(t, t.TempDir(), func(db *store.DB) Options {
		library := db.Library()
		return adjust(Options{
			Videos: library, Folders: library, Library: library, FolderGroups: db.FolderGroups(),
			Playback: db.Playback(), Tags: db.Tags(), Visibility: db.Visibility(),
			Favorites: db.Favorites(), Versions: db.Versions(), Playlists: db.Playlists(), Groups: db.Groups(),
			PlaylistPlayback: db.Playlists(),
			Organizer:        app.NewOrganizer(app.OrganizerOptions{Store: db.Organize()}),
		})
	})
	db := f.env.db
	f.owner = f.env.setup()
	root, err := db.Settings().AddMediaFolder(ctx, mediaDir)
	if err != nil {
		t.Fatal(err)
	}
	f.rootID = root.ID

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for index, rel := range []string{"show/ep1", "show/ep2", "show/ep10", "pair/p1", "pair/p2", "solo"} {
		path := filepath.Join(mediaDir, filepath.FromSlash(rel)+".mp4")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(rel)
		result, err := db.ScanIndex().UpsertVideo(ctx, domain.VideoFile{
			Path: path, Title: name, ContentKey: "key-" + name, SizeBytes: 100,
			MTime: base, AddedAt: base.Add(time.Duration(index) * time.Hour), Container: "mp4",
		})
		if err != nil {
			t.Fatal(err)
		}
		f.ids[name] = result.ID
		probe := domain.Probe{DurationMs: 60_000, Width: 640, Height: 360, VideoCodec: "h264", AudioCodec: "aac"}
		if err := db.Ingest().ApplyProbe(ctx, result.ID, probe, domain.Playability{Playable: true}); err != nil {
			t.Fatal(err)
		}
	}
	// ep1 にはサムネイルが無い。グループの絵柄はサムネイル生成済みのメンバーだけを差し込む。
	for _, name := range []string{"ep2", "ep10"} {
		if err := db.Ingest().SetThumbnailState(ctx, f.ids[name], domain.ThumbnailStateDone); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.ScanIndex().RebuildFolderIndex(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Playback().SaveProgress(ctx, "key-ep1", domain.Progress{PositionMs: 60_000, DurationMs: 60_000, Completed: true}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Playback().SaveProgress(ctx, "key-ep2", domain.Progress{PositionMs: 30_000, DurationMs: 60_000}, nil); err != nil {
		t.Fatal(err)
	}
	manual, err := db.Tags().CreateTag(ctx, "手")
	if err != nil {
		t.Fatal(err)
	}
	f.manual = manual.ID
	if _, _, err := db.Tags().AttachTagByID(ctx, []int64{f.ids["ep1"]}, manual.ID); err != nil {
		t.Fatal(err)
	}
	folder, err := db.Tags().CreateTag(ctx, "show")
	if err != nil {
		t.Fatal(err)
	}
	f.folder = folder.ID
	if _, err := db.Visibility().SetVideosPublic(ctx, []int64{f.ids["ep1"], f.ids["ep2"], f.ids["p1"], f.ids["solo"]}, true); err != nil {
		t.Fatal(err)
	}
	return f
}

// previewIDs はグループの絵柄に差し込むサムネイルの動画の id を並びの順に返す。
func previewIDs(previews []gen.FolderPreview) []int64 {
	ids := make([]int64, 0, len(previews))
	for _, preview := range previews {
		ids = append(ids, preview.VideoId)
	}
	return ids
}

// groupID は所有者のライブラリの一覧から、名前 name のグループの id を返す。
func (f *libraryFixture) groupID(t *testing.T, name string) int64 {
	t.Helper()
	return libraryGroupNamed(t, decode[gen.LibraryPage](t, f.env.get("/api/library", f.owner)), name).Id
}

// groupPath は名前 name のグループの GET /api/groups/{id} の経路である。
func (f *libraryFixture) groupPath(t *testing.T, name string) string {
	t.Helper()
	return "/api/groups/" + strconv.FormatInt(f.groupID(t, name), 10)
}

// libraryItemNames は項目を「group:名前」か動画の題名にして並べ替えて返す。
func libraryItemNames(page gen.LibraryPage) []string {
	names := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		switch item.Kind {
		case gen.LibraryItemKindGroup:
			names = append(names, "group:"+item.Group.Name)
		default:
			names = append(names, item.Video.Title)
		}
	}
	slices.Sort(names)
	return names
}

func libraryGroupNamed(t *testing.T, page gen.LibraryPage, name string) gen.LibraryGroup {
	t.Helper()
	for _, item := range page.Items {
		if item.Kind == gen.LibraryItemKindGroup && item.Group != nil && item.Group.Name == name {
			if item.Video != nil {
				t.Errorf("グループの項目に video がある: %+v", item)
			}
			return *item.Group
		}
	}
	t.Fatalf("グループ %s が無い: %v", name, libraryItemNames(page))
	return gen.LibraryGroup{}
}

// 受け入れ条件 1・2・12・14: 所有者にはグループが1件の項目で出て、全メンバーの本数・
// 合計・絵柄のサムネイル・並んだ id・視聴状態・見終えた本数・開くメンバー・タグの和集合が入る。
func TestListLibraryForOwner(t *testing.T) {
	f := newLibraryFixture(t)
	rec := f.env.get("/api/library?limit=1&sort=titleAsc", f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if first := decode[gen.LibraryPage](t, rec); first.Total != 3 || len(first.Items) != 1 || first.NextCursor == nil {
		t.Fatalf("1ページ目 = %+v", first)
	}

	page := decode[gen.LibraryPage](t, f.env.get("/api/library", f.owner))
	if names := libraryItemNames(page); !slices.Equal(names, []string{"group:pair", "group:show", "solo"}) || page.Total != 3 {
		t.Fatalf("項目 = %v・total %d", names, page.Total)
	}
	show := libraryGroupNamed(t, page, "show")
	if show.Folder.RootId != f.rootID || show.Folder.Path != "show" {
		t.Errorf("folder = %+v", show.Folder)
	}
	if show.VideoCount != 3 || !slices.Equal(show.VideoIds, []int64{f.ids["ep1"], f.ids["ep2"], f.ids["ep10"]}) {
		t.Errorf("本数・id = %d・%v", show.VideoCount, show.VideoIds)
	}
	if show.DurationMs == nil || *show.DurationMs != 180_000 || show.SizeBytes != 300 {
		t.Errorf("長さ・大きさ = %v・%d", show.DurationMs, show.SizeBytes)
	}
	if ids := previewIDs(show.Previews); !slices.Equal(ids, []int64{f.ids["ep2"], f.ids["ep10"]}) {
		t.Errorf("絵柄のサムネイル = %v, want ep2・ep10（サムネイルの無い ep1 は差し込まない）", ids)
	}
	if show.OpenVideoId != f.ids["ep2"] {
		t.Errorf("開くメンバー = %d, want ep2", show.OpenVideoId)
	}
	if show.WatchState == nil || *show.WatchState != gen.LibraryGroupWatchStateInProgress || show.WatchedCount == nil || *show.WatchedCount != 1 {
		t.Errorf("視聴状態・見終えた本数 = %v・%v", show.WatchState, show.WatchedCount)
	}
	if show.LastPlayedAt == nil {
		t.Error("lastPlayedAt が無い")
	}
	wantTags := map[int64][2]bool{f.manual: {true, false}, f.folder: {false, true}}
	if len(show.Tags) != len(wantTags) {
		t.Errorf("タグ = %+v", show.Tags)
	}
	for _, tag := range show.Tags {
		if want, ok := wantTags[tag.Id]; !ok || want != [2]bool{tag.Manual, tag.FromFolder} {
			t.Errorf("タグ %+v, want %v", tag, want)
		}
	}

	// 027 の受け入れ条件 1: 1本のメンバーだけが検索語に当たれば、その動画が動画の項目で出る。
	hit := decode[gen.LibraryPage](t, f.env.get("/api/library?query=ep10", f.owner))
	if names := libraryItemNames(hit); !slices.Equal(names, []string{"ep10"}) || hit.Total != 1 {
		t.Fatalf("ep10 の検索 = %v・total %d", names, hit.Total)
	}
	if item := hit.Items[0]; item.Kind != gen.LibraryItemKindVideo || item.Video == nil || item.Group != nil || item.Video.Id != f.ids["ep10"] {
		t.Errorf("ep10 の検索の項目 = %+v, want ep10 の動画の項目", item)
	}

	// 視聴状態の絞り込みは項目の視聴状態に掛かる。
	if names := libraryItemNames(decode[gen.LibraryPage](t, f.env.get("/api/library?watch=inProgress", f.owner))); !slices.Equal(names, []string{"group:show"}) {
		t.Errorf("途中まで = %v", names)
	}
	if names := libraryItemNames(decode[gen.LibraryPage](t, f.env.get("/api/library?watch=watched", f.owner))); len(names) != 0 {
		t.Errorf("見終えた = %v, want 空（ep1 だけ完了のグループは watched ではない）", names)
	}
	// フォルダ由来のタグでも絞れる。
	if names := libraryItemNames(decode[gen.LibraryPage](t, f.env.get("/api/library?tag="+strconv.FormatInt(f.folder, 10), f.owner))); !slices.Equal(names, []string{"group:show"}) {
		t.Errorf("タグ show = %v", names)
	}

	rec = f.env.get("/api/library?cursor=%21", f.owner)
	assertErrorBody(t, "解釈できないカーソル", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonInvalidCursor})
	rec = f.env.get("/api/library?query="+strings.Repeat("a", 101), f.owner)
	assertErrorBody(t, "長すぎる検索語", rec.Code, rec.Body.Bytes(), wantError{
		status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonSearchTooLong, limit: maxQueryLength,
	})
	rec = f.env.get("/api/library?sort=bogus", f.owner)
	assertErrorBody(t, "不明な並び順（reason を返さない）", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest})
}

// failingRootsFolders は登録フォルダの一覧だけを読めないフォルダの問い合わせ先である。
type failingRootsFolders struct{ Folders }

func (failingRootsFolders) ListMediaFolders(context.Context) ([]domain.MediaFolder, error) {
	return nil, errors.New("登録フォルダを読めない")
}

// strayGroupLibrary は、登録フォルダの下に無いフォルダのグループを返す、索引の
// 食い違った問い合わせ先である。
type strayGroupLibrary struct{ LibraryItems }

func (l strayGroupLibrary) ListLibrary(ctx context.Context, audience domain.Audience, q domain.VideoQuery) (domain.LibraryPage, error) {
	page, err := l.LibraryItems.ListLibrary(ctx, audience, q)
	for i := range page.Items {
		if page.Items[i].Group != nil {
			group := *page.Items[i].Group
			group.Path = "/not-registered/" + group.Name
			page.Items[i].Group = &group
		}
	}
	return page, err
}

// 一覧の項目・total・カーソルと、グループの folder は同じスナップショットから作る。
// 登録フォルダを別に読めなくてもグループは落ちず、フォルダが登録フォルダの下に無いグループは
// folder を省いて返す（specs/048-groups/contracts/screen-api.md）。
func TestListLibraryResolvesGroupFoldersFromSameSnapshot(t *testing.T) {
	f := newLibraryFixtureWith(t, func(options Options) Options {
		options.Folders = failingRootsFolders{options.Folders}
		return options
	})
	rec := f.env.get("/api/library", f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	page := decode[gen.LibraryPage](t, rec)
	if names := libraryItemNames(page); !slices.Equal(names, []string{"group:pair", "group:show", "solo"}) || page.Total != 3 {
		t.Fatalf("項目 = %v・total %d", names, page.Total)
	}
	if show := libraryGroupNamed(t, page, "show"); show.Folder == nil || show.Folder.RootId != f.rootID || show.Folder.Path != "show" {
		t.Errorf("folder = %+v", show.Folder)
	}

	stray := newLibraryFixtureWith(t, func(options Options) Options {
		options.Library = strayGroupLibrary{options.Library}
		return options
	})
	rec = stray.env.get("/api/library", stray.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("登録フォルダの下に無いグループ: status = %d: %s", rec.Code, rec.Body)
	}
	if show := libraryGroupNamed(t, decode[gen.LibraryPage](t, rec), "show"); show.Folder != nil {
		t.Errorf("登録フォルダの下に無いグループの folder = %+v, want 無し", show.Folder)
	}
}

// 「すべて選択」の id は一覧の項目に合わせる。一部だけが当たったグループは当たった
// メンバーの id だけで、全メンバーが当たったグループは全メンバーの id である
// （027 の受け入れ条件 7）。
func TestListLibraryIdsFollowItems(t *testing.T) {
	f := newLibraryFixture(t)
	rec := f.env.get("/api/library/ids?query=ep10&tag=999", f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	got := decode[gen.VideoIdsResponse](t, rec)
	if !slices.Equal(got.Ids, []int64{f.ids["ep10"]}) {
		t.Errorf("ep10 の ids = %v, want [%d]", got.Ids, f.ids["ep10"])
	}
	whole := decode[gen.VideoIdsResponse](t, f.env.get("/api/library/ids?query=show", f.owner))
	slices.Sort(whole.Ids)
	want := []int64{f.ids["ep1"], f.ids["ep2"], f.ids["ep10"]}
	slices.Sort(want)
	if !slices.Equal(whole.Ids, want) {
		t.Errorf("show の ids = %v, want %v", whole.Ids, want)
	}
	if got.MissingTagIds == nil || !slices.Equal(*got.MissingTagIds, []int64{999}) {
		t.Errorf("missingTagIds = %v", got.MissingTagIds)
	}
	tags := "/api/library/ids?" + strings.Repeat("tag=1&", maxTagFilterCount+1)
	rec = f.env.get(tags, f.owner)
	assertErrorBody(t, "17 個のタグ", rec.Code, rec.Body.Bytes(), wantError{
		status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonTooManyTagFilters, limit: maxTagFilterCount,
	})
}

// ゲストには公開のメンバーだけで数えた項目を返し、公開のメンバーが1本のグループも
// グループの項目にする（specs/048-groups/research.md R-6）。視聴の値とタグは出さず、所有者の
// データに依る条件は 400、「すべて選択」は 401 である（data-model.md §7）。
func TestListLibraryForGuest(t *testing.T) {
	f := newLibraryFixture(t)
	rec := f.env.get("/api/library")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	page := decode[gen.LibraryPage](t, rec)
	if names := libraryItemNames(page); !slices.Equal(names, []string{"group:pair", "group:show", "solo"}) || page.Total != 3 {
		t.Fatalf("ゲストの項目 = %v・total %d", names, page.Total)
	}
	if pair := libraryGroupNamed(t, page, "pair"); !slices.Equal(pair.VideoIds, []int64{f.ids["p1"]}) {
		t.Errorf("ゲストの pair = %+v, want 公開の p1 だけ", pair)
	}
	show := libraryGroupNamed(t, page, "show")
	if show.VideoCount != 2 || !slices.Equal(show.VideoIds, []int64{f.ids["ep1"], f.ids["ep2"]}) || show.OpenVideoId != f.ids["ep1"] {
		t.Errorf("ゲストの show = %+v", show)
	}
	if show.DurationMs == nil || *show.DurationMs != 120_000 || show.SizeBytes != 200 {
		t.Errorf("ゲストの show の長さ・大きさ = %v・%d", show.DurationMs, show.SizeBytes)
	}

	// 省いた欄が JSON に無いこと。
	items := rawItems(t, rec.Body.Bytes(), "items")
	for _, item := range items {
		if string(item["kind"]) != `"group"` {
			var video map[string]json.RawMessage
			if err := json.Unmarshal(item["video"], &video); err != nil {
				t.Fatal(err)
			}
			assertGuestVideo(t, "ゲストの動画の項目", video)
			continue
		}
		var group map[string]json.RawMessage
		if err := json.Unmarshal(item["group"], &group); err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{"watchedCount", "watchState", "lastPlayedAt", "favorite"} {
			if value, ok := group[field]; ok {
				t.Errorf("ゲストのグループに %s = %s", field, value)
			}
		}
		if got := string(group["tags"]); got != "[]" {
			t.Errorf("ゲストのグループの tags = %s", got)
		}
		var previews []gen.FolderPreview
		if err := json.Unmarshal(group["previews"], &previews); err != nil {
			t.Fatal(err)
		}
		if ids := previewIDs(previews); string(group["name"]) == `"show"` && !slices.Equal(ids, []int64{f.ids["ep2"]}) {
			t.Errorf("ゲストのグループの絵柄のサムネイル = %v, want 公開の ep2 だけ", ids)
		}
	}

	for _, query := range []string{"watch=watched", "sort=playedAsc", "sort=playedDesc", "tag=" + strconv.FormatInt(f.manual, 10)} {
		rec := f.env.get("/api/library?" + query)
		assertErrorBody(t, "ゲストの "+query, rec.Code, rec.Body.Bytes(),
			wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonGuestFilterNotAllowed})
		if rec := f.env.get("/api/library?"+query, f.owner); rec.Code != http.StatusOK {
			t.Errorf("所有者の %s: status = %d, want 200", query, rec.Code)
		}
	}
	assertUnauthenticated(t, "ゲストの /api/library/ids", f.env.get("/api/library/ids"))
}

// グループの画面は絞り込みに関係なく子孫の動画の全部から作ったグループと、項目を位置の順に返し、
// 無いグループは 404 である。ゲストは公開の動画だけで読み、公開の動画が 1 本でも読める
// （specs/048-groups/contracts/screen-api.md「Statuses」）。
func TestGetGroup(t *testing.T) {
	f := newLibraryFixture(t)
	path := f.groupPath(t, "show")

	rec := f.env.get(path+"?trail=999", f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if rec.Header().Get("Cache-Control") != cacheNoStore {
		t.Errorf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
	detail := decode[gen.GroupDetail](t, rec)
	show := detail.Group
	if show.Name != "show" || show.VideoCount != 3 || show.Folder == nil || show.Folder.Path != "show" || show.OpenVideoId != f.ids["ep2"] {
		t.Errorf("show = %+v", show)
	}
	if len(detail.Trail) != 0 || detail.NextCursor != nil {
		t.Errorf("trail = %+v, nextCursor = %v; want 空・無し", detail.Trail, detail.NextCursor)
	}
	var titles []string
	for _, item := range detail.Items {
		if item.Kind != gen.GroupItemKindVideo || item.Video == nil {
			t.Fatalf("項目 = %+v, want 動画", item)
		}
		titles = append(titles, item.Video.Title)
	}
	if !slices.Equal(titles, []string{"ep1", "ep2", "ep10"}) {
		t.Errorf("項目 = %v, want [ep1 ep2 ep10]", titles)
	}

	// 続きのページはカーソルで読み、読めないカーソルと範囲外の limit は 400。
	first := decode[gen.GroupItemPage](t, f.env.get(path+"/items?limit=2", f.owner))
	if len(first.Items) != 2 || first.NextCursor == nil {
		t.Fatalf("1 ページ目 = %+v", first)
	}
	second := decode[gen.GroupItemPage](t, f.env.get(path+"/items?limit=2&cursor="+url.QueryEscape(*first.NextCursor), f.owner))
	if len(second.Items) != 1 || second.Items[0].Video == nil || second.Items[0].Video.Title != "ep10" || second.NextCursor != nil {
		t.Errorf("2 ページ目 = %+v", second)
	}
	rec = f.env.get(path+"/items?cursor=broken", f.owner)
	assertErrorBody(t, "読めないカーソル", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonInvalidCursor})
	for _, query := range []string{"limit=0", "limit=201"} {
		if rec := f.env.get(path+"/items?"+query, f.owner); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", query, rec.Code)
		}
	}
	if rec := f.env.get(path+"?trail=x", f.owner); rec.Code != http.StatusBadRequest {
		t.Errorf("読めない trail: status = %d, want 400", rec.Code)
	}
	for _, target := range []string{"/api/groups/999999", "/api/groups/999999/items"} {
		rec := f.env.get(target, f.owner)
		assertErrorBody(t, target, rec.Code, rec.Body.Bytes(), wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound})
	}

	guest := f.env.get(path)
	if guest.Code != http.StatusOK {
		t.Fatalf("ゲストの show: status = %d: %s", guest.Code, guest.Body)
	}
	if got := decode[gen.GroupDetail](t, guest); got.Group.VideoCount != 2 || got.Group.WatchState != nil || len(got.Group.Tags) != 0 || len(got.Items) != 2 {
		t.Errorf("ゲストの show = %+v", got)
	}
	if rec := f.env.get(f.groupPath(t, "pair")); rec.Code != http.StatusOK {
		t.Errorf("ゲストの pair（公開1本）: status = %d, want 200", rec.Code)
	}
}
