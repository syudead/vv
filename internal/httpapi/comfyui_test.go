package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/store"
)

type comfyUIFixture struct {
	env   *authEnv
	owner *http.Cookie
}

// newComfyUIFixture は本物の保存先と app.ComfyUIConnection で経路を組む。接続の確かめだけが偽である。
func newComfyUIFixture(t *testing.T, probe app.ComfyUIProbe) *comfyUIFixture {
	t.Helper()
	f := &comfyUIFixture{}
	f.env = newAuthEnvWith(t, t.TempDir(), func(db *store.DB) Options {
		return Options{ComfyUI: app.NewComfyUIConnection(db.Settings(), probe)}
	})
	f.owner = f.env.setup()
	return f
}

func (f *comfyUIFixture) put(t *testing.T, body string) (int, string) {
	t.Helper()
	rec := f.env.serve(authRequest{method: http.MethodPut, target: "/api/settings/comfyui", body: body,
		cookies: []*http.Cookie{f.owner}})
	return rec.Code, rec.Body.String()
}

func (f *comfyUIFixture) stored(t *testing.T) domain.ComfyUISettings {
	t.Helper()
	got, err := f.env.db.Settings().ComfyUISettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// 保存前は provider を省き、保存した値は整えて返す。どの応答にもキーは出ず、apiKeySet だけが出る。
func TestComfyUISettingsSaveNeverReturnsKey(t *testing.T) {
	f := newComfyUIFixture(t, ollamaProbe{})
	rec := f.env.get("/api/settings/comfyui", f.owner)
	got := decode[gen.ComfyUISettings](t, rec)
	if rec.Code != http.StatusOK || got.Provider != nil || got.Url != nil || got.ApiKeySet ||
		rec.Header().Get("Cache-Control") != cacheNoStore {
		t.Fatalf("before save = %d %+v %q", rec.Code, got, rec.Header().Get("Cache-Control"))
	}

	code, body := f.put(t, `{"provider":"cloud","apiKey":"sk-secret-key"}`)
	if code != http.StatusOK || strings.Contains(body, "sk-secret-key") || !strings.Contains(body, `"apiKeySet":true`) {
		t.Fatalf("PUT = %d %s", code, body)
	}
	rec = f.env.get("/api/settings/comfyui", f.owner)
	got = decode[gen.ComfyUISettings](t, rec)
	if strings.Contains(rec.Body.String(), "sk-secret-key") || got.Provider == nil || *got.Provider != gen.Cloud || !got.ApiKeySet {
		t.Fatalf("GET = %s", rec.Body)
	}

	// apiKey を省いた保存はキーを残す。
	code, body = f.put(t, `{"provider":"self_hosted","url":" http://gpu:8188/ "}`)
	if code != http.StatusOK || strings.Contains(body, "sk-secret-key") {
		t.Fatalf("PUT self_hosted = %d %s", code, body)
	}
	saved := decode[gen.ComfyUISettings](t, f.env.serve(authRequest{method: http.MethodGet, target: "/api/settings/comfyui",
		cookies: []*http.Cookie{f.owner}}))
	if saved.Url == nil || *saved.Url != "http://gpu:8188" || !saved.ApiKeySet || *saved.Provider != gen.SelfHosted {
		t.Fatalf("after self_hosted = %+v", saved)
	}
	want := domain.ComfyUISettings{Provider: domain.ComfyUISelfHosted, URL: "http://gpu:8188", APIKey: "sk-secret-key"}
	if got := f.stored(t); got != want {
		t.Fatalf("stored = %+v", got)
	}

	// 空文字列のキーは消す。
	if code, body = f.put(t, `{"provider":"self_hosted","url":"http://gpu:8188","apiKey":""}`); code != http.StatusOK ||
		!strings.Contains(body, `"apiKeySet":false`) {
		t.Fatalf("clear = %d %s", code, body)
	}
	if got := f.stored(t); got.APIKey != "" {
		t.Fatalf("the key was kept: %+v", got)
	}
}

// 契約の 400 の reason を返し、何も保存しない。
func TestComfyUISettingsRejectsInvalidBodies(t *testing.T) {
	f := newComfyUIFixture(t, ollamaProbe{})
	for _, c := range []struct {
		body   string
		reason gen.ErrorReason
	}{
		{`{"provider":"self_hosted"}`, gen.ErrorReasonComfyuiUrlInvalid},
		{`{"provider":"self_hosted","url":"ftp://h"}`, gen.ErrorReasonComfyuiUrlInvalid},
		{`{"provider":"cloud"}`, gen.ErrorReasonComfyuiApiKeyMissing},
		{`{"provider":"cloud","apiKey":""}`, gen.ErrorReasonComfyuiApiKeyMissing},
		{`{}`, ""},
		{`{"provider":"other","url":"http://h"}`, ""},
		{`{"provider":"cloud","apiKey":"k","extra":1}`, ""},
	} {
		code, body := f.put(t, c.body)
		if code != http.StatusBadRequest || !strings.Contains(body, string(gen.ErrorCodeInvalidRequest)) ||
			(c.reason != "" && !strings.Contains(body, `"reason":"`+string(c.reason)+`"`)) {
			t.Errorf("body %s: %d %s", c.body, code, body)
		}
	}
	if got := f.stored(t); got.Saved() {
		t.Errorf("an invalid body saved the settings: %+v", got)
	}
}

// 確かめは要求の本文を使わず保存した接続先だけを確かめ、使えないことは 200 で返す。
func TestCheckComfyUI(t *testing.T) {
	var checks int
	ok := newComfyUIFixture(t, ollamaProbe{checks: &checks})
	rec := ok.env.serve(authRequest{method: http.MethodPost, target: "/api/settings/comfyui/check",
		body: `{"url":"http://attacker.example"}`, cookies: []*http.Cookie{ok.owner}})
	if got := decode[gen.CheckResult](t, rec); rec.Code != http.StatusOK || !got.Available || got.Message != nil || checks != 1 {
		t.Fatalf("available check = %d %+v, checks = %d", rec.Code, got, checks)
	}

	down := newComfyUIFixture(t, ollamaProbe{err: errors.New("no ComfyUI connection is saved")})
	rec = down.env.serve(authRequest{method: http.MethodPost, target: "/api/settings/comfyui/check",
		cookies: []*http.Cookie{down.owner}})
	got := decode[gen.CheckResult](t, rec)
	if rec.Code != http.StatusOK || got.Available || got.Message == nil || !strings.Contains(*got.Message, "no ComfyUI connection") {
		t.Fatalf("unavailable check = %d %+v", rec.Code, got)
	}
}

// 足りないノードとモデルファイルは missing に挙げ、モデルにだけフォルダと入手先を付ける。
func TestCheckComfyUIListsMissingItems(t *testing.T) {
	missing := &domain.ComfyUIMissingError{Items: []domain.ComfyUIMissingItem{
		{Kind: domain.ComfyUIMissingNode, Name: "Flux2Scheduler"},
		{Kind: domain.ComfyUIMissingModel, Name: "flux2-vae.safetensors", Folder: "vae", Source: "https://example.test/vae"},
	}}
	f := newComfyUIFixture(t, ollamaProbe{err: missing})
	rec := f.env.serve(authRequest{method: http.MethodPost, target: "/api/settings/comfyui/check",
		cookies: []*http.Cookie{f.owner}})
	got := decode[gen.CheckResult](t, rec)
	if rec.Code != http.StatusOK || got.Available || got.Message == nil || *got.Message != missing.Error() ||
		got.Missing == nil || len(*got.Missing) != 2 {
		t.Fatalf("check = %d %s", rec.Code, rec.Body)
	}
	node, model := (*got.Missing)[0], (*got.Missing)[1]
	if node.Kind != gen.Node || node.Name != "Flux2Scheduler" || node.Folder != nil || node.Source != nil {
		t.Errorf("node = %+v", node)
	}
	if model.Kind != gen.Model || model.Name != "flux2-vae.safetensors" || model.Folder == nil || *model.Folder != "vae" ||
		model.Source == nil || *model.Source != "https://example.test/vae" {
		t.Errorf("model = %+v", model)
	}
}

// readiness の connection は保存の前は false、保存の後は true である。確かめは呼ばない。
func TestThumbnailEditReadinessFollowsSavedConnection(t *testing.T) {
	var checks int
	f := newComfyUIFixture(t, ollamaProbe{checks: &checks, err: errors.New("unreachable")})
	readiness := func() gen.ThumbnailEditReadiness {
		t.Helper()
		rec := f.env.get("/api/thumbnail-edits/readiness", f.owner)
		if rec.Code != http.StatusOK {
			t.Fatalf("readiness = %d %s", rec.Code, rec.Body)
		}
		return decode[gen.ThumbnailEditReadiness](t, rec)
	}
	if readiness().Connection {
		t.Fatal("connection before any save")
	}
	if code, body := f.put(t, `{"provider":"self_hosted","url":"http://gpu:8188"}`); code != http.StatusOK {
		t.Fatalf("PUT = %d %s", code, body)
	}
	if !readiness().Connection {
		t.Fatal("no connection after a save")
	}
	if checks != 0 {
		t.Errorf("readiness ran the connection test %d time(s)", checks)
	}
}

func TestComfyUISettingsAreOwnerOnly(t *testing.T) {
	f := newComfyUIFixture(t, ollamaProbe{})
	assertUnauthenticated(t, "GET comfyui", f.env.get("/api/settings/comfyui"))
	assertUnauthenticated(t, "GET readiness", f.env.get("/api/thumbnail-edits/readiness"))
	for _, req := range []authRequest{
		{method: http.MethodPut, target: "/api/settings/comfyui", body: `{"provider":"self_hosted","url":"http://h"}`},
		{method: http.MethodPost, target: "/api/settings/comfyui/check"},
	} {
		assertUnauthenticated(t, req.method+" "+req.target, f.env.serve(req))
	}
	if got := f.stored(t); got.Saved() {
		t.Errorf("a guest saved the settings: %+v", got)
	}
}
