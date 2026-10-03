package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/extgen"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/store"
)

// 外部連携 API の名前でのタグの一括操作（specs/026-external-api/contracts/external-api.md §4）を、
// 本物の保存先と Bearer の境界で確かめる。

type externalTagsFixture struct {
	env    *authEnv
	owner  *http.Cookie
	secret string
	dir    string
	// ids は a.mp4・Anime/b.mp4 の動画の id。
	ids []int64
}

func newExternalTagsFixture(t *testing.T) externalTagsFixture {
	t.Helper()
	env := newAuthEnvWith(t, t.TempDir(), func(db *store.DB) Options {
		library := db.Library()
		return Options{Videos: library, Tags: db.Tags(), ExternalVideos: library}
	})
	f := externalTagsFixture{env: env, owner: env.setup(), dir: t.TempDir()}
	f.secret = env.createAPIToken(f.owner, "scraper").Secret
	ctx := context.Background()
	if _, err := env.db.Settings().AddMediaFolder(ctx, f.dir); err != nil {
		t.Fatal(err)
	}
	for i, file := range []struct{ path, key string }{
		{filepath.Join(f.dir, "a.mp4"), "key-a"},
		{filepath.Join(f.dir, "Anime", "b.mp4"), "key-b"},
	} {
		result, err := env.db.ScanIndex().UpsertVideo(ctx, domain.VideoFile{
			Path: file.path, Title: file.key, ContentKey: file.key, SizeBytes: 1,
			MTime: time.Unix(0, 0), AddedAt: time.Unix(int64(1000+i), 0), Container: "mp4",
		})
		if err != nil {
			t.Fatal(err)
		}
		f.ids = append(f.ids, result.ID)
	}
	if err := env.db.ScanIndex().RebuildFolderIndex(ctx); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f externalTagsFixture) post(t *testing.T, body string) (int, []byte) {
	t.Helper()
	rec := f.env.serve(authRequest{method: http.MethodPost, target: "/api/v1/video-tags", body: body, header: bearer(f.secret)})
	if got := rec.Header().Get("Cache-Control"); got != cacheNoStore {
		t.Errorf("Cache-Control = %q", got)
	}
	return rec.Code, rec.Body.Bytes()
}

