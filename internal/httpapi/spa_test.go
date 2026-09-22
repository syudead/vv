package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUnknownPathFallsBackToIndexHTML(t *testing.T) {
	router := newTestRouter(t, stubPinger{})

	for _, path := range []string{"/", "/anything", "/videos/42"} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

		if rec.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want %d", path, rec.Code, http.StatusOK)
		}
		if !strings.Contains(rec.Body.String(), "<!doctype html>") {
			t.Errorf("%s: index.html が返っていない: %s", path, rec.Body.String())
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
			t.Errorf("%s: Cache-Control = %q, want %q", path, got, "no-cache")
		}
	}
}

// フォールバックを無条件にすると、綴りを誤った API 呼び出しに HTML が 200 で返り、
// クライアント側では「JSON 解析の失敗」としてしか観測できなくなる。
func TestUnknownAPIPathReturnsJSONNotFound(t *testing.T) {
	router := newTestRouter(t, stubPinger{})

	for _, path := range []string{"/api/nope", "/api/", "/api/health/extra"} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want %d", path, rec.Code, http.StatusNotFound)
		}
		if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
			t.Errorf("%s: Content-Type = %q", path, got)
		}
		if strings.Contains(rec.Body.String(), "<!doctype html>") {
			t.Errorf("%s: HTML が返っている: %s", path, rec.Body.String())
		}

		// api/openapi.yaml の Error は code と message を required にしている。
		var body struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Errorf("%s: JSON として解釈できない: %v (%s)", path, err, rec.Body.String())
			continue
		}
		if body.Code == "" || body.Message == "" {
			t.Errorf("%s: Error スキーマを満たしていない: %+v", path, body)
		}
	}
}

func TestUnknownMutationAPIPathReturnsJSONNotFoundBeforeBodyValidation(t *testing.T) {
	router := newTestRouter(t, stubPinger{})

	for _, method := range []string{http.MethodPost, http.MethodPut} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, "/api/unknown", strings.NewReader(`{"ignored":true}`))
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want %d: %s", method, rec.Code, http.StatusNotFound, rec.Body.String())
		}
		if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
			t.Errorf("%s: Content-Type = %q", method, got)
		}
	}
}

func TestAssetsAreServedWithImmutableCacheControl(t *testing.T) {
	router := newTestRouter(t, stubPinger{})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/app-abc123.js", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	want := "public, max-age=31536000, immutable"
	if got := rec.Header().Get("Cache-Control"); got != want {
		t.Errorf("Cache-Control = %q, want %q", got, want)
	}
	if !strings.Contains(rec.Body.String(), "console.log") {
		t.Errorf("資産の内容が返っていない: %s", rec.Body.String())
	}
}

func TestMissingAssetDoesNotFallBackToIndexHTML(t *testing.T) {
	router := newTestRouter(t, stubPinger{})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/missing-000.js", nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if strings.Contains(rec.Body.String(), "<!doctype html>") {
		t.Errorf("index.html が返っている: %s", rec.Body.String())
	}
}
