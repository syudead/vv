package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/store"
)

// 作品情報プラグインの経路（specs/049-work-info-plugins/contracts/screen-api.md）は、本物の認証・保存層と
// app.WorkInfoPlugins で確かめる。検査だけが偽で、送ったバイト列をマニフェストの JSON として読む。

// manifestRuntime は送られたモジュールをマニフェストの JSON として読む検査である。brokenImportPrefix で
// 始まるモジュールは、残りをマニフェストとして読めたあと import の検査で断られたものとして、本物の検査と
// 同じく読めたマニフェストと問題を一緒に返す。
type manifestRuntime struct{}

const brokenImportPrefix = "import_unsupported:"

func (manifestRuntime) Inspect(_ context.Context, module []byte) (domain.WorkInfoPluginManifest, domain.WorkInfoPluginProblem, error) {
	rest, broken := bytes.CutPrefix(module, []byte(brokenImportPrefix))
	manifest, problem := domain.ParseWorkInfoPluginManifest(rest)
	if broken && problem == "" {
		problem = domain.WorkInfoPluginImportUnsupported
	}
	return manifest, problem, nil
}

func (manifestRuntime) Run(context.Context, string, []byte, domain.WorkInfoRunRequest, domain.WorkInfoRunNetwork) (domain.WorkInfoRunResult, error) {
	return domain.WorkInfoRunResult{}, errors.New("not used")
}

const pluginSecret = "s3cret-value-never-returned"

const pluginManifest = `{"contract":1,"name":"example.works","version":"1.0.0","operations":["search","fetch"],` +
	`"settings":[{"key":"apiKey","label":"API key","secret":true,"required":true},{"key":"lang","label":"Language"}]}`

type pluginEnv struct {
	*authEnv
	owner *http.Cookie
}

func newPluginEnv(t *testing.T) *pluginEnv {
	t.Helper()
	env := newAuthEnvWith(t, t.TempDir(), func(db *store.DB) Options {
		return Options{WorkInfoPlugins: app.NewWorkInfoPlugins(app.WorkInfoPluginsOptions{
			Store: db.WorkInfoPlugins(), Runtime: manifestRuntime{},
		})}
	})
	return &pluginEnv{authEnv: env, owner: env.setup()}
}

// moduleBody は module 部分を parts の数だけ持つ multipart の本文を作る。
func moduleBody(t *testing.T, module []byte, parts int) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for range parts {
		part, err := writer.CreateFormFile("module", "example.wasm")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(module); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf, writer.FormDataContentType()
}

func (e *pluginEnv) uploadParts(module []byte, parts int, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	e.t.Helper()
	body, contentType := moduleBody(e.t, module, parts)
	r := httptest.NewRequest(http.MethodPost, "/api/work-info-plugins", body)
	r.Header.Set("Content-Type", contentType)
	for _, cookie := range cookies {
		r.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, r)
	return rec
}

func (e *pluginEnv) json(method, target, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	e.t.Helper()
	return e.serve(authRequest{method: method, target: target, body: body, cookies: cookies})
}

func decodePlugin(t *testing.T, label string, rec *httptest.ResponseRecorder, status int) gen.WorkInfoPlugin {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("%s: status = %d, want %d: %s", label, rec.Code, status, rec.Body)
	}
	var plugin gen.WorkInfoPlugin
	if err := json.Unmarshal(rec.Body.Bytes(), &plugin); err != nil {
		t.Fatalf("%s: %v: %s", label, err, rec.Body)
	}
	return plugin
}

func pluginPath(id int64, rest string) string {
	return "/api/work-info-plugins/" + strconv.FormatInt(id, 10) + rest
}

