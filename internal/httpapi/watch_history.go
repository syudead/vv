package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// Watch-history routes are owner-only and excluded from accessRoutes.

// WatchHistory is implemented directly by the store because each operation fits in one statement.
type WatchHistory interface {
	// ListWatchHistory pages by (played_at DESC, id DESC). Video is present only when accessible to the audience; invalid cursors return ErrInvalidCursor.
	ListWatchHistory(
		ctx context.Context, audience domain.Audience, query domain.WatchHistoryQuery, cursor string, limit int,
	) (domain.WatchHistoryPage, error)
	// ListWatchHistoryDays returns matching YYYY-MM-DD dates in loc, newest first, ignoring query.Before.
	ListWatchHistoryDays(
		ctx context.Context, audience domain.Audience, query domain.WatchHistoryQuery, loc *time.Location,
	) ([]string, error)
	// DeleteWatchHistoryEntry reports whether the entry existed.
	DeleteWatchHistoryEntry(ctx context.Context, id int64) (bool, error)
	ClearWatchHistory(ctx context.Context) error
}

// Keep page-size limits aligned with OpenAPI.
const (
	defaultWatchHistoryLimit = 60
	maxWatchHistoryLimit     = 200
)

// Apply history filters before pagination; each video uses the list shape with progress, tags, favorites, and folder metadata.
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
	query, ok := s.parseWatchHistoryQuery(w, params.Watch, params.Query)
	if !ok {
		return
	}
	if params.Date != nil {
		period, err := domain.ParseWatchHistoryPeriod(*params.Date)
		if err != nil {
			s.invalidRequest(w, "date must be YYYY-MM-DD or YYYY-MM.")
			return
		}
		if params.Tz == nil {
			s.invalidRequest(w, "tz is required with date.")
			return
		}
		loc, ok := s.parseTimeZone(w, *params.Tz)
		if !ok {
			return
		}
		query.Before = period.End(loc)
	} else if params.Tz != nil {
		// Validate tz even without date, although it is unused in that case.
		if _, ok := s.parseTimeZone(w, *params.Tz); !ok {
			return
		}
	}

	audience := audienceFrom(r.Context())
	page, err := s.watchHistory.ListWatchHistory(r.Context(), audience, query, cursor, limit)
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
	var roots []domain.MediaFolder
	if len(videos) > 0 {
		roots = s.registeredRoots(r.Context())
	}

	out := gen.WatchHistoryPage{Items: make([]gen.WatchHistoryEntry, 0, len(page.Items))}
	next := 0
	for _, entry := range page.Items {
		item := gen.WatchHistoryEntry{Id: entry.ID, PlayedAt: entry.PlayedAt, Title: entry.Title}
		if entry.Video != nil {
			view := views[next]
			next++
			video := withTags(withProgress(toAPIVideo(view), progress, view.Video.UserKey), tags, view.Video.UserKey)
			if folder, ok := domain.LocateVideoFolder(roots, view.Video.Path); ok {
				video.Folder = &gen.VideoFolder{RootId: folder.RootID, Path: folder.Path}
			}
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

// ListWatchHistoryDates returns dates in tz, newest first. Generated routing rejects a missing tz with 400.
func (s *server) ListWatchHistoryDates(w http.ResponseWriter, r *http.Request, params gen.ListWatchHistoryDatesParams) {
	if s.watchHistory == nil {
		s.internalError(w, "Watch history storage is not configured.", nil)
		return
	}
	loc, ok := s.parseTimeZone(w, params.Tz)
	if !ok {
		return
	}
	query, ok := s.parseWatchHistoryQuery(w, params.Watch, params.Query)
	if !ok {
		return
	}
	days, err := s.watchHistory.ListWatchHistoryDays(r.Context(), audienceFrom(r.Context()), query, loc)
	if err != nil {
		s.internalError(w, "Could not load the watch history dates.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.WatchHistoryDates{Days: days}, s.logger)
}

func (s *server) parseWatchHistoryQuery(
	w http.ResponseWriter, watch *gen.WatchHistoryFilter, search *string,
) (domain.WatchHistoryQuery, bool) {
	query := domain.WatchHistoryQuery{Filter: domain.WatchHistoryAll}
	if watch != nil {
		filter, err := domain.ParseWatchHistoryFilter(string(*watch))
		if err != nil {
			s.invalidRequest(w, "Unknown watch filter.")
			return domain.WatchHistoryQuery{}, false
		}
		query.Filter = filter
	}
	text, ok := s.parseSearchQuery(w, search)
	if !ok {
		return domain.WatchHistoryQuery{}, false
	}
	query.Search = domain.ParseSearchQuery(text)
	return query, true
}

// parseTimeZone accepts IANA names, rejecting empty, Local, and unknown zones. cmd/mdm embeds time/tzdata for hosts without a zone database.
func (s *server) parseTimeZone(w http.ResponseWriter, name string) (*time.Location, bool) {
	if name == "" || name == "Local" {
		s.invalidRequest(w, "tz must be an IANA time zone name.")
		return nil, false
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		s.invalidRequest(w, "Unknown time zone.")
		return nil, false
	}
	return loc, true
}

// DeleteWatchHistoryEntry returns 404 when absent. Validate IDs here because generated routing does not enforce minimum: 1.
func (s *server) DeleteWatchHistoryEntry(w http.ResponseWriter, r *http.Request, id gen.WatchHistoryEntryId) {
	if id < 1 {
		s.invalidRequest(w, "Specify a valid watch history entry id.")
		return
	}
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

// ClearWatchHistory returns 204 even for an empty history.
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
