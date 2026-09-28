package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// 取り込みの開始は 202 とスキャンを返す。応答は即座に返り、取り込みは
// 背後で進む。
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
// 利用者の意図が「今の状態を進めたい」だからである。
func TestStartScanReturnsRunningInsteadOfConflict(t *testing.T) {
	scans := &fakeScans{
		hasScan: true,
		current: domain.Scan{
			ID: 5, State: domain.ScanRunning, Total: 100, Completed: 40,
			Import: domain.ImportProgress{Status: domain.ImportRunning, Counted: true, Total: 100, Settled: 40},
		},
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
	if scan.Videos == nil || scan.Videos.Total != 100 || scan.Videos.Settled != 40 {
		t.Errorf("進捗が引き継がれていない: %+v", scan.Videos)
	}
}

// 直近の状態を返す。古い項目（startedAt・finishedAt・total・completed・failed）は
// 返さない（specs/024-import-progress/contracts/scan-api.md §2）。
func TestGetCurrentScan(t *testing.T) {
	handler := newTestServer(t, Options{Scans: &fakeScans{
		hasScan: true,
		current: domain.Scan{
			ID: 3, State: domain.ScanDone, Total: 12, Completed: 11, Failed: 1,
			StartedAt: time.Unix(1_700_000_000, 0).UTC(), FinishedAt: time.Unix(1_700_000_050, 0).UTC(),
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
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"startedAt", "finishedAt", "total", "completed", "failed"} {
		if _, ok := raw[key]; ok {
			t.Errorf("古い項目 %q を返した: %s", key, rec.Body)
		}
	}
}

// 取り込みの状態・本数・完了の時刻は、internal/app が組み立てた値をそのまま返す。
// finding のあいだは本数を、done・partial 以外では完了の時刻を省く
// （specs/024-import-progress/contracts/scan-api.md §2）。
func TestGetCurrentScanReturnsImportProgress(t *testing.T) {
	settledAt := time.Unix(1_700_000_100, 0).UTC()
	for _, tc := range []struct {
		name        string
		progress    domain.ImportProgress
		wantStatus  gen.ScanStatus
		wantVideos  *gen.ScanVideos
		wantSettled bool
	}{
		{
			name:       "finding",
			progress:   domain.ImportProgress{Status: domain.ImportFinding},
			wantStatus: gen.ScanStatusFinding,
		},
		{
			name:       "running",
			progress:   domain.ImportProgress{Status: domain.ImportRunning, Counted: true, Total: 10, Settled: 4},
			wantStatus: gen.ScanStatusRunning,
			wantVideos: &gen.ScanVideos{Total: 10, Settled: 4},
		},
		{
			name: "done",
			progress: domain.ImportProgress{Status: domain.ImportDone, Counted: true, Total: 10, Settled: 10,
				SettledAt: settledAt},
			wantStatus:  gen.ScanStatusDone,
			wantVideos:  &gen.ScanVideos{Total: 10, Settled: 10},
			wantSettled: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler := newTestServer(t, Options{Scans: &fakeScans{
				hasScan: true,
				current: domain.Scan{ID: 3, State: domain.ScanDone, Import: tc.progress},
			}})
			rec := do(t, handler, http.MethodGet, "/api/scans/current")
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
			}
			var raw map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
				t.Fatal(err)
			}
			if _, ok := raw["status"]; !ok {
				t.Fatalf("status が無い: %s", rec.Body)
			}
			scan := decode[gen.Scan](t, rec)
			if scan.Status != tc.wantStatus {
				t.Errorf("status = %q, want %q", scan.Status, tc.wantStatus)
			}
			if (scan.Videos == nil) != (tc.wantVideos == nil) ||
				(scan.Videos != nil && *scan.Videos != *tc.wantVideos) {
				t.Errorf("videos = %+v, want %+v", scan.Videos, tc.wantVideos)
			}
			if tc.wantSettled {
				if scan.SettledAt == nil || !scan.SettledAt.Equal(settledAt) {
					t.Errorf("settledAt = %v, want %v", scan.SettledAt, settledAt)
				}
			} else if scan.SettledAt != nil {
				t.Errorf("settledAt = %v, want 省略", scan.SettledAt)
			}
		})
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
	assertErrorBody(t, "取り込みが無い", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: reasonNoScan})
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

func TestStartScanRejectsEmptyMediaFolderSet(t *testing.T) {
	handler := newTestServer(t, Options{Scans: &fakeScans{startErr: domain.ErrNoMediaFolders}})
	rec := do(t, handler, http.MethodPost, "/api/scans")
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", rec.Code, rec.Body)
	}
	if got := decode[gen.Error](t, rec); got.Code != codeMediaFoldersNotConfigured {
		t.Fatalf("code = %q, want %q", got.Code, codeMediaFoldersNotConfigured)
	}
}

func TestStartScanRejectsCrossOriginRequest(t *testing.T) {
	scans := &fakeScans{}
	handler := newTestServer(t, Options{Scans: scans})
	rec := request(t, handler, http.MethodPost, "/api/scans", `{}`, map[string]string{
		"Content-Type": "application/json", "Origin": "https://attacker.example",
	})
	if rec.Code != http.StatusForbidden || scans.started != 0 {
		t.Fatalf("response = %d, started = %d", rec.Code, scans.started)
	}
}

func TestStartScanRejectsInvalidJSONBody(t *testing.T) {
	scans := &fakeScans{}
	handler := newTestServer(t, Options{Scans: scans})
	for _, body := range []string{"", `{"unexpected":true}`} {
		rec := request(t, handler, http.MethodPost, "/api/scans", body, map[string]string{"Content-Type": "application/json"})
		if rec.Code != http.StatusBadRequest || scans.started != 0 {
			t.Fatalf("body=%q response=%d started=%d", body, rec.Code, scans.started)
		}
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

// scanIssueFixture は並びを確かめるための問題である。domain.ScanIssues が並べる。
func scanIssueFixture() []domain.ScanIssue {
	media := filepath.FromSlash("/media")
	return domain.ScanIssues([]domain.MediaFolder{{ID: 7, Path: media}}, []domain.ScanIssueRecord{
		{VideoID: 11, Path: filepath.Join(media, "b.mp4"), Kinds: []domain.ScanIssueKind{domain.IssueThumbnailFirstFrame}, InImport: true},
		{VideoID: 12, Path: filepath.Join(media, "sub", "c.mp4"),
			Kinds: []domain.ScanIssueKind{domain.IssueThumbnailFirstFrame, domain.IssueProbeFailed}, InImport: true},
		{Path: filepath.Join(media, "a.mp4"), Kinds: []domain.ScanIssueKind{domain.IssueUnreadable}},
	})
}

// 問題の一覧は、失敗を先に、同じ重さの中はファイル名の順に返し、続きがあるときだけ
// nextCursor を返す（specs/024-import-progress/contracts/scan-api.md §3）。
func TestListCurrentScanIssuesOrderAndCursor(t *testing.T) {
	handler := newTestServer(t, Options{Scans: &fakeScans{
		hasScan: true, current: domain.Scan{ID: 9, State: domain.ScanDone}, issues: scanIssueFixture(),
	}})

	rec := do(t, handler, http.MethodGet, "/api/scans/current/issues?limit=2")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Cache-Control"); got != cacheNoStore {
		t.Errorf("Cache-Control = %q", got)
	}
	first := decode[gen.ScanIssuePage](t, rec)
	if first.ScanId != 9 || len(first.Items) != 2 || first.NextCursor == nil {
		t.Fatalf("1ページ目 = %+v", first)
	}
	a, c := first.Items[0], first.Items[1]
	if a.FileName != "a.mp4" || a.Severity != gen.ScanIssueSeverityFailed || a.VideoId != nil ||
		!slices.Equal(a.Kinds, []gen.ScanIssueKind{gen.ScanIssueKindUnreadable}) || a.Folder.RootId != 7 || a.Folder.Path != "" {
		t.Errorf("1件目 = %+v, want 未登録の a.mp4", a)
	}
	if c.FileName != "c.mp4" || c.VideoId == nil || *c.VideoId != 12 || c.Folder.Path != "sub" ||
		c.Folder.RootName == nil || *c.Folder.RootName != "media" ||
		!slices.Equal(c.Kinds, []gen.ScanIssueKind{gen.ScanIssueKindProbeFailed, gen.ScanIssueKindThumbnailFirstFrame}) {
		t.Errorf("2件目 = %+v, want 動画12の c.mp4", c)
	}

	rec = do(t, handler, http.MethodGet, "/api/scans/current/issues?limit=2&cursor="+url.QueryEscape(*first.NextCursor))
	second := decode[gen.ScanIssuePage](t, rec)
	if len(second.Items) != 1 || second.Items[0].FileName != "b.mp4" ||
		second.Items[0].Severity != gen.ScanIssueSeveritySubstituted || second.NextCursor != nil {
		t.Fatalf("2ページ目 = %+v, want 代用の b.mp4 だけで続きなし", second)
	}

	// 既定の件数は 50 で、すべて入れば nextCursor を返さない。
	all := decode[gen.ScanIssuePage](t, do(t, handler, http.MethodGet, "/api/scans/current/issues"))
	if len(all.Items) != 3 || all.NextCursor != nil {
		t.Fatalf("既定の件数 = %+v", all)
	}
}

func TestListCurrentScanIssuesRejectsInvalidRequests(t *testing.T) {
	handler := newTestServer(t, Options{Scans: &fakeScans{
		hasScan: true, current: domain.Scan{ID: 9, State: domain.ScanDone}, issues: scanIssueFixture(),
	}})
	rec := do(t, handler, http.MethodGet, "/api/scans/current/issues?cursor=not-a-cursor")
	assertErrorBody(t, "不正なカーソル", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusBadRequest, code: gen.ErrorCodeInvalidRequest, reason: reasonInvalidCursor})
	for _, limit := range []string{"0", "201"} {
		if rec := do(t, handler, http.MethodGet, "/api/scans/current/issues?limit="+limit); rec.Code != http.StatusBadRequest {
			t.Errorf("limit=%s: status = %d, want 400", limit, rec.Code)
		}
	}
}

// 一度も走査していなければ 404。
func TestListCurrentScanIssuesWhenNeverScanned(t *testing.T) {
	handler := newTestServer(t, Options{Scans: &fakeScans{}})
	rec := do(t, handler, http.MethodGet, "/api/scans/current/issues")
	assertErrorBody(t, "取り込みが無い", rec.Code, rec.Body.Bytes(),
		wantError{status: http.StatusNotFound, code: gen.ErrorCodeNotFound, reason: reasonNoScan})
}

// 問題の一覧は所有者だけの経路で、ゲストには未認証を返し、中身を出さない（要件 11）。
func TestListCurrentScanIssuesIsOwnerOnly(t *testing.T) {
	if _, ok := accessRoutes["GET /api/scans/current/issues"]; ok {
		t.Fatal("問題の一覧がゲストにも返す経路の表に入っている")
	}
	env := newAuthEnv(t, t.TempDir(), Options{Scans: &fakeScans{
		hasScan: true, current: domain.Scan{ID: 9, State: domain.ScanDone}, issues: scanIssueFixture(),
	}})
	cookie := env.setup()

	guest := env.get("/api/scans/current/issues")
	assertUnauthenticated(t, "ゲストの問題の一覧", guest)
	if strings.Contains(guest.Body.String(), "a.mp4") {
		t.Fatalf("ゲストに問題を返した: %s", guest.Body)
	}
	if owner := env.get("/api/scans/current/issues", cookie); owner.Code != http.StatusOK {
		t.Fatalf("所有者: status = %d: %s", owner.Code, owner.Body)
	}
}

// Scan.issues は internal/app が数えた本数と、保存された番号を返す。
func TestGetCurrentScanReturnsIssueCounts(t *testing.T) {
	handler := newTestServer(t, Options{Scans: &fakeScans{
		hasScan: true,
		current: domain.Scan{
			ID: 3, State: domain.ScanDone, IssuesRevision: 4,
			Issues: domain.ScanIssueCounts{Failed: 2, Substituted: 1, Unregistered: 1},
			Import: domain.ImportProgress{Status: domain.ImportPartial, Counted: true, Total: 5, Settled: 5},
		},
	}})
	scan := decode[gen.Scan](t, do(t, handler, http.MethodGet, "/api/scans/current"))
	if scan.Status != gen.ScanStatusPartial || scan.Issues != (gen.ScanIssueCounts{Failed: 2, Substituted: 1, Revision: 4}) {
		t.Fatalf("scan = %+v, want partial・失敗2・代用1・番号4", scan)
	}
}
