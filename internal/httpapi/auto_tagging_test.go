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

// autoTagClassifier は判定モデルの代わりで、err があれば失敗を返す。
type autoTagClassifier struct{ err error }

func (c autoTagClassifier) Classify(context.Context, domain.AutoTagRequest) (map[string]float64, error) {
	return map[string]float64{}, c.err
}

type autoTaggingFixture struct {
	env   *authEnv
	owner *http.Cookie
}

// newAutoTaggingFixture は本物の保存先と app.AutoTagger で経路を組む。判定モデルだけが偽で、
// 判定は動かさない（積むところまでを確かめる）。
func newAutoTaggingFixture(t *testing.T, classifier app.AutoTagClassifier) *autoTaggingFixture {
	t.Helper()
	f := &autoTaggingFixture{}
	f.env = newAuthEnvWith(t, t.TempDir(), func(db *store.DB) Options {
		tagger := app.NewAutoTagger(app.AutoTaggerOptions{
			Queue: db.AutoTags(), Settings: db.Settings(), Classifier: classifier, Logger: discardLogger(),
		})
		return Options{AutoTagging: tagger, Videos: db.Library()}
	})
	f.owner = f.env.setup()
	return f
}

func (f *autoTaggingFixture) send(method, target, body string) *http.Response {
	return f.env.serve(authRequest{method: method, target: target, body: body, cookies: []*http.Cookie{f.owner}}).Result()
}

func TestAutoTaggingSettingsDefaultsAndSave(t *testing.T) {
	f := newAutoTaggingFixture(t, autoTagClassifier{})

	rec := f.env.get("/api/settings/auto-tagging", f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	got := decode[gen.AutoTaggingSettings](t, rec)
	if got.Enabled || got.Endpoint != domain.DefaultAutoTagEndpoint || got.Model != domain.DefaultAutoTagModel ||
		got.Threshold != domain.DefaultAutoTagThreshold || got.Queue != (gen.AutoTaggingQueue{}) {
		t.Fatalf("defaults = %+v", got)
	}

	res := f.send(http.MethodPut, "/api/settings/auto-tagging",
		`{"enabled":true,"endpoint":"http://gpu:11434/","model":"clef","threshold":0.7}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	stored, err := f.env.db.Settings().AutoTagSettings(context.Background())
	want := domain.AutoTagSettings{Enabled: true, Endpoint: "http://gpu:11434", Model: "clef", Threshold: 0.7}
	if err != nil || stored != want {
		t.Fatalf("stored = %+v, %v", stored, err)
	}
}

func TestAutoTaggingSettingsRejectInvalidBodies(t *testing.T) {
	f := newAutoTaggingFixture(t, autoTagClassifier{})
	for _, body := range []string{
		`{}`,
		`{"enabled":true,"endpoint":"http://h","model":"clef"}`,
		`{"enabled":true,"endpoint":"ftp://h","model":"clef","threshold":0.5}`,
		`{"enabled":true,"endpoint":"http://h","model":"clef","threshold":0}`,
		`{"enabled":true,"endpoint":"http://h","model":"clef","threshold":0.5,"x":1}`,
	} {
		if res := f.send(http.MethodPut, "/api/settings/auto-tagging", body); res.StatusCode != http.StatusBadRequest {
			t.Errorf("body %s: status = %d", body, res.StatusCode)
		}
	}
	if stored, _ := f.env.db.Settings().AutoTagSettings(context.Background()); stored != domain.DefaultAutoTagSettings() {
		t.Errorf("an invalid body changed the settings: %+v", stored)
	}
}

// recordingClassifier は問い合わせ先を覚え、err があれば失敗を返す。
type recordingClassifier struct {
	err       error
	endpoints *[]string
}

func (c recordingClassifier) Classify(_ context.Context, req domain.AutoTagRequest) (map[string]float64, error) {
	*c.endpoints = append(*c.endpoints, req.Endpoint)
	return map[string]float64{}, c.err
}

// 確かめは保存した問い合わせ先にだけ送り、要求の本文の URL は使わない。
func TestCheckAutoTaggingUsesSavedEndpoint(t *testing.T) {
	var endpoints []string
	ok := newAutoTaggingFixture(t, recordingClassifier{endpoints: &endpoints})
	rec := ok.env.serve(authRequest{method: http.MethodPost, target: "/api/settings/auto-tagging/check",
		body: `{"endpoint":"http://attacker.example","model":"clef"}`, cookies: []*http.Cookie{ok.owner}})
	if got := decode[gen.AutoTaggingCheck](t, rec); rec.Code != http.StatusOK || !got.Available || got.Message != nil {
		t.Fatalf("available check = %d %+v", rec.Code, got)
	}
	if len(endpoints) != 1 || endpoints[0] != domain.DefaultAutoTagEndpoint {
		t.Fatalf("問い合わせ先 = %v", endpoints)
	}

	down := newAutoTaggingFixture(t, recordingClassifier{
		err: errors.New("cannot reach Ollama at http://127.0.0.1:11434"), endpoints: &endpoints,
	})
	rec = down.env.serve(authRequest{method: http.MethodPost, target: "/api/settings/auto-tagging/check",
		cookies: []*http.Cookie{down.owner}})
	got := decode[gen.AutoTaggingCheck](t, rec)
	if rec.Code != http.StatusOK || got.Available || got.Message == nil || !strings.Contains(*got.Message, "cannot reach Ollama") {
		t.Fatalf("unavailable check = %d %+v", rec.Code, got)
	}
}

func TestStartAutoTaggingValidatesScope(t *testing.T) {
	f := newAutoTaggingFixture(t, autoTagClassifier{})
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
	f := newAutoTaggingFixture(t, autoTagClassifier{})
	if res := f.send(http.MethodPost, "/api/videos/999/auto-tag", ""); res.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d", res.StatusCode)
	}
}

func TestAutoTaggingIsOwnerOnly(t *testing.T) {
	f := newAutoTaggingFixture(t, autoTagClassifier{})
	assertUnauthenticated(t, "GET", f.env.get("/api/settings/auto-tagging"))
	for _, req := range []authRequest{
		{method: http.MethodPut, target: "/api/settings/auto-tagging", body: `{"enabled":true,"endpoint":"http://h","model":"m","threshold":0.5}`},
		{method: http.MethodPost, target: "/api/settings/auto-tagging/check"},
		{method: http.MethodPost, target: "/api/auto-tagging/runs", body: `{"scope":"all"}`},
		{method: http.MethodPost, target: "/api/videos/1/auto-tag"},
	} {
		assertUnauthenticated(t, req.method+" "+req.target, f.env.serve(req))
	}
}
