package httpapi

import (
	"context"
	"fmt"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// Collections removes groups and smart folders atomically; missing IDs are ignored and videos are never deleted.
type Collections interface {
	DeleteCollections(ctx context.Context, del domain.CollectionDelete) (domain.CollectionsDeleted, error)
}

// DeleteCollections counts unique IDs across both arrays against the favorites limit.
func (s *server) DeleteCollections(w http.ResponseWriter, r *http.Request) {
	var body struct {
		GroupIds       []int64 `json:"groupIds"`
		SmartFolderIds []int64 `json:"smartFolderIds"`
	}
	if !s.readLargeJSONBody(w, r, favoritesBodyLimit, &body) {
		return
	}
	groupIDs := uniqueInt64s(body.GroupIds)
	smartFolderIDs := uniqueInt64s(body.SmartFolderIds)
	if total := len(groupIDs) + len(smartFolderIDs); total < 1 || total > maxVideoTagsIDs {
		s.invalidRequestLimit(w, reasonTooManyVideos, maxVideoTagsIDs,
			fmt.Sprintf("groupIds and smartFolderIds must contain between 1 and %d items in total.", maxVideoTagsIDs))
		return
	}
	if s.collections == nil {
		s.internalError(w, "Collection storage is not configured.", nil)
		return
	}
	deleted, err := s.collections.DeleteCollections(r.Context(), domain.CollectionDelete{
		GroupIDs: groupIDs, SmartFolderIDs: smartFolderIDs,
	})
	if err != nil {
		s.internalError(w, "Could not delete the collections.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.CollectionsDeleteResponse{
		DissolvedGroups: deleted.DissolvedGroups, DeletedSmartFolders: deleted.DeletedSmartFolders,
	}, s.logger)
}
