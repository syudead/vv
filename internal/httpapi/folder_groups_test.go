package httpapi

import (
	"context"
	"encoding/json"
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
)

// フォルダのまとめ方の例外（specs/017-folder-groups/contracts/folder-groups-api.md §1）と、
// フォルダの自動グループのタグ化（specs/048-groups/research.md R-11）を、本物の認証・保存層・
// 経路をつないで確かめる。フォルダの構成は newLibraryFixture のとおり。

func (f *libraryFixture) groupingPath(rel string) string {
	return "/api/folders/" + strconv.FormatInt(f.rootID, 10) + "/grouping?path=" + url.QueryEscape(rel)
}

// groupingResult は変更の経路の応答である。
type groupingResult struct {
	code int
	body []byte
}

func (f *libraryFixture) putGrouping(rel, body string) groupingResult {
	rec := f.env.serve(authRequest{method: http.MethodPut, target: f.groupingPath(rel), body: body, cookies: []*http.Cookie{f.owner}})
	return groupingResult{code: rec.Code, body: rec.Body.Bytes()}
}

// folderGroupID はフォルダ rel の grouping が名指す自動グループの id を返す。無ければ失敗にする。
func (f *libraryFixture) folderGroupID(t *testing.T, rel string) int64 {
	t.Helper()
	grouping := f.ownerFolder(t, rel).Folder.Grouping
	if grouping == nil || grouping.GroupId == nil {
		t.Fatalf("フォルダ %q の grouping に groupId が無い: %+v", rel, grouping)
	}
	return *grouping.GroupId
}

// assertGrouping は grouping の mode と、groupId の有無が want どおりかを確かめる。
func assertGrouping(t *testing.T, label string, got *gen.FolderGrouping, mode gen.FolderGroupingMode, hasGroup bool) {
	t.Helper()
	if got == nil || got.Mode != mode || (got.GroupId != nil) != hasGroup {
		t.Errorf("%s: grouping = %+v, want mode %s, groupId あり %v", label, got, mode, hasGroup)
	}
}

func decodeBytes[T any](t *testing.T, body []byte) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("応答を JSON として読めない: %s", body)
	}
	return out
}

// addVideo は登録フォルダの下の rel（拡張子なし）に動画を足し、索引を作り直す。
func (f *libraryFixture) addVideo(t *testing.T, rel string) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(f.mediaDir, filepath.FromSlash(rel)+".mp4")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	name := filepath.Base(rel)
	result, err := f.env.db.ScanIndex().UpsertVideo(ctx, domain.VideoFile{
		Path: path, Title: name, ContentKey: "key-" + name, SizeBytes: 100,
		MTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), AddedAt: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), Container: "mp4",
	})
	if err != nil {
		t.Fatal(err)
	}
	f.ids[name] = result.ID
	if err := f.env.db.ScanIndex().RebuildFolderIndex(ctx); err != nil {
		t.Fatal(err)
	}
}

func (f *libraryFixture) ownerLibraryNames(t *testing.T) []string {
	t.Helper()
	return libraryItemNames(decode[gen.LibraryPage](t, f.env.get("/api/library", f.owner)))
}

// ownerFolder は所有者の GET /api/folders/{rootId}?path=rel を読む。
func (f *libraryFixture) ownerFolder(t *testing.T, rel string) gen.FolderListing {
	t.Helper()
	rec := f.env.get("/api/folders/"+strconv.FormatInt(f.rootID, 10)+"?path="+url.QueryEscape(rel), f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET フォルダ %q: status = %d: %s", rel, rec.Code, rec.Body)
	}
	return decode[gen.FolderListing](t, rec)
}

func tagCount(t *testing.T, f *libraryFixture) int {
	t.Helper()
	return len(decode[gen.TagList](t, f.env.get("/api/tags", f.owner)).Items)
}

