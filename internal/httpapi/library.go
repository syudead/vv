package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// LibraryItems はライブラリの項目（動画とグループ）の問い合わせ先である
// （specs/017-folder-groups/contracts/library-api.md）。internal/store の
// *LibraryStore がこれを満たす。
type LibraryItems interface {
	// ListLibrary は見る人に見せる項目1ページを返す（GET /api/library）。
	ListLibrary(ctx context.Context, audience domain.Audience, q domain.VideoQuery) (domain.LibraryPage, error)
	// LibraryIDs は ListLibrary と同じ条件（並び順・カーソル・件数を除く）に合う項目の、
	// 動画の項目の id と、グループの項目ごとのグループの id と再生順の動画の id を返す
	// （GET /api/library/ids、所有者だけ）。
	LibraryIDs(ctx context.Context, q domain.VideoQuery) (selection domain.LibrarySelection, missingTagIDs []int64, err error)
	// Group は見る人に見せるグループ id と、その道筋と項目の最初のページを返す
	// （GET /api/groups/{id}）。無いか見せないときは domain.ErrNotFound である。
	Group(ctx context.Context, audience domain.Audience, id int64, trail []int64) (domain.GroupPage, error)
	// GroupItems はグループ id の項目の続きのページを返す（GET /api/groups/{id}/items）。
	// 読めないカーソルは domain.ErrInvalidCursor である。
	GroupItems(ctx context.Context, audience domain.Audience, id int64, cursor string, limit int) (domain.GroupPage, error)
	// ListGroups は見る人に見せるグループを、どの深さのものも自然な名前の順に、名前が query を
	// 大文字小文字を区別せずに含むものだけ 1 ページ返す（GET /api/groups）。読めないカーソルは
	// domain.ErrInvalidCursor である。
	ListGroups(ctx context.Context, audience domain.Audience, query, cursor string, limit int) (domain.GroupListPage, error)
}

// ListLibrary はライブラリの項目の一覧を返す（GET /api/library）。パラメータの検査と
// 誤りは listVideos と同じである。
func (s *server) ListLibrary(w http.ResponseWriter, r *http.Request, params gen.ListLibraryParams) {
	if s.library == nil {
		s.internalError(w, "Library queries are not configured.", nil)
		return
	}
	audience := audienceFrom(r.Context())
	query, ok := s.parseVideoQuery(w, audience, videoQueryParams{
		query: params.Query, watch: params.Watch, playable: params.Playable, sort: params.Sort,
		seed: params.Seed, cursor: params.Cursor, limit: params.Limit, tag: params.Tag,
		favorite: params.Favorite, person: params.Person, organization: params.Organization,
		addedWithin: params.AddedWithin, minDuration: params.MinDuration, maxDuration: params.MaxDuration,
		tagKind: params.TagKind,
	})
	if !ok {
		return
	}
	// 棚に無い動画のみはライブラリの項目の一覧だけが受け、ゲストも使える（specs/057-home/research.md R-11）。
	query.Unshelved = params.Unshelved != nil && *params.Unshelved

	page, err := s.library.ListLibrary(r.Context(), audience, query)
	switch {
	case errors.Is(err, domain.ErrInvalidCursor):
		s.invalidRequestReason(w, reasonInvalidCursor, "Cannot read the cursor. Reload the list.")
		return
	case err != nil:
		s.internalError(w, "Could not load the list.", err)
		return
	}

	// 登録フォルダは項目と同じスナップショットから読んだものを使う。別に読むと、
	// 読み取りの失敗や間に確定した登録フォルダの解除で、total とカーソルに数えた
	// グループを応答から落としてしまう。
	items, err := s.presentLibraryItems(r.Context(), audience, page.Roots, page.Items)
	if err != nil {
		s.internalError(w, "Could not load the list.", err)
		return
	}
	payload := gen.LibraryPage{Items: items, Total: page.Total}
	if page.NextCursor != "" {
		next := page.NextCursor
		payload.NextCursor = &next
	}
	if len(page.MissingTagIDs) > 0 {
		missing := page.MissingTagIDs
		payload.MissingTagIds = &missing
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, payload, s.logger)
}

