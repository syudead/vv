package httpapi

import (
	"context"
	"net/http"
	"testing"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/store"
)

type versionJudgingFixture struct {
	env   *authEnv
	owner *http.Cookie
}

func newVersionJudgingFixture(t *testing.T) *versionJudgingFixture {
	t.Helper()
	f := &versionJudgingFixture{}
	f.env = newAuthEnvWith(t, t.TempDir(), func(db *store.DB) Options {
		judge := app.NewVersionJudge(app.VersionJudgeOptions{
			Store: db.Versions(), Settings: db.Settings(), Logger: discardLogger(),
		})
		return Options{VersionJudging: judge}
	})
	f.owner = f.env.setup()
	return f
}

func (f *versionJudgingFixture) put(body string, cookies ...*http.Cookie) *http.Response {
	rec := f.env.serve(authRequest{method: http.MethodPut, target: "/api/settings/version-judging", body: body, cookies: cookies})
	return rec.Result()
}

func (f *versionJudgingFixture) stored(t *testing.T) bool {
	t.Helper()
	enabled, err := f.env.db.Settings().VersionJudgingEnabled(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return enabled
}

func TestGetVersionJudgingSettingsIsOffBeforeAnySave(t *testing.T) {
	f := newVersionJudgingFixture(t)

	rec := f.env.get("/api/settings/version-judging", f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Cache-Control"); got != cacheNoStore {
		t.Errorf("Cache-Control = %q", got)
	}
	if got := decode[gen.VersionJudgingSettings](t, rec); got.Enabled {
		t.Errorf("enabled = true on a new database")
	}
}

func TestPutVersionJudgingSettingsAnswersSavedValue(t *testing.T) {
	f := newVersionJudgingFixture(t)

	for _, want := range []bool{true, false} {
		body := `{"enabled":false}`
		if want {
			body = `{"enabled":true}`
		}
		rec := f.env.serve(authRequest{method: http.MethodPut, target: "/api/settings/version-judging", body: body, cookies: []*http.Cookie{f.owner}})
		if rec.Code != http.StatusOK {
			t.Fatalf("PUT %s: status = %d: %s", body, rec.Code, rec.Body)
		}
		if got := decode[gen.VersionJudgingSettings](t, rec); got.Enabled != want {
			t.Errorf("PUT %s answered enabled = %v", body, got.Enabled)
		}
		if got := f.stored(t); got != want {
			t.Errorf("PUT %s stored %v", body, got)
		}
		get := f.env.get("/api/settings/version-judging", f.owner)
		if got := decode[gen.VersionJudgingSettings](t, get); got.Enabled != want {
			t.Errorf("GET after PUT %s = %v", body, got.Enabled)
		}
	}
}

func TestPutVersionJudgingSettingsRejectsInvalidBody(t *testing.T) {
	f := newVersionJudgingFixture(t)

	for _, body := range []string{`{}`, `{"enabled":"yes"}`, `{"enabled":null}`, `{"enabled":true,"x":1}`, `nope`} {
		res := f.put(body, f.owner)
		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, want 400", body, res.StatusCode)
		}
	}
	if f.stored(t) {
		t.Errorf("an invalid body changed the stored choice")
	}
}

func TestVersionJudgingSettingsAreOwnerOnly(t *testing.T) {
	f := newVersionJudgingFixture(t)

	assertUnauthenticated(t, "GET", f.env.get("/api/settings/version-judging"))
	rec := f.env.serve(authRequest{method: http.MethodPut, target: "/api/settings/version-judging", body: `{"enabled":true}`})
	assertUnauthenticated(t, "PUT", rec)
	if f.stored(t) {
		t.Errorf("a request with no owner session changed the stored choice")
	}
}
