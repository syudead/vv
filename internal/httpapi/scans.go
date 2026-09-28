package httpapi

import (
	"errors"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// StartScan は取り込みを開始する（POST /api/scans）。
//
// すでに実行中なら新しく始めず、実行中のものを返す。409 にしないのは、
// 利用者の意図が「今の状態を進めたい」であり、進行中ならそれを返すのが
// 素直だからである。
//
// 応答は即座に返り、取り込みは背後で進む。走査中も一覧・再生の経路は通常
// どおり応答する。
func (s *server) StartScan(w http.ResponseWriter, r *http.Request) {
	var body struct{}
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if s.scans == nil {
		s.internalError(w, "Scanning is not configured.", nil)
		return
	}

	// 画面は新しく始めたかを区別せず、どちらも 202 にする（外部連携 API は区別する）。
	scan, _, err := s.scans.StartScan(r.Context())
	if errors.Is(err, domain.ErrNoMediaFolders) {
		s.writeError(w, http.StatusConflict, codeMediaFoldersNotConfigured, "Add a media folder first.")
		return
	}
	if err != nil {
		s.internalError(w, "Could not start the scan.", err)
		return
	}

	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusAccepted, toAPIScan(scan), s.logger)
}

// GetCurrentScan は直近のスキャンの状態を返す（GET /api/scans/current）。
// 実行中のものがあればそれを、無ければ最後に終わったものを返す。
func (s *server) GetCurrentScan(w http.ResponseWriter, r *http.Request) {
	if s.scans == nil {
		s.internalError(w, "Scanning is not configured.", nil)
		return
	}

	scan, err := s.scans.CurrentScan(r.Context())
	switch {
	case errors.Is(err, domain.ErrNotFound):
		s.notFoundReason(w, reasonNoScan, "No scan has been run yet.")
		return
	case err != nil:
		s.internalError(w, "Could not load the scan status.", err)
		return
	}

	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, toAPIScan(scan), s.logger)
}

// 問題の一覧の1ページの件数（api/openapi.yaml の limit と同じ値）。
const (
	defaultScanIssueLimit = 50
	maxScanIssueLimit     = 200
)

// ListCurrentScanIssues は直近の取り込みの問題の一覧を返す
// （GET /api/scans/current/issues、specs/024-import-progress/contracts/scan-api.md §3）。
// 所有者だけの経路で、ゲストに返す経路の表（accessRoutes）には入れない。
func (s *server) ListCurrentScanIssues(w http.ResponseWriter, r *http.Request, params gen.ListCurrentScanIssuesParams) {
	if s.scans == nil {
		s.internalError(w, "Scanning is not configured.", nil)
		return
	}
	limit := defaultScanIssueLimit
	if params.Limit != nil {
		if *params.Limit < 1 || *params.Limit > maxScanIssueLimit {
			s.invalidRequest(w, "limit must be between 1 and 200.")
			return
		}
		limit = *params.Limit
	}
	cursor := ""
	if params.Cursor != nil {
		cursor = *params.Cursor
	}

	page, err := s.scans.ListScanIssues(r.Context(), cursor, limit)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		s.notFoundReason(w, reasonNoScan, "No scan has been run yet.")
		return
	case errors.Is(err, domain.ErrInvalidCursor):
		s.invalidRequestReason(w, reasonInvalidCursor, "Cannot read the cursor. Reload the list.")
		return
	case err != nil:
		s.internalError(w, "Could not load the import issues.", err)
		return
	}

	out := gen.ScanIssuePage{ScanId: page.ScanID, Items: make([]gen.ScanIssue, 0, len(page.Items))}
	for _, issue := range page.Items {
		item := gen.ScanIssue{
			Severity: gen.ScanIssueSeverity(issue.Severity),
			Kinds:    make([]gen.ScanIssueKind, 0, len(issue.Kinds)),
			FileName: issue.FileName,
			Folder:   gen.VideoFolder{RootId: issue.Folder.RootID, Path: issue.Folder.Path},
		}
		if issue.RootName != "" {
			rootName := issue.RootName
			item.Folder.RootName = &rootName
		}
		for _, kind := range issue.Kinds {
			item.Kinds = append(item.Kinds, gen.ScanIssueKind(kind))
		}
		if issue.VideoID != 0 {
			videoID := issue.VideoID
			item.VideoId = &videoID
		}
		out.Items = append(out.Items, item)
	}
	if page.NextCursor != "" {
		next := page.NextCursor
		out.NextCursor = &next
	}

	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, out, s.logger)
}

// toAPIScan は domain.Scan を契約の形へ写す。
func toAPIScan(scan domain.Scan) gen.Scan {
	out := gen.Scan{
		Id:     scan.ID,
		Status: gen.ScanStatus(scan.Import.Status),
		Issues: gen.ScanIssueCounts{
			Failed: scan.Issues.Failed, Substituted: scan.Issues.Substituted, Revision: scan.IssuesRevision,
		},
		State: gen.ScanState(scan.State),
	}
	// 状態と本数は internal/app が組み立てたものを写すだけにする。
	if scan.Import.Counted {
		out.Videos = &gen.ScanVideos{Total: scan.Import.Total, Settled: scan.Import.Settled}
	}
	if !scan.Import.SettledAt.IsZero() {
		settledAt := scan.Import.SettledAt
		out.SettledAt = &settledAt
	}
	if scan.Activity.Active() {
		out.Activity = toAPIScanActivity(scan.Activity)
	}
	if scan.Error != "" {
		reason := scan.Error
		out.Error = &reason
	}
	// コードと場所は failed の行だけに出す。アップグレード前の失敗にはコードが無い
	// （specs/023-english-i18n/contracts/error-api.md §3）。
	if scan.State == domain.ScanFailed && scan.ErrorCode != "" {
		code := gen.ScanErrorCode(scan.ErrorCode)
		out.ErrorCode = &code
		if scan.ErrorPath != "" {
			path := scan.ErrorPath
			out.ErrorPath = &path
		}
	}
	return out
}

// toAPIScanActivity は今の処理を契約の形へ写す。利用者の言葉は SPA が kind から
// 組み立てる（specs/024-import-progress/research.md R-10）。
func toAPIScanActivity(activity domain.ScanActivity) *gen.ScanActivity {
	out := &gen.ScanActivity{Kind: gen.ScanActivityKind(activity.Kind), FileName: activity.FileName()}
	if activity.Located {
		folder := gen.VideoFolder{RootId: activity.Folder.RootID, Path: activity.Folder.Path}
		if activity.RootName != "" {
			rootName := activity.RootName
			folder.RootName = &rootName
		}
		out.Folder = &folder
	}
	if activity.VideoID != 0 {
		videoID := activity.VideoID
		out.VideoId = &videoID
	}
	return out
}