// 受け入れ条件 3 のサーバー側: 例外の直後の一覧の項目が変わり、auto で戻る。
func TestSetFolderGrouping(t *testing.T) {
	f := newLibraryFixture(t)

	res := f.putGrouping("show", `{"mode":"ungroup"}`)
	if res.code != http.StatusOK {
		t.Fatalf("ungroup: status = %d: %s", res.code, res.body)
	}
	got := decodeBytes[gen.FolderGrouping](t, res.body)
	assertGrouping(t, "ungroup の応答", &got, gen.FolderGroupingModeUngroup, false)
	// ライブラリはグループ（specs/048-groups）から読む。ungroup は show の自動グループを解くので、
	// メンバーは動画として並ぶ（specs/048-groups/research.md R-4）。
	if names := f.ownerLibraryNames(t); !slices.Equal(names, []string{"ep1", "ep10", "ep2", "group:pair", "solo"}) {
		t.Errorf("ungroup の後の項目 = %v", names)
	}
	// 同じ値の再設定も 200。
	if res := f.putGrouping("show", `{"mode":"ungroup"}`); res.code != http.StatusOK {
		t.Errorf("同じ値の再設定: status = %d: %s", res.code, res.body)
	}

	res = f.putGrouping("show", `{"mode":"auto"}`)
	got = decodeBytes[gen.FolderGrouping](t, res.body)
	if res.code != http.StatusOK {
		t.Errorf("auto: status = %d", res.code)
	}
	assertGrouping(t, "auto の応答", &got, gen.FolderGroupingModeAuto, true)
	if names := f.ownerLibraryNames(t); !slices.Equal(names, []string{"group:pair", "group:show", "solo"}) {
		t.Errorf("auto の後の項目 = %v", names)
	}
	// groupId はそのフォルダの自動グループで、フォルダの読み出しも同じ id を返す。
	if got.GroupId != nil && *got.GroupId != f.folderGroupID(t, "show") {
		t.Errorf("auto の応答の groupId = %d, フォルダの groupId = %d", *got.GroupId, f.folderGroupID(t, "show"))
	}

	// 直下をまとめる: 子フォルダを持つ登録フォルダそのものでも、直下の動画がグループになる。
	f.addVideo(t, "solo2")
	res = f.putGrouping("", `{"mode":"groupDirect"}`)
	got = decodeBytes[gen.FolderGrouping](t, res.body)
	if res.code != http.StatusOK {
		t.Errorf("登録フォルダの groupDirect: status = %d", res.code)
	}
	assertGrouping(t, "登録フォルダの groupDirect の応答", &got, gen.FolderGroupingModeGroupDirect, true)
	root := filepath.Base(f.mediaDir)
	if names := f.ownerLibraryNames(t); !slices.Equal(names, []string{"group:" + root, "group:pair", "group:show"}) {
		t.Errorf("groupDirect の後の項目 = %v", names)
	}

	// FolderSummary.grouping は所有者の応答に入る。
	listing := f.ownerFolder(t, "")
	assertGrouping(t, "登録フォルダ", listing.Folder.Grouping, gen.FolderGroupingModeGroupDirect, true)
	for _, child := range listing.Folders {
		assertGrouping(t, "子フォルダ "+child.Name, child.Grouping, gen.FolderGroupingModeAuto, true)
	}
	roots := decode[gen.RootFolderListing](t, f.env.get("/api/folders", f.owner))
	if len(roots.Folders) != 1 || roots.Folders[0].Grouping == nil || roots.Folders[0].Grouping.Mode != gen.FolderGroupingModeGroupDirect {
		t.Errorf("GET /api/folders の grouping = %+v", roots.Folders)
	}

	cases := []struct {
		name, rel, body string
		want            int
	}{
		{"mode が不明", "show", `{"mode":"both"}`, http.StatusBadRequest},
		{"mode が無い", "show", `{}`, http.StatusBadRequest},
		{"パスが不正", "../show", `{"mode":"ungroup"}`, http.StatusBadRequest},
		{"フォルダが無い", "gone", `{"mode":"ungroup"}`, http.StatusNotFound},
	}
	for _, c := range cases {
		if res := f.putGrouping(c.rel, c.body); res.code != c.want {
			t.Errorf("%s: status = %d, want %d: %s", c.name, res.code, c.want, res.body)
		}
	}
	unknown := f.env.serve(authRequest{method: http.MethodPut, target: "/api/folders/999999/grouping", body: `{"mode":"ungroup"}`, cookies: []*http.Cookie{f.owner}})
	if unknown.Code != http.StatusNotFound {
		t.Errorf("登録されていない rootId: status = %d", unknown.Code)
	}
}

