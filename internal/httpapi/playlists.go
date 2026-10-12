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

// プレイリストの管理（specs/044-playlists/contracts/screen-api.md）。どの経路も /api/* の既定どおり
// 所有者だけで、accessRoutes には入れない（research.md R-8）。

// Playlists はプレイリストの保存先である。internal/store の *PlaylistStore がこれを満たす。
// どの操作も 1 つの取引で済むので、internal/app は通さない（plan.md の Constitution Check）。
type Playlists interface {
	// ListPlaylists はプレイリストを更新日時の新しい順（同じなら id の大きい順）に cursor の次から
	// limit 件返す。Covers は返したページの分だけ埋まる。読めない cursor は domain.ErrInvalidCursor。
	ListPlaylists(ctx context.Context, cursor string, limit int) (domain.PlaylistPage, error)
	// CreatePlaylist は名前 name の空のプレイリストを作る。名前の規則違反は
	// *domain.InvalidPlaylistNameError。
	CreatePlaylist(ctx context.Context, name string) (domain.Playlist, error)
	// RenamePlaylist はプレイリスト id の名前を変える。無ければ domain.ErrPlaylistNotFound。
	RenamePlaylist(ctx context.Context, id int64, name string) (domain.Playlist, error)
	// DeletePlaylist はプレイリスト id とその項目を消す。無ければ domain.ErrPlaylistNotFound。
	DeletePlaylist(ctx context.Context, id int64) error
	// Playlist はプレイリスト id の要約と見せる項目を位置の順に、それらと同じ読み取りスナップショットの
	// 登録フォルダの一覧とともに返す。無ければ domain.ErrPlaylistNotFound。
	Playlist(ctx context.Context, id int64) (domain.Playlist, []domain.PlaylistItem, []domain.MediaFolder, error)
	// AddPlaylistItems は add の動画とグループをプレイリスト id の末尾に足す。
	AddPlaylistItems(ctx context.Context, id int64, add domain.PlaylistAdd) (domain.PlaylistAdded, error)
	// MovePlaylistItem は項目 itemID を項目 afterItemID のすぐ後ろ（nil なら先頭）へ移す。
	MovePlaylistItem(ctx context.Context, id, itemID int64, afterItemID *int64) error
	// RemovePlaylistItem は項目 itemID を外す。
	RemovePlaylistItem(ctx context.Context, id, itemID int64) error
}

// playlistsBodyLimit は POST /api/playlists/{id}/items が読む本文の上限（バイト）である。
// PUT /api/favorites と同じ形と件数の上限なので、同じ大きさにする。
const playlistsBodyLimit = favoritesBodyLimit

// ListPlaylists はプレイリストを更新日時の新しい順に、cursor の次から limit 件返す
// （GET /api/playlists）。limit の既定と上限はライブラリの一覧と同じ（domain.DefaultLimit・
// domain.MaxLimit）で、範囲の外は 400 invalid_request、読めない cursor は 400 invalid_cursor。
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
	out := gen.PlaylistList{Items: make([]gen.PlaylistSummary, 0, len(page.Items)), Total: page.Total}
	for _, playlist := range page.Items {
		summary := toAPIPlaylistSummary(playlist)
		// 一覧のカードの絵柄のサムネイル。previewUrl はグループのカードと同じ folderPreviews を通す。
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

// CreatePlaylist は空のプレイリストを作る（POST /api/playlists）。
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

// RenamePlaylist はプレイリストの名前を変える（PATCH /api/playlists/{id}）。
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

// DeletePlaylist はプレイリストを消す（DELETE /api/playlists/{id}）。
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

// GetPlaylist はプレイリスト 1 つと見せる項目を返す（GET /api/playlists/{id}）。動画とグループは
// ライブラリの項目と同じ形で、所有者の再生位置・視聴の値・お気に入り・タグを載せる。
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
				// 同じスナップショットの登録フォルダの下に無いグループは索引の不整合である。
				// 黙って落とすと items が itemCount と食い違うので、要求を失敗させる（一覧と同じ）。
				s.internalError(w, "Could not load the playlist.",
					fmt.Errorf("group folder is not under any media folder: %s", item.Group.Path))
				return
			}
			out.Items = append(out.Items, gen.PlaylistItem{Id: item.ID, Kind: gen.PlaylistItemKindGroup, Group: &group})
		}
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, out, s.logger)
}

// AddPlaylistItems はプレイリストの末尾に動画とグループを足す（POST /api/playlists/{id}/items、
// research.md R-5）。本文の形と件数の上限は PUT /api/favorites と同じで、合計が 0 なら
// invalid_request、20000 を超えれば too_many_videos にする。
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
	var folders []gen.VideoFolder
	if body.Folders != nil {
		folders = uniqueVideoFolders(*body.Folders)
	}
	total := len(videoIDs) + len(folders)
	if total == 0 {
		s.invalidRequest(w, "videoIds or folders must contain at least one item.")
		return
	}
	if total > maxVideoTagsIDs {
		s.invalidRequestLimit(w, reasonTooManyVideos, maxVideoTagsIDs,
			fmt.Sprintf("videoIds and folders must contain at most %d items in total.", maxVideoTagsIDs))
		return
	}
	for _, folder := range folders {
		if err := domain.ValidateFolderPath(folder.Path); err != nil {
			s.invalidRequestReason(w, reasonInvalidFolderPath,
				"Invalid folder path. Empty segments, leading or trailing /, ., and .. are not allowed.")
			return
		}
	}
	if s.playlists == nil {
		s.internalError(w, "Playlist storage is not configured.", nil)
		return
	}
	paths, ok := s.favoriteFolderPaths(w, r, folders)
	if !ok {
		return
	}
	added, err := s.playlists.AddPlaylistItems(r.Context(), id, domain.PlaylistAdd{VideoIDs: videoIDs, FolderPaths: paths})
	if err != nil {
		s.writePlaylistError(w, err, "Could not add to the playlist.")
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.PlaylistItemsAdded{Added: added.Added, Existing: added.Existing}, s.logger)
}

// MovePlaylistItem はプレイリストの項目 1 つを afterItemId のすぐ後ろへ移す
// （POST /api/playlists/{id}/items/{itemId}/move、research.md R-3）。null なら先頭へ移す。
func (s *server) MovePlaylistItem(w http.ResponseWriter, r *http.Request, id gen.PlaylistId, itemID gen.PlaylistItemId) {
	if !s.validPlaylistID(w, id) || !s.validPlaylistItemID(w, itemID) {
		return
	}
	// gen.PlaylistItemMove の afterItemId はポインタなので、欠けても null に読めてしまう。
	// 欠けた本文で先頭へ移さないよう、有無を区別して読む。
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

// RemovePlaylistItem はプレイリストから項目を 1 つ外す（DELETE /api/playlists/{id}/items/{itemId}）。
// 動画・グループ、そのお気に入りと再生位置は変えない。
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

// writePlaylistError は保存層の誤りを応答にする。無いプレイリストと項目は 404 not_found、
// 名前の規則違反は 400 invalid_request、それ以外は message の 500 である。
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

// playlistNameMessage は名前の規則違反の種類を、画面に出してよい英語の文にする。
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
