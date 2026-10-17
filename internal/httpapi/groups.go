package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// グループの一覧と編集。一覧
// （GET /api/groups）は所有者とゲストが読め（accessRoutes）、書き込みは /api/* の既定どおり所有者
// だけである。

// Groups はグループの編集の保存先である。internal/store の *GroupStore がこれを満たす。どの操作も
// 1 つの取引で済むので、internal/app は通さない。グループが無ければ
// domain.ErrGroupNotFound、要求が名指した動画・グループ・項目が無ければ
// domain.ErrGroupTargetNotFound で、どちらも何も書かない。
type Groups interface {
	// CreateGroup は名前 name の手で作るグループを作り、add を項目にする。名前の規則違反は
	// *domain.InvalidGroupNameError。
	CreateGroup(ctx context.Context, name string, add domain.GroupAdd) (domain.Group, error)
	// RenameGroup はグループ id の名前を変える。名前の規則違反は *domain.InvalidGroupNameError。
	RenameGroup(ctx context.Context, id int64, name string) (domain.Group, error)
	// AddGroupItems は add をグループ id の末尾に足す。add のグループが id 自身か id の祖先なら
	// domain.ErrGroupCycle。
	AddGroupItems(ctx context.Context, id int64, add domain.GroupAdd) (domain.Group, error)
	// RemoveGroupItems はグループ id の項目 itemIDs を外す。項目が 1 つも残らなければグループを
	// 解散し、dissolved に true を返す。
	RemoveGroupItems(ctx context.Context, id int64, itemIDs []int64) (group domain.Group, dissolved bool, err error)
	// MoveGroupItem は項目 itemID を項目 afterItemID のすぐ後ろ（nil なら先頭）へ移す。
	MoveGroupItem(ctx context.Context, id, itemID int64, afterItemID *int64) error
	// DissolveGroup はグループ id を解散する。
	DissolveGroup(ctx context.Context, id int64) error
	// TagGroup はグループ id をタグにして解散する。名前がタグ名の規則に合わなければ
	// domain.ErrInvalidTagName。
	TagGroup(ctx context.Context, id int64) (domain.GroupTag, error)
}

// groupsBodyLimit はグループの作成・追加・項目の除去が読む本文の上限（バイト）である。
// PUT /api/favorites と同じ件数の上限なので、同じ大きさにする。
const groupsBodyLimit = favoritesBodyLimit

// ListGroups は見る人に見せるグループを名前の順に 1 ページ返す（GET /api/groups）。「Add to group」
// のピッカーが読む。
func (s *server) ListGroups(w http.ResponseWriter, r *http.Request, params gen.ListGroupsParams) {
	if s.library == nil {
		s.internalError(w, "Group queries are not configured.", nil)
		return
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

// CreateGroup は動画とグループからグループを作る（POST /api/groups）。
func (s *server) CreateGroup(w http.ResponseWriter, r *http.Request) {
	var body gen.GroupCreate
	if !s.readLargeJSONBody(w, r, groupsBodyLimit, &body) {
		return
	}
	add, ok := s.groupAdd(w, body.VideoIds, body.GroupIds)
	if !ok || !s.groupsConfigured(w) {
		return
	}
	group, err := s.groups.CreateGroup(r.Context(), body.Name, add)
	s.writeGroup(w, r, http.StatusCreated, group, err, "Could not create the group.")
}

// RenameGroup はグループの名前を変える（PATCH /api/groups/{id}）。undoToken があり、ドラッグの操作の
// 先があれば、名前の変更をそのトークンの取り消しに含めるため internal/app の Organizer を通す。応答と誤りはどちらでも同じである。
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

// DissolveGroup はグループを解散する（DELETE /api/groups/{id}）。
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

// AddGroupItems はグループの末尾に動画とグループを足す（POST /api/groups/{id}/items）。
func (s *server) AddGroupItems(w http.ResponseWriter, r *http.Request, id gen.GroupId) {
	var body gen.GroupMembers
	if !s.readLargeJSONBody(w, r, groupsBodyLimit, &body) {
		return
	}
	add, ok := s.groupAdd(w, body.VideoIds, body.GroupIds)
	if !ok || !s.groupsConfigured(w) {
		return
	}
	group, err := s.groups.AddGroupItems(r.Context(), id, add)
	s.writeGroup(w, r, http.StatusOK, group, err, "Could not add to the group.")
}

// RemoveGroupItems はグループから項目を外す（POST /api/groups/{id}/items/remove）。最後の項目を外して
// 空になったグループは解散されるので、本文の無い 204 を返す。
func (s *server) RemoveGroupItems(w http.ResponseWriter, r *http.Request, id gen.GroupId) {
	var body gen.GroupItemRemove
	if !s.readLargeJSONBody(w, r, groupsBodyLimit, &body) {
		return
	}
	// 上限は契約の maxItems どおり、重複を除く前の配列の長さで数える。
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

// MoveGroupItem はグループの項目 1 つを after の項目のすぐ後ろへ移す。after が無ければ先頭へ移す
// （POST /api/groups/{id}/items/{itemId}/move）。
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

// TagGroup はグループをタグに変える（POST /api/groups/{id}/tag）。
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

// groupsConfigured は保存先が無ければ 500 を書いて false を返す。
func (s *server) groupsConfigured(w http.ResponseWriter) bool {
	if s.groups == nil {
		s.internalError(w, "Group storage is not configured.", nil)
		return false
	}
	return true
}

// groupAdd は作成と追加の本文の動画とグループを、重複を除いて保存先の入力にする。両方が空なら
// 400 invalid_request、どちらかが 20000 件を超えれば 400 too_many_videos を書いて false を返す。
func (s *server) groupAdd(w http.ResponseWriter, videoIDs, groupIDs *[]int64) (domain.GroupAdd, bool) {
	var add domain.GroupAdd
	if videoIDs != nil {
		add.VideoIDs = *videoIDs
	}
	if groupIDs != nil {
		add.GroupIDs = *groupIDs
	}
	// 上限は契約の maxItems どおり、重複を除く前の配列の長さで数える。
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

// writeGroup は書き込んだあとのグループを LibraryGroup にして返す。err があれば writeGroupError で
// 応答にし、failure はその 500 の文である。値は GET /api/groups/{id} と
// 同じく子孫の動画から作る。見せる動画の無いグループ（子グループの解散で項目を失った親など）は、
// ライブラリには出ないが書き込みの結果としては返すので、本数 0 の値にする。
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

// presentWrittenGroup は writeGroup の応答の値を作る。
func (s *server) presentWrittenGroup(ctx context.Context, audience domain.Audience, group domain.Group) (gen.LibraryGroup, error) {
	if s.library == nil {
		return gen.LibraryGroup{}, errors.New("group queries are not configured")
	}
	page, err := s.library.GroupItems(ctx, audience, group.ID, "", 1)
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

// emptyGroup は見せる動画の無いグループの値である。folder は自動グループのフォルダが登録フォルダの
// 下にあるときだけ載せる（ほかのグループの値と同じ）。お気に入りは動画が無くても残るので、保存層の
// 値を載せる。
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

// writeGroupError は保存層の誤りを応答にする。要求が名指した
// 対象が無ければ 404 group_target_not_found、グループが無ければ 404 not_found、入れ子の循環は
// 409 group_cycle、名前の規則違反は 400 invalid_request、それ以外は message の 500 である。
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

// groupNameMessage は名前の規則違反の種類を、画面に出してよい英語の文にする。
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