func (f externalTagsFixture) apply(t *testing.T, body string) extgen.VideoTagsResponse {
	t.Helper()
	status, raw := f.post(t, body)
	if status != http.StatusOK {
		t.Fatalf("status = %d: %s", status, raw)
	}
	var out extgen.VideoTagsResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func externalTagLabels(tags []extgen.ExternalVideoTag) []string {
	out := []string{}
	for _, tag := range tags {
		label := tag.Name + ":"
		if tag.Manual {
			label += "m"
		}
		if tag.FromFolder {
			label += "f"
		}
		out = append(out, label)
	}
	return out
}

func videoTagsBody(videos []map[string]any, action string, tags []string) string {
	body, _ := json.Marshal(map[string]any{"videos": videos, "action": action, "tags": tags})
	return string(body)
}

// 受け入れ条件 4・5: パスで指定した動画に、無いタグ名とシノニムで add すると、タグが作られ、
// シノニムは元のタグとして付き、画面の API の動画詳細と tag の絞り込みに出る。同じ要求の
// 繰り返しは 200 で状態が変わらない。
func TestExternalVideoTagsAddByPathShowsInScreenAPI(t *testing.T) {
	f := newExternalTagsFixture(t)
	ctx := context.Background()
	cat, err := f.env.db.Tags().CreateTag(ctx, "猫")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.env.db.Tags().AddSynonym(ctx, cat.ID, "ねこ", nil); err != nil {
		t.Fatal(err)
	}
	body := videoTagsBody([]map[string]any{{"path": filepath.Join(f.dir, "a.mp4")}}, "add", []string{"犬", "ねこ"})

	for round := range 2 {
		out := f.apply(t, body)
		if len(out.Items) != 1 || out.Items[0].Video.Id != f.ids[0] || out.Items[0].Video.ContentKey != "key-a" {
			t.Fatalf("round %d: items = %+v", round, out.Items)
		}
		if got := externalTagLabels(out.Items[0].Tags); strings.Join(got, ",") != "犬:m,猫:m" {
			t.Errorf("round %d: tags = %v", round, got)
		}
	}
	tagsPage, err := f.env.db.Tags().ListTags(ctx, domain.TagListQuery{})
	tags := tagsPage.Items
	if err != nil || len(tags) != 2 {
		t.Fatalf("タグ = %+v, err=%v (犬を 1 度だけ作る)", tags, err)
	}

	detail := decode[gen.Video](t, f.env.get("/api/videos/"+strconv.FormatInt(f.ids[0], 10), f.owner))
	names := []string{}
	for _, tag := range detail.Tags {
		names = append(names, tag.Name)
	}
	if strings.Join(names, ",") != "犬,猫" {
		t.Errorf("画面の詳細のタグ = %v", names)
	}
	page := decode[gen.VideoPage](t, f.env.get("/api/videos?tag="+strconv.FormatInt(cat.ID, 10), f.owner))
	if page.Total != 1 || page.Items[0].Id != f.ids[0] {
		t.Errorf("画面の tag の絞り込み = %+v", page)
	}
}

// specs/033-video-dates/data-model.md §3: video-tags で付けると lookup の updatedAt が進み、
// 同じタグをもう一度付けても（行が変わらないので）進まない。
func TestExternalVideoTagsAdvanceUpdatedAt(t *testing.T) {
	f := newExternalTagsFixture(t)
	lookup := func() extgen.ExternalVideo {
		t.Helper()
		rec := f.env.serve(authRequest{
			method: http.MethodGet, target: "/api/v1/videos/lookup?id=" + strconv.FormatInt(f.ids[0], 10), header: bearer(f.secret),
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("lookup: %d %s", rec.Code, rec.Body)
		}
		var video extgen.ExternalVideo
		if err := json.Unmarshal(rec.Body.Bytes(), &video); err != nil {
			t.Fatal(err)
		}
		return video
	}
	before := lookup()
	if !before.UpdatedAt.Equal(before.AddedAt) {
		t.Fatalf("編集前の updatedAt = %v, addedAt = %v", before.UpdatedAt, before.AddedAt)
	}
	body := videoTagsBody([]map[string]any{{"id": f.ids[0]}}, "add", []string{"犬"})
	f.apply(t, body)
	tagged := lookup()
	if !tagged.UpdatedAt.After(before.UpdatedAt) {
		t.Fatalf("付けた後の updatedAt = %v, 前 = %v", tagged.UpdatedAt, before.UpdatedAt)
	}
	f.apply(t, body)
	if again := lookup(); !again.UpdatedAt.Equal(tagged.UpdatedAt) {
		t.Errorf("同じタグの後の updatedAt = %v, want %v", again.UpdatedAt, tagged.UpdatedAt)
	}
}

// replace の後、手で付けたタグはちょうど指定の集合になり、フォルダ由来のタグは残る。
// remove は当たらない名前を飛ばす。
func TestExternalVideoTagsReplaceAndRemove(t *testing.T) {
	f := newExternalTagsFixture(t)
	if _, err := f.env.db.Tags().CreateTag(context.Background(), "Anime"); err != nil {
		t.Fatal(err)
	}
	videos := []map[string]any{{"contentKey": "key-b"}, {"id": f.ids[0]}}
	f.apply(t, videoTagsBody(videos, "add", []string{"猫", "犬"}))

	out := f.apply(t, videoTagsBody(videos, "replace", []string{"鳥", "Anime"}))
	if got := externalTagLabels(out.Items[0].Tags); strings.Join(got, ",") != "Anime:mf,鳥:m" {
		t.Errorf("b の replace の後 = %v", got)
	}
	if got := externalTagLabels(out.Items[1].Tags); strings.Join(got, ",") != "Anime:m,鳥:m" {
		t.Errorf("a の replace の後 = %v", got)
	}

	out = f.apply(t, videoTagsBody(videos, "remove", []string{"Anime", "無い名前"}))
	if got := externalTagLabels(out.Items[0].Tags); strings.Join(got, ",") != "Anime:f,鳥:m" {
		t.Errorf("b の remove の後 = %v", got)
	}

	out = f.apply(t, videoTagsBody(videos[:1], "replace", []string{}))
	if got := externalTagLabels(out.Items[0].Tags); strings.Join(got, ",") != "Anime:f" {
		t.Errorf("b の空の replace の後 = %v", got)
	}
}

// 引けない動画を 1 つ含む要求は 404・index 付きで、何も反映しない。
func TestExternalVideoTagsMissingVideoChangesNothing(t *testing.T) {
	f := newExternalTagsFixture(t)
	status, raw := f.post(t, videoTagsBody([]map[string]any{
		{"contentKey": "key-a"}, {"path": filepath.Join(f.dir, "missing.mp4")},
	}, "add", []string{"猫"}))
	var e extgen.Error
	if err := json.Unmarshal(raw, &e); err != nil {
		t.Fatal(err)
	}
	if status != http.StatusNotFound || e.Code != extgen.ErrorCodeNotFound || e.Reason == nil ||
		*e.Reason != extgen.VideoNotFound || e.Index == nil || *e.Index != 1 {
		t.Fatalf("status = %d: %s", status, raw)
	}
	tagsPage, err := f.env.db.Tags().ListTags(context.Background(), domain.TagListQuery{})
	tags := tagsPage.Items
	if err != nil || len(tags) != 0 {
		t.Errorf("失敗した要求がタグを作った: %+v, err=%v", tags, err)
	}
}

func TestExternalVideoTagsRejectsBadRequests(t *testing.T) {
	f := newExternalTagsFixture(t)
	one := []map[string]any{{"contentKey": "key-a"}}
	tooManyVideos := make([]map[string]any, maxVideoTagsIDs+1)
	for i := range tooManyVideos {
		tooManyVideos[i] = map[string]any{"id": i + 1}
	}
	tooManyTags := make([]string, domain.ExternalVideoTagsMaxNames+1)
	for i := range tooManyTags {
		tooManyTags[i] = "t" + strconv.Itoa(i)
	}
	idx := func(i int) *int { return &i }
	limit := idx
	reason := func(r extgen.ErrorReason) *extgen.ErrorReason { return &r }

	for _, tc := range []struct {
		name   string
		body   string
		reason *extgen.ErrorReason
		limit  *int
		index  *int
	}{
		{name: "不正な action", body: videoTagsBody(one, "toggle", []string{"猫"})},
		{name: "知らない項目", body: `{"videos":[{"id":1}],"action":"add","tags":["猫"],"extra":1}`},
		{name: "tags が無い", body: `{"videos":[{"id":1}],"action":"replace"}`},
		{name: "指定が 2 つ", body: videoTagsBody([]map[string]any{{"id": f.ids[0]}, {"id": f.ids[0], "contentKey": "key-a"}}, "add", []string{"猫"}), index: idx(1)},
		{name: "指定が無い", body: videoTagsBody([]map[string]any{{}}, "add", []string{"猫"}), index: idx(0)},
		{name: "空のパス", body: videoTagsBody([]map[string]any{{"path": ""}}, "add", []string{"猫"}), index: idx(0)},
		{name: "動画が 0 件", body: videoTagsBody([]map[string]any{}, "add", []string{"猫"}), reason: reason(extgen.TooManyVideos), limit: limit(maxVideoTagsIDs)},
		{name: "動画が多すぎる", body: videoTagsBody(tooManyVideos, "add", []string{"猫"}), reason: reason(extgen.TooManyVideos), limit: limit(maxVideoTagsIDs)},
		{name: "add の名前が 0 件", body: videoTagsBody(one, "add", []string{}), reason: reason(extgen.TooManyTags), limit: limit(domain.ExternalVideoTagsMaxNames)},
		{name: "remove の名前が 0 件", body: videoTagsBody(one, "remove", []string{}), reason: reason(extgen.TooManyTags), limit: limit(domain.ExternalVideoTagsMaxNames)},
		{name: "名前が多すぎる", body: videoTagsBody(one, "replace", tooManyTags), reason: reason(extgen.TooManyTags), limit: limit(domain.ExternalVideoTagsMaxNames)},
		{name: "空の名前", body: videoTagsBody(one, "add", []string{"猫", " "}), reason: reason(extgen.TagNameEmpty), index: idx(1)},
		{name: "制御文字", body: videoTagsBody(one, "add", []string{"a\u0007b"}), reason: reason(extgen.TagNameControlCharacters), index: idx(0)},
		{name: "長すぎる名前", body: videoTagsBody(one, "replace", []string{"a", "b", strings.Repeat("x", domain.TagNameMaxLength+1)}), reason: reason(extgen.TagNameTooLong), limit: limit(domain.TagNameMaxLength), index: idx(2)},
	} {
		status, raw := f.post(t, tc.body)
		var e extgen.Error
		if err := json.Unmarshal(raw, &e); err != nil {
			t.Fatalf("%s: %v: %s", tc.name, err, raw)
		}
		if status != http.StatusBadRequest || e.Code != extgen.ErrorCodeInvalidRequest ||
			!sameExternalPtr(e.Reason, tc.reason) || !sameExternalPtr(e.Limit, tc.limit) || !sameExternalPtr(e.Index, tc.index) {
			t.Errorf("%s: status = %d: %s", tc.name, status, raw)
		}
	}
	if tags, err := f.env.db.Tags().ListTags(context.Background(), domain.TagListQuery{}); err != nil || len(tags.Items) != 0 {
		t.Errorf("誤りの要求がタグを作った: %+v, err=%v", tags, err)
	}
}

// 上限を超えた本文は、途中で切れた JSON としてではなく、上限を示す 400 で断る。
func TestExternalVideoTagsRejectsOversizedBody(t *testing.T) {
	f := newExternalTagsFixture(t)
	path := "/" + strings.Repeat("x", externalBodyLimit/2)
	status, raw := f.post(t, videoTagsBody([]map[string]any{{"path": path}, {"path": path}}, "add", []string{"猫"}))
	var e extgen.Error
	if err := json.Unmarshal(raw, &e); err != nil {
		t.Fatalf("%v: %s", err, raw)
	}
	want := "The body must be at most " + strconv.Itoa(externalBodyLimit) + " bytes."
	if status != http.StatusBadRequest || e.Code != extgen.ErrorCodeInvalidRequest || !strings.HasPrefix(e.Message, want) {
		t.Errorf("status = %d: %s", status, raw)
	}
}

func sameExternalPtr[T comparable](got, want *T) bool {
	if got == nil || want == nil {
		return got == want
	}
	return *got == *want
}

// 本文を取る操作は Content-Type: application/json を要し、Bearer の無い要求は 401。
// 別の Origin でも Bearer なら通る。
func TestExternalVideoTagsBoundary(t *testing.T) {
	f := newExternalTagsFixture(t)
	body := videoTagsBody([]map[string]any{{"contentKey": "key-a"}}, "add", []string{"猫"})

	assertBearerUnauthenticated(t, "トークン無し",
		f.env.serve(authRequest{method: http.MethodPost, target: "/api/v1/video-tags", body: body}))
	assertBearerUnauthenticated(t, "Cookie だけ",
		f.env.serve(authRequest{method: http.MethodPost, target: "/api/v1/video-tags", body: body, cookies: []*http.Cookie{f.owner}}))

	header := bearer(f.secret)
	header["Content-Type"] = "text/plain"
	rec := f.env.serve(authRequest{method: http.MethodPost, target: "/api/v1/video-tags", body: body, header: header})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("text/plain: status = %d: %s", rec.Code, rec.Body)
	}

	header = bearer(f.secret)
	header["Origin"] = "https://scraper.example"
	rec = f.env.serve(authRequest{method: http.MethodPost, target: "/api/v1/video-tags", body: body, header: header})
	if rec.Code != http.StatusOK {
		t.Errorf("別の Origin: status = %d: %s", rec.Code, rec.Body)
	}
}
