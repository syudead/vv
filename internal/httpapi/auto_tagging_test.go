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

type autoTagClassifier struct{}

func (autoTagClassifier) Classify(_ context.Context, req domain.ClassifierRequest) (map[string]float64, error) {
	out := map[string]float64{}
	for _, q := range req.Questions {
		out[q.Key] = 0
	}
	return out, nil
}

type ollamaProbe struct {
	err    error
	checks *int
	models []string
}

func (p ollamaProbe) Check(context.Context) error {
	if p.checks != nil {
		*p.checks++
	}
	return p.err
}

func (p ollamaProbe) Models(context.Context) ([]string, error) {
	return p.models, p.err
}

type autoTaggingFixture struct {
	env   *authEnv
	owner *http.Cookie
}

// The fixture tests queueing with real app/storage and fake classifier/connection probes; classification does not run.
func newAutoTaggingFixture(t *testing.T, probe app.OllamaProbe) *autoTaggingFixture {
	t.Helper()
	f := &autoTaggingFixture{}
	f.env = newAuthEnvWith(t, t.TempDir(), func(db *store.DB) Options {
		tagger := app.NewAutoTagger(app.AutoTaggerOptions{
			Queue: db.AutoTags(), Settings: db.Settings(), Classifier: autoTagClassifier{}, Logger: discardLogger(),
		})
		return Options{AutoTagging: tagger, Ollama: app.NewOllamaConnection(db.Settings(), probe), Videos: db.Library()}
	})
	f.owner = f.env.setup()
	return f
}

