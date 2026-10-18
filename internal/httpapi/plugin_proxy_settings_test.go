package httpapi

import (
	"context"
	"net/http"
	"testing"

	"github.com/syudead/vv/internal/app"
	"github.com/syudead/vv/internal/httpapi/gen"
	"github.com/syudead/vv/internal/store"
)

const pluginProxyPath = "/api/settings/plugin-proxy"

type pluginProxyFixture struct {
	env   *authEnv
	owner *http.Cookie
}

func newPluginProxyFixture(t *testing.T) *pluginProxyFixture {
	t.Helper()
	f := &pluginProxyFixture{}
	f.env = newAuthEnvWith(t, t.TempDir(), func(db *store.DB) Options {
		return Options{PluginProxy: app.NewWorkInfoPluginProxy(db.Settings())}
	})
	f.owner = f.env.setup()
	return f
}

func (f *pluginProxyFixture) put(body string) (int, []byte) {
	rec := f.env.serve(authRequest{method: http.MethodPut, target: pluginProxyPath, body: body, cookies: []*http.Cookie{f.owner}})
	return rec.Code, rec.Body.Bytes()
}

func (f *pluginProxyFixture) stored(t *testing.T) string {
	t.Helper()
	value, err := f.env.db.Settings().WorkInfoPluginProxyURL(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestPluginProxySettingsRoundTrip(t *testing.T) {
	f := newPluginProxyFixture(t)

	rec := f.env.get(pluginProxyPath, f.owner)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET before saving: status = %d: %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Cache-Control"); got != cacheNoStore {
		t.Errorf("Cache-Control = %q", got)
	}
	if got := decode[gen.PluginProxySettings](t, rec); got.Url != "" {
		t.Fatalf("GET before saving = %q, want \"\"", got.Url)
	}

	for _, want := range []string{"socks5://user:pass@proxy.example:1080", ""} {
		status, body := f.put(`{"url":"` + want + `"}`)
		if status != http.StatusOK {
			t.Fatalf("PUT %q: status = %d: %s", want, status, body)
		}
		if got := decode[gen.PluginProxySettings](t, f.env.get(pluginProxyPath, f.owner)); got.Url != want {
			t.Fatalf("GET after saving %q = %q", want, got.Url)
		}
		if got := f.stored(t); got != want {
			t.Fatalf("stored = %q, want %q", got, want)
		}
	}
}

func TestPutPluginProxySettingsRejectsInvalidURL(t *testing.T) {
	f := newPluginProxyFixture(t)
	const kept = "http://proxy.example:3128"
	if status, body := f.put(`{"url":"` + kept + `"}`); status != http.StatusOK {
		t.Fatalf("status = %d: %s", status, body)
	}

	for _, body := range []string{
		`{}`, `{"url":null}`, `{"url":1}`,
		`{"url":"http://proxy.example:3128/path"}`,
		`{"url":"http://proxy.example?x=1"}`,
		`{"url":"ftp://proxy.example"}`,
		`{"url":"proxy.example:3128"}`,
	} {
		status, got := f.put(body)
		assertErrorBody(t, body, status, got, wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest})
	}
	if got := f.stored(t); got != kept {
		t.Fatalf("a refused body changed the stored URL to %q", got)
	}
}

func TestPluginProxySettingsAreOwnerOnly(t *testing.T) {
	f := newPluginProxyFixture(t)

	assertUnauthenticated(t, "ゲストの GET", f.env.get(pluginProxyPath))
	assertUnauthenticated(t, "ゲストの PUT", f.env.serve(authRequest{
		method: http.MethodPut, target: pluginProxyPath, body: `{"url":"http://proxy.example"}`,
	}))
	if got := f.stored(t); got != "" {
		t.Fatalf("a guest saved %q", got)
	}
}