// ListLibraryIds は絞り込みに合う項目の動画の id を返す（GET /api/library/ids、
// 「すべて選択」用。specs/017-folder-groups/contracts/library-api.md §2）。グループの
// 項目は groups にもグループの id と再生順の動画として書く（specs/035-favorites/contracts/screen-api.md §3、
// specs/048-groups/contracts/screen-api.md）。スマートフォルダの項目は smartFolderIds に書く
// （specs/052-smart-folders/data-model.md「Library items」）。
func (s *server) ListLibraryIds(w http.ResponseWriter, r *http.Request, params gen.ListLibraryIdsParams) {
	if s.library == nil {
		s.internalError(w, "Library queries are not configured.", nil)
		return
	}
	query, ok := s.parseIDsQuery(w, videoQueryParams{
		query: params.Query, watch: params.Watch, playable: params.Playable, tag: params.Tag,
		favorite: params.Favorite, person: params.Person, organization: params.Organization,
		addedWithin: params.AddedWithin, minDuration: params.MinDuration, maxDuration: params.MaxDuration,
		tagKind: params.TagKind,
	})
	if !ok {
		return
	}
	selection, missingTagIDs, err := s.library.LibraryIDs(r.Context(), query)
	if err != nil {
		s.internalError(w, "Could not load the ids.", err)
		return
	}
	s.writeVideoIDs(w, selection.AllVideoIDs(), missingTagIDs, libraryGroupIDs(selection.Groups), selection.SmartFolderIDs)
}

// libraryGroupIDs はグループの項目を応答の LibraryGroupIds にする。
func libraryGroupIDs(groups []domain.LibraryGroupSelection) []gen.LibraryGroupIds {
	if len(groups) == 0 {
		return nil
	}
	out := make([]gen.LibraryGroupIds, 0, len(groups))
	for _, group := range groups {
		ids := group.VideoIDs
		if ids == nil {
			ids = []int64{}
		}
		out = append(out, gen.LibraryGroupIds{Id: group.ID, VideoIds: ids})
	}
	return out
}

// presentLibraryItems はライブラリの項目を応答の形にする。roots は項目と同じスナップショットから
// 読んだ登録フォルダである。見せる動画の無いグループの項目は、黙って落とすと items が total・カーソルと
// 食い違うので誤りにする。
func (s *server) presentLibraryItems(ctx context.Context, audience domain.Audience, roots []domain.MediaFolder, items []domain.LibraryItem) ([]gen.LibraryItem, error) {
	// 再生位置は動画の項目にだけ載せる。グループの視聴の値は保存層が決めているので、
	// メンバーの再生位置は引かない（ページのメンバーが多いと引数の上限に掛かる）。
	// タグは全メンバーの和集合を載せるので、全メンバーを引く。
	var shown, tagged []domain.Video
	for _, item := range items {
		if item.Video != nil {
			shown = append(shown, *item.Video)
			tagged = append(tagged, *item.Video)
		} else if item.Group != nil {
			tagged = append(tagged, item.Group.Members...)
		}
	}
	lookup := s.itemLookups(ctx, shown, tagged)
	out := make([]gen.LibraryItem, 0, len(items))
	for _, item := range items {
		if item.SmartFolder != nil {
			// スマートフォルダの項目は要約だけを載せる。カードの値は画面が listSmartFolderCards で読む
			// （specs/052-smart-folders/research.md R-14）。
			folder := toAPISmartFolderSummary(audience, *item.SmartFolder)
			out = append(out, gen.LibraryItem{Kind: gen.LibraryItemKindSmartFolder, SmartFolder: &folder})
			continue
		}
		if item.Video != nil {
			video := lookup.video(ctx, audience, roots, *item.Video)
			out = append(out, gen.LibraryItem{Kind: gen.LibraryItemKindVideo, Video: &video})
			continue
		}
		group, ok := lookup.group(ctx, audience, roots, *item.Group)
		if !ok {
			return nil, fmt.Errorf("group %d has no video", item.Group.ID)
		}
		out = append(out, gen.LibraryItem{Kind: gen.LibraryItemKindGroup, Group: &group})
	}
	return out, nil
}

// itemLookup は項目に載せる再生位置とタグをまとめて引いたものである。
type itemLookup struct {
	s        *server
	progress map[string]domain.Progress
	tags     map[string][]domain.VideoTag
}

// itemLookups は、応答に載せる動画（shown）の再生位置と、tagged のタグをまとめて引く。
// ゲストには読まない（progressFor・tagsFor と同じ）。
func (s *server) itemLookups(ctx context.Context, shown, tagged []domain.Video) itemLookup {
	return itemLookup{s: s, progress: s.progressFor(ctx, shown), tags: s.tagsFor(ctx, tagged)}
}

