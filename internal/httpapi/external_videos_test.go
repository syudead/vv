package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/extgen"
)

// 外部連携 API の動画の一覧と引き当て（specs/026-external-api/contracts/external-api.md §2）を、
// 本物の保存先と Bearer の境界で確かめる。

type externalVideosFixture struct {
	env    *authEnv
	secret string
	// ids は取り込んだ順（追加時刻の順）の動画の id。
	ids []int64
	// nfdPath は NFD の綴りの所在のパス。
	nfdPath string
}

func newExternalVideosFixture(t *testing.T) externalVideosFixture {
	t.Helper()
	env, cookie := newExternalEnv(t, Options{})
	ctx := context.Background()
	mediaDir := t.TempDir()
	if _, err := env.db.Settings().AddMediaFolder(ctx, mediaDir); err != nil {
		t.Fatal(err)
	}
	f := externalVideosFixture{env: env, secret: env.createAPIToken(cookie, "scraper").Secret,
		nfdPath: filepath.Join(mediaDir, "が.mp4")}
	for i, file := range []struct{ path, title, key string }{
		{filepath.Join(mediaDir, "a.mp4"), "a", "key-a"},
		{f.nfdPath, "が", "key-nfd"},
		{filepath.Join(mediaDir, "c.mp4"), "c", "key-c"},
	} {
		result, err := env.db.ScanIndex().UpsertVideo(ctx, domain.VideoFile{
			Path: file.path, Title: file.title, ContentKey: file.key, SizeBytes: 1,
			MTime: time.Unix(0, 0), AddedAt: time.Unix(int64(1000+i), 0), Container: "mp4",
		})
		if err != nil {
			t.Fatal(err)
		}
		f.ids = append(f.ids, result.ID)
	}
	// 1 本だけ公開にし、残りの非公開の動画も返ることを確かめる。
	if _, err := env.db.Visibility().SetVideosPublic(ctx, []int64{f.ids[0]}, true); err != nil {
		t.Fatal(err)
	}
	if _, _, err := env.db.Tags().AttachTagByName(ctx, []int64{f.ids[1]}, "猫"); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f externalVideosFixture) get(t *testing.T, target string) (int, []byte) {
	t.Helper()
	rec := f.env.serve(authRequest{method: http.MethodGet, target: target, header: bearer(f.secret)})
	if got := rec.Header().Get("Cache-Control"); got != cacheNoStore {
		t.Errorf("%s: Cache-Control = %q", target, got)
	}
	return rec.Code, rec.Body.Bytes()
}

func (f externalVideosFixture) assertError(t *testing.T, target string, status int, code extgen.ErrorCode, reason *extgen.ErrorReason) {
	t.Helper()
	got, body := f.get(t, target)
	if got != status {
		t.Errorf("%s: status = %d, want %d: %s", target, got, status, body)
		return
	}
	var e extgen.Error
	if err := json.Unmarshal(body, &e); err != nil {
		t.Fatalf("%s: %v: %s", target, err, body)
	}
	if e.Code != code || (reason == nil) != (e.Reason == nil) || (reason != nil && *e.Reason != *reason) {
		t.Errorf("%s: 本文 = %s", target, body)
	}
}

