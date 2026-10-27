package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// Group reads allow guests; writes remain owner-only by default.

// GroupStore operations are atomic and write nothing on missing-group or missing-target errors.
type Groups interface {
	CreateGroup(ctx context.Context, name string, add domain.GroupAdd) (domain.Group, error)
	RenameGroup(ctx context.Context, id int64, name string) (domain.Group, error)
	// Adding self or an ancestor returns ErrGroupCycle.
	AddGroupItems(ctx context.Context, id int64, add domain.GroupAdd) (domain.Group, error)
	// Removing the final item dissolves the group and returns dissolved=true.
	RemoveGroupItems(ctx context.Context, id int64, itemIDs []int64) (group domain.Group, dissolved bool, err error)
	// Nil afterItemID moves to the beginning.
	MoveGroupItem(ctx context.Context, id, itemID int64, afterItemID *int64) error
	DissolveGroup(ctx context.Context, id int64) error
	TagGroup(ctx context.Context, id int64) (domain.GroupTag, error)
}

// Use the same body cap as favorites because item limits match.
const groupsBodyLimit = favoritesBodyLimit

func (s *server) ListGroups(w http.ResponseWriter, r *http.Request, params gen.ListGroupsParams) {
	if s.library == nil {
		s.internalError(w, "Group queries are not configured.", nil)
		return
	}
	if params.Node != nil {
		if !params.Node.Valid() {
			s.invalidRequest(w, "node must be local or peer.")
			return
		}
		if *params.Node == gen.Peer {
			s.listPeerGroups(w, r)
			return
		}
	}
	limit := domain.GroupListDefaultLimit
	if params.Limit != nil {
		if *params.Limit < 1 || *params.Limit > domain.GroupListMaxLimit {
			s.invalidRequest(w, "limit must be between 1 and 200.")
			return
		}
		limit = *params.Limit
	}
	query, cursor := "", ""
	if params.Q != nil {
		query = *params.Q
	}
	if params.Cursor != nil {
		cursor = *params.Cursor
	}
	ctx := r.Context()
	audience := audienceFrom(ctx)
	page, err := s.library.ListGroups(ctx, audience, query, cursor, limit)
	switch {
	case errors.Is(err, domain.ErrInvalidCursor):
		s.invalidRequestReason(w, reasonInvalidCursor, "Cannot read the cursor. Reload the list.")
		return
	case err != nil:
		s.internalError(w, "Could not load the groups.", err)
		return
	}
	var tagged []domain.Video
	for _, group := range page.Groups {
		tagged = append(tagged, group.Members...)
	}
	lookup := s.itemLookups(ctx, nil, tagged)
	out := gen.GroupListPage{Groups: make([]gen.LibraryGroup, 0, len(page.Groups))}
	for _, group := range page.Groups {
		if item, ok := lookup.group(ctx, audience, page.Roots, group); ok {
			out.Groups = append(out.Groups, item)
		}
	}
	if page.NextCursor != "" {
		next := page.NextCursor
		out.NextCursor = &next
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, out, s.logger)
}

func (s *server) CreateGroup(w http.ResponseWriter, r *http.Request) {
	var body gen.GroupCreate
	if !s.readLargeJSONBody(w, r, groupsBodyLimit, &body) || !s.createsGroupLocally(w, r, &body) {
		return
	}
	add, ok := s.groupAdd(w, body.VideoIds, body.GroupIds)
	if !ok || !s.groupsConfigured(w) {
		return
	}
	group, err := s.groups.CreateGroup(r.Context(), body.Name, add)
	s.writeGroup(w, r, http.StatusCreated, group, err, "Could not create the group.")
}

// Renames carrying undoToken pass through Organizer so they join the drag operation's undo history.
func (s *server) RenameGroup(w http.ResponseWriter, r *http.Request, id gen.GroupId) {
	var body gen.GroupRename
	if !s.readJSONBody(w, r, &body) || !s.groupsConfigured(w) {
		return
	}
	var group domain.Group
	var err error
	if body.UndoToken != nil && *body.UndoToken != "" && s.organizer != nil {
		group, err = s.organizer.RenameGroup(r.Context(), id, body.Name, *body.UndoToken)
	} else {
		group, err = s.groups.RenameGroup(r.Context(), id, body.Name)
	}
	s.writeGroup(w, r, http.StatusOK, group, err, "Could not rename the group.")
}