// video は動画の項目を応答の形にする。一覧（writeVideoPage）と同じく、再生位置・タグ・
// 一覧に出す所在のフォルダを載せ、見る人に合わせる。
func (l itemLookup) video(ctx context.Context, audience domain.Audience, roots []domain.MediaFolder, video domain.Video) gen.Video {
	view := l.s.presentVideos(ctx, []domain.Video{video})[0]
	item := withTags(withProgress(toAPIVideo(view), l.progress, video.UserKey), l.tags, video.UserKey)
	if folder, ok := domain.LocateVideoFolder(roots, video.Path); ok {
		item.Folder = &gen.VideoFolder{RootId: folder.RootID, Path: folder.Path}
	}
	return forAudience(audience, item)
}

// group はグループの項目を応答の形にする。見せる動画が無ければ false を返す。folder は
// 自動グループのフォルダが登録フォルダ roots の下にあるときだけ載せる（specs/048-groups/
// contracts/screen-api.md）。ゲストには視聴の値を載せず、タグを空にする（data-model.md §7）。
func (l itemLookup) group(ctx context.Context, audience domain.Audience, roots []domain.MediaFolder, group domain.LibraryGroup) (gen.LibraryGroup, bool) {
	card, ok := l.card(ctx, audience, roots, group)
	if !ok {
		return gen.LibraryGroup{}, false
	}
	ids := make([]int64, 0, len(group.Members))
	for _, member := range group.Members {
		ids = append(ids, member.ID)
	}
	out := gen.LibraryGroup{
		Id: card.Id, Folder: card.Folder, Name: card.Name, VideoCount: card.VideoCount, WatchedCount: card.WatchedCount,
		WatchState: (*gen.LibraryGroupWatchState)(card.WatchState), DurationMs: card.DurationMs, SizeBytes: card.SizeBytes,
		AddedAt: card.AddedAt, LastPlayedAt: card.LastPlayedAt, Previews: card.Previews, OpenVideoId: card.OpenVideoId,
		Favorite: card.Favorite, VideoIds: ids, Tags: []gen.VideoTag{},
	}
	if audience.IsOwner() {
		for _, tag := range unionMemberTags(group.Members, l.tags) {
			out.Tags = append(out.Tags, toAPIVideoTag(tag))
		}
	}
	return out, true
}

// card はグループのカードの値を応答の形にする（ホームの棚の項目。specs/057-home/research.md R-12）。
// 見せる動画が無ければ false を返す。folder は自動グループのフォルダが登録フォルダ roots の下にあるときだけ
// 載せる（specs/048-groups/contracts/screen-api.md）。ゲストには視聴の値を載せない（data-model.md §7）。
func (l itemLookup) card(ctx context.Context, audience domain.Audience, roots []domain.MediaFolder, group domain.LibraryGroup) (gen.LibraryGroupCard, bool) {
	if group.VideoCount == 0 {
		return gen.LibraryGroupCard{}, false
	}
	out := gen.LibraryGroupCard{
		Id:          group.ID,
		Name:        group.Name,
		VideoCount:  group.VideoCount,
		DurationMs:  group.DurationMs,
		SizeBytes:   group.SizeBytes,
		AddedAt:     group.AddedAt,
		Previews:    l.s.folderPreviews(ctx, previewVideos(group.Previews)),
		OpenVideoId: group.OpenVideoID,
	}
	if group.Path != "" {
		if folder, ok := domain.LocateFolder(roots, group.Path); ok {
			out.Folder = &gen.VideoFolder{RootId: folder.RootID, Path: folder.Path}
		}
	}
	if !audience.IsOwner() {
		return out, true
	}
	state := gen.LibraryGroupCardWatchState(group.WatchState)
	watched := group.WatchedCount
	out.WatchState, out.WatchedCount = &state, &watched
	favorite := group.Favorite
	out.Favorite = &favorite
	if group.LastPlayedAt != nil {
		played := *group.LastPlayedAt
		out.LastPlayedAt = &played
	}
	return out, true
}

// unionMemberTags はメンバーのタグの和集合を返す。同じタグは1件にまとめ、出所は
// どれかのメンバーで真なら真にする（contracts/library-api.md §1）。
func unionMemberTags(members []domain.Video, tags map[string][]domain.VideoTag) []domain.VideoTag {
	index := map[int64]int{}
	var out []domain.VideoTag
	for _, member := range members {
		for _, tag := range tags[member.UserKey] {
			if i, ok := index[tag.ID]; ok {
				out[i].Manual = out[i].Manual || tag.Manual
				out[i].FromFolder = out[i].FromFolder || tag.FromFolder
				continue
			}
			index[tag.ID] = len(out)
			out = append(out, tag)
		}
	}
	domain.SortVideoTags(out)
	return out
}
