package httpapi

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/extgen"
)

// External handlers use a separate generated interface to avoid UI-operation name collisions while
// sharing dependencies; bearerBoundary authenticates all requests first (api/external-v1.yaml).

// Generated interface conformance catches unimplemented schema operations at compile time.
type externalServer struct {
	s *server
	// maxBody overrides readJSONBody's limit; zero uses externalBodyLimit.
	maxBody int64
}

func registerExternalAPI(mux *http.ServeMux, srv *server) {
	extgen.HandlerWithOptions(&externalServer{s: srv}, extgen.StdHTTPServerOptions{
		BaseURL:    externalAPIBase,
		BaseRouter: mux,
		ErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
			writeExternalError(w, srv, http.StatusBadRequest, extgen.Error{
				Code: extgen.ErrorCodeInvalidRequest, Message: err.Error(),
			})
		},
	})
}

func writeExternalError(w http.ResponseWriter, srv *server, status int, body extgen.Error) {
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, status, body, srv.logger)
}

// Log internal causes without exposing them in responses.
func (e *externalServer) internalError(w http.ResponseWriter, message string, err error) {
	e.s.logger.Error(message, slog.Any("error", err))
	writeExternalError(w, e.s, http.StatusInternalServerError, extgen.Error{
		Code: extgen.ErrorCodeInternal, Message: message,
	})
}

// External tag lists share UI filters/storage; omitted limit returns all matches without a cursor.
func (e *externalServer) ListTags(w http.ResponseWriter, r *http.Request, params extgen.ListTagsParams) {
	sort := sortParam(params.Sort)
	query, problem := buildTagListQuery(params.Q, params.Tentative, params.Unused, sort, params.Cursor, params.Limit)
	if !e.acceptListQuery(w, problem) {
		return
	}
	if params.Kind != nil {
		kind, ok := parseTagKindFilter(*params.Kind)
		if !ok {
			e.invalidRequest(w, nil, "kind must be a positive integer or none.")
			return
		}
		query.Kind = kind
	}
	if e.s.tags == nil {
		e.internalError(w, "Tag storage is not configured.", nil)
		return
	}
	page, err := e.s.tags.ListTags(r.Context(), query)
	if errors.Is(err, domain.ErrInvalidCursor) {
		reason := extgen.InvalidCursor
		e.invalidRequest(w, &reason, "Cannot read the cursor. Read the list again from the start with the same q, tentative, unused, kind and sort.")
		return
	}
	if err != nil {
		e.internalError(w, "Could not load tags.", err)
		return
	}
	items := make([]extgen.Tag, 0, len(page.Items))
	for _, tag := range page.Items {
		items = append(items, toExternalTag(tag))
	}
	body := extgen.TagList{Items: items, Total: page.Total, TotalAll: page.TotalAll}
	if page.NextCursor != "" {
		body.NextCursor = &page.NextCursor
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, body, e.s.logger)
}

func (e *externalServer) acceptListQuery(w http.ResponseWriter, problem tagListProblem) bool {
	switch problem {
	case tagListSortUnknown:
		e.invalidRequest(w, nil, "Unknown sort order. Use name, countDesc, countAsc, createdDesc or createdAsc.")
		return false
	case tagListLimitOutOfRange:
		e.invalidRequest(w, nil, fmt.Sprintf("limit must be between 1 and %d.", domain.MaxTagPageLimit))
		return false
	case tagListSearchTooLong:
		e.invalidRequest(w, nil, fmt.Sprintf("q must be at most %d characters.", maxQueryLength))
		return false
	}
	return true
}

func toExternalTag(tag domain.Tag) extgen.Tag {
	synonyms := tag.Synonyms
	if synonyms == nil {
		synonyms = []string{}
	}
	return extgen.Tag{
		Id: tag.ID, Name: tag.Name, Synonyms: synonyms, VideoCount: tag.VideoCount, Tentative: tag.Tentative,
		CreatedAt: tag.CreatedAt, Kind: toExternalTagKindRef(tag.Kind),
	}
}

// Use StartScan's transactional started result for 201 versus 200; rereading before response would
// introduce a race.
func (e *externalServer) StartScan(w http.ResponseWriter, r *http.Request) {
	if e.s.scans == nil {
		e.internalError(w, "Scanning is not configured.", nil)
		return
	}
	scan, started, err := e.s.scans.StartScan(r.Context())
	if errors.Is(err, domain.ErrNoMediaFolders) {
		writeExternalError(w, e.s, http.StatusConflict, extgen.Error{
			Code: extgen.ErrorCodeMediaFoldersNotConfigured, Message: "Add a media folder first.",
		})
		return
	}
	if err != nil {
		e.internalError(w, "Could not start the scan.", err)
		return
	}
	status := http.StatusOK
	if started {
		status = http.StatusCreated
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, status, toExternalScan(scan), e.s.logger)
}

func (e *externalServer) GetCurrentScan(w http.ResponseWriter, r *http.Request) {
	if e.s.scans == nil {
		e.internalError(w, "Scanning is not configured.", nil)
		return
	}
	scan, err := e.s.scans.CurrentScan(r.Context())
	if errors.Is(err, domain.ErrNotFound) {
		reason := extgen.NoScan
		writeExternalError(w, e.s, http.StatusNotFound, extgen.Error{
			Code: extgen.ErrorCodeNotFound, Reason: &reason, Message: "No scan has been run yet.",
		})
		return
	}
	if err != nil {
		e.internalError(w, "Could not load the scan status.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, toExternalScan(scan), e.s.logger)
}

// Counts remain null during enumeration, distinguishing unknown totals from a completed empty scan.
func toExternalScan(scan domain.Scan) extgen.ExternalScan {
	out := extgen.ExternalScan{
		Id:        scan.ID,
		Status:    extgen.ExternalScanStatus(scan.Import.Status),
		StartedAt: scan.StartedAt.UTC(),
	}
	if !scan.FinishedAt.IsZero() {
		finishedAt := scan.FinishedAt.UTC()
		out.FinishedAt = &finishedAt
	}
	if scan.Import.Counted {
		videos, settled := scan.Import.Total, scan.Import.Settled
		out.Videos, out.SettledVideos = &videos, &settled
	}
	// Only failed rows expose codes; older failures may lack one.
	if scan.State == domain.ScanFailed && scan.ErrorCode != "" {
		code := string(scan.ErrorCode)
		out.ErrorCode = &code
	}
	return out
}
