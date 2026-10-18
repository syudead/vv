package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// Playlist routes remain owner-only by omission from accessRoutes.

type Playlists interface {
	// ListPlaylists orders by newest update then largest ID and populates covers only for the returned page.
	ListPlaylists(ctx context.Context, cursor string, limit int) (domain.PlaylistPage, error)
	CreatePlaylist(ctx context.Context, name string) (domain.Playlist, error)
	RenamePlaylist(ctx context.Context, id int64, name string) (domain.Playlist, error)
	DeletePlaylist(ctx context.Context, id int64) error
	// Playlist returns visible position-ordered items and roots from the same read snapshot.
	Playlist(ctx context.Context, id int64) (domain.Playlist, []domain.PlaylistItem, []domain.MediaFolder, error)
	AddPlaylistItems(ctx context.Context, id int64, add domain.PlaylistAdd) (domain.PlaylistAdded, error)
	MovePlaylistItem(ctx context.Context, id, itemID int64, afterItemID *int64) error
	RemovePlaylistItem(ctx context.Context, id, itemID int64) error
}

// Use the favorites body cap because request shape and item limits match.
const playlistsBodyLimit = favoritesBodyLimit

func (s *server) ListPlaylists(w http.ResponseWriter, r *http.Request, params gen.ListPlaylistsParams) {
	if s.playlists == nil {
		s.internalError(w, "Playlist storage is not configured.", nil)
		return
	}
	limit := domain.DefaultLimit
	if params.Limit != nil {
		if *params.Limit < 1 || *params.Limit > domain.MaxLimit {
			s.invalidRequest(w, "limit must be between 1 and 200.")
			return
		}
		limit = *params.Limit
	}
	cursor := ""
	if params.Cursor != nil {
		cursor = *params.Cursor
	}
	ctx := r.Context()
	page, err := s.playlists.ListPlaylists(ctx, cursor, limit)
	switch {
	case errors.Is(err, domain.ErrInvalidCursor):
		s.invalidRequestReason(w, reasonInvalidCursor, "Cannot read the cursor. Reload the list.")
		return
	case err != nil:
		s.internalError(w, "Could not load the playlists.", err)
		return
	}
	out := gen.PlaylistList{
		Items: make([]gen.PlaylistSummary, 0, len(page.Items)), Total: page.Total, Version: page.Version,
	}
	for _, playlist := range page.Items {
		summary := toAPIPlaylistSummary(playlist)
		covers := s.folderPreviews(ctx, previewVideos(playlist.Covers))
		summary.Covers = &covers
		out.Items = append(out.Items, summary)
	}
	if page.NextCursor != "" {
		nextCursor := page.NextCursor
		out.NextCursor = &nextCursor
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, out, s.logger)
}

func (s *server) CreatePlaylist(w http.ResponseWriter, r *http.Request) {
	var body gen.PlaylistName
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if s.playlists == nil {
		s.internalError(w, "Playlist storage is not configured.", nil)
		return
	}
	playlist, err := s.playlists.CreatePlaylist(r.Context(), body.Name)
	if err != nil {
		s.writePlaylistError(w, err, "Could not create the playlist.")
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusCreated, toAPIPlaylistSummary(playlist), s.logger)
}

func (s *server) RenamePlaylist(w http.ResponseWriter, r *http.Request, id gen.PlaylistId) {
	if !s.validPlaylistID(w, id) {
		return
	}
	var body gen.PlaylistName
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if s.playlists == nil {
		s.internalError(w, "Playlist storage is not configured.", nil)
		return
	}
	playlist, err := s.playlists.RenamePlaylist(r.Context(), id, body.Name)
	if err != nil {
		s.writePlaylistError(w, err, "Could not rename the playlist.")
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, toAPIPlaylistSummary(playlist), s.logger)
}

