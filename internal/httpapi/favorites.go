package httpapi

import (
	"context"
	"fmt"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

type Favorites interface {
	SetFavorites(ctx context.Context, change domain.FavoriteChange) (domain.FavoriteApplied, error)
}

// Match the external body cap so maximum-size favorite batches fit.
const favoritesBodyLimit = externalBodyLimit

func (s *server) favoriteBodyLimit() int64 {
	if s.favoritesMaxBody > 0 {
		return s.favoritesMaxBody
	}
	return favoritesBodyLimit
}

// Favorites count unique IDs across all types, ignore missing targets, and emit no SSE
// notification.
func (s *server) UpdateFavorites(w http.ResponseWriter, r *http.Request) {
	// Track required favorite presence so omission cannot silently clear favorites.
	var body struct {
		VideoIds       []int64 `json:"videoIds"`
		GroupIds       []int64 `json:"groupIds"`
		SmartFolderIds []int64 `json:"smartFolderIds"`
		Favorite       *bool   `json:"favorite"`
	}
	if !s.readLargeJSONBody(w, r, s.favoriteBodyLimit(), &body) {
		return
	}
	videoIDs := uniqueInt64s(body.VideoIds)
	groupIDs := uniqueInt64s(body.GroupIds)
	smartFolderIDs := uniqueInt64s(body.SmartFolderIds)
	if total := len(videoIDs) + len(groupIDs) + len(smartFolderIDs); total < 1 || total > maxVideoTagsIDs {
		s.invalidRequestLimit(w, reasonTooManyVideos, maxVideoTagsIDs,
			fmt.Sprintf("videoIds, groupIds and smartFolderIds must contain between 1 and %d items in total.", maxVideoTagsIDs))
		return
	}
	if body.Favorite == nil {
		s.invalidRequest(w, "favorite is required.")
		return
	}
	if s.favorites == nil {
		s.internalError(w, "Favorite storage is not configured.", nil)
		return
	}

	applied, err := s.favorites.SetFavorites(r.Context(), domain.FavoriteChange{
		VideoIDs: videoIDs, GroupIDs: groupIDs, SmartFolderIDs: smartFolderIDs, Favorite: *body.Favorite,
	})
	if err != nil {
		s.internalError(w, "Could not change the favorites.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.FavoritesResponse{
		AppliedVideos: applied.Videos, AppliedGroups: applied.Groups, AppliedSmartFolders: applied.SmartFolders,
	}, s.logger)
}

func uniqueInt64s(values []int64) []int64 {
	seen := make(map[int64]struct{}, len(values))
	out := make([]int64, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