func (f *autoTaggingFixture) turnOn(t *testing.T) {
	t.Helper()
	res := f.send(http.MethodPut, "/api/settings/auto-tagging",
		`{"enabled":true,"afterImport":false,"threshold":0.8,"tentativeThreshold":0.5}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
}

func (f *autoTaggingFixture) send(method, target, body string) *http.Response {
	return f.env.serve(authRequest{method: method, target: target, body: body, cookies: []*http.Cookie{f.owner}}).Result()
}

func TestAutoTaggingSettingsDefaultsAndSave(t *testing.T) {
	f := newAutoTaggingFixture(t, ollamaProbe{})

	rec := f.env.get("/api/settings/auto-tagging", f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	got := decode[gen.AutoTaggingSettings](t, rec)
	if got.Enabled || got.AfterImport ||
		got.Threshold != domain.DefaultAutoTagThreshold || got.TentativeThreshold != domain.DefaultAutoTagTentativeThreshold ||
		got.Queue != (gen.AutoTaggingQueue{}) {
		t.Fatalf("defaults = %+v", got)
	}

	res := f.send(http.MethodPut, "/api/settings/auto-tagging",
		`{"enabled":true,"afterImport":true,"threshold":0.7,"tentativeThreshold":0.4}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	stored, err := f.env.db.Settings().AutoTagSettings(context.Background())
	want := domain.AutoTagSettings{Enabled: true, AfterImport: true, Threshold: 0.7, TentativeThreshold: 0.4}
	if err != nil || stored != want {
		t.Fatalf("stored = %+v, %v", stored, err)
	}
	if ollama, _ := f.env.db.Settings().OllamaSettings(context.Background()); ollama != domain.DefaultOllamaSettings() {
		t.Fatalf("機能の設定が Ollama の接続先を変えた: %+v", ollama)
	}
}

func TestAutoTaggingSettingsRejectInvalidBodies(t *testing.T) {
	f := newAutoTaggingFixture(t, ollamaProbe{})
	for _, body := range []string{
		`{}`,
		`{"enabled":true,"afterImport":true,"threshold":0.5}`,
		`{"enabled":true,"threshold":0.5,"tentativeThreshold":0.3}`,
		`{"enabled":true,"afterImport":true,"threshold":0.5,"tentativeThreshold":0.6}`,
		`{"enabled":true,"afterImport":true,"threshold":0,"tentativeThreshold":0.3}`,
		`{"enabled":true,"afterImport":true,"threshold":0.5,"tentativeThreshold":0.3,"endpoint":"http://h"}`,
	} {
		if res := f.send(http.MethodPut, "/api/settings/auto-tagging", body); res.StatusCode != http.StatusBadRequest {
			t.Errorf("body %s: status = %d", body, res.StatusCode)
		}
	}
	if stored, _ := f.env.db.Settings().AutoTagSettings(context.Background()); stored != domain.DefaultAutoTagSettings() {
		t.Errorf("an invalid body changed the settings: %+v", stored)
	}
}

func TestOllamaSettingsDefaultsAndSave(t *testing.T) {
	f := newAutoTaggingFixture(t, ollamaProbe{})

	rec := f.env.get("/api/settings/ollama", f.owner)
	if got := decode[gen.OllamaSettings](t, rec); rec.Code != http.StatusOK ||
		got.Endpoint != domain.DefaultOllamaEndpoint || got.Model != domain.DefaultOllamaModel || got.Saved {
		t.Fatalf("defaults = %d %+v", rec.Code, got)
	}
	// saved is response-only and must be rejected in requests.
	if res := f.send(http.MethodPut, "/api/settings/ollama", `{"endpoint":"http://gpu:11434","model":"clef","saved":true}`); res.StatusCode != http.StatusBadRequest {
		t.Errorf("body with saved: status = %d", res.StatusCode)
	}
	rec = f.env.serve(authRequest{method: http.MethodPut, target: "/api/settings/ollama",
		body: `{"endpoint":"http://gpu:11434/","model":"clef"}`, cookies: []*http.Cookie{f.owner}})
	if got := decode[gen.OllamaSettings](t, rec); rec.Code != http.StatusOK || got.Endpoint != "http://gpu:11434" || got.Model != "clef" || !got.Saved {
		t.Fatalf("saved = %d %+v", rec.Code, got)
	}
	rec = f.env.get("/api/settings/ollama", f.owner)
	if got := decode[gen.OllamaSettings](t, rec); rec.Code != http.StatusOK || got.Endpoint != "http://gpu:11434" || !got.Saved {
		t.Fatalf("after save = %d %+v", rec.Code, got)
	}
	if stored, _ := f.env.db.Settings().AutoTagSettings(context.Background()); stored != domain.DefaultAutoTagSettings() {
		t.Fatalf("接続先の保存が機能の設定を変えた: %+v", stored)
	}
	for _, body := range []string{`{}`, `{"endpoint":"ftp://h","model":"clef"}`, `{"endpoint":"http://h","model":""}`, `{"endpoint":"http://h","model":"clef","enabled":true}`} {
		if res := f.send(http.MethodPut, "/api/settings/ollama", body); res.StatusCode != http.StatusBadRequest {
			t.Errorf("body %s: status = %d", body, res.StatusCode)
		}
	}
}

func TestCheckOllama(t *testing.T) {
	var checks int
	ok := newAutoTaggingFixture(t, ollamaProbe{checks: &checks})
	rec := ok.env.serve(authRequest{method: http.MethodPost, target: "/api/settings/ollama/check",
		body: `{"endpoint":"http://attacker.example","model":"clef"}`, cookies: []*http.Cookie{ok.owner}})
	if got := decode[gen.CheckResult](t, rec); rec.Code != http.StatusOK || !got.Available || got.Message != nil || checks != 1 {
		t.Fatalf("available check = %d %+v, checks = %d", rec.Code, got, checks)
	}

	down := newAutoTaggingFixture(t, ollamaProbe{err: errors.New("cannot reach Ollama at http://127.0.0.1:11434")})
	rec = down.env.serve(authRequest{method: http.MethodPost, target: "/api/settings/ollama/check",
		cookies: []*http.Cookie{down.owner}})
	got := decode[gen.CheckResult](t, rec)
	if rec.Code != http.StatusOK || got.Available || got.Message == nil || !strings.Contains(*got.Message, "cannot reach Ollama") {
		t.Fatalf("unavailable check = %d %+v", rec.Code, got)
	}
}

func TestCheckAutoTaggingClassifier(t *testing.T) {
	f := newAutoTaggingFixture(t, ollamaProbe{})
	rec := f.env.serve(authRequest{method: http.MethodPost, target: "/api/auto-tagging/check", cookies: []*http.Cookie{f.owner}})
	if got := decode[gen.CheckResult](t, rec); rec.Code != http.StatusOK || !got.Available || got.Message != nil {
		t.Fatalf("check = %d %+v", rec.Code, got)
	}
}

func TestAutoTaggingRefusesWhileOff(t *testing.T) {
	f := newAutoTaggingFixture(t, ollamaProbe{})
	for _, req := range []authRequest{
		{method: http.MethodPost, target: "/api/auto-tagging/runs", body: `{"scope":"missing"}`},
	} {
		req.cookies = []*http.Cookie{f.owner}
		rec := f.env.serve(req)
		if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), string(gen.ErrorReasonAutoTaggingOff)) {
			t.Errorf("%s: status = %d: %s", req.target, rec.Code, rec.Body)
		}
	}
}

