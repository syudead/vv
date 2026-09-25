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

// フォルダのまとめ方の例外とグループのタグ化
// （specs/017-folder-groups/contracts/folder-groups-api.md §1・§2）を、本物の認証・
// 保存層・経路をつないで確かめる。フォルダの構成は newLibraryFixture のとおり。

func (f *libraryFixture) groupingPath(rel string) string {
	return "/api/folders/" + strconv.FormatInt(f.rootID, 10) + "/grouping?path=" + url.QueryEscape(rel)
}

func (f *libraryFixture) groupingTagPath(rel string) string {
	return "/api/folders/" + strconv.FormatInt(f.rootID, 10) + "/grouping/tag?path=" + url.QueryEscape(rel)
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

func (f *libraryFixture) postGroupingTag(rel string) groupingResult {
	rec := f.env.serve(authRequest{method: http.MethodPost, target: f.groupingTagPath(rel), cookies: []*http.Cookie{f.owner}})
	return groupingResult{code: rec.Code, body: rec.Body.Bytes()}
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
	if got := decodeBytes[gen.FolderGrouping](t, res.body); got != (gen.FolderGrouping{Mode: gen.Ungroup}) {
		t.Errorf("ungroup の応答 = %+v", got)
	}
	if names := f.ownerLibraryNames(t); !slices.Equal(names, []string{"ep1", "ep10", "ep2", "group:pair", "solo"}) {
		t.Errorf("ungroup の後の項目 = %v", names)
	}
	// 同じ値の再設定も 200。
	if res := f.putGrouping("show", `{"mode":"ungroup"}`); res.code != http.StatusOK {
		t.Errorf("同じ値の再設定: status = %d: %s", res.code, res.body)
	}

	res = f.putGrouping("show", `{"mode":"auto"}`)
	if got := decodeBytes[gen.FolderGrouping](t, res.body); res.code != http.StatusOK || got != (gen.FolderGrouping{Mode: gen.Auto, Grouped: true, Taggable: true}) {
		t.Errorf("auto: status = %d, 応答 = %+v", res.code, got)
	}
	if names := f.ownerLibraryNames(t); !slices.Equal(names, []string{"group:pair", "group:show", "solo"}) {
		t.Errorf("auto の後の項目 = %v", names)
	}

	// 直下をまとめる: 子フォルダを持つ登録フォルダそのものでも、直下の動画がグループになる。
	f.addVideo(t, "solo2")
	res = f.putGrouping("", `{"mode":"groupDirect"}`)
	if got := decodeBytes[gen.FolderGrouping](t, res.body); res.code != http.StatusOK || got != (gen.FolderGrouping{Mode: gen.GroupDirect, Grouped: true}) {
		t.Errorf("登録フォルダの groupDirect: status = %d, 応答 = %+v（taggable は false）", res.code, got)
	}
	root := filepath.Base(f.mediaDir)
	if names := f.ownerLibraryNames(t); !slices.Equal(names, []string{"group:" + root, "group:pair", "group:show"}) {
		t.Errorf("groupDirect の後の項目 = %v", names)
	}

	// FolderSummary.grouping は所有者の応答に入る。
	listing := f.ownerFolder(t, "")
	if listing.Folder.Grouping == nil || *listing.Folder.Grouping != (gen.FolderGrouping{Mode: gen.GroupDirect, Grouped: true}) {
		t.Errorf("登録フォルダの grouping = %+v", listing.Folder.Grouping)
	}
	for _, child := range listing.Folders {
		if child.Grouping == nil || *child.Grouping != (gen.FolderGrouping{Mode: gen.Auto, Grouped: true, Taggable: true}) {
			t.Errorf("子フォルダ %s の grouping = %+v", child.Name, child.Grouping)
		}
	}
	roots := decode[gen.RootFolderListing](t, f.env.get("/api/folders", f.owner))
	if len(roots.Folders) != 1 || roots.Folders[0].Grouping == nil || roots.Folders[0].Grouping.Mode != gen.GroupDirect {
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

// 受け入れ条件 4・5 のサーバー側: タグ化の後にメンバーが単体に戻ってそれぞれにタグが
// 付き、同名のタグやシノニムがあればそれを使う。
func TestTagFolderGroup(t *testing.T) {
	t.Run("同名のタグを使う", func(t *testing.T) {
		f := newLibraryFixture(t)
		before := tagCount(t, f)
		res := f.postGroupingTag("show")
		if res.code != http.StatusOK {
			t.Fatalf("status = %d: %s", res.code, res.body)
		}
		got := decodeBytes[gen.FolderGroupTagResult](t, res.body)
		if got.Tag.Id != f.folder || got.Tag.Name != "show" || got.Created || got.Grouping != (gen.FolderGrouping{Mode: gen.Ungroup}) {
			t.Errorf("応答 = %+v", got)
		}
		if after := tagCount(t, f); after != before {
			t.Errorf("タグの数 = %d, want %d", after, before)
		}
		page := decode[gen.LibraryPage](t, f.env.get("/api/library", f.owner))
		if names := libraryItemNames(page); !slices.Equal(names, []string{"ep1", "ep10", "ep2", "group:pair", "solo"}) {
			t.Fatalf("タグ化の後の項目 = %v", names)
		}
		for _, item := range page.Items {
			if item.Video == nil || !strings.HasPrefix(item.Video.Title, "ep") {
				continue
			}
			if !slices.ContainsFunc(item.Video.Tags, func(tag gen.VideoTag) bool { return tag.Id == f.folder && tag.FromFolder }) {
				t.Errorf("%s のタグ = %+v, want show（フォルダ由来）", item.Video.Title, item.Video.Tags)
			}
		}
		if listing := f.ownerFolder(t, "show"); listing.Folder.Grouping == nil || *listing.Folder.Grouping != (gen.FolderGrouping{Mode: gen.Ungroup}) {
			t.Errorf("タグ化の後の grouping = %+v", listing.Folder.Grouping)
		}
		// もうグループではないので、2回目は 409。
		if res := f.postGroupingTag("show"); res.code != http.StatusConflict || decodeBytes[gen.Error](t, res.body).Code != gen.ErrorCodeConflict {
			t.Errorf("2回目: status = %d: %s", res.code, res.body)
		}
	})

	t.Run("シノニムで引けたタグを使う", func(t *testing.T) {
		f := newLibraryFixture(t)
		if _, err := f.env.db.Tags().AddSynonym(context.Background(), f.manual, "pair", nil); err != nil {
			t.Fatal(err)
		}
		res := f.postGroupingTag("pair")
		got := decodeBytes[gen.FolderGroupTagResult](t, res.body)
		if res.code != http.StatusOK || got.Tag.Id != f.manual || got.Tag.Name != "手" || got.Created {
			t.Errorf("status = %d, 応答 = %+v", res.code, got)
		}
	})

	t.Run("無ければ作る", func(t *testing.T) {
		f := newLibraryFixture(t)
		before := tagCount(t, f)
		res := f.postGroupingTag("pair")
		got := decodeBytes[gen.FolderGroupTagResult](t, res.body)
		if res.code != http.StatusOK || got.Tag.Name != "pair" || !got.Created {
			t.Fatalf("status = %d, 応答 = %+v", res.code, got)
		}
		if after := tagCount(t, f); after != before+1 {
			t.Errorf("タグの数 = %d, want %d", after, before+1)
		}
		video := decode[gen.Video](t, f.env.get("/api/videos/"+strconv.FormatInt(f.ids["p2"], 10), f.owner))
		if !slices.ContainsFunc(video.Tags, func(tag gen.VideoTag) bool { return tag.Id == got.Tag.Id && tag.FromFolder && !tag.Manual }) {
			t.Errorf("p2 のタグ = %+v", video.Tags)
		}
	})

	t.Run("タグ名に使えないフォルダ名", func(t *testing.T) {
		f := newLibraryFixture(t)
		long := strings.Repeat("a", domain.TagNameMaxLength+1)
		f.addVideo(t, long+"/x1")
		f.addVideo(t, long+"/x2")
		before := tagCount(t, f)
		res := f.postGroupingTag(long)
		if res.code != http.StatusBadRequest || decodeBytes[gen.Error](t, res.body).Code != gen.ErrorCodeInvalidRequest {
			t.Fatalf("status = %d: %s", res.code, res.body)
		}
		if after := tagCount(t, f); after != before {
			t.Errorf("タグの数 = %d, want %d", after, before)
		}
		if listing := f.ownerFolder(t, long); listing.Folder.Grouping == nil || *listing.Folder.Grouping != (gen.FolderGrouping{Mode: gen.Auto, Grouped: true, Taggable: true}) {
			t.Errorf("例外が書かれた: grouping = %+v", listing.Folder.Grouping)
		}
	})

	t.Run("グループでないフォルダと登録フォルダそのもの", func(t *testing.T) {
		f := newLibraryFixture(t)
		if res := f.putGrouping("pair", `{"mode":"ungroup"}`); res.code != http.StatusOK {
			t.Fatalf("ungroup: status = %d", res.code)
		}
		before := tagCount(t, f)
		if res := f.postGroupingTag("pair"); res.code != http.StatusConflict {
			t.Errorf("グループでないフォルダ: status = %d: %s", res.code, res.body)
		}
		f.addVideo(t, "solo2")
		if res := f.putGrouping("", `{"mode":"groupDirect"}`); res.code != http.StatusOK {
			t.Fatalf("groupDirect: status = %d", res.code)
		}
		if res := f.postGroupingTag(""); res.code != http.StatusConflict || decodeBytes[gen.Error](t, res.body).Code != gen.ErrorCodeConflict {
			t.Errorf("登録フォルダそのもの: status = %d: %s", res.code, res.body)
		}
		if after := tagCount(t, f); after != before {
			t.Errorf("タグの数 = %d, want %d", after, before)
		}
		if res := f.postGroupingTag("gone"); res.code != http.StatusNotFound {
			t.Errorf("フォルダが無い: status = %d", res.code)
		}
		if res := f.postGroupingTag("/show"); res.code != http.StatusBadRequest {
			t.Errorf("パスが不正: status = %d", res.code)
		}
	})
}

// ゲストは2つの変更の経路で 401 になり、FolderSummary に grouping が入らない。
func TestFolderGroupingForGuest(t *testing.T) {
	f := newLibraryFixture(t)

	assertUnauthenticated(t, "PUT grouping", f.env.serve(authRequest{method: http.MethodPut, target: f.groupingPath("show"), body: `{"mode":"ungroup"}`}))
	assertUnauthenticated(t, "POST grouping/tag", f.env.serve(authRequest{method: http.MethodPost, target: f.groupingTagPath("show")}))
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