func TestWorkInfoPluginRoutesAddApproveAndReplace(t *testing.T) {
	e := newPluginEnv(t)
	added := decodePlugin(t, "追加", e.uploadParts([]byte(pluginManifest), 1, e.owner), http.StatusCreated)
	if added.State != gen.WorkInfoPluginStateUnapproved || added.Name == nil || *added.Name != "example.works" ||
		added.FileName != "example.wasm" || !added.Network || len(added.Settings) != 2 || len(added.Operations) != 2 {
		t.Fatalf("added = %+v", added)
	}

	approved := decodePlugin(t, "承認", e.json(http.MethodPut, pluginPath(added.Id, "/approval"),
		`{"digest":"`+added.Digest+`"}`, e.owner), http.StatusOK)
	if approved.State != gen.WorkInfoPluginStateApproved {
		t.Fatalf("approved state = %s", approved.State)
	}

	changedManifest := strings.Replace(pluginManifest, `"1.0.0"`, `"1.0.1"`, 1)
	replaced := decodePlugin(t, "差し替え", e.uploadParts([]byte(changedManifest), 1, e.owner), http.StatusOK)
	if replaced.Id != added.Id || replaced.State != gen.WorkInfoPluginStateUnapproved || replaced.Digest == added.Digest {
		t.Fatalf("replaced = %+v", replaced)
	}
	rec := e.json(http.MethodPut, pluginPath(added.Id, "/approval"), `{"digest":"`+added.Digest+`"}`, e.owner)
	assertErrorBody(t, "古いダイジェストの承認", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusConflict, code: gen.ErrorCodePluginChanged})

	revoked := decodePlugin(t, "取り消し", e.json(http.MethodDelete, pluginPath(added.Id, "/approval"), "", e.owner), http.StatusOK)
	if revoked.State != gen.WorkInfoPluginStateUnapproved {
		t.Fatalf("revoked state = %s", revoked.State)
	}
	if rec := e.json(http.MethodDelete, pluginPath(added.Id, ""), "", e.owner); rec.Code != http.StatusNoContent {
		t.Fatalf("削除: status = %d: %s", rec.Code, rec.Body)
	}
	for _, req := range []struct{ method, rest, body string }{
		{http.MethodDelete, "", ""},
		{http.MethodDelete, "/approval", ""},
		{http.MethodPut, "/approval", `{"digest":"x"}`},
		{http.MethodPut, "/settings", `{"values":{}}`},
	} {
		rec := e.json(req.method, pluginPath(added.Id, req.rest), req.body, e.owner)
		assertErrorBody(t, req.method+" "+req.rest+" of a deleted plugin", rec.Code, rec.Body.Bytes(),
			wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound})
	}
}

// 承認済みのプラグインを同じ名前の壊れたモジュールで差し替えると同じ行が invalid になり、承認した
// バイト列へ戻しても、同じバイト列を送り直しても、承認前に戻る（data-model.md の状態の遷移）。
func TestWorkInfoPluginReplacementNeverRestoresTheApproval(t *testing.T) {
	e := newPluginEnv(t)
	added := decodePlugin(t, "追加", e.uploadParts([]byte(pluginManifest), 1, e.owner), http.StatusCreated)
	approve := func() {
		t.Helper()
		decodePlugin(t, "承認", e.json(http.MethodPut, pluginPath(added.Id, "/approval"),
			`{"digest":"`+added.Digest+`"}`, e.owner), http.StatusOK)
	}

	approve()
	broken := decodePlugin(t, "壊れた差し替え",
		e.uploadParts([]byte(brokenImportPrefix+pluginManifest), 1, e.owner), http.StatusOK)
	if broken.Id != added.Id || broken.State != gen.WorkInfoPluginStateInvalid || broken.Problem == nil ||
		*broken.Problem != gen.WorkInfoPluginProblem(domain.WorkInfoPluginImportUnsupported) {
		t.Fatalf("broken replacement = %+v; want the same row invalid", broken)
	}
	back := decodePlugin(t, "承認したバイト列へ戻す", e.uploadParts([]byte(pluginManifest), 1, e.owner), http.StatusOK)
	if back.Id != added.Id || back.Digest != added.Digest || back.State != gen.WorkInfoPluginStateUnapproved {
		t.Fatalf("back = %+v; want the same row unapproved", back)
	}

	approve()
	same := decodePlugin(t, "同じバイト列", e.uploadParts([]byte(pluginManifest), 1, e.owner), http.StatusOK)
	if same.Id != added.Id || same.State != gen.WorkInfoPluginStateUnapproved {
		t.Fatalf("same bytes again = %+v; want the same row unapproved", same)
	}
	if list := e.get("/api/work-info-plugins", e.owner); strings.Count(list.Body.String(), `"id":`) != 1 {
		t.Fatalf("差し替えが別の行を足した: %s", list.Body)
	}
}

func TestWorkInfoPluginInvalidUploadIsListedAndUnusable(t *testing.T) {
	e := newPluginEnv(t)
	broken := decodePlugin(t, "壊れたファイル", e.uploadParts([]byte("not a manifest"), 1, e.owner), http.StatusCreated)
	if broken.State != gen.WorkInfoPluginStateInvalid || broken.Problem == nil ||
		*broken.Problem != gen.WorkInfoPluginProblem(domain.WorkInfoPluginManifestInvalid) || broken.Name != nil ||
		len(broken.Operations) != 0 {
		t.Fatalf("broken = %+v", broken)
	}
	rec := e.json(http.MethodPut, pluginPath(broken.Id, "/approval"), `{"digest":"`+broken.Digest+`"}`, e.owner)
	assertErrorBody(t, "使えないプラグインの承認", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusConflict, code: gen.ErrorCodePluginUnusable})
	rec = e.json(http.MethodPut, pluginPath(broken.Id, "/settings"), `{"values":{}}`, e.owner)
	assertErrorBody(t, "使えないプラグインの設定", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusConflict, code: gen.ErrorCodePluginUnusable})
}