func TestStartAutoTaggingValidatesScope(t *testing.T) {
	f := newAutoTaggingFixture(t, ollamaProbe{})
	f.turnOn(t)
	rec := f.env.serve(authRequest{method: http.MethodPost, target: "/api/auto-tagging/runs",
		body: `{"scope":"missing"}`, cookies: []*http.Cookie{f.owner}})
	if rec.Code != http.StatusAccepted || decode[gen.AutoTaggingQueued](t, rec).Queued != 0 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	for _, body := range []string{`{}`, `{"scope":"some"}`} {
		if res := f.send(http.MethodPost, "/api/auto-tagging/runs", body); res.StatusCode != http.StatusBadRequest {
			t.Errorf("body %s: status = %d", body, res.StatusCode)
		}
	}
}

func TestAutoTagVideoNotFound(t *testing.T) {
	f := newAutoTaggingFixture(t, ollamaProbe{})
	f.turnOn(t)
	for _, target := range []string{"/api/videos/999/auto-tag", "/api/videos/999/tags/1/confirm", "/api/videos/999/tags/1/reject"} {
		if res := f.send(http.MethodPost, target, ""); res.StatusCode != http.StatusNotFound {
			t.Errorf("%s: status = %d", target, res.StatusCode)
		}
	}
}

func TestAutoTaggingIsOwnerOnly(t *testing.T) {
	f := newAutoTaggingFixture(t, ollamaProbe{})
	assertUnauthenticated(t, "GET", f.env.get("/api/settings/auto-tagging"))
	assertUnauthenticated(t, "GET ollama", f.env.get("/api/settings/ollama"))
	for _, req := range []authRequest{
		{method: http.MethodPut, target: "/api/settings/auto-tagging", body: `{"enabled":true,"afterImport":true,"threshold":0.5,"tentativeThreshold":0.3}`},
		{method: http.MethodPut, target: "/api/settings/ollama", body: `{"endpoint":"http://h","model":"m"}`},
		{method: http.MethodPost, target: "/api/settings/ollama/check"},
		{method: http.MethodPost, target: "/api/auto-tagging/runs", body: `{"scope":"all"}`},
		{method: http.MethodPost, target: "/api/auto-tagging/check"},
		{method: http.MethodPost, target: "/api/videos/1/auto-tag"},
		{method: http.MethodPost, target: "/api/videos/1/tags/1/confirm"},
		{method: http.MethodPost, target: "/api/videos/1/tags/1/reject"},
	} {
		assertUnauthenticated(t, req.method+" "+req.target, f.env.serve(req))
	}
}
