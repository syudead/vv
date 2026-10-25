package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// VideoCodeEdits reads and writes the codes of a selection for the table editor; each call is one store transaction.
type VideoCodeEdits interface {
	// CodeSummary returns one row per user key of the listed videos, in first-listed order.
	CodeSummary(ctx context.Context, videoIDs []int64) ([]domain.VideoCodeRow, error)
	// ApplyCodes writes every item or nothing and returns the number of keys whose code changed.
	ApplyCodes(ctx context.Context, items []domain.VideoCodeEdit) (int, error)
}

func (s *server) SummarizeVideoCodeEdits(w http.ResponseWriter, r *http.Request) {
	var body gen.VideoTagsSummaryRequest
	if !s.readJSONBody(w, r, &body) || !s.videoCodeEditsConfigured(w) {
		return
	}
	sel, ok := s.resolveSelection(w, r, body.VideoIds, body.SmartFolderIds, domain.VideoCodeEditsMaxItems)
	if !ok {
		return
	}
	out := gen.VideoCodeEditSummary{Rows: []gen.VideoCodeEditRow{}}
	if len(sel.local) > 0 {
		rows, err := s.codeEdits.CodeSummary(r.Context(), sel.local)
		if err != nil {
			s.internalError(w, "Could not load the codes.", err)
			return
		}
		videos := make([]domain.Video, len(rows))
		for i, row := range rows {
			videos[i] = row.Video
		}
		items := s.listItems(r.Context(), videos, s.registeredRoots(r.Context()))
		for i, row := range rows {
			out.Rows = append(out.Rows, gen.VideoCodeEditRow{Video: items[i], Code: optionalString(row.Code)})
		}
	}
	var peer gen.VideoCodeEditSummary
	if sel.hasPeer() && s.peerSelectionRead(r, "/api/video-code-edits/summary", sel, &peer) {
		join, err := s.vocabularyJoin(r.Context())
		if err != nil {
			s.internalError(w, "Could not load the codes.", err)
			return
		}
		for _, row := range peer.Rows {
			rewriteRemoteVideo(&row.Video, join)
			out.Rows = append(out.Rows, row)
		}
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, out, s.logger)
}

// ApplyVideoCodeEdits validates every item before the store resolves the videos. Items of the peer's videos are
// applied there after the local items, which stay when the peer's fail.
func (s *server) ApplyVideoCodeEdits(w http.ResponseWriter, r *http.Request) {
	var body gen.VideoCodeEditRequest
	if !s.readJSONBody(w, r, &body) {
		return
	}
	items := make([]domain.VideoCodeEdit, len(body.Items))
	for i, item := range body.Items {
		items[i] = domain.VideoCodeEdit{VideoID: item.VideoId, ContentVersion: item.ContentVersion, Code: item.Code}
	}
	items, err := domain.NormalizeVideoCodeEdits(items)
	if err != nil {
		s.writeVideoCodeEditError(w, err)
		return
	}
	if !s.videoCodeEditsConfigured(w) {
		return
	}
	local, positions, peer := splitCodeEdits(r.Context(), items)
	var out gen.VideoCodeEditResponse
	if len(local) > 0 {
		if out.Applied, err = s.codeEdits.ApplyCodes(r.Context(), local); err != nil {
			var at *domain.VideoCodeEditAtError
			if errors.As(err, &at) {
				err = &domain.VideoCodeEditAtError{Index: positions[at.Index], Err: at.Err}
			}
			s.writeVideoCodeEditError(w, err)
			return
		}
	}
	if len(peer.Items) > 0 {
		var answer gen.VideoCodeEditResponse
		err := s.peerCall(r, http.MethodPost, "/api/video-code-edits", nil, peer, &answer)
		ids := make([]int64, len(peer.Items))
		for i, item := range peer.Items {
			ids[i] = item.VideoId
		}
		out.Remote = s.remoteSelectionResult(answer.Applied, ids, err)
		out.Applied += out.Remote.Applied
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, out, s.logger)
}

// splitCodeEdits separates this node's items, with each one's index in items, from the peer's request; a request of
// the peer keeps every item here.
func splitCodeEdits(ctx context.Context, items []domain.VideoCodeEdit) ([]domain.VideoCodeEdit, []int, gen.VideoCodeEditRequest) {
	var local []domain.VideoCodeEdit
	var positions []int
	peer := gen.VideoCodeEditRequest{}
	for i, item := range items {
		if !domain.IsRemoteID(item.VideoID) || isPeerRequest(ctx) {
			local, positions = append(local, item), append(positions, i)
			continue
		}
		peer.Items = append(peer.Items, gen.VideoCodeEditItem{
			VideoId: domain.FromRemoteID(item.VideoID), ContentVersion: item.ContentVersion, Code: item.Code,
		})
	}
	return local, positions, peer
}

// writeVideoCodeEditError names the failing item in field; every error leaves the codes unchanged.
func (s *server) writeVideoCodeEditError(w http.ResponseWriter, err error) {
	if errors.Is(err, domain.ErrVideoCodeEditCount) {
		s.invalidRequestAt(w, "items", fmt.Sprintf("items must have between 1 and %d entries.", domain.VideoCodeEditsMaxItems))
		return
	}
	var at *domain.VideoCodeEditAtError
	if !errors.As(err, &at) {
		s.internalError(w, "Could not save the codes.", err)
		return
	}
	item := fmt.Sprintf("items[%d]", at.Index)
	failure, ok := workInfoFailureOf(at.Err)
	switch {
	case ok:
		failure.field = item + "." + failure.field
	case errors.Is(at.Err, domain.ErrDuplicateVideoCodeEdit):
		s.invalidRequestAt(w, item+".videoId", "Another item already writes the code of this video.")
		return
	case errors.Is(at.Err, domain.ErrNotFound):
		failure = workInfoFailure{
			status: http.StatusNotFound, code: string(codeNotFound), reason: string(reasonVideoNotFound),
			field: item + ".videoId", message: "Video not found.",
		}
	case errors.Is(at.Err, domain.ErrContentChanged):
		failure = workInfoFailure{
			status: http.StatusConflict, code: string(codeConflict), reason: string(reasonContentChanged),
			field: item + ".contentVersion", message: "The video changed in the meantime.",
		}
	default:
		s.internalError(w, "Could not save the codes.", err)
		return
	}
	s.writeWorkInfoFailure(w, failure)
}

func (s *server) invalidRequestAt(w http.ResponseWriter, field, message string) {
	s.writeErrorBody(w, http.StatusBadRequest, gen.Error{Code: codeInvalidRequest, Field: &field, Message: message})
}

func (s *server) videoCodeEditsConfigured(w http.ResponseWriter) bool {
	if s.codeEdits == nil {
		s.internalError(w, "Video code storage is not configured.", nil)
		return false
	}
	return true
}
