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
	local := domain.CollectionDelete{GroupIDs: groupIDs, SmartFolderIDs: smartFolderIDs}
	var remote domain.CollectionDelete
	if !isPeerRequest(r.Context()) {
		local.GroupIDs, remote.GroupIDs = domain.SplitRemoteIDs(groupIDs)
		local.SmartFolderIDs, remote.SmartFolderIDs = domain.SplitRemoteIDs(smartFolderIDs)
	}
	// Two databases share no transaction, so the local part commits first and stays when the peer fails.
	var deleted domain.CollectionsDeleted
	if len(local.GroupIDs)+len(local.SmartFolderIDs) > 0 {
		var err error
		if deleted, err = s.collections.DeleteCollections(r.Context(), local); err != nil {
			s.internalError(w, "Could not delete the collections.", err)
			return
		}
	}
	var out gen.CollectionsDeleteResponse
	if len(remote.GroupIDs)+len(remote.SmartFolderIDs) > 0 {
		var peer domain.CollectionsDeleted
		peer, out.Remote = s.deletePeerCollections(r, remote)
		deleted.DissolvedGroups += peer.DissolvedGroups
		deleted.DeletedSmartFolders += peer.DeletedSmartFolders
	}
	out.DissolvedGroups, out.DeletedSmartFolders = deleted.DissolvedGroups, deleted.DeletedSmartFolders
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, out, s.logger)
}