// 要件 15 のサーバー側: フォルダの grouping が名指す自動グループを POST /api/groups/{id}/tag で
// タグに変えると、メンバーが単体に戻ってフォルダ由来のタグを得て、grouping は ungroup になり
// groupId が消える。登録フォルダそのもののグループも変えられる（research.md R-11）。旧ルートは無い。
func TestTagFolderAutomaticGroup(t *testing.T) {
	f := newLibraryFixture(t)
	before := tagCount(t, f)
	rec := f.groupRequest(http.MethodPost, groupWritePath(f.folderGroupID(t, "show"), "/tag"), "", f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if got := decode[gen.GroupTagResult](t, rec); got.Tag.Id != f.folder || got.Tag.Name != "show" || got.Created {
		t.Errorf("応答 = %+v", got)
	}
	if after := tagCount(t, f); after != before {
		t.Errorf("タグの数 = %d, want %d", after, before)
	}
	if names := f.ownerLibraryNames(t); !slices.Equal(names, []string{"ep1", "ep10", "ep2", "group:pair", "solo"}) {
		t.Fatalf("タグ化の後の項目 = %v", names)
	}
	page := decode[gen.VideoPage](t, f.env.get("/api/videos", f.owner))
	for _, item := range page.Items {
		if strings.HasPrefix(item.Title, "ep") && !slices.ContainsFunc(item.Tags, func(tag gen.VideoTag) bool { return tag.Id == f.folder }) {
			t.Errorf("%s のタグ = %+v, want show", item.Title, item.Tags)
		}
	}
	assertGrouping(t, "タグ化の後", f.ownerFolder(t, "show").Folder.Grouping, gen.FolderGroupingModeUngroup, false)

	// 登録フォルダそのもののグループ。
	f.addVideo(t, "solo2")
	if res := f.putGrouping("", `{"mode":"groupDirect"}`); res.code != http.StatusOK {
		t.Fatalf("groupDirect: status = %d", res.code)
	}
	rec = f.groupRequest(http.MethodPost, groupWritePath(f.folderGroupID(t, ""), "/tag"), "", f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("登録フォルダのグループ: status = %d: %s", rec.Code, rec.Body)
	}
	assertGrouping(t, "登録フォルダのタグ化の後", f.ownerFolder(t, "").Folder.Grouping, gen.FolderGroupingModeUngroup, false)

	old := f.env.serve(authRequest{method: http.MethodPost, target: "/api/folders/" + strconv.FormatInt(f.rootID, 10) + "/grouping/tag?path=pair", cookies: []*http.Cookie{f.owner}})
	if old.Code == http.StatusOK {
		t.Errorf("旧ルート POST …/grouping/tag が残っている: status = %d", old.Code)
	}
}

// ゲストは変更の経路で 401 になり、FolderSummary に grouping が入らない。
func TestFolderGroupingForGuest(t *testing.T) {
	f := newLibraryFixture(t)

	assertUnauthenticated(t, "PUT grouping", f.env.serve(authRequest{method: http.MethodPut, target: f.groupingPath("show"), body: `{"mode":"ungroup"}`}))
	if names := f.ownerLibraryNames(t); !slices.Equal(names, []string{"group:pair", "group:show", "solo"}) {
		t.Errorf("ゲストの要求で一覧が変わった: %v", names)
	}

	for _, target := range []string{"/api/folders", "/api/folders/" + strconv.FormatInt(f.rootID, 10)} {
		rec := f.env.get(target)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d: %s", target, rec.Code, rec.Body)
		}
		if strings.Contains(rec.Body.String(), `"grouping"`) {
			t.Errorf("%s: ゲストの応答に grouping がある: %s", target, rec.Body)
		}
	}
	if rec := f.env.get("/api/folders", f.owner); !strings.Contains(rec.Body.String(), `"grouping"`) {
		t.Errorf("所有者の応答に grouping が無い: %s", rec.Body)
	}
}
