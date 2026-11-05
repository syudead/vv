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
		VideoIds        []int64 `json:"videoIds"`
		GroupIds        []int64 `json:"groupIds"`
		SmartFolderIds  []int64 `json:"smartFolderIds"`
		PersonIds       []int64 `json:"personIds"`
		OrganizationIds []int64 `json:"organizationIds"`
		Favorite        *bool   `json:"favorite"`
	}
	if !s.readLargeJSONBody(w, r, s.favoriteBodyLimit(), &body) {
		return
	}
	videoIDs := uniqueInt64s(body.VideoIds)
	groupIDs := uniqueInt64s(body.GroupIds)
	smartFolderIDs := uniqueInt64s(body.SmartFolderIds)
	personIDs := uniqueInt64s(body.PersonIds)
	organizationIDs := uniqueInt64s(body.OrganizationIds)
	total := len(videoIDs) + len(groupIDs) + len(smartFolderIDs) + len(personIDs) + len(organizationIDs)
	if total < 1 || total > maxVideoTagsIDs {
		s.invalidRequestLimit(w, reasonTooManyVideos, maxVideoTagsIDs, fmt.Sprintf(
			"videoIds, groupIds, smartFolderIds, personIds and organizationIds must contain between 1 and %d items in total.",
			maxVideoTagsIDs))
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

	change := domain.FavoriteChange{
		VideoIDs: videoIDs, GroupIDs: groupIDs, SmartFolderIDs: smartFolderIDs, PersonIDs: personIDs,
		OrganizationIDs: organizationIDs, Favorite: *body.Favorite,
	}
	// The peer's own request names this node's entities only; a remote id there is missing and ignored.
	local, links := change, map[int64]domain.FavoriteChange{}
	if !isPeerRequest(r.Context()) {
		local, links = splitFavoriteChange(change)
	}
	peerApplied, ok := s.setLinkFavorites(w, r, links)
	if !ok {
		return
	}
	var applied domain.FavoriteApplied
	if favoriteChangeNames(local) {
		var err error
		if applied, err = s.favorites.SetFavorites(r.Context(), local); err != nil {
			s.internalError(w, "Could not change the favorites.", err)
			return
		}
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.FavoritesResponse{
		AppliedVideos:       applied.Videos + peerApplied.AppliedVideos,
		AppliedGroups:       applied.Groups + peerApplied.AppliedGroups,
		AppliedSmartFolders: applied.SmartFolders + peerApplied.AppliedSmartFolders,
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

// splitFavoriteChange separates change into this node's part and each link's part in that link's own ids.
func splitFavoriteChange(change domain.FavoriteChange) (domain.FavoriteChange, map[int64]domain.FavoriteChange) {
	local := domain.FavoriteChange{Favorite: change.Favorite}
	links := map[int64]domain.FavoriteChange{}
	for _, field := range []struct {
		ids  []int64
		pick func(*domain.FavoriteChange) *[]int64
	}{
		{change.VideoIDs, func(c *domain.FavoriteChange) *[]int64 { return &c.VideoIDs }},
		{change.GroupIDs, func(c *domain.FavoriteChange) *[]int64 { return &c.GroupIDs }},
		{change.SmartFolderIDs, func(c *domain.FavoriteChange) *[]int64 { return &c.SmartFolderIDs }},
		{change.PersonIDs, func(c *domain.FavoriteChange) *[]int64 { return &c.PersonIDs }},
		{change.OrganizationIDs, func(c *domain.FavoriteChange) *[]int64 { return &c.OrganizationIDs }},
	} {
		own, bySlot := domain.SlotIDs(field.ids)
		*field.pick(&local) = own
		for slot, ids := range bySlot {
			part := links[slot]
			part.Favorite = change.Favorite
			*field.pick(&part) = ids
			links[slot] = part
		}
	}
	return local, links
}

// setLinkFavorites applies each link's part in slot order before this node's part, once every link it names is
// connected, so the common failure, a link that is down, changes nothing on any node; the answer sums the links'
// counts. When a link fails, it answers w and reports false.
func (s *server) setLinkFavorites(
	w http.ResponseWriter, r *http.Request, links map[int64]domain.FavoriteChange,
) (gen.FavoritesResponse, bool) {
	var sum gen.FavoritesResponse
	slots := sortedSlots(links)
	connected, err := s.linksConnected(r.Context(), slots)
	switch {
	case err != nil:
		s.internalError(w, "Could not read the pairing.", err)
		return sum, false
	case !connected:
		s.peerUnreachable(w)
		return sum, false
	}
	for _, slot := range slots {
		applied, ok := s.setPeerFavorites(w, r, slot, links[slot])
		if !ok {
			return gen.FavoritesResponse{}, false
		}
		sum.AppliedVideos += applied.AppliedVideos
		sum.AppliedGroups += applied.AppliedGroups
		sum.AppliedSmartFolders += applied.AppliedSmartFolders
	}
	return sum, true
}
