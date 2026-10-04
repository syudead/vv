package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/extgen"
	"github.com/syudead/vv/internal/store"
)

// 外部連携 API v1 の Bearer の境界とタグ・スキャンの操作
// （specs/026-external-api/contracts/external-api.md §1・§3・§5、research.md R-3・R-4・R-9）を、
// 本物の Auth と保存先で確かめる。

func bearer(secret string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + secret}
}

// assertBearerUnauthenticated は Bearer の扱いの未認証の応答であることを確かめる。
func assertBearerUnauthenticated(t *testing.T, label string, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("%s: status = %d, want 401: %s", label, rec.Code, rec.Body)
		return
	}
	if got := rec.Header().Get("WWW-Authenticate"); got != "Bearer" {
		t.Errorf("%s: WWW-Authenticate = %q, want Bearer", label, got)
	}
	var body extgen.Error
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Code != extgen.ErrorCodeUnauthenticated {
		t.Errorf("%s: 本文 = %s", label, rec.Body)
	}
	if got := rec.Header().Get("Cache-Control"); got != cacheNoStore {
		t.Errorf("%s: Cache-Control = %q", label, got)
	}
}

// newExternalEnv は本物のタグと動画の保存先でつないだ経路を組み立て、初回設定の Cookie を返す。
func newExternalEnv(t *testing.T, opts Options) (*authEnv, *http.Cookie) {
	t.Helper()
	env := newAuthEnvWith(t, t.TempDir(), func(db *store.DB) Options {
		if opts.Tags == nil {
			opts.Tags = db.Tags()
		}
		if opts.ExternalVideos == nil {
			opts.ExternalVideos = db.Library()
		}
		return opts
	})
	return env, env.setup()
}

