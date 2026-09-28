package httpapi

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/extgen"
)

// ExternalVideos は外部連携 API の動画の一覧と引き当てである（specs/026-external-api/
// contracts/external-api.md §2）。internal/store の *LibraryStore がこれを満たす。見る人は
// 常に所有者なので、画面の Library と違って audience を取らない。
type ExternalVideos interface {
	// ListExternalVideos は登録フォルダの下に所在を持つ動画を (added_at, id) の昇順で
	// 1 ページ返す。解釈できないカーソルは domain.ErrInvalidCursor。
	ListExternalVideos(ctx context.Context, q domain.ExternalVideoQuery) (domain.ExternalVideoPage, error)
	// LookupExternalVideo は ref の動画を返す。無い、または登録フォルダの下に所在が
	// 無ければ domain.ErrNotFound。
	LookupExternalVideo(ctx context.Context, ref domain.VideoRef) (domain.ExternalVideo, error)
}

// ListVideos は動画の一覧 1 ページを返す（GET /api/v1/videos）。limit の範囲外は丸めずに
// 400 にする（画面の API と違い、契約が範囲外を誤りと決めている）。
func (e *externalServer) ListVideos(w http.ResponseWriter, r *http.Request, params extgen.ListVideosParams) {
	if e.s.externalVideos == nil {
		e.internalError(w, "Video storage is not configured.", nil)
		return
	}
	query := domain.ExternalVideoQuery{Limit: domain.ExternalVideoDefaultLimit}
	if params.Limit != nil {
		if *params.Limit < 1 || *params.Limit > domain.MaxLimit {
			e.invalidRequest(w, nil, "limit must be between 1 and 200.")
			return
		}
		query.Limit = *params.Limit
	}
	if params.Cursor != nil {
		query.Cursor = *params.Cursor
	}
	page, err := e.s.externalVideos.ListExternalVideos(r.Context(), query)
	if errors.Is(err, domain.ErrInvalidCursor) {
		reason := extgen.InvalidCursor
		e.invalidRequest(w, &reason, "Cannot read the cursor. Read the list again from the start.")
		return
	}
	if err != nil {
		e.internalError(w, "Could not load the videos.", err)
		return
	}
	out := extgen.ExternalVideoPage{Items: make([]extgen.ExternalVideo, 0, len(page.Items)), NextCursor: page.NextCursor}
	for _, item := range page.Items {
		out.Items = append(out.Items, toExternalVideo(item))
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, out, e.s.logger)
}

// LookupVideo は id・内容キー・パスのちょうど 1 つで動画を引く（GET /api/v1/videos/lookup）。
// 指定の数は引数が要求にあるかで数え、空の値も 1 つと数えたうえで誤りにする。
func (e *externalServer) LookupVideo(w http.ResponseWriter, r *http.Request, params extgen.LookupVideoParams) {
	if e.s.externalVideos == nil {
		e.internalError(w, "Video storage is not configured.", nil)
		return
	}
	var ref domain.VideoRef
	given := 0
	if params.Id != nil {
		given++
		ref.ID = *params.Id
	}
	if params.ContentKey != nil {
		given++
		ref.ContentKey = *params.ContentKey
	}
	if params.Path != nil {
		given++
		ref.Path = *params.Path
	}
	if given != 1 {
		e.invalidRequest(w, nil, "Give exactly one of id, contentKey and path.")
		return
	}
	if (params.ContentKey != nil && ref.ContentKey == "") || (params.Path != nil && ref.Path == "") {
		e.invalidRequest(w, nil, "contentKey and path must not be empty.")
		return
	}
	// 1 未満の id は、どの動画にも当たらない id として 404 にする（ref.Valid が偽になり、
	// 保存先が domain.ErrNotFound を返す）。
	video, err := e.s.externalVideos.LookupExternalVideo(r.Context(), ref)
	if errors.Is(err, domain.ErrNotFound) {
		reason := extgen.VideoNotFound
		writeExternalError(w, e.s, http.StatusNotFound, extgen.Error{
			Code: extgen.ErrorCodeNotFound, Reason: &reason, Message: "The video is not in the library.",
		})
		return
	}
	if err != nil {
		e.internalError(w, "Could not look up the video.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, toExternalVideo(video), e.s.logger)
}

// invalidRequest は 400 invalid_request を返す。
func (e *externalServer) invalidRequest(w http.ResponseWriter, reason *extgen.ErrorReason, message string) {
	writeExternalError(w, e.s, http.StatusBadRequest, extgen.Error{
		Code: extgen.ErrorCodeInvalidRequest, Reason: reason, Message: message,
	})
}

// toExternalVideo は domain.ExternalVideo を外部連携 API の形へ写す。
func toExternalVideo(item domain.ExternalVideo) extgen.ExternalVideo {
	out := extgen.ExternalVideo{
		Id:         item.Video.ID,
		ContentKey: item.Video.ContentKey,
		Title:      item.Video.Title,
		DurationMs: item.Video.DurationMs,
		AddedAt:    item.Video.AddedAt.UTC(),
		Locations:  make([]extgen.ExternalVideoLocation, 0, len(item.Locations)),
		Tags:       toExternalVideoTags(item.Tags),
	}
	for _, path := range item.Locations {
		out.Locations = append(out.Locations, extgen.ExternalVideoLocation{Path: path, FileName: filepath.Base(path)})
	}
	return out
}

// toExternalVideoTags は動画のタグを出所つきで写す。後の単位（タグの一括操作）の応答も
// 同じ形を使う。
func toExternalVideoTags(tags []domain.VideoTag) []extgen.ExternalVideoTag {
	out := make([]extgen.ExternalVideoTag, 0, len(tags))
	for _, tag := range tags {
		out = append(out, extgen.ExternalVideoTag{Id: tag.ID, Name: tag.Name, Manual: tag.Manual, FromFolder: tag.FromFolder})
	}
	return out
}
