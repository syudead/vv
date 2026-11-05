package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// A remote video's playback position, history and favorite live on the node holding it; this node forwards them
// and merges the peer's history into its own (docs/design-docs/peer-library.md#progress-history-and-favorites).

const watchHistoryPath = "/api/watch-history"

// writePeerWriteFailure answers a write the peer did not apply, or may have applied: the peer's own error when it
// refused with one, else 503 peer_unreachable.
func (s *server) writePeerWriteFailure(w http.ResponseWriter, err error) {
	var refusal *peerRefusal
	if errors.As(err, &refusal) && refusal.problem != nil {
		s.writeErrorBody(w, refusal.status, *refusal.problem)
		return
	}
	s.logger.Info("the other VVMDM did not apply the change", slog.Any("error", err))
	s.peerUnreachable(w)
}

// favoriteChangeNames reports whether change names any entity.
func favoriteChangeNames(change domain.FavoriteChange) bool {
	return len(change.VideoIDs)+len(change.GroupIDs)+len(change.SmartFolderIDs)+len(change.PersonIDs)+
		len(change.OrganizationIDs) > 0
}

// setPeerFavorites applies change, in the ids of the link in slot, on that link in one request; when it fails, it
// answers w and reports false.
func (s *server) setPeerFavorites(
	w http.ResponseWriter, r *http.Request, slot int64, change domain.FavoriteChange,
) (gen.FavoritesResponse, bool) {
	var applied gen.FavoritesResponse
	if !favoriteChangeNames(change) {
		return applied, true
	}
	body := gen.FavoritesRequest{
		Favorite: change.Favorite, VideoIds: optionalIDList(change.VideoIDs), GroupIds: optionalIDList(change.GroupIDs),
		SmartFolderIds: optionalIDList(change.SmartFolderIDs), PersonIds: optionalIDList(change.PersonIDs),
		OrganizationIds: optionalIDList(change.OrganizationIDs),
	}
	if err := s.peerCall(r, slot, http.MethodPut, "/api/favorites", nil, body, &applied); err != nil {
		s.writePeerWriteFailure(w, err)
		return gen.FavoritesResponse{}, false
	}
	return applied, true
}

func optionalIDList(ids []int64) *[]int64 {
	if len(ids) == 0 {
		return nil
	}
	return &ids
}

// listMergedWatchHistory answers the history of this node and every link newest first with one composite cursor.
// A link that is not connected on the first page, or fails, is left out for the rest of the listing, as a merged
// video list leaves it out.
func (s *server) listMergedWatchHistory(
	w http.ResponseWriter, r *http.Request, query domain.WatchHistoryQuery, cursorText string, limit int,
) {
	ctx := r.Context()
	cursor, err := domain.ParseMergedCursor(cursorText)
	if !s.checkWatchHistoryRead(w, err) {
		return
	}
	cursor = cursor.Begin(s.linkReaches(ctx))
	join, err := s.vocabularyJoin(ctx, 0)
	if err != nil {
		s.internalError(w, "Could not read the names of tags, people and organizations.", err)
		return
	}
	var local domain.MergeSide[gen.WatchHistoryEntry]
	if side := cursor.Side(0); !side.Done {
		page, err := s.watchHistory.ListWatchHistory(ctx, audienceFrom(ctx), query, side.Cursor, limit)
		if !s.checkWatchHistoryRead(w, err) {
			return
		}
		local = domain.MergeSide[gen.WatchHistoryEntry]{
			Items: s.watchHistoryEntries(ctx, page.Items), Cursors: watchHistoryCursors(page.Items), Next: page.NextCursor,
		}
	}
	parts, cursor := readLinkParts(s, watchHistoryPath, cursor,
		func(slot int64, side domain.SideCursor) (listPart[gen.WatchHistoryEntry], error) {
			if side.Done {
				return listPart[gen.WatchHistoryEntry]{}, nil
			}
			page, err := s.peerWatchHistory(r, join.InSlot(slot), side.Cursor, limit)
			return listPart[gen.WatchHistoryEntry]{side: page}, err
		})
	sides := []domain.MergeSide[gen.WatchHistoryEntry]{local}
	for _, part := range parts {
		sides = append(sides, part.side)
	}
	items, next, more := domain.MergeWatchHistoryPages(cursor, sides, limit)
	out := gen.WatchHistoryPage{Items: items}
	if more {
		nextCursor := next.Encode()
		out.NextCursor = &nextCursor
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, out, s.logger)
}

// peerWatchHistory reads the page of join's link after cursor with the request's filters, in this node's ids.
func (s *server) peerWatchHistory(
	r *http.Request, join domain.VocabularyJoin, cursor string, limit int,
) (domain.MergeSide[gen.WatchHistoryEntry], error) {
	query := r.URL.Query()
	query.Set("limit", strconv.Itoa(limit))
	query.Del("cursor")
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	var answer gen.WatchHistoryPage
	if err := s.peerGet(r, join.Slot, watchHistoryPath, query, &answer); err != nil {
		return domain.MergeSide[gen.WatchHistoryEntry]{}, err
	}
	for i := range answer.Items {
		entry := &answer.Items[i]
		entry.Id = join.Remote(entry.Id)
		if entry.Video != nil {
			rewriteRemoteVideo(entry.Video, join)
		}
	}
	return peerSide(answer.Items, answer.PeerItems, answer.NextCursor)
}

// withPeerWatchHistoryDays adds every connected link's days to days; a link that fails is left out.
func (s *server) withPeerWatchHistoryDays(r *http.Request, days []string) []string {
	nodes := [][]string{days}
	for _, answer := range linkAnswers[gen.WatchHistoryDates](s, r, watchHistoryPath+"/dates", r.URL.Query()) {
		nodes = append(nodes, answer.value.Days)
	}
	return domain.MergeWatchHistoryDays(nodes...)
}

// deletePeerWatchHistoryEntry deletes the peer's entry named by the remote id; the peer's own request with a remote
// id names this node's entry on the peer, which is not sent back.
func (s *server) deletePeerWatchHistoryEntry(w http.ResponseWriter, r *http.Request, id int64) {
	if isPeerRequest(r.Context()) {
		s.notFound(w, "Watch history entry not found.")
		return
	}
	path := watchHistoryPath + "/" + strconv.FormatInt(domain.FromRemoteID(id), 10)
	if err := s.peerCall(r, domain.RemoteSlot(id), http.MethodDelete, path, nil, nil, nil); err != nil {
		s.writePeerWriteFailure(w, err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	w.WriteHeader(http.StatusNoContent)
}
