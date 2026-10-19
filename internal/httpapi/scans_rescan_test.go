package httpapi

import (
	"net/http"
	"path/filepath"
	"slices"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

func TestStartRescanAcceptsEachScope(t *testing.T) {
	folderID, videoID := int64(2), int64(9)
	for _, tc := range []struct {
		name, body string
		scope      domain.RescanScope
		want       gen.ScanRescan
	}{
		{"library", `{"scope":"library"}`, domain.RescanScope{Kind: domain.RescanLibrary},
			gen.ScanRescan{Scope: gen.RescanScopeLibrary}},
		{"folder", `{"scope":"folder","folderId":2}`, domain.RescanScope{Kind: domain.RescanFolder, FolderID: 2},
			gen.ScanRescan{Scope: gen.RescanScopeFolder, Folder: &gen.ScanRescanFolder{Id: folderID}}},
		{"video", `{"scope":"video","videoId":9}`, domain.RescanScope{Kind: domain.RescanVideo, VideoID: 9},
			gen.ScanRescan{Scope: gen.RescanScopeVideo, VideoId: &videoID}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scans := &fakeScans{}
			handler := newTestServer(t, Options{Scans: scans})

			rec := postJSON(t, handler, "/api/scans/rescan", tc.body)
			if rec.Code != http.StatusAccepted {
				t.Fatalf("status = %d, want 202: %s", rec.Code, rec.Body)
			}
			if !slices.Equal(scans.rescans, []domain.RescanScope{tc.scope}) {
				t.Fatalf("StartRescan の範囲 = %+v, want %+v", scans.rescans, tc.scope)
			}
			scan := decode[gen.Scan](t, rec)
			if scan.Rescan == nil || scan.Rescan.Scope != tc.want.Scope ||
				!equalPtr(scan.Rescan.VideoId, tc.want.VideoId) || !equalPtr(scan.Rescan.Folder, tc.want.Folder) {
				t.Fatalf("rescan = %+v, want %+v", scan.Rescan, tc.want)
			}
		})
	}
}

func TestScanRescanCarriesFolderPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "b")
	got := toAPIScan(domain.Scan{Rescan: &domain.RescanScope{Kind: domain.RescanFolder, FolderID: 2, Path: path}})
	if got.Rescan == nil || got.Rescan.Folder == nil || *got.Rescan.Folder != (gen.ScanRescanFolder{Id: 2, Path: path}) {
		t.Fatalf("rescan = %+v, want folder 2 at %s", got.Rescan, path)
	}
	if plain := toAPIScan(domain.Scan{}); plain.Rescan != nil {
		t.Fatalf("通常の走査の rescan = %+v, want 省略", plain.Rescan)
	}
}

func TestStartRescanErrors(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		err        error
		status     int
		code       gen.ErrorCode
	}{
		{"unknown scope", `{"scope":"tag"}`, nil, http.StatusBadRequest, codeInvalidRequest},
		{"folder without folderId", `{"scope":"folder"}`, nil, http.StatusBadRequest, codeInvalidRequest},
		{"video with folderId", `{"scope":"video","videoId":9,"folderId":2}`, nil, http.StatusBadRequest, codeInvalidRequest},
		{"library with videoId", `{"scope":"library","videoId":9}`, nil, http.StatusBadRequest, codeInvalidRequest},
		{"missing video", `{"scope":"video","videoId":9}`, domain.ErrNotFound, http.StatusNotFound, codeNotFound},
		{"missing folder", `{"scope":"folder","folderId":2}`, domain.ErrNotFound, http.StatusNotFound, codeNotFound},
		{"no media folder", `{"scope":"library"}`, domain.ErrNoMediaFolders, http.StatusConflict, codeMediaFoldersNotConfigured},
		{"scan running", `{"scope":"library"}`, domain.ErrScanRunning, http.StatusConflict, codeScanInProgress},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scans := &fakeScans{rescanErr: tc.err}
			handler := newTestServer(t, Options{Scans: scans})

			rec := postJSON(t, handler, "/api/scans/rescan", tc.body)
			assertErrorCode(t, rec, tc.status, tc.code)
			if tc.status == http.StatusBadRequest && len(scans.rescans) != 0 {
				t.Fatalf("StartRescan = %+v, want 呼ばれない", scans.rescans)
			}
		})
	}
}

func equalPtr[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
