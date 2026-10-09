package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// 視聴履歴の一覧と削除（specs/043-watch-history/contracts/screen-api.md）。3 つの経路は /api/* の
// 既定どおり所有者だけで、accessRoutes には入れない（research.md R-7）。

// WatchHistory は視聴履歴の読みと削除の先である。internal/store の *PlaybackStore がこれを満たす。
// どの操作も 1 つの文で済むので、internal/app は通さない。
type WatchHistory interface {
	// ListWatchHistory は (played_at desc, id desc) の順に cursor の次から limit 件返す。各件の
	// Video は audience が開ける動画があるときだけ入る。読めない cursor は domain.ErrInvalidCursor。
	ListWatchHistory(ctx context.Context, audience domain.Audience, cursor string, limit int) (domain.WatchHistoryPage, error)
	// DeleteWatchHistoryEntry は 1 件消し、その件があったかを返す。
	DeleteWatchHistoryEntry(ctx context.Context, id int64) (bool, error)
	// ClearWatchHistory はすべて消す。
	ClearWatchHistory(ctx context.Context) error
}

// 視聴履歴の 1 ページの件数（api/openapi.yaml の limit と同じ値）。
const (
	defaultWatchHistoryLimit = 60
	maxWatchHistoryLimit     = 200
)

// ListWatchHistory は視聴履歴を新しい順に返す（GET /api/watch-history）。各件の video には一覧と
// 同じく再生位置・タグ・お気に入りを載せる。
func (s *server) ListWatchHistory(w http.ResponseWriter, r *http.Request, params gen.ListWatchHistoryParams) {
	if s.watchHistory == nil {
		s.internalError(w, "Watch history storage is not configured.", nil)
		return
	}
	limit := defaultWatchHistoryLimit
	if params.Limit != nil {
		if *params.Limit < 1 || *params.Limit > maxWatchHistoryLimit {
			s.invalidRequest(w, "limit must be between 1 and 200.")
			return
		}
		limit = *params.Limit
	}
	cursor := ""
	if params.Cursor != nil {
		cursor = *params.Cursor
	}

	audience := audienceFrom(r.Context())
	page, err := s.watchHistory.ListWatchHistory(r.Context(), audience, cursor, limit)
	switch {
	case errors.Is(err, domain.ErrInvalidCursor):
		s.invalidRequestReason(w, reasonInvalidCursor, "Cannot read the cursor. Reload the list.")
		return
	case err != nil:
		s.internalError(w, "Could not load the watch history.", err)
		return
	}

	videos := make([]domain.Video, 0, len(page.Items))
	for _, entry := range page.Items {
		if entry.Video != nil {
			videos = append(videos, *entry.Video)
		}
	}
	progress := s.progressFor(r.Context(), videos)
	tags := s.tagsFor(r.Context(), videos)
	views := s.presentVideos(r.Context(), videos)

	out := gen.WatchHistoryPage{Items: make([]gen.WatchHistoryEntry, 0, len(page.Items))}
	next := 0
	for _, entry := range page.Items {
		item := gen.WatchHistoryEntry{Id: entry.ID, PlayedAt: entry.PlayedAt, Title: entry.Title}
		if entry.Video != nil {
			view := views[next]
			next++
			video := withTags(withProgress(toAPIVideo(view), progress, view.Video.UserKey), tags, view.Video.UserKey)
			video = forAudience(audience, video)
			item.Video = &video
		}
		out.Items = append(out.Items, item)
	}
	if page.NextCursor != "" {
		nextCursor := page.NextCursor
		out.NextCursor = &nextCursor
	}

	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, out, s.logger)
}

// DeleteWatchHistoryEntry は視聴履歴を 1 件消す（DELETE /api/watch-history/{id}）。無ければ
// 404 not_found で、画面は一覧を読み直す（research.md R-6）。
func (s *server) DeleteWatchHistoryEntry(w http.ResponseWriter, r *http.Request, id gen.WatchHistoryEntryId) {
	if s.watchHistory == nil {
		s.internalError(w, "Watch history storage is not configured.", nil)
		return
	}
	deleted, err := s.watchHistory.DeleteWatchHistoryEntry(r.Context(), id)
	if err != nil {
		s.internalError(w, "Could not delete the watch history entry.", err)
		return
	}
	if !deleted {
		s.notFound(w, "Watch history entry not found.")
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	w.WriteHeader(http.StatusNoContent)
}

// ClearWatchHistory は視聴履歴をすべて消す（DELETE /api/watch-history）。空でも 204。
func (s *server) ClearWatchHistory(w http.ResponseWriter, r *http.Request) {
	if s.watchHistory == nil {
		s.internalError(w, "Watch history storage is not configured.", nil)
		return
	}
	if err := s.watchHistory.ClearWatchHistory(r.Context()); err != nil {
		s.internalError(w, "Could not clear the watch history.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	w.WriteHeader(http.StatusNoContent)
}
