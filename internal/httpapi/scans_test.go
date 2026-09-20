package httpapi

import (
	"errors"
	"net/http"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// 取り込みの開始は 202 とスキャンを返す。応答は即座に返り、取り込みは
// 背後で進む（R-108）。
func TestStartScanReturnsAccepted(t *testing.T) {
	scans := &fakeScans{}
	handler := newTestServer(t, Options{Scans: scans})

	rec := do(t, handler, http.MethodPost, "/api/scans")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202: %s", rec.Code, rec.Body)
	}

	scan := decode[gen.Scan](t, rec)
	if scan.State != gen.ScanStateRunning {
		t.Errorf("state = %q, want running", scan.State)
	}
	if scans.started != 1 {
		t.Errorf("開始回数 = %d, want 1", scans.started)
	}
}

// 実行中に呼んでも新しく始めず、実行中のものを返す。409 にしないのは、
// 利用者の意図が「今の状態を進めたい」だからである（R-108）。
func TestStartScanReturnsRunningInsteadOfConflict(t *testing.T) {
	scans := &fakeScans{
		hasScan: true,
		current: domain.Scan{ID: 5, State: domain.ScanRunning, Total: 100, Completed: 40},
	}
	handler := newTestServer(t, Options{Scans: scans})

	rec := do(t, handler, http.MethodPost, "/api/scans")
	if rec.Code == http.StatusConflict {
		t.Fatal("実行中に 409 を返した")
	}
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202: %s", rec.Code, rec.Body)
	}

	scan := decode[gen.Scan](t, rec)
	if scan.Id != 5 {
		t.Errorf("id = %d, want 5（実行中のものを返す）", scan.Id)
	}
	if scan.Total != 100 || scan.Completed != 40 {
		t.Errorf("進捗が引き継がれていない: %+v", scan)
	}
}

// 直近の状態を返す。取り込みの規模と残りが分かる（FR-006）。
func TestGetCurrentScan(t *testing.T) {
	handler := newTestServer(t, Options{Scans: &fakeScans{
		hasScan: true,
		current: domain.Scan{
			ID: 3, State: domain.ScanDone, Total: 12, Completed: 11, Failed: 1,
		},
	}})

	rec := do(t, handler, http.MethodGet, "/api/scans/current")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}

	scan := decode[gen.Scan](t, rec)
	if scan.State != gen.ScanStateDone {
		t.Errorf("state = %q, want done", scan.State)
	}
	if scan.Total != 12 || scan.Completed != 11 || scan.Failed != 1 {
		t.Errorf("進捗 = %+v", scan)
	}
}

// 走査そのものが失敗した理由は応答に出す。利用者が次の一手を取れるように
// するため（対象ディレクトリの権限を直す等）。
func TestGetCurrentScanExposesError(t *testing.T) {
	handler := newTestServer(t, Options{Scans: &fakeScans{
		hasScan: true,
		current: domain.Scan{
			ID: 4, State: domain.ScanFailed, Error: "メディアフォルダを読み取れません",
		},
	}})

	scan := decode[gen.Scan](t, do(t, handler, http.MethodGet, "/api/scans/current"))
	if scan.Error == nil || *scan.Error == "" {
		t.Error("失敗の理由が応答に出ていない")
	}
}

// 一度もスキャンしていなければ 404。
func TestGetCurrentScanWhenNeverScanned(t *testing.T) {
	handler := newTestServer(t, Options{Scans: &fakeScans{}})

	rec := do(t, handler, http.MethodGet, "/api/scans/current")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body)
	}
	if got := decode[gen.Error](t, rec); got.Code != codeNotFound {
		t.Errorf("code = %q, want %s", got.Code, codeNotFound)
	}
}

// スキャンの経路も中間キャッシュに残さない。
func TestScanResponsesAreNotCached(t *testing.T) {
	handler := newTestServer(t, Options{Scans: &fakeScans{
		hasScan: true, current: domain.Scan{ID: 1, State: domain.ScanRunning},
	}})

	for _, tc := range []struct{ method, target string }{
		{http.MethodPost, "/api/scans"},
		{http.MethodGet, "/api/scans/current"},
	} {
		rec := do(t, handler, tc.method, tc.target)
		if got := rec.Header().Get("Cache-Control"); got != cacheNoStore {
			t.Errorf("%s %s: Cache-Control = %q, want %q", tc.method, tc.target, got, cacheNoStore)
		}
	}
}

// 開始に失敗した場合は 500。取り込みが始まっていないことが分かる必要がある。
func TestStartScanReportsFailure(t *testing.T) {
	handler := newTestServer(t, Options{Scans: &fakeScans{
		startErr: errors.New("走査を始められません"),
	}})

	rec := do(t, handler, http.MethodPost, "/api/scans")
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}

// 取り込みの経路が組み込まれていない構成でも 500 で応える。
func TestScansWithoutController(t *testing.T) {
	handler := newTestServer(t, Options{})

	if rec := do(t, handler, http.MethodPost, "/api/scans"); rec.Code != http.StatusInternalServerError {
		t.Errorf("POST: status = %d, want 500", rec.Code)
	}
	if rec := do(t, handler, http.MethodGet, "/api/scans/current"); rec.Code != http.StatusInternalServerError {
		t.Errorf("GET: status = %d, want 500", rec.Code)
	}
}
