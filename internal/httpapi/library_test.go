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
	if _, err := db.Playback().SaveProgress(ctx, "key-ep1", domain.Progress{PositionMs: 60_000, DurationMs: 60_000, Completed: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Playback().SaveProgress(ctx, "key-ep2", domain.Progress{PositionMs: 30_000, DurationMs: 60_000}); err != nil {
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

func (f *libraryFixture) groupPath(rel string) string {
	return "/api/folders/" + strconv.FormatInt(f.rootID, 10) + "/group?path=" + url.QueryEscape(rel)
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

	// 受け入れ条件 11: 1本のメンバーだけが検索語に当たっても、グループは全メンバーで出る。
	hit := decode[gen.LibraryPage](t, f.env.get("/api/library?query=ep10", f.owner))
	if names := libraryItemNames(hit); !slices.Equal(names, []string{"group:show"}) || hit.Total != 1 {
		t.Fatalf("ep10 の検索 = %v", names)
	}
	if got := libraryGroupNamed(t, hit, "show").VideoCount; got != 3 {
		t.Errorf("ep10 の検索のグループの本数 = %d, want 3", got)
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

	if rec := f.env.get("/api/library?cursor=%21", f.owner); rec.Code != http.StatusBadRequest {
		t.Errorf("解釈できないカーソル: status = %d", rec.Code)
	}
	if rec := f.env.get("/api/library?query="+strings.Repeat("a", 101), f.owner); rec.Code != http.StatusBadRequest {
		t.Errorf("長すぎる検索語: status = %d", rec.Code)
	}
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
// 登録フォルダを別に読めなくてもグループは落ちず、登録フォルダの下に無いグループは
// 黙って落とさずに 500 にする（items が total と食い違わない）。
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
	if show := libraryGroupNamed(t, page, "show"); show.Folder.RootId != f.rootID || show.Folder.Path != "show" {
		t.Errorf("folder = %+v", show.Folder)
	}

	stray := newLibraryFixtureWith(t, func(options Options) Options {
		options.Library = strayGroupLibrary{options.Library}
		return options
	})
	if rec := stray.env.get("/api/library", stray.owner); rec.Code != http.StatusInternalServerError {
		t.Errorf("登録フォルダの下に無いグループ: status = %d: %s", rec.Code, rec.Body)
	}
}

// 「すべて選択」の id は当たったグループの全メンバーを含む（要件 21）。
func TestListLibraryIdsIncludesAllMembers(t *testing.T) {
	f := newLibraryFixture(t)
	rec := f.env.get("/api/library/ids?query=ep10&tag=999", f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	got := decode[gen.VideoIdsResponse](t, rec)
	slices.Sort(got.Ids)
	want := []int64{f.ids["ep1"], f.ids["ep2"], f.ids["ep10"]}
	slices.Sort(want)
	if !slices.Equal(got.Ids, want) {
		t.Errorf("ids = %v, want %v", got.Ids, want)
	}
	if got.MissingTagIds == nil || !slices.Equal(*got.MissingTagIds, []int64{999}) {
		t.Errorf("missingTagIds = %v", got.MissingTagIds)
	}
	tags := "/api/library/ids?" + strings.Repeat("tag=1&", maxTagFilterCount+1)
	if rec := f.env.get(tags, f.owner); rec.Code != http.StatusBadRequest {
		t.Errorf("17 個のタグ: status = %d", rec.Code)
	}
}

// ゲストには公開のメンバーだけで数えた項目を返し、公開のメンバーが1本のグループは
// 動画の項目にする。視聴の値とタグは出さず、所有者のデータに依る条件は 400、
// 「すべて選択」は 401 である（data-model.md §7）。
func TestListLibraryForGuest(t *testing.T) {
	f := newLibraryFixture(t)
	rec := f.env.get("/api/library")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	page := decode[gen.LibraryPage](t, rec)
	if names := libraryItemNames(page); !slices.Equal(names, []string{"group:show", "p1", "solo"}) || page.Total != 3 {
		t.Fatalf("ゲストの項目 = %v・total %d", names, page.Total)
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
		for _, field := range []string{"watchedCount", "watchState", "lastPlayedAt"} {
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
		if ids := previewIDs(previews); !slices.Equal(ids, []int64{f.ids["ep2"]}) {
			t.Errorf("ゲストのグループの絵柄のサムネイル = %v, want 公開の ep2 だけ", ids)
		}
	}

	for _, query := range []string{"watch=watched", "sort=playedAsc", "sort=playedDesc", "tag=" + strconv.FormatInt(f.manual, 10)} {
		if rec := f.env.get("/api/library?" + query); rec.Code != http.StatusBadRequest {
			t.Errorf("ゲストの %s: status = %d, want 400", query, rec.Code)
		}
		if rec := f.env.get("/api/library?"+query, f.owner); rec.Code != http.StatusOK {
			t.Errorf("所有者の %s: status = %d, want 200", query, rec.Code)
		}
	}
	assertUnauthenticated(t, "ゲストの /api/library/ids", f.env.get("/api/library/ids"))
}

// グループ1件は絞り込みに関係なく全メンバーから作り、グループでないフォルダは 404、
// パスの規則違反は 400 である。ゲストは公開のメンバーが2本以上なければ 404。
func TestGetFolderGroup(t *testing.T) {
	f := newLibraryFixture(t)

	rec := f.env.get(f.groupPath("show"), f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	show := decode[gen.LibraryGroup](t, rec)
	if show.Name != "show" || show.VideoCount != 3 || show.Folder.Path != "show" || show.OpenVideoId != f.ids["ep2"] {
		t.Errorf("show = %+v", show)
	}
	if rec.Header().Get("Cache-Control") != cacheNoStore {
		t.Errorf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
	if pair := decode[gen.LibraryGroup](t, f.env.get(f.groupPath("pair"), f.owner)); pair.VideoCount != 2 {
		t.Errorf("pair = %+v", pair)
	}
	for _, rel := range []string{"", "none", "show/ep1.mp4"} {
		if rec := f.env.get(f.groupPath(rel), f.owner); rec.Code != http.StatusNotFound {
			t.Errorf("%q: status = %d, want 404", rel, rec.Code)
		}
	}
	if rec := f.env.get(f.groupPath("../show"), f.owner); rec.Code != http.StatusBadRequest {
		t.Errorf("規則違反のパス: status = %d, want 400", rec.Code)
	}
	if rec := f.env.get("/api/folders/999/group?path=show", f.owner); rec.Code != http.StatusNotFound {
		t.Errorf("無い登録フォルダ: status = %d, want 404", rec.Code)
	}

	guest := f.env.get(f.groupPath("show"))
	if guest.Code != http.StatusOK {
		t.Fatalf("ゲストの show: status = %d: %s", guest.Code, guest.Body)
	}
	if got := decode[gen.LibraryGroup](t, guest); got.VideoCount != 2 || got.WatchState != nil || len(got.Tags) != 0 {
		t.Errorf("ゲストの show = %+v", got)
	}
	if rec := f.env.get(f.groupPath("pair")); rec.Code != http.StatusNotFound {
		t.Errorf("ゲストの pair（公開1本）: status = %d, want 404", rec.Code)
	}
}
