package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi"
)

// stubPinger は保存層への疎通結果を差し替える。
type stubPinger struct{ err error }

func (s stubPinger) Ping(context.Context) error { return s.err }

// testAssets は SPA の配信に必要な最小の埋め込み相当を返す。
func testAssets() fstest.MapFS {
	return fstest.MapFS{
		"index.html":            {Data: []byte("<!doctype html><title>vv</title>")},
		"assets/app-abc123.js":  {Data: []byte("console.log('vv')")},
		"assets/app-abc123.css": {Data: []byte(".vv{}")},
	}
}

func newTestRouter(t *testing.T, pinger httpapi.Pinger) http.Handler {
	t.Helper()
	return httpapi.NewRouter(httpapi.Options{
		Build: domain.BuildInfo{
			Version: "test",
			Commit:  "9f1c2ab",
			BuiltAt: time.Date(2026, 9, 12, 4, 21, 7, 0, time.UTC),
		},
		Pinger: pinger,
		Assets: testAssets(),
	})
}

func TestHealthReturnsOKWhenStoreIsReachable(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestRouter(t, stubPinger{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want %q", got, "no-store")
	}

	// api/openapi.yaml の Health は status と version を required にしている。
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("JSON として解釈できない: %v (%s)", err, rec.Body.String())
	}
	if body["status"] != "ok" {
		t.Errorf("status = %v, want ok", body["status"])
	}
	version, ok := body["version"].(string)
	if !ok || version == "" {
		t.Errorf("version が空: %v", body["version"])
	}
}

func TestHealthReturnsDegradedWhenStoreIsUnreachable(t *testing.T) {
	rec := httptest.NewRecorder()
	router := newTestRouter(t, stubPinger{err: errors.New("疎通できない")})
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("JSON として解釈できない: %v (%s)", err, rec.Body.String())
	}
	if body["status"] != "degraded" {
		t.Errorf("status = %v, want degraded", body["status"])
	}
	if _, ok := body["version"].(string); !ok {
		t.Error("degraded でも version は必須")
	}
}