func (s *server) DissolveGroup(w http.ResponseWriter, r *http.Request, id gen.GroupId) {
	if !s.groupsConfigured(w) {
		return
	}
	if err := s.groups.DissolveGroup(r.Context(), id); err != nil {
		s.writeGroupError(w, err, "Could not dissolve the group.")
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) AddGroupItems(w http.ResponseWriter, r *http.Request, id gen.GroupId) {
	var body gen.GroupMembers
	if !s.readLargeJSONBody(w, r, groupsBodyLimit, &body) || !s.addsGroupItemsLocally(w, r, id, &body) {
		return
	}
	add, ok := s.groupAdd(w, body.VideoIds, body.GroupIds)
	if !ok || !s.groupsConfigured(w) {
		return
	}
	group, err := s.groups.AddGroupItems(r.Context(), id, add)
	s.writeGroup(w, r, http.StatusOK, group, err, "Could not add to the group.")
}

// Removing the final group item dissolves it and returns a bodyless 204.
func (s *server) RemoveGroupItems(w http.ResponseWriter, r *http.Request, id gen.GroupId) {
	var body gen.GroupItemRemove
	if !s.readLargeJSONBody(w, r, groupsBodyLimit, &body) {
		return
	}
	// OpenAPI maxItems counts the raw array length before deduplication.
	if len(body.ItemIds) > maxVideoTagsIDs {
		s.invalidRequestLimit(w, reasonTooManyVideos, maxVideoTagsIDs,
			fmt.Sprintf("itemIds must contain at most %d items.", maxVideoTagsIDs))
		return
	}
	itemIDs := uniqueInt64s(body.ItemIds)
	if len(itemIDs) == 0 {
		s.invalidRequest(w, "itemIds must contain at least one item.")
		return
	}
	if !s.groupsConfigured(w) {
		return
	}
	group, dissolved, err := s.groups.RemoveGroupItems(r.Context(), id, itemIDs)
	if err == nil && dissolved {
		w.Header().Set("Cache-Control", cacheNoStore)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.writeGroup(w, r, http.StatusOK, group, err, "Could not remove the group items.")
}

func (s *server) MoveGroupItem(w http.ResponseWriter, r *http.Request, id gen.GroupId, itemID gen.GroupItemId) {
	var body gen.GroupItemMove
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if body.After != nil && *body.After == itemID {
		s.invalidRequest(w, "An item cannot be moved after itself.")
		return
	}
	if !s.groupsConfigured(w) {
		return
	}
	if err := s.groups.MoveGroupItem(r.Context(), id, itemID, body.After); err != nil {
		s.writeGroupError(w, err, "Could not move the group item.")
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) TagGroup(w http.ResponseWriter, r *http.Request, id gen.GroupId) {
	if !s.groupsConfigured(w) {
		return
	}
	result, err := s.groups.TagGroup(r.Context(), id)
	if errors.Is(err, domain.ErrInvalidTagName) {
		s.invalidTagName(w, err, "This group name cannot be used as a tag name. ")
		return
	}
	if err != nil {
		s.writeGroupError(w, err, "Could not turn the group into a tag.")
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.GroupTagResult{Tag: toAPITagRef(result.Tag), Created: result.Created}, s.logger)
}

func (s *server) groupsConfigured(w http.ResponseWriter) bool {
	if s.groups == nil {
		s.internalError(w, "Group storage is not configured.", nil)
		return false
	}
	return true
}

func (s *server) groupAdd(w http.ResponseWriter, videoIDs, groupIDs *[]int64) (domain.GroupAdd, bool) {
	var add domain.GroupAdd
	if videoIDs != nil {
		add.VideoIDs = *videoIDs
	}
	if groupIDs != nil {
		add.GroupIDs = *groupIDs
	}
	// OpenAPI maxItems counts the raw array length before deduplication.
	if len(add.VideoIDs) > maxVideoTagsIDs || len(add.GroupIDs) > maxVideoTagsIDs {
		s.invalidRequestLimit(w, reasonTooManyVideos, maxVideoTagsIDs,
			fmt.Sprintf("videoIds and groupIds must each contain at most %d items.", maxVideoTagsIDs))
		return domain.GroupAdd{}, false
	}
	add.VideoIDs, add.GroupIDs = uniqueInt64s(add.VideoIDs), uniqueInt64s(add.GroupIDs)
	if len(add.VideoIDs)+len(add.GroupIDs) == 0 {
		s.invalidRequest(w, "videoIds or groupIds must contain at least one item.")
		return domain.GroupAdd{}, false
	}
	return add, true
}

// Write results return zero-count groups even when they have no visible videos and therefore do not appear in library reads.
func (s *server) writeGroup(w http.ResponseWriter, r *http.Request, status int, group domain.Group, err error, failure string) {
	if err != nil {
		s.writeGroupError(w, err, failure)
		return
	}
	ctx := r.Context()
	audience := audienceFrom(ctx)
	out, err := s.presentWrittenGroup(ctx, audience, group)
	if err != nil {
		s.internalError(w, "Could not load the group.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, status, out, s.logger)
}

func (s *server) presentWrittenGroup(ctx context.Context, audience domain.Audience, group domain.Group) (gen.LibraryGroup, error) {
	if s.library == nil {
		return gen.LibraryGroup{}, errors.New("group queries are not configured")
	}
	page, err := s.library.GroupItems(ctx, audience, group.ID, "", 1, domain.VideoQuery{})
	switch {
	case err == nil:
		lookup := s.itemLookups(ctx, nil, page.Group.Members)
		if out, ok := lookup.group(ctx, audience, page.Roots, page.Group); ok {
			return out, nil
		}
	case !errors.Is(err, domain.ErrNotFound):
		return gen.LibraryGroup{}, err
	}
	return s.emptyGroup(ctx, audience, group)
}

// Empty groups retain favorites; include folder only when the automatic group's path is under registered roots.
func (s *server) emptyGroup(ctx context.Context, audience domain.Audience, group domain.Group) (gen.LibraryGroup, error) {
	out := gen.LibraryGroup{
		Id: group.ID, Name: group.Name, Previews: []gen.FolderPreview{}, VideoIds: []int64{}, Tags: []gen.VideoTag{},
	}
	if group.Automatic && s.folders != nil {
		roots, err := s.folders.ListMediaFolders(ctx)
		if err != nil {
			return gen.LibraryGroup{}, err
		}
		if folder, ok := domain.LocateFolder(roots, group.OriginFolder); ok {
			out.Folder = &gen.VideoFolder{RootId: folder.RootID, Path: folder.Path}
		}
	}
	if audience.IsOwner() {
		state, watched, favorite := gen.LibraryGroupWatchStateUnwatched, 0, group.Favorite
		out.WatchState, out.WatchedCount, out.Favorite = &state, &watched, &favorite
	}
	return out, nil
}

func (s *server) writeGroupError(w http.ResponseWriter, err error, message string) {
	var invalidName *domain.InvalidGroupNameError
	switch {
	case errors.Is(err, domain.ErrGroupTargetNotFound):
		s.notFoundReason(w, reasonGroupTargetNotFound, "A video, group or item in the request no longer exists. Reload and try again.")
	case errors.Is(err, domain.ErrGroupNotFound):
		s.notFound(w, "The group does not exist.")
	case errors.Is(err, domain.ErrGroupCycle):
		s.conflictReason(w, reasonGroupCycle, "A group cannot be put inside itself or one of its own groups.")
	case errors.As(err, &invalidName):
		s.invalidRequest(w, groupNameMessage(invalidName.Problem))
	default:
		s.internalError(w, message, err)
	}
}

func groupNameMessage(problem domain.GroupNameProblem) string {
	switch problem {
	case domain.GroupNameEmpty:
		return "Enter a group name."
	case domain.GroupNameTooLong:
		return fmt.Sprintf("A group name must be at most %d characters.", domain.GroupNameMaxLength)
	default:
		return "Invalid group name."
	}
}