func (s *server) DeletePlaylist(w http.ResponseWriter, r *http.Request, id gen.PlaylistId) {
	if !s.validPlaylistID(w, id) {
		return
	}
	if s.playlists == nil {
		s.internalError(w, "Playlist storage is not configured.", nil)
		return
	}
	if err := s.playlists.DeletePlaylist(r.Context(), id); err != nil {
		s.writePlaylistError(w, err, "Could not delete the playlist.")
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) GetPlaylist(w http.ResponseWriter, r *http.Request, id gen.PlaylistId) {
	if !s.validPlaylistID(w, id) {
		return
	}
	if s.playlists == nil {
		s.internalError(w, "Playlist storage is not configured.", nil)
		return
	}
	ctx := r.Context()
	playlist, items, roots, err := s.playlists.Playlist(ctx, id)
	if err != nil {
		s.writePlaylistError(w, err, "Could not load the playlist.")
		return
	}

	var shown, tagged []domain.Video
	for _, item := range items {
		switch {
		case item.Video != nil:
			shown = append(shown, *item.Video)
			tagged = append(tagged, *item.Video)
		case item.Group != nil:
			tagged = append(tagged, item.Group.Members...)
		}
	}
	lookup := s.itemLookups(ctx, shown, tagged)
	audience := audienceFrom(ctx)

	out := gen.Playlist{
		Id: playlist.ID, Name: playlist.Name, ItemCount: playlist.ItemCount, UpdatedAt: playlist.UpdatedAt.UTC(),
		Items: make([]gen.PlaylistItem, 0, len(items)),
	}
	for _, item := range items {
		switch {
		case item.Video != nil:
			video := lookup.video(ctx, audience, roots, *item.Video)
			out.Items = append(out.Items, gen.PlaylistItem{Id: item.ID, Kind: gen.PlaylistItemKindVideo, Video: &video})
		case item.Group != nil:
			group, ok := lookup.group(ctx, audience, roots, *item.Group)
			if !ok {
				// Fail instead of dropping empty groups so items stays consistent with itemCount.
				s.internalError(w, "Could not load the playlist.", fmt.Errorf("group %d has no video", item.Group.ID))
				return
			}
			out.Items = append(out.Items, gen.PlaylistItem{Id: item.ID, Kind: gen.PlaylistItemKindGroup, Group: &group})
		case item.SmartFolder != nil:
			// Empty smart folders remain visible playlist items.
			folder := lookup.smartFolder(ctx, audience, *item.SmartFolder)
			out.Items = append(out.Items, gen.PlaylistItem{Id: item.ID, Kind: gen.PlaylistItemKindSmartFolder, SmartFolder: &folder})
		}
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, out, s.logger)
}

func (s *server) AddPlaylistItems(w http.ResponseWriter, r *http.Request, id gen.PlaylistId) {
	if !s.validPlaylistID(w, id) {
		return
	}
	var body gen.PlaylistItemsAdd
	if !s.readLargeJSONBody(w, r, playlistsBodyLimit, &body) {
		return
	}
	var videoIDs []int64
	if body.VideoIds != nil {
		videoIDs = uniqueInt64s(*body.VideoIds)
	}
	var groupIDs []int64
	if body.GroupIds != nil {
		groupIDs = uniqueInt64s(*body.GroupIds)
	}
	var smartFolderIDs []int64
	if body.SmartFolderIds != nil {
		smartFolderIDs = uniqueInt64s(*body.SmartFolderIds)
	}
	total := len(videoIDs) + len(groupIDs) + len(smartFolderIDs)
	if total == 0 {
		s.invalidRequest(w, "videoIds, groupIds or smartFolderIds must contain at least one item.")
		return
	}
	if total > maxVideoTagsIDs {
		s.invalidRequestLimit(w, reasonTooManyVideos, maxVideoTagsIDs,
			fmt.Sprintf("videoIds, groupIds and smartFolderIds must contain at most %d items in total.", maxVideoTagsIDs))
		return
	}
	if s.playlists == nil {
		s.internalError(w, "Playlist storage is not configured.", nil)
		return
	}
	added, err := s.playlists.AddPlaylistItems(r.Context(), id, domain.PlaylistAdd{VideoIDs: videoIDs, GroupIDs: groupIDs, SmartFolderIDs: smartFolderIDs})
	if err != nil {
		s.writePlaylistError(w, err, "Could not add to the playlist.")
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.PlaylistItemsAdded{Added: added.Added, Existing: added.Existing}, s.logger)
}

func (s *server) MovePlaylistItem(w http.ResponseWriter, r *http.Request, id gen.PlaylistId, itemID gen.PlaylistItemId) {
	if !s.validPlaylistID(w, id) || !s.validPlaylistItemID(w, itemID) {
		return
	}
	// Track afterItemId presence so omission cannot silently move an item to the beginning.
	var body struct {
		AfterItemID json.RawMessage `json:"afterItemId"`
	}
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if len(body.AfterItemID) == 0 {
		s.invalidRequest(w, "afterItemId is required.")
		return
	}
	var after *int64
	if err := json.Unmarshal(body.AfterItemID, &after); err != nil {
		s.invalidRequest(w, "afterItemId must be an integer or null.")
		return
	}
	if after != nil && *after == itemID {
		s.invalidRequest(w, "An item cannot be moved after itself.")
		return
	}
	if s.playlists == nil {
		s.internalError(w, "Playlist storage is not configured.", nil)
		return
	}
	if err := s.playlists.MovePlaylistItem(r.Context(), id, itemID, after); err != nil {
		s.writePlaylistError(w, err, "Could not move the playlist item.")
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	w.WriteHeader(http.StatusNoContent)
}

// Removing a playlist item preserves its video/group, favorites, and playback progress.
func (s *server) RemovePlaylistItem(w http.ResponseWriter, r *http.Request, id gen.PlaylistId, itemID gen.PlaylistItemId) {
	if !s.validPlaylistID(w, id) || !s.validPlaylistItemID(w, itemID) {
		return
	}
	if s.playlists == nil {
		s.internalError(w, "Playlist storage is not configured.", nil)
		return
	}
	if err := s.playlists.RemovePlaylistItem(r.Context(), id, itemID); err != nil {
		s.writePlaylistError(w, err, "Could not remove the playlist item.")
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) validPlaylistID(w http.ResponseWriter, id int64) bool {
	if id < 1 {
		s.invalidRequest(w, "Specify a valid playlist id.")
		return false
	}
	return true
}

func (s *server) validPlaylistItemID(w http.ResponseWriter, id int64) bool {
	if id < 1 {
		s.invalidRequest(w, "Specify a valid playlist item id.")
		return false
	}
	return true
}

func (s *server) writePlaylistError(w http.ResponseWriter, err error, message string) {
	var invalidName *domain.InvalidPlaylistNameError
	switch {
	case errors.Is(err, domain.ErrPlaylistNotFound):
		s.notFound(w, "Playlist not found.")
	case errors.Is(err, domain.ErrPlaylistItemNotFound):
		s.notFound(w, "Playlist item not found.")
	case errors.As(err, &invalidName):
		s.invalidRequest(w, playlistNameMessage(invalidName.Problem))
	default:
		s.internalError(w, message, err)
	}
}

func playlistNameMessage(problem domain.PlaylistNameProblem) string {
	switch problem {
	case domain.PlaylistNameEmpty:
		return "Enter a playlist name."
	case domain.PlaylistNameTooLong:
		return fmt.Sprintf("A playlist name must be at most %d characters.", domain.PlaylistNameMaxLength)
	default:
		return "Invalid playlist name."
	}
}
