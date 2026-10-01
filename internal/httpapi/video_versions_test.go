package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"testing"

	"github.com/syudead/vv/internal/httpapi/gen"
)

// バージョンと束ねの画面の API（specs/030-video-versions/contracts/screen-api.md §0〜§4）は、
// 本物の認証・保存層で確かめる（#580）。guestFixture の a（公開、タグと再生位置あり）と
// b（非公開）を束ねる。

func bundleBody(videoIDs []int64, representativeID int64) string {
	body, _ := json.Marshal(map[string]any{"videoIds": videoIDs, "representativeId": representativeID})
	return string(body)
}

func (f *guestFixture) bundle(body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	return f.env.serve(authRequest{method: http.MethodPost, target: "/api/video-bundles", body: body, cookies: cookies})
}

func (f *guestFixture) versionAction(name, action string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	return f.env.serve(authRequest{method: http.MethodPost, target: f.videoPath(name, "/"+action), cookies: cookies})
}

// attachNewTag は名前 name のタグを作って動画 video に付ける。
func (f *guestFixture) attachNewTag(t *testing.T, video, name string) {
	t.Helper()
	ctx := context.Background()
	tag, err := f.env.db.Tags().CreateTag(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.env.db.Tags().AttachTagByID(ctx, []int64{f.ids[video]}, tag.ID); err != nil {
		t.Fatal(err)
	}
}

// versionIDs は VideoVersions の項目の id を並びのまま返す。
func versionIDs(versions gen.VideoVersions) []int64 {
	ids := make([]int64, 0, len(versions.Items))
	for _, item := range versions.Items {
		ids = append(ids, item.Id)
	}
	return ids
}

// libraryVideo はライブラリの応答から id の動画の項目を探す。
func libraryVideo(page gen.LibraryPage, id int64) (gen.Video, bool) {
	for _, item := range page.Items {
		if item.Video != nil && item.Video.Id == id {
			return *item.Video, true
		}
	}
	return gen.Video{}, false
}

func tagNames(tags []gen.VideoTag) []string {
	names := make([]string, 0, len(tags))
	for _, tag := range tags {
		names = append(names, tag.Name)
	}
	return names
}

// 束ねる・代表を替える・外すの一巡（受け入れ条件 8・9）。どの操作も確定後に全メンバーの
// video が /api/events に流れる。
func TestVideoVersionsBundleChangeRepresentativeAndUnbundle(t *testing.T) {
	f := newGuestFixture(t, true)
	env := f.env
	f.attachNewTag(t, "a", "X")
	f.attachNewTag(t, "b", "Y")
	a, b := f.ids["a"], f.ids["b"]

	events := NewEvents()
	env.db.PublishTo(eventsPublisher{events: events})
	sub, unsubscribe := events.subscribe()
	defer unsubscribe()
	sub.take() // つないだ直後の送信を除く。

	rec := f.bundle(bundleBody([]int64{b, a, b}, a), f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("束ねる: status = %d: %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Cache-Control"); got != cacheNoStore {
		t.Errorf("Cache-Control = %q", got)
	}
	bundled := decode[gen.VideoVersions](t, rec)
	if bundled.RepresentativeId != a || !slices.Equal(versionIDs(bundled), []int64{a, b}) {
		t.Fatalf("束ねた応答: representativeId = %d, items = %v, want %d, [%d %d]",
			bundled.RepresentativeId, versionIDs(bundled), a, a, b)
	}
	for _, item := range bundled.Items {
		if item.Width == nil || item.Height == nil || item.VideoCodec == nil || item.Container == nil ||
			item.SizeBytes == 0 || item.Location == nil || item.Folder == nil {
			t.Errorf("項目 %d に解像度・コーデック・大きさ・所在・フォルダが無い: %+v", item.Id, item)
		}
		if item.Versions == nil || item.Versions.Count != 2 || item.Versions.RepresentativeId != a {
			t.Errorf("項目 %d の versions = %+v", item.Id, item.Versions)
		}
	}
	if _, videos := sub.take(); !slices.Equal(videos, []int64{a, b}) {
		t.Errorf("束ねたあとの /api/events の video = %v, want [%d %d]", videos, a, b)
	}

	listed := decode[gen.VideoVersions](t, env.get(f.videoPath("b", "/versions"), f.owner))
	if listed.RepresentativeId != a || !slices.Equal(versionIDs(listed), []int64{a, b}) {
		t.Errorf("b のバージョン: representativeId = %d, items = %v", listed.RepresentativeId, versionIDs(listed))
	}
	detail := decode[gen.Video](t, env.get(f.videoPath("b", ""), f.owner))
	if detail.Versions == nil || detail.Versions.Count != 2 || detail.Versions.RepresentativeId != a {
		t.Errorf("b の詳細の versions = %+v, want count 2, representativeId %d", detail.Versions, a)
	}
	library := decode[gen.LibraryPage](t, env.get("/api/library", f.owner))
	if _, ok := libraryVideo(library, b); ok {
		t.Error("束ねたあとのライブラリに代表以外の b が出る")
	}
	if item, ok := libraryVideo(library, a); !ok || !slices.Contains(tagNames(item.Tags), "X") {
		t.Errorf("束ねたあとのライブラリの a: found = %v, tags = %v", ok, tagNames(item.Tags))
	}
	// 一覧の項目には versions が入らない。
	for _, item := range rawItems(t, env.get("/api/videos", f.owner).Body.Bytes(), "items") {
		if _, ok := item["versions"]; ok {
			t.Errorf("一覧の項目に versions がある: %s", item["id"])
		}
	}

	// 代表を b に替えると、ライブラリの 1 件が b になり、タグは集まりの値（X）のまま（受け入れ条件 8）。
	rec = f.versionAction("b", "make-representative", f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("代表を替える: status = %d: %s", rec.Code, rec.Body)
	}
	changed := decode[gen.VideoVersions](t, rec)
	if changed.RepresentativeId != b || !slices.Equal(versionIDs(changed), []int64{b, a}) {
		t.Errorf("代表を替えた応答: representativeId = %d, items = %v", changed.RepresentativeId, versionIDs(changed))
	}
	if _, videos := sub.take(); !slices.Equal(videos, []int64{a, b}) {
		t.Errorf("代表を替えたあとの /api/events の video = %v, want [%d %d]", videos, a, b)
	}
	// 既に代表でも同じ応答。
	if rec := f.versionAction("b", "make-representative", f.owner); rec.Code != http.StatusOK {
		t.Errorf("既に代表: status = %d: %s", rec.Code, rec.Body)
	}
	library = decode[gen.LibraryPage](t, env.get("/api/library", f.owner))
	if _, ok := libraryVideo(library, a); ok {
		t.Error("代表を替えたあとのライブラリに a が出る")
	}
	item, ok := libraryVideo(library, b)
	if !ok {
		t.Fatal("代表を替えたあとのライブラリに b が無い")
	}
	if names := tagNames(item.Tags); !slices.Contains(names, "X") || slices.Contains(names, "Y") {
		t.Errorf("代表を替えたあとの b のタグ = %v, want X を含み Y を含まない", names)
	}

	// b を外すと、b は束ねる前の値（Y）に戻って一覧に戻る（受け入れ条件 9）。
	sub.take()
	rec = f.versionAction("b", "unbundle", f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("外す: status = %d: %s", rec.Code, rec.Body)
	}
	removed := decode[gen.Video](t, rec)
	if removed.Id != b || removed.Versions != nil || removed.Location == nil {
		t.Errorf("外した応答: id = %d, versions = %+v, location = %v", removed.Id, removed.Versions, removed.Location)
	}
	if names := tagNames(removed.Tags); !slices.Contains(names, "Y") || slices.Contains(names, "X") {
		t.Errorf("外した b のタグ = %v, want Y を含み X を含まない", names)
	}
	if _, videos := sub.take(); !slices.Equal(videos, []int64{a, b}) {
		t.Errorf("外したあとの /api/events の video = %v, want [%d %d]", videos, a, b)
	}
	// a と b が同じフォルダに並ぶとライブラリではフォルダのグループに畳まれるので、動画ごとの
	// 一覧で確かめる。
	list := decode[gen.VideoPage](t, env.get("/api/videos", f.owner))
	index := slices.IndexFunc(list.Items, func(v gen.Video) bool { return v.Id == b })
	if index < 0 || !slices.Contains(tagNames(list.Items[index].Tags), "Y") {
		t.Errorf("外したあとの一覧の b: index = %d, items = %v", index, titlesOf(list.Items))
	}
	if !slices.ContainsFunc(list.Items, func(v gen.Video) bool { return v.Id == a }) {
		t.Errorf("外したあとの一覧に a が無い: %v", titlesOf(list.Items))
	}
	if got := decode[gen.Video](t, env.get(f.videoPath("a", ""), f.owner)); got.Versions != nil {
		t.Errorf("集まりが解けたあとの a の versions = %+v", got.Versions)
	}
}

// 束ねた b の詳細の再生位置は集まりのもので、b の再生位置の記録が一覧の a を進める（受け入れ条件 7）。
func TestVideoVersionsShareProgress(t *testing.T) {
	f := newGuestFixture(t, true)
	env := f.env
	a := f.ids["a"]
	if rec := f.bundle(bundleBody([]int64{a, f.ids["b"]}, a), f.owner); rec.Code != http.StatusOK {
		t.Fatalf("束ねる: status = %d: %s", rec.Code, rec.Body)
	}

	detail := decode[gen.Video](t, env.get(f.videoPath("b", ""), f.owner))
	if detail.Progress == nil || detail.Progress.PositionMs != 30_000 {
		t.Fatalf("b の詳細の progress = %+v, want 集まり（a）の 30000", detail.Progress)
	}
	rec := env.serve(authRequest{
		method: http.MethodPut, target: f.videoPath("b", "/progress"),
		body: `{"positionMs": 45000}`, cookies: []*http.Cookie{f.owner},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("b の再生位置: status = %d: %s", rec.Code, rec.Body)
	}
	library := decode[gen.LibraryPage](t, env.get("/api/library", f.owner))
	item, ok := libraryVideo(library, a)
	if !ok || item.Progress == nil || item.Progress.PositionMs != 45_000 {
		t.Errorf("ライブラリの a の progress = %+v (found = %v), want 45000", item.Progress, ok)
	}
}

// ゲストは公開の集まりの全バージョンを見られ、束ねの操作は 401 になる（受け入れ条件 12）。
func TestVideoVersionsForGuest(t *testing.T) {
	f := newGuestFixture(t, true)
	env := f.env
	a, b := f.ids["a"], f.ids["b"]
	// 代表の a は公開なので、集まりが公開になる。
	if rec := f.bundle(bundleBody([]int64{a, b}, a), f.owner); rec.Code != http.StatusOK {
		t.Fatalf("束ねる: status = %d: %s", rec.Code, rec.Body)
	}

	rec := env.get(f.videoPath("b", "/versions"))
	if rec.Code != http.StatusOK {
		t.Fatalf("ゲストのバージョン: status = %d: %s", rec.Code, rec.Body)
	}
	if got := decode[gen.VideoVersions](t, rec); got.RepresentativeId != a || !slices.Equal(versionIDs(got), []int64{a, b}) {
		t.Errorf("ゲストのバージョン: representativeId = %d, items = %v", got.RepresentativeId, versionIDs(got))
	}
	for _, item := range rawItems(t, rec.Body.Bytes(), "items") {
		assertGuestVideo(t, "ゲストのバージョン", item)
		if _, ok := item["folder"]; !ok {
			t.Errorf("ゲストのバージョンの項目に folder が無い: %s", item["id"])
		}
	}
	detail := decode[gen.Video](t, env.get(f.videoPath("b", "")))
	if detail.Versions == nil || detail.Versions.Count != 2 {
		t.Errorf("ゲストの b の詳細の versions = %+v", detail.Versions)
	}
	// 非公開の動画はゲストには無い動画と同じ。
	hidden := env.get(f.videoPath("c", "/versions"))
	assertErrorBody(t, "ゲストの非公開のバージョン", hidden.Code, hidden.Body.Bytes(),
		wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: reasonVideoNotFound})

	for label, rec := range map[string]*httptest.ResponseRecorder{
		"束ねる":    f.bundle(bundleBody([]int64{a, f.ids["d"]}, a)),
		"代表を替える": f.versionAction("b", "make-representative"),
		"外す":     f.versionAction("b", "unbundle"),
	} {
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("ゲストの%s: status = %d, want 401: %s", label, rec.Code, rec.Body)
		}
	}
	// ゲストの操作で何も変わっていない。
	if got := decode[gen.VideoVersions](t, env.get(f.videoPath("a", "/versions"), f.owner)); got.RepresentativeId != a || len(got.Items) != 2 {
		t.Errorf("ゲストの操作のあと: representativeId = %d, items = %v", got.RepresentativeId, versionIDs(got))
	}
}

// 1 本だけ・代表が含まれない・上限を超える・無い動画・束ねていない動画への操作は契約の誤りになり、
// 何も変えない。
func TestVideoVersionsErrors(t *testing.T) {
	f := newGuestFixture(t, true)
	env := f.env
	a, b := f.ids["a"], f.ids["b"]
	tooMany := make([]int64, maxBundleVideoIDs+1)
	for i := range tooMany {
		tooMany[i] = int64(i + 1)
	}
	missing := int64(999_999)

	cases := []struct {
		label string
		rec   *httptest.ResponseRecorder
		want  wantError
	}{
		{"1 本だけ", f.bundle(bundleBody([]int64{a}, a), f.owner),
			wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonTooFewVideos}},
		{"重複で 1 本", f.bundle(bundleBody([]int64{a, a}, a), f.owner),
			wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonTooFewVideos}},
		{"空", f.bundle(bundleBody([]int64{}, a), f.owner),
			wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonTooFewVideos}},
		{"代表が含まれない", f.bundle(bundleBody([]int64{a, b}, f.ids["c"]), f.owner),
			wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonRepresentativeNotSelected}},
		{"上限を超える", f.bundle(bundleBody(tooMany, 1), f.owner),
			wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonTooManyVideos, limit: maxBundleVideoIDs}},
		{"無い動画を含む", f.bundle(bundleBody([]int64{a, missing}, a), f.owner),
			wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: reasonVideoNotFound}},
		{"束ねていない動画の代表", f.versionAction("c", "make-representative", f.owner),
			wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonNotBundled}},
		{"束ねていない動画を外す", f.versionAction("c", "unbundle", f.owner),
			wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonNotBundled}},
		{"無い動画の代表", env.serve(authRequest{method: http.MethodPost,
			target: "/api/videos/" + strconv.FormatInt(missing, 10) + "/make-representative", cookies: []*http.Cookie{f.owner}}),
			wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: reasonVideoNotFound}},
		{"無い動画のバージョン", env.get("/api/videos/"+strconv.FormatInt(missing, 10)+"/versions", f.owner),
			wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: reasonVideoNotFound}},
	}
	for _, c := range cases {
		assertErrorBody(t, c.label, c.rec.Code, c.rec.Body.Bytes(), c.want)
	}

	// 束ねていない動画のバージョンはその 1 本。
	got := decode[gen.VideoVersions](t, env.get(f.videoPath("a", "/versions"), f.owner))
	if got.RepresentativeId != a || !slices.Equal(versionIDs(got), []int64{a}) {
		t.Errorf("誤りのあとの a のバージョン: representativeId = %d, items = %v", got.RepresentativeId, versionIDs(got))
	}
	if detail := decode[gen.Video](t, env.get(f.videoPath("a", ""), f.owner)); detail.Versions != nil {
		t.Errorf("束ねていない a の詳細に versions がある: %+v", detail.Versions)
	}
}
