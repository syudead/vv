package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"maps"
	"net/http"
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

// ゲストへの応答（specs/016-single-account-auth/contracts/guest-api.md §1〜§3・§5）は、
// 本物の認証・保存層・関連動画の組み立てで確かめる（#300）。

// guestArtifactFiles は生成物がすべてあると答える。
type guestArtifactFiles struct{}

func (guestArtifactFiles) PreviewAvailable(string) bool        { return true }
func (guestArtifactFiles) SeekThumbnailsAvailable(string) bool { return true }

// guestFixture は公開と非公開の動画を混ぜたライブラリである。
//
//	root   = <media>        登録フォルダ1
//	  pub/a.mp4             公開。タグ「秘密の名前」と再生位置を持つ
//	  pub/b.mp4             非公開
//	  private/c.mp4         非公開（このフォルダには非公開の動画しか無い）
//	  d.mp4                 公開。読み取りに失敗し、誤りの文に絶対パスを含む
//	other  = <other>        登録フォルダ2（非公開の動画しか無い）
//	  e.mp4                 非公開
type guestFixture struct {
	env       *authEnv
	owner     *http.Cookie
	ids       map[string]int64
	rootID    int64
	otherID   int64
	mediaDir  string
	otherDir  string
	tagID     int64
	transcode *fakeTranscoder
}

const guestSecretTag = "秘密の名前"

