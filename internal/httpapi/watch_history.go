package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// 視聴履歴の一覧と削除（specs/043-watch-history/contracts/screen-api.md）。3 つの経路は /api/* の
// 既定どおり所有者だけで、accessRoutes には入れない（research.md R-7）。

// WatchHistory は視聴履歴の読みと削除の先である。internal/store の *PlaybackStore がこれを満たす。
// どの操作も 1 つの文で済むので、internal/app は通さない。
type WatchHistory interface {
	// ListWatchHistory は query を満たす件を (played_at desc, id desc) の順に cursor の次から limit 件
	// 返す。各件の Video は audience が開ける動画があるときだけ入る。読めない cursor は
	// domain.ErrInvalidCursor。
	ListWatchHistory(
		ctx context.Context, audience domain.Audience, query domain.WatchHistoryQuery, cursor string, limit int,
	) (domain.WatchHistoryPage, error)
	// ListWatchHistoryDays は query の絞り込みと検索を満たす件のある日を、loc の暦の YYYY-MM-DD で
	// 新しい順に返す。query.Before は見ない。
	ListWatchHistoryDays(
		ctx context.Context, audience domain.Audience, query domain.WatchHistoryQuery, loc *time.Location,
	) ([]string, error)
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

// ListWatchHistory は視聴履歴を新しい順に返す（GET /api/watch-history）。watch・query・date は
// ページを切る前に保存層が掛ける（research.md R-8）。各件の video は一覧の
// 項目と同じ形で、再生位置・タグ・お気に入りと置かれたフォルダ（Video.folder）を載せる。
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
		// date の無い tz は使わないが、読めない値は date があるときと同じく誤りにする。
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

// ListWatchHistoryDates は絞り込みと検索を満たす件のある日を、tz の暦で新しい順に返す
// （GET /api/watch-history/dates、research.md R-11）。tz の欠けは生成された経路の読み取りが 400 にする。
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

// parseWatchHistoryQuery は一覧と日付の一覧が共通に受ける watch と query を検査して条件にする。
// 誤りなら 400 を書いて false を返す。query の長さの上限と書き方は動画の一覧と同じ
// （parseSearchQuery、domain.ParseSearchQuery）。
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

// parseTimeZone は IANA のタイムゾーン名を読む。空と Local（サーバーの地域）は名前ではないので
// 受け付けない。サーバーが知らない名前と合わせて 400 を書いて false を返す。cmd/mdm が time/tzdata を
// 埋め込むので、地域の情報を持たないホストでも読める（research.md R-11）。
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

// DeleteWatchHistoryEntry は視聴履歴を 1 件消す（DELETE /api/watch-history/{id}）。無ければ
// 404 not_found で、画面は一覧を読み直す（research.md R-6）。生成された経路の読み取りは
// minimum: 1 を確かめないので、1 未満の id はここで 400 にする。
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