func TestExternalListVideosPagesThroughAll(t *testing.T) {
	f := newExternalVideosFixture(t)
	var got []int64
	var last extgen.ExternalVideoPage
	target := "/api/v1/videos?limit=2"
	for range 10 {
		status, body := f.get(t, target)
		if status != http.StatusOK {
			t.Fatalf("%s: status = %d: %s", target, status, body)
		}
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(body, &raw); err != nil {
			t.Fatal(err)
		}
		if _, ok := raw["nextCursor"]; !ok {
			t.Errorf("%s: nextCursor が無い: %s", target, body)
		}
		last = extgen.ExternalVideoPage{}
		if err := json.Unmarshal(body, &last); err != nil {
			t.Fatal(err)
		}
		for _, item := range last.Items {
			got = append(got, item.Id)
		}
		if last.NextCursor == "" {
			break
		}
		target = "/api/v1/videos?limit=2&cursor=" + url.QueryEscape(last.NextCursor)
	}
	if len(got) != 3 || got[0] != f.ids[0] || got[1] != f.ids[1] || got[2] != f.ids[2] {
		t.Fatalf("たどった id = %v, want %v", got, f.ids)
	}

	// 既定の件数で全件。非公開の動画・NFD のパス・タグ・解析前の長さ（null）。
	status, body := f.get(t, "/api/v1/videos")
	if status != http.StatusOK {
		t.Fatalf("status = %d: %s", status, body)
	}
	var page extgen.ExternalVideoPage
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 3 || page.NextCursor != "" {
		t.Fatalf("既定の件数: %s", body)
	}
	video := page.Items[1]
	if video.ContentKey != "key-nfd" || video.Title != "が" || video.DurationMs != nil ||
		!video.AddedAt.Equal(time.Unix(1001, 0)) {
		t.Errorf("動画 = %+v", video)
	}
	if len(video.Locations) != 1 || video.Locations[0].Path != f.nfdPath || video.Locations[0].FileName != "が.mp4" {
		t.Errorf("所在 = %+v", video.Locations)
	}
	if len(video.Tags) != 1 || video.Tags[0].Name != "猫" || !video.Tags[0].Manual || video.Tags[0].FromFolder {
		t.Errorf("タグ = %+v", video.Tags)
	}
	var items []map[string]json.RawMessage
	var rawPage struct {
		Items *[]map[string]json.RawMessage `json:"items"`
	}
	rawPage.Items = &items
	if err := json.Unmarshal(body, &rawPage); err != nil {
		t.Fatal(err)
	}
	if string(items[0]["durationMs"]) != "null" || string(items[0]["tags"]) != "[]" {
		t.Errorf("解析前の長さとタグの無い動画: %s", body)
	}
}

func TestExternalListVideosRejectsBadParameters(t *testing.T) {
	f := newExternalVideosFixture(t)
	cursor := extgen.InvalidCursor
	f.assertError(t, "/api/v1/videos?cursor=not-a-cursor", http.StatusBadRequest, extgen.ErrorCodeInvalidRequest, &cursor)
	for _, limit := range []string{"0", "201", "-1", "abc"} {
		f.assertError(t, "/api/v1/videos?limit="+limit, http.StatusBadRequest, extgen.ErrorCodeInvalidRequest, nil)
	}
	if status, body := f.get(t, "/api/v1/videos?limit=200"); status != http.StatusOK {
		t.Errorf("limit=200: status = %d: %s", status, body)
	}
}

func TestExternalLookupVideo(t *testing.T) {
	f := newExternalVideosFixture(t)
	id := f.ids[1]
	for _, query := range []string{
		"id=" + strconv.FormatInt(id, 10),
		"contentKey=key-nfd",
		"path=" + url.QueryEscape(f.nfdPath),
	} {
		status, body := f.get(t, "/api/v1/videos/lookup?"+query)
		if status != http.StatusOK {
			t.Errorf("%s: status = %d: %s", query, status, body)
			continue
		}
		var video extgen.ExternalVideo
		if err := json.Unmarshal(body, &video); err != nil {
			t.Fatal(err)
		}
		if video.Id != id || len(video.Tags) != 1 {
			t.Errorf("%s: %s", query, body)
		}
	}

	notFound := extgen.VideoNotFound
	nfc := filepath.Join(filepath.Dir(f.nfdPath), "が.mp4")
	for _, query := range []string{"id=99999", "id=0", "contentKey=missing", "path=" + url.QueryEscape(nfc)} {
		f.assertError(t, "/api/v1/videos/lookup?"+query, http.StatusNotFound, extgen.ErrorCodeNotFound, &notFound)
	}
	for _, query := range []string{"", "id=1&contentKey=key-a", "contentKey=key-a&path=%2Fa", "path=", "contentKey="} {
		f.assertError(t, "/api/v1/videos/lookup?"+query, http.StatusBadRequest, extgen.ErrorCodeInvalidRequest, nil)
	}
}

func TestExternalVideosRequireBearer(t *testing.T) {
	f := newExternalVideosFixture(t)
	for _, target := range []string{"/api/v1/videos", "/api/v1/videos/lookup?id=1"} {
		assertBearerUnauthenticated(t, target, f.env.serve(authRequest{method: http.MethodGet, target: target}))
	}
}
