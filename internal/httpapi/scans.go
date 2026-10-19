package httpapi

import (
	"errors"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// StartScan returns the ongoing scan rather than rejecting duplicates, while scanning continues asynchronously.
func (s *server) StartScan(w http.ResponseWriter, r *http.Request) {
	var body struct{}
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if s.scans == nil {
		s.internalError(w, "Scanning is not configured.", nil)
		return
	}

	// The UI API always returns 202, unlike the external API's new/existing distinction.
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

// StartRescan refuses a running manual scan or force rescan instead of returning it, unlike StartScan.
func (s *server) StartRescan(w http.ResponseWriter, r *http.Request) {
	var body gen.StartRescanRequest
	if !s.readJSONBody(w, r, &body) {
		return
	}
	scope, ok := rescanScopeOf(body)
	if !ok {
		s.invalidRequest(w, "scope must be library, folder with folderId, or video with videoId.")
		return
	}
	if s.scans == nil {
		s.internalError(w, "Scanning is not configured.", nil)
		return
	}

	scan, err := s.scans.StartRescan(r.Context(), scope)
	switch {
	case errors.Is(err, domain.ErrNoMediaFolders):
		s.writeError(w, http.StatusConflict, codeMediaFoldersNotConfigured, "Add a media folder first.")
		return
	case errors.Is(err, domain.ErrScanRunning):
		s.writeError(w, http.StatusConflict, codeScanInProgress, "A scan is in progress.")
		return
	case errors.Is(err, domain.ErrNotFound) && scope.Kind == domain.RescanVideo:
		s.notFound(w, "Video not found.")
		return
	case errors.Is(err, domain.ErrNotFound):
		s.notFound(w, "Media folder not found.")
		return
	case err != nil:
		s.internalError(w, "Could not start the force rescan.", err)
		return
	}

	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusAccepted, toAPIScan(scan), s.logger)
}

// rescanScopeOf accepts exactly the target its scope names: folderId for folder, videoId for video, neither for
// library.
func rescanScopeOf(body gen.StartRescanRequest) (domain.RescanScope, bool) {
	switch body.Scope {
	case gen.RescanScopeLibrary:
		return domain.RescanScope{Kind: domain.RescanLibrary}, body.FolderId == nil && body.VideoId == nil
	case gen.RescanScopeFolder:
		if body.FolderId == nil || body.VideoId != nil {
			return domain.RescanScope{}, false
		}
		return domain.RescanScope{Kind: domain.RescanFolder, FolderID: *body.FolderId}, true
	case gen.RescanScopeVideo:
		if body.VideoId == nil || body.FolderId != nil {
			return domain.RescanScope{}, false
		}
		return domain.RescanScope{Kind: domain.RescanVideo, VideoID: *body.VideoId}, true
	default:
		return domain.RescanScope{}, false
	}
}

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

// Keep issue page limits aligned with api/openapi.yaml.
const (
	defaultScanIssueLimit = 50
	maxScanIssueLimit     = 200
)

// Scan issues remain owner-only by omission from accessRoutes (docs/design-docs/import-pipeline.md).
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

func toAPIScan(scan domain.Scan) gen.Scan {
	out := gen.Scan{
		Id:     scan.ID,
		Origin: gen.ScanOrigin(scan.Origin),
		Status: gen.ScanStatus(scan.Import.Status),
		Issues: gen.ScanIssueCounts{
			Failed: scan.Issues.Failed, Substituted: scan.Issues.Substituted, Revision: scan.IssuesRevision,
		},
		State: gen.ScanState(scan.State),
	}
	// Use app-computed state and counts without recomputing them here.
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
	if scan.Rescan != nil {
		out.Rescan = toAPIScanRescan(*scan.Rescan)
	}
	if scan.Error != "" {
		reason := scan.Error
		out.Error = &reason
	}
	// Only failed rows expose error code/path; older failures may lack a code.
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

func toAPIScanRescan(scope domain.RescanScope) *gen.ScanRescan {
	out := &gen.ScanRescan{Scope: gen.RescanScope(scope.Kind)}
	switch scope.Kind {
	case domain.RescanFolder:
		out.Folder = &gen.ScanRescanFolder{Id: scope.FolderID, Path: scope.Path}
	case domain.RescanVideo:
		videoID := scope.VideoID
		out.VideoId = &videoID
	case domain.RescanLibrary:
	}
	return out
}

// The SPA translates activity kind into user-facing text.
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