func TestExternalListTagsWithValidToken(t *testing.T) {
	env, cookie := newExternalEnv(t, Options{})
	ctx := context.Background()
	tag, err := env.db.Tags().CreateTag(ctx, "猫")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.db.Tags().AddSynonym(ctx, tag.ID, "ねこ", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := env.db.Tags().CreateTag(ctx, "犬"); err != nil {
		t.Fatal(err)
	}
	created := env.createAPIToken(cookie, "scraper")

	rec := env.serve(authRequest{method: http.MethodGet, target: "/api/v1/tags", header: bearer(created.Secret)})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get(audienceHeader); got != "owner" {
		t.Errorf("X-VV-Audience = %q, want owner", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != cacheNoStore {
		t.Errorf("Cache-Control = %q", got)
	}
	var list extgen.TagList
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	// 並びは画面の ListTags と同じ。
	screen := env.get("/api/tags", cookie)
	var screenList struct{ Items []struct{ Name string } }
	if err := json.Unmarshal(screen.Body.Bytes(), &screenList); err != nil {
		t.Fatal(err)
	}
	var names, screenNames []string
	for _, item := range list.Items {
		names = append(names, item.Name)
		if item.Name == "猫" && !slices.Equal(item.Synonyms, []string{"ねこ"}) {
			t.Errorf("シノニム = %v", item.Synonyms)
		}
	}
	for _, item := range screenList.Items {
		screenNames = append(screenNames, item.Name)
	}
	if len(names) != 2 || !slices.Equal(names, screenNames) {
		t.Errorf("タグ = %v, 画面 = %v", names, screenNames)
	}
}

// 無い・形式違い・無効・失効済み・Cookie だけは、原因を区別せずに 401 と
// WWW-Authenticate: Bearer にする（contracts/external-api.md §1）。
func TestExternalRejectsMissingInvalidAndRevokedTokens(t *testing.T) {
	env, cookie := newExternalEnv(t, Options{})
	revoked := env.createAPIToken(cookie, "revoked")
	rec := env.serve(authRequest{
		method: http.MethodDelete, target: "/api/api-tokens/" + strconv.FormatInt(revoked.Token.Id, 10),
		cookies: []*http.Cookie{cookie},
	})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("失効: status = %d", rec.Code)
	}
	valid := env.createAPIToken(cookie, "valid")
	unknown := "vvt_" + "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

	cases := []struct {
		name string
		req  authRequest
	}{
		{"トークン無し", authRequest{}},
		{"Cookie だけ", authRequest{cookies: []*http.Cookie{cookie}}},
		{"無効", authRequest{header: bearer(unknown)}},
		{"失効済み", authRequest{header: bearer(revoked.Secret)}},
		{"接頭辞が無い", authRequest{header: bearer(valid.Secret[len("vvt_"):])}},
		{"方式が違う", authRequest{header: map[string]string{"Authorization": "Basic " + valid.Secret}}},
		{"値が空", authRequest{header: map[string]string{"Authorization": "Bearer "}}},
		{"値が空白を含む", authRequest{header: map[string]string{"Authorization": "Bearer " + valid.Secret + " x"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.req.method, tc.req.target = http.MethodGet, "/api/v1/tags"
			rec := env.serve(tc.req)
			assertBearerUnauthenticated(t, tc.name, rec)
			if got := rec.Header().Get(audienceHeader); got != "guest" {
				t.Errorf("X-VV-Audience = %q, want guest", got)
			}
		})
	}

	// 方式の名前は大文字小文字を区別しない。
	rec = env.serve(authRequest{
		method: http.MethodGet, target: "/api/v1/tags",
		header: map[string]string{"Authorization": "bearer " + valid.Secret},
	})
	if rec.Code != http.StatusOK {
		t.Errorf("小文字の bearer: status = %d: %s", rec.Code, rec.Body)
	}
}

// 画面の API は Bearer を読まない。Bearer だけの「ゲストも」の要求はゲストとして処理し、
// 非公開の動画を出さない（受け入れ条件 7）。
func TestScreenAPITreatsBearerOnlyAsGuest(t *testing.T) {
	f := newGuestFixture(t, true)
	created := f.env.createAPIToken(f.owner, "scraper")

	rec := f.env.serve(authRequest{method: http.MethodGet, target: "/api/videos", header: bearer(created.Secret)})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get(audienceHeader); got != "guest" {
		t.Errorf("X-VV-Audience = %q, want guest", got)
	}
	var page struct{ Items []struct{ ID int64 } }
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	for _, item := range page.Items {
		if item.ID != f.ids["a"] && item.ID != f.ids["d"] {
			t.Errorf("非公開の動画 %d を返した", item.ID)
		}
	}
	assertUnauthenticated(t, "Bearer だけの GET /api/api-tokens",
		f.env.serve(authRequest{method: http.MethodGet, target: "/api/api-tokens", header: bearer(created.Secret)}))
}

func TestExternalRecordsLastUsed(t *testing.T) {
	env, cookie := newExternalEnv(t, Options{})
	created := env.createAPIToken(cookie, "scraper")

	if rec := env.serve(authRequest{method: http.MethodGet, target: "/api/v1/tags", header: bearer(created.Secret)}); rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	list, _ := env.listAPITokens(cookie)
	if len(list.Items) != 1 || list.Items[0].LastUsedAt == nil {
		t.Fatalf("最終使用日時が入っていない: %+v", list)
	}
	first := *list.Items[0].LastUsedAt

	// 60 秒に満たないうちは書き直さず、過ぎたら書き直す（research.md R-9）。
	env.advance(30 * time.Second)
	env.serve(authRequest{method: http.MethodGet, target: "/api/v1/tags", header: bearer(created.Secret)})
	list, _ = env.listAPITokens(cookie)
	if !list.Items[0].LastUsedAt.Equal(first) {
		t.Errorf("30 秒後に書き直した: %v -> %v", first, list.Items[0].LastUsedAt)
	}
	env.advance(31 * time.Second)
	env.serve(authRequest{method: http.MethodGet, target: "/api/v1/tags", header: bearer(created.Secret)})
	list, _ = env.listAPITokens(cookie)
	if !list.Items[0].LastUsedAt.After(first) {
		t.Errorf("61 秒後に書き直していない: %v", list.Items[0].LastUsedAt)
	}
}

// /api/v1 の下の未定義の経路も Bearer の扱いで、認証の後に JSON の 404 を返す。
func TestExternalUnknownRouteIsBearerThenNotFound(t *testing.T) {
	env, cookie := newExternalEnv(t, Options{})
	created := env.createAPIToken(cookie, "scraper")

	assertBearerUnauthenticated(t, "トークン無し", env.get("/api/v1/nothing"))
	rec := env.serve(authRequest{method: http.MethodGet, target: "/api/v1/nothing", header: bearer(created.Secret)})
	if rec.Code != http.StatusNotFound || rec.Header().Get("Content-Type") != contentTypeJSON {
		t.Errorf("status = %d, Content-Type = %q: %s", rec.Code, rec.Header().Get("Content-Type"), rec.Body)
	}
}

// Bearer の要求には同一オリジンの検査をかけない（research.md R-4）。新しく始めたら 201、
// 実行中のものを返したら 200 で、画面の POST /api/scans は今どおり 202 である。
func TestExternalStartScanAcceptsForeignOrigin(t *testing.T) {
	scans := &fakeScans{}
	env, cookie := newExternalEnv(t, Options{Scans: scans})
	created := env.createAPIToken(cookie, "scraper")
	header := bearer(created.Secret)
	header["Origin"] = "https://elsewhere.example"

	rec := env.serve(authRequest{method: http.MethodPost, target: "/api/v1/scans", header: header})
	if rec.Code != http.StatusCreated {
		t.Fatalf("最初の開始: status = %d: %s", rec.Code, rec.Body)
	}
	var scan extgen.ExternalScan
	if err := json.Unmarshal(rec.Body.Bytes(), &scan); err != nil {
		t.Fatal(err)
	}
	rec = env.serve(authRequest{method: http.MethodPost, target: "/api/v1/scans", header: header})
	if rec.Code != http.StatusOK {
		t.Fatalf("実行中の開始: status = %d: %s", rec.Code, rec.Body)
	}
	var again extgen.ExternalScan
	if err := json.Unmarshal(rec.Body.Bytes(), &again); err != nil {
		t.Fatal(err)
	}
	if again.Id != scan.Id {
		t.Errorf("実行中の開始の id = %d, want %d", again.Id, scan.Id)
	}

	// 画面の API は今どおり 202 で、別のオリジンは拒む。
	rec = env.serve(authRequest{method: http.MethodPost, target: "/api/scans", body: "{}", cookies: []*http.Cookie{cookie}})
	if rec.Code != http.StatusAccepted {
		t.Errorf("画面の開始: status = %d: %s", rec.Code, rec.Body)
	}
	rec = env.serve(authRequest{
		method: http.MethodPost, target: "/api/scans", body: "{}", cookies: []*http.Cookie{cookie},
		header: map[string]string{"Origin": "https://elsewhere.example"},
	})
	if rec.Code != http.StatusForbidden {
		t.Errorf("画面の別オリジン: status = %d", rec.Code)
	}
}

func TestExternalStartScanWithoutMediaFolders(t *testing.T) {
	env, cookie := newExternalEnv(t, Options{Scans: &fakeScans{startErr: domain.ErrNoMediaFolders}})
	created := env.createAPIToken(cookie, "scraper")

	rec := env.serve(authRequest{method: http.MethodPost, target: "/api/v1/scans", header: bearer(created.Secret)})
	var body extgen.Error
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusConflict || body.Code != extgen.ErrorCodeMediaFoldersNotConfigured {
		t.Errorf("status = %d: %s", rec.Code, rec.Body)
	}
}

// 始めた直後（finding）は本数が両方 null、対象 0 本で終わった走査は両方 0 になる
// （contracts/external-api.md §5）。
func TestExternalCurrentScanCounts(t *testing.T) {
	started := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	finished := started.Add(time.Minute)
	cases := []struct {
		name          string
		scan          domain.Scan
		wantVideos    string
		wantFinished  string
		wantErrorCode string
	}{
		{
			name:       "finding",
			scan:       domain.Scan{ID: 1, State: domain.ScanRunning, StartedAt: started, Import: domain.ImportProgress{Status: domain.ImportFinding}},
			wantVideos: `null null`, wantFinished: `null`, wantErrorCode: `null`,
		},
		{
			name: "対象 0 本で done",
			scan: domain.Scan{
				ID: 2, State: domain.ScanDone, StartedAt: started, FinishedAt: finished,
				Import: domain.ImportProgress{Status: domain.ImportDone, Counted: true},
			},
			wantVideos: `0 0`, wantFinished: `"2026-09-01T10:01:00Z"`, wantErrorCode: `null`,
		},
		{
			name: "failed",
			scan: domain.Scan{
				ID: 3, State: domain.ScanFailed, StartedAt: started, FinishedAt: finished,
				ErrorCode: domain.ScanErrorCode("media_folder_unavailable"),
				Import:    domain.ImportProgress{Status: domain.ImportFailed, Counted: true, Total: 3, Settled: 1},
			},
			wantVideos: `3 1`, wantFinished: `"2026-09-01T10:01:00Z"`, wantErrorCode: `"media_folder_unavailable"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env, cookie := newExternalEnv(t, Options{Scans: &fakeScans{hasScan: true, current: tc.scan}})
			created := env.createAPIToken(cookie, "scraper")

			rec := env.serve(authRequest{method: http.MethodGet, target: "/api/v1/scans/current", header: bearer(created.Secret)})
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d: %s", rec.Code, rec.Body)
			}
			var raw map[string]json.RawMessage
			if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
				t.Fatal(err)
			}
			if got := string(raw["videos"]) + " " + string(raw["settledVideos"]); got != tc.wantVideos {
				t.Errorf("videos settledVideos = %s, want %s", got, tc.wantVideos)
			}
			if got := string(raw["finishedAt"]); got != tc.wantFinished {
				t.Errorf("finishedAt = %s, want %s", got, tc.wantFinished)
			}
			if got := string(raw["errorCode"]); got != tc.wantErrorCode {
				t.Errorf("errorCode = %s, want %s", got, tc.wantErrorCode)
			}
			if got := string(raw["startedAt"]); got != `"2026-09-01T10:00:00Z"` {
				t.Errorf("startedAt = %s", got)
			}
			if got := string(raw["status"]); got != `"`+string(tc.scan.Import.Status)+`"` {
				t.Errorf("status = %s", got)
			}
		})
	}
}

func TestExternalCurrentScanWithoutScan(t *testing.T) {
	env, cookie := newExternalEnv(t, Options{Scans: &fakeScans{}})
	created := env.createAPIToken(cookie, "scraper")

	rec := env.serve(authRequest{method: http.MethodGet, target: "/api/v1/scans/current", header: bearer(created.Secret)})
	var body extgen.Error
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusNotFound || body.Code != extgen.ErrorCodeNotFound ||
		body.Reason == nil || *body.Reason != extgen.NoScan {
		t.Errorf("status = %d: %s", rec.Code, rec.Body)
	}
}

// blockingTags は ListTags で、要求の context が取り消されるまで待つ。実行中の応答の
// 打ち切りを確かめるのに使う。
type blockingTags struct {
	Tags
	entered chan struct{}
	ended   chan error
}

func (b blockingTags) ListTags(ctx context.Context, _ domain.TagListQuery) (domain.TagPage, error) {
	close(b.entered)
	select {
	case <-ctx.Done():
		b.ended <- ctx.Err()
		return domain.TagPage{}, ctx.Err()
	case <-time.After(10 * time.Second):
		b.ended <- nil
		return domain.TagPage{}, nil
	}
}

// 画面での失効は、そのトークンの実行中の応答をすぐに打ち切る（research.md R-9）。
func TestExternalRevokeAbortsInFlightRequest(t *testing.T) {
	tags := blockingTags{entered: make(chan struct{}), ended: make(chan error, 1)}
	env, cookie := newExternalEnv(t, Options{Tags: tags})
	created := env.createAPIToken(cookie, "scraper")
	other := env.createAPIToken(cookie, "other")

	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- env.serve(authRequest{method: http.MethodGet, target: "/api/v1/tags", header: bearer(created.Secret)})
	}()
	<-tags.entered

	// 別のトークンの失効では打ち切らない。
	env.serve(authRequest{
		method: http.MethodDelete, target: "/api/api-tokens/" + strconv.FormatInt(other.Token.Id, 10),
		cookies: []*http.Cookie{cookie},
	})
	select {
	case err := <-tags.ended:
		t.Fatalf("別のトークンの失効で打ち切った: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	rec := env.serve(authRequest{
		method: http.MethodDelete, target: "/api/api-tokens/" + strconv.FormatInt(created.Token.Id, 10),
		cookies: []*http.Cookie{cookie},
	})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("失効: status = %d", rec.Code)
	}
	select {
	case err := <-tags.ended:
		if err == nil {
			t.Fatal("失効で打ち切られなかった")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("失効で打ち切られなかった")
	}
	<-done
}

// ホストのコマンドによる失効（資格情報の変更でトークンの行が消える）は、長く続く要求を
// 確かめ直しで打ち切る（research.md R-9）。
func TestExternalCredentialChangeAbortsInFlightRequestOnRecheck(t *testing.T) {
	tags := blockingTags{entered: make(chan struct{}), ended: make(chan error, 1)}
	env, cookie := newExternalEnv(t, Options{Tags: tags, SessionRecheck: 20 * time.Millisecond})
	created := env.createAPIToken(cookie, "scraper")

	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- env.serve(authRequest{method: http.MethodGet, target: "/api/v1/tags", header: bearer(created.Secret)})
	}()
	<-tags.entered
	select {
	case err := <-tags.ended:
		t.Fatalf("有効なうちに打ち切った: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err := env.db.Auth().ChangePassword(context.Background(), "new-hash", time.Now()); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-tags.ended:
		if err == nil {
			t.Fatal("確かめ直しで打ち切られなかった")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("確かめ直しで打ち切られなかった")
	}
	<-done
}