func TestWorkInfoPluginUploadLimits(t *testing.T) {
	e := newPluginEnv(t)
	rec := e.uploadParts(bytes.Repeat([]byte{0}, 17<<20), 1, e.owner)
	assertErrorBody(t, "17 MiB の部分", rec.Code, rec.Body.Bytes(), wantError{
		status: http.StatusRequestEntityTooLarge, code: gen.ErrorCodePluginTooLarge, limit: domain.WorkInfoPluginModuleMaxBytes,
	})
	rec = e.uploadParts(nil, 0, e.owner)
	assertErrorBody(t, "module なし", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest})
	rec = e.uploadParts([]byte(pluginManifest), 2, e.owner)
	assertErrorBody(t, "module 2 つ", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest})
	if list := e.get("/api/work-info-plugins", e.owner); !strings.Contains(list.Body.String(), `"plugins":[]`) {
		t.Fatalf("断った追加が残った: %s", list.Body)
	}
}

func TestWorkInfoPluginSettingsHideSecretValues(t *testing.T) {
	e := newPluginEnv(t)
	added := decodePlugin(t, "追加", e.uploadParts([]byte(pluginManifest), 1, e.owner), http.StatusCreated)
	decodePlugin(t, "承認", e.json(http.MethodPut, pluginPath(added.Id, "/approval"), `{"digest":"`+added.Digest+`"}`, e.owner), http.StatusOK)

	var bodies []string
	rec := e.json(http.MethodPut, pluginPath(added.Id, "/settings"),
		`{"values":{"apiKey":"`+pluginSecret+`","lang":"ja"}}`, e.owner)
	written := decodePlugin(t, "設定", rec, http.StatusOK)
	bodies = append(bodies, rec.Body.String())
	if written.State != gen.WorkInfoPluginStateApproved {
		t.Fatalf("設定のあとの状態 = %s, want approved", written.State)
	}
	secret, lang := written.Settings[0], written.Settings[1]
	if !secret.Set || secret.Value != nil || !lang.Set || lang.Value == nil || *lang.Value != "ja" {
		t.Fatalf("settings = %+v", written.Settings)
	}

	list := e.get("/api/work-info-plugins", e.owner)
	bodies = append(bodies, list.Body.String())
	rec = e.json(http.MethodDelete, pluginPath(added.Id, "/approval"), "", e.owner)
	bodies = append(bodies, rec.Body.String())
	rec = e.json(http.MethodPut, pluginPath(added.Id, "/approval"), `{"digest":"`+added.Digest+`"}`, e.owner)
	bodies = append(bodies, rec.Body.String())
	rec = e.uploadParts([]byte(pluginManifest), 1, e.owner)
	bodies = append(bodies, rec.Body.String())
	for i, body := range bodies {
		if strings.Contains(body, pluginSecret) {
			t.Errorf("応答 %d に秘密の値が出た: %s", i, body)
		}
	}
	if !strings.Contains(list.Body.String(), `"set":true`) {
		t.Errorf("一覧に set が無い: %s", list.Body)
	}

	rec = e.json(http.MethodPut, pluginPath(added.Id, "/settings"), `{"values":{"region":"jp"}}`, e.owner)
	assertErrorBody(t, "宣言に無いキー", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest})
	rec = e.json(http.MethodPut, pluginPath(added.Id, "/settings"),
		`{"values":{"lang":"`+strings.Repeat("あ", domain.WorkInfoPluginSettingValueMaxRunes+1)+`"}}`, e.owner)
	assertErrorBody(t, "2000 文字を超える値", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest})
	rec = e.json(http.MethodPut, pluginPath(added.Id, "/settings"), `{}`, e.owner)
	assertErrorBody(t, "values なし", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest})
}

// どの経路も所有者だけで、セッションの無い要求（ゲスト）はほかの所有者の経路と同じく 401 で断り、
// 何も書かない。
func TestWorkInfoPluginRoutesAreOwnerOnly(t *testing.T) {
	e := newPluginEnv(t)
	added := decodePlugin(t, "追加", e.uploadParts([]byte(pluginManifest), 1, e.owner), http.StatusCreated)

	assertUnauthenticated(t, "POST", e.uploadParts([]byte(pluginManifest), 1))
	for _, req := range []struct{ method, target, body string }{
		{http.MethodGet, "/api/work-info-plugins", ""},
		{http.MethodPut, pluginPath(added.Id, "/approval"), `{"digest":"` + added.Digest + `"}`},
		{http.MethodDelete, pluginPath(added.Id, "/approval"), ""},
		{http.MethodPut, pluginPath(added.Id, "/settings"), `{"values":{"lang":"en"}}`},
		{http.MethodDelete, pluginPath(added.Id, ""), ""},
	} {
		assertUnauthenticated(t, req.method+" "+req.target, e.json(req.method, req.target, req.body))
	}
	list := e.get("/api/work-info-plugins", e.owner)
	if !strings.Contains(list.Body.String(), `"state":"unapproved"`) || strings.Contains(list.Body.String(), `"value":"en"`) {
		t.Fatalf("ゲストの要求が書いた: %s", list.Body)
	}
}