func newGuestFixture(t *testing.T, configure bool) *guestFixture {
	t.Helper()
	ctx := context.Background()
	f := &guestFixture{mediaDir: t.TempDir(), otherDir: t.TempDir(), ids: map[string]int64{}, transcode: &fakeTranscoder{body: "fragmented-mp4"}}
	artifactDir := t.TempDir()
	artifacts := &fakeArtifacts{thumbnails: map[string]string{}, previews: map[string]string{}, image: []byte{0xff, 0xd8, 0xff, 0xd9}}

	f.env = newAuthEnvWith(t, t.TempDir(), func(db *store.DB) Options {
		library := db.Library()
		return Options{
			Videos:     library,
			Folders:    library,
			Playback:   db.Playback(),
			Tags:       db.Tags(),
			Catalog:    app.NewCatalog(app.CatalogOptions{Index: library, Ingest: db.Ingest(), Files: guestArtifactFiles{}}),
			Artifacts:  artifacts,
			Transcoder: f.transcode,
		}
	})
	db := f.env.db
	if configure {
		f.owner = f.env.setup()
	}

	for _, dir := range []string{f.mediaDir, f.otherDir} {
		folder, err := db.Settings().AddMediaFolder(ctx, dir)
		if err != nil {
			t.Fatal(err)
		}
		if dir == f.mediaDir {
			f.rootID = folder.ID
		} else {
			f.otherID = folder.ID
		}
	}

	files := []struct{ name, path string }{
		{"a", filepath.Join(f.mediaDir, "pub", "a.mp4")},
		{"b", filepath.Join(f.mediaDir, "pub", "b.mp4")},
		{"c", filepath.Join(f.mediaDir, "private", "c.mp4")},
		{"d", filepath.Join(f.mediaDir, "d.mp4")},
		{"e", filepath.Join(f.otherDir, "e.mp4")},
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for index, file := range files {
		if err := os.MkdirAll(filepath.Dir(file.path), 0o755); err != nil {
			t.Fatal(err)
		}
		content := []byte(strings.Repeat(file.name, 4096))
		if err := os.WriteFile(file.path, content, 0o600); err != nil {
			t.Fatal(err)
		}
		key := "content-key-" + file.name
		result, err := db.ScanIndex().UpsertVideo(ctx, domain.VideoFile{
			Path: file.path, Title: file.name, ContentKey: key, SizeBytes: int64(len(content)),
			MTime: base, AddedAt: base.Add(time.Duration(index) * time.Hour), Container: "mp4",
		})
		if err != nil {
			t.Fatal(err)
		}
		id := result.ID
		f.ids[file.name] = id
		if file.name == "d" {
			if err := db.Ingest().MarkProbeFailed(ctx, id, "ffprobe: "+file.path+": 読めません"); err != nil {
				t.Fatal(err)
			}
			continue
		}
		probe := domain.Probe{DurationMs: 60_000, Width: 640, Height: 360, VideoCodec: "h264", AudioCodec: "aac"}
		if err := db.Ingest().ApplyProbe(ctx, id, probe, domain.Playability{Playable: true}); err != nil {
			t.Fatal(err)
		}
		if err := db.Ingest().SetThumbnailState(ctx, id, domain.ThumbnailStateDone); err != nil {
			t.Fatal(err)
		}
		if err := db.Ingest().SetPreviewState(ctx, id, domain.PreviewStateDone); err != nil {
			t.Fatal(err)
		}
		for kind, target := range map[string]map[string]string{"thumbnail": artifacts.thumbnails, "preview": artifacts.previews} {
			path := filepath.Join(artifactDir, file.name+"."+kind)
			if err := os.WriteFile(path, []byte(kind+"-"+file.name), 0o600); err != nil {
				t.Fatal(err)
			}
			target[key] = path
		}
	}

	tag, err := db.Tags().CreateTag(ctx, guestSecretTag)
	if err != nil {
		t.Fatal(err)
	}
	f.tagID = tag.ID
	if _, _, err := db.Tags().AttachTagByID(ctx, []int64{f.ids["a"], f.ids["b"]}, tag.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Playback().SaveProgress(ctx, "content-key-a", domain.Progress{PositionMs: 30_000, DurationMs: 60_000}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Visibility().SetVideosPublic(ctx, []int64{f.ids["a"], f.ids["d"]}, true); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *guestFixture) videoPath(name, suffix string) string {
	return "/api/videos/" + strconv.FormatInt(f.ids[name], 10) + suffix
}

func (f *guestFixture) folderPath(rootID int64, suffix string) string {
	return "/api/folders/" + strconv.FormatInt(rootID, 10) + suffix
}

// rawItems は応答の項目を JSON のまま返す。省いた欄が無いことを確かめるのに使う。
func rawItems(t *testing.T, body []byte, field string) []map[string]json.RawMessage {
	t.Helper()
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("JSON を読めない: %v: %s", err, body)
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(payload[field], &items); err != nil {
		t.Fatalf("%s を読めない: %v: %s", field, err, body)
	}
	return items
}

// assertGuestVideo は動画の JSON に所有者のデータが無いことを確かめる（guest-api.md §1）。
func assertGuestVideo(t *testing.T, label string, video map[string]json.RawMessage) {
	t.Helper()
	for _, field := range []string{"location", "progress", "probeError"} {
		if value, ok := video[field]; ok {
			t.Errorf("%s: ゲストの応答に %s = %s", label, field, value)
		}
	}
	if got := string(video["tags"]); got != "[]" {
		t.Errorf("%s: tags = %s, want []", label, got)
	}
}

func titlesOfItems(t *testing.T, items []map[string]json.RawMessage) []string {
	t.Helper()
	titles := make([]string, 0, len(items))
	for _, item := range items {
		var title string
		if err := json.Unmarshal(item["title"], &title); err != nil {
			t.Fatal(err)
		}
		titles = append(titles, title)
	}
	slices.Sort(titles)
	return titles
}

func assertStatus(t *testing.T, label string, rec interface{ Result() *http.Response }, want int) {
	t.Helper()
	if got := rec.Result().StatusCode; got != want {
		t.Errorf("%s: status = %d, want %d", label, got, want)
	}
}

// Cookie なしの一覧・件数・詳細・関連動画には公開の動画だけが、所有者のデータを外した形で出る。
func TestGuestSeesOnlyPublicVideosWithoutOwnerData(t *testing.T) {
	f := newGuestFixture(t, true)
	env := f.env

	rec := env.get("/api/videos")
	if rec.Code != http.StatusOK {
		t.Fatalf("ゲストの一覧: status = %d: %s", rec.Code, rec.Body)
	}
	assertAudience(t, "ゲストの一覧", rec, "guest")
	items := rawItems(t, rec.Body.Bytes(), "items")
	if got := titlesOfItems(t, items); !slices.Equal(got, []string{"a", "d"}) {
		t.Errorf("ゲストの一覧 = %v, want [a d]", got)
	}
	if page := decode[gen.VideoPage](t, rec); page.Total != 2 {
		t.Errorf("ゲストの total = %d, want 2", page.Total)
	}
	for _, item := range items {
		assertGuestVideo(t, "ゲストの一覧の項目", item)
		if _, ok := item["folder"]; !ok {
			t.Errorf("ゲストの一覧の項目に folder が無い: %v", item)
		}
	}

	// 所有者には全件と、所有者のデータが出る（比較のため）。
	owner := env.get("/api/videos", f.owner)
	ownerPage := decode[gen.VideoPage](t, owner)
	if ownerPage.Total != 5 {
		t.Errorf("所有者の total = %d, want 5", ownerPage.Total)
	}
	for _, video := range ownerPage.Items {
		switch video.Title {
		case "a":
			if video.Progress == nil || len(video.Tags) != 1 {
				t.Errorf("所有者の a に再生位置かタグが無い: %+v", video)
			}
		case "d":
			if video.ProbeError == nil {
				t.Errorf("所有者の d に probeError が無い: %+v", video)
			}
		}
	}

	// 検索はタグの名前に当たらない（guest-api.md §3）。
	if page := decode[gen.VideoPage](t, env.get("/api/videos?query="+guestSecretTag)); page.Total != 0 {
		t.Errorf("ゲストのタグ名での検索が %d 件に当たった", page.Total)
	}
	if page := decode[gen.VideoPage](t, env.get("/api/videos?query="+guestSecretTag, f.owner)); page.Total != 2 {
		t.Errorf("所有者のタグ名での検索 = %d 件, want 2", page.Total)
	}

	for _, name := range []string{"a", "d"} {
		detail := env.get(f.videoPath(name, ""))
		if detail.Code != http.StatusOK {
			t.Fatalf("ゲストの詳細 %s: status = %d: %s", name, detail.Code, detail.Body)
		}
		var video map[string]json.RawMessage
		if err := json.Unmarshal(detail.Body.Bytes(), &video); err != nil {
			t.Fatal(err)
		}
		assertGuestVideo(t, "ゲストの詳細 "+name, video)
	}
	if video := decode[gen.Video](t, env.get(f.videoPath("a", ""), f.owner)); video.Location == nil || video.Progress == nil {
		t.Errorf("所有者の詳細に location か progress が無い: %+v", video)
	}

	related := env.get(f.videoPath("a", "/related"))
	if related.Code != http.StatusOK {
		t.Fatalf("ゲストの関連動画: status = %d: %s", related.Code, related.Body)
	}
	relatedItems := rawItems(t, related.Body.Bytes(), "items")
	if got := titlesOfItems(t, relatedItems); !slices.Equal(got, []string{"d"}) {
		t.Errorf("ゲストの関連動画 = %v, want [d]", got)
	}
	for _, item := range relatedItems {
		assertGuestVideo(t, "ゲストの関連動画", item)
	}
	if got := decode[gen.RelatedVideos](t, env.get(f.videoPath("a", "/related"), f.owner)); len(got.Items) != 4 {
		t.Errorf("所有者の関連動画 = %d 件, want 4", len(got.Items))
	}
}

// フォルダには公開の動画から導いたものだけが、登録フォルダの絶対パスを外して出る。
// 公開の動画を含まないフォルダは、存在しないフォルダと同じ 404 になる。
func TestGuestFoldersHideFoldersWithoutPublicVideos(t *testing.T) {
	f := newGuestFixture(t, true)
	env := f.env

	roots := env.get("/api/folders")
	if roots.Code != http.StatusOK {
		t.Fatalf("ゲストの登録フォルダ: status = %d: %s", roots.Code, roots.Body)
	}
	folders := rawItems(t, roots.Body.Bytes(), "folders")
	if len(folders) != 1 {
		t.Fatalf("ゲストの登録フォルダ = %d 件, want 1: %s", len(folders), roots.Body)
	}
	listing := decode[gen.RootFolderListing](t, roots)
	if root := listing.Folders[0]; root.RootId != f.rootID || root.VideoCount != 1 || root.FolderCount != 1 {
		t.Errorf("ゲストの登録フォルダ = %+v", root)
	}
	if _, ok := folders[0]["rootPath"]; ok {
		t.Errorf("ゲストの応答に rootPath: %s", roots.Body)
	}
	if strings.Contains(roots.Body.String(), f.mediaDir) {
		t.Errorf("ゲストの応答に絶対パスが出た: %s", roots.Body)
	}
	ownerRoots := decode[gen.RootFolderListing](t, env.get("/api/folders", f.owner))
	if len(ownerRoots.Folders) != 2 || ownerRoots.Folders[0].RootPath == nil {
		t.Errorf("所有者の登録フォルダ = %+v", ownerRoots.Folders)
	}

	folder := env.get(f.folderPath(f.rootID, ""))
	if folder.Code != http.StatusOK {
		t.Fatalf("ゲストのフォルダ: status = %d: %s", folder.Code, folder.Body)
	}
	if strings.Contains(folder.Body.String(), "rootPath") || strings.Contains(folder.Body.String(), `"private"`) {
		t.Errorf("ゲストのフォルダに rootPath か非公開だけのフォルダが出た: %s", folder.Body)
	}
	videos := env.get(f.folderPath(f.rootID, "/videos?scope=subtree"))
	if got := titlesOfItems(t, rawItems(t, videos.Body.Bytes(), "items")); !slices.Equal(got, []string{"a", "d"}) {
		t.Errorf("ゲストのフォルダの動画 = %v, want [a d]", got)
	}
	if page := decode[gen.VideoPage](t, videos); page.Total != 2 {
		t.Errorf("ゲストのフォルダの動画の total = %d", page.Total)
	}

	// 公開の動画を含まないフォルダと、存在しないフォルダが同じ応答になる。
	pairs := [][2]string{
		{f.folderPath(f.otherID, ""), "/api/folders/9999"},
		{f.folderPath(f.otherID, "/videos"), "/api/folders/9999/videos"},
		{f.folderPath(f.otherID, "/videos?scope=subtree&watch=watched"), "/api/folders/9999/videos?scope=subtree&watch=watched"},
		{f.folderPath(f.rootID, "?path=private"), f.folderPath(f.rootID, "?path=missing")},
		{f.folderPath(f.rootID, "/videos?path=private"), f.folderPath(f.rootID, "/videos?path=missing")},
	}
	for _, pair := range pairs {
		hidden, missing := env.get(pair[0]), env.get(pair[1])
		assertSameResponse(t, pair[0], hidden, missing, http.StatusNotFound)
		if owner := env.get(pair[0], f.owner); owner.Code != http.StatusOK {
			t.Errorf("所有者の %s: status = %d", pair[0], owner.Code)
		}
	}
}

// assertSameResponse は、見せない対象と存在しない対象の応答が状態・本文・ヘッダーまで
// 同じであることを確かめる。
func assertSameResponse(t *testing.T, label string, hidden, missing interface {
	Result() *http.Response
}, want int,
) {
	t.Helper()
	h, m := hidden.Result(), missing.Result()
	if h.StatusCode != want || m.StatusCode != want {
		t.Errorf("%s: status = %d / %d, want %d", label, h.StatusCode, m.StatusCode, want)
	}
	hb, mb := readAll(t, h), readAll(t, m)
	if hb != mb {
		t.Errorf("%s: 本文が違う:\n%s\n%s", label, hb, mb)
	}
	if !maps.EqualFunc(h.Header, m.Header, slices.Equal[[]string]) {
		t.Errorf("%s: ヘッダーが違う:\n%v\n%v", label, h.Header, m.Header)
	}
}

func readAll(t *testing.T, resp *http.Response) string {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// Cookie なしで公開の動画の配信と生成物が返り、非公開の動画のそれらは存在しない動画と
// 同じ 404 になる。
func TestGuestMediaRoutesServeOnlyPublicVideos(t *testing.T) {
	f := newGuestFixture(t, true)
	env := f.env

	routes := []struct {
		suffix string
		header map[string]string
		want   int
	}{
		{"", nil, http.StatusOK},
		{"/related", nil, http.StatusOK},
		{"/stream", map[string]string{"Range": "bytes=0-9"}, http.StatusPartialContent},
		{"/thumbnail?v=abc", nil, http.StatusOK},
		{"/seek-thumbnail?positionMs=1000&v=abc", nil, http.StatusOK},
		{"/preview?v=content-key-a", nil, http.StatusOK},
		{"/transcode.mp4", nil, http.StatusOK},
	}
	for _, route := range routes {
		public := env.serve(authRequest{method: http.MethodGet, target: f.videoPath("a", route.suffix), header: route.header})
		assertStatus(t, "公開の動画 "+route.suffix, public, route.want)
		assertAudience(t, "公開の動画 "+route.suffix, public, "guest")

		hidden := env.serve(authRequest{method: http.MethodGet, target: f.videoPath("b", route.suffix), header: route.header})
		missing := env.serve(authRequest{method: http.MethodGet, target: "/api/videos/9999" + route.suffix, header: route.header})
		assertSameResponse(t, "非公開の動画 "+route.suffix, hidden, missing, http.StatusNotFound)

		owner := env.serve(authRequest{method: http.MethodGet, target: f.videoPath("b", route.suffix), header: route.header, cookies: []*http.Cookie{f.owner}})
		assertStatus(t, "所有者の非公開の動画 "+route.suffix, owner, route.want)
	}
}

// ゲストは所有者のデータに依る一覧の条件を使えない（guest-api.md §3）。
func TestGuestRejectsOwnerOnlyListConditions(t *testing.T) {
	f := newGuestFixture(t, true)
	env := f.env

	rejected := []string{
		"/api/videos?watch=watched",
		"/api/videos?watch=unwatched",
		"/api/videos?watch=inProgress",
		"/api/videos?sort=playedAsc",
		"/api/videos?sort=playedDesc",
		"/api/videos?tag=" + strconv.FormatInt(f.tagID, 10),
		f.folderPath(f.rootID, "/videos?watch=watched"),
		f.folderPath(f.rootID, "/videos?sort=playedAsc"),
	}
	for _, target := range rejected {
		rec := env.get(target)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"invalid_request"`) {
			t.Errorf("ゲストの %s: status = %d: %s", target, rec.Code, rec.Body)
		}
		if owner := env.get(target, f.owner); owner.Code != http.StatusOK {
			t.Errorf("所有者の %s: status = %d: %s", target, owner.Code, owner.Body)
		}
	}
	for _, target := range []string{"/api/videos?watch=all&sort=addedDesc", "/api/videos?sort=random&seed=3", f.folderPath(f.rootID, "/videos?watch=all")} {
		if rec := env.get(target); rec.Code != http.StatusOK {
			t.Errorf("ゲストの %s: status = %d: %s", target, rec.Code, rec.Body)
		}
	}
}

// 生成物の成功の応答は、所有者にもゲストにも private, no-cache と ETag を持ち、
// If-None-Match が一致すれば 304 になる。ログアウトした Cookie では、非公開の動画の
// サムネイルは If-None-Match を付けても 404 になる（guest-api.md §5）。
func TestArtifactResponsesRevalidateForOwnerAndGuest(t *testing.T) {
	f := newGuestFixture(t, true)
	env := f.env

	for _, suffix := range []string{"/thumbnail?v=abc", "/seek-thumbnail?positionMs=1000&v=abc", "/preview?v=content-key-a"} {
		for _, viewer := range []struct {
			label   string
			cookies []*http.Cookie
		}{{"所有者", []*http.Cookie{f.owner}}, {"ゲスト", nil}} {
			label := viewer.label + " " + suffix
			rec := env.serve(authRequest{method: http.MethodGet, target: f.videoPath("a", suffix), cookies: viewer.cookies})
			assertStatus(t, label, rec, http.StatusOK)
			if got := rec.Header().Get("Cache-Control"); got != "private, no-cache" {
				t.Errorf("%s: Cache-Control = %q", label, got)
			}
			etag := rec.Header().Get("ETag")
			if etag == "" {
				t.Fatalf("%s: ETag が無い", label)
			}
			again := env.serve(authRequest{
				method: http.MethodGet, target: f.videoPath("a", suffix), cookies: viewer.cookies,
				header: map[string]string{"If-None-Match": etag},
			})
			assertStatus(t, label+" If-None-Match", again, http.StatusNotModified)
			if again.Body.Len() != 0 || again.Header().Get("ETag") != etag || again.Header().Get("Cache-Control") != "private, no-cache" {
				t.Errorf("%s: 304 = %d バイト, %v", label, again.Body.Len(), again.Header())
			}
		}
	}

	// 所有者として非公開の動画のサムネイルを得てから、ログアウトする。
	target := f.videoPath("b", "/thumbnail?v=abc")
	ownerRec := env.get(target, f.owner)
	assertStatus(t, "所有者の非公開のサムネイル", ownerRec, http.StatusOK)
	etag := ownerRec.Header().Get("ETag")
	logout := env.serve(authRequest{method: http.MethodPost, target: "/api/auth/logout", cookies: []*http.Cookie{f.owner}})
	assertStatus(t, "ログアウト", logout, http.StatusNoContent)

	after := env.serve(authRequest{method: http.MethodGet, target: target, cookies: []*http.Cookie{f.owner}, header: map[string]string{"If-None-Match": etag}})
	assertStatus(t, "ログアウトした Cookie の非公開のサムネイル", after, http.StatusNotFound)
	assertAudience(t, "ログアウトした Cookie", after, "guest")
	if got := after.Header().Get("Cache-Control"); got != cacheNoStore {
		t.Errorf("404 の Cache-Control = %q", got)
	}
}

// 「ゲストも」の操作は、Cookie なしでも未認証にならずゲストとして処理される。
// アカウントが未設定なら、公開の動画があっても未認証になる（親 Issue 要件 2）。
func TestGuestOperationsRequireConfiguredAccount(t *testing.T) {
	configured := newGuestFixture(t, true)
	unconfigured := newGuestFixture(t, false)

	for op, class := range openAPISecurity(t) {
		if class != accessGuest {
			continue
		}
		target := strings.ReplaceAll(operationTarget(op), "/1", "/"+strconv.FormatInt(configured.ids["a"], 10))
		if op.path == "/api/videos/{id}/seek-thumbnail" {
			target += "?positionMs=0"
		}
		rec := configured.env.get(target)
		if rec.Code == http.StatusUnauthorized {
			t.Errorf("設定済みの %s: 401: %s", target, rec.Body)
		}
		assertAudience(t, "設定済みの "+target, rec, "guest")

		assertUnauthenticated(t, "未設定の "+target, unconfigured.env.get(target))
		assertAudience(t, "未設定の "+target, unconfigured.env.get(target), "guest")
	}
}
