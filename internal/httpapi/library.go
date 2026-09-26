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
	// 動画の id とグループの全メンバーの id を返す（GET /api/library/ids、所有者だけ）。
	LibraryIDs(ctx context.Context, q domain.VideoQuery) (ids, missingTagIDs []int64, err error)
	// FolderGroup はフォルダ dir（絶対パス）のグループを、見せてよい全メンバーから作って
	// 返す。今グループでなければ domain.ErrNotFound である。
	FolderGroup(ctx context.Context, audience domain.Audience, dir string) (domain.LibraryGroup, error)
}

// ListLibrary はライブラリの項目の一覧を返す（GET /api/library）。パラメータの検査と
// 誤りは listVideos と同じである。
func (s *server) ListLibrary(w http.ResponseWriter, r *http.Request, params gen.ListLibraryParams) {
	if s.library == nil {
		s.internalError(w, "一覧の問い合わせ先が設定されていません", nil)
		return
	}
	audience := audienceFrom(r.Context())
	query, ok := s.parseVideoQuery(w, audience, videoQueryParams{
		query: params.Query, watch: params.Watch, playable: params.Playable, sort: params.Sort,
		seed: params.Seed, cursor: params.Cursor, limit: params.Limit, tag: params.Tag,
	})
	if !ok {
		return
	}

	page, err := s.library.ListLibrary(r.Context(), audience, query)
	switch {
	case errors.Is(err, domain.ErrInvalidCursor):
		s.invalidRequest(w, "読み込み位置を解釈できません。一覧を開き直してください")
		return
	case err != nil:
		s.internalError(w, "一覧を取得できませんでした", err)
		return
	}

	// 登録フォルダは項目と同じスナップショットから読んだものを使う。別に読むと、
	// 読み取りの失敗や間に確定した登録フォルダの解除で、total とカーソルに数えた
	// グループを応答から落としてしまう。
	roots := page.Roots
	// 再生位置は動画の項目にだけ載せる。グループの視聴の値は保存層が決めているので、
	// メンバーの再生位置は引かない（ページのメンバーが多いと引数の上限に掛かる）。
	// タグは全メンバーの和集合を載せるので、全メンバーを引く。
	var shown, tagged []domain.Video
	for _, item := range page.Items {
		if item.Video != nil {
			shown = append(shown, *item.Video)
			tagged = append(tagged, *item.Video)
		} else {
			tagged = append(tagged, item.Group.Members...)
		}
	}
	lookup := s.itemLookups(r.Context(), shown, tagged)

	payload := gen.LibraryPage{Items: make([]gen.LibraryItem, 0, len(page.Items)), Total: page.Total}
	for _, item := range page.Items {
		if item.Video != nil {
			video := lookup.video(r.Context(), audience, roots, *item.Video)
			payload.Items = append(payload.Items, gen.LibraryItem{Kind: gen.LibraryItemKindVideo, Video: &video})
			continue
		}
		group, ok := lookup.group(r.Context(), audience, roots, *item.Group)
		if !ok {
			// 同じスナップショットの登録フォルダの下に無いグループは索引の不整合である。
			// 黙って落とすと items が total・カーソルと食い違うので、要求を失敗させる。
			s.internalError(w, "一覧を取得できませんでした",
				fmt.Errorf("グループのフォルダを登録フォルダから引けません: %s", item.Group.Path))
			return
		}
		payload.Items = append(payload.Items, gen.LibraryItem{Kind: gen.LibraryItemKindGroup, Group: &group})
	}
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
// 「すべて選択」用。specs/017-folder-groups/contracts/library-api.md §2）。
func (s *server) ListLibraryIds(w http.ResponseWriter, r *http.Request, params gen.ListLibraryIdsParams) {
	if s.library == nil {
		s.internalError(w, "一覧の問い合わせ先が設定されていません", nil)
		return
	}
	query, ok := s.parseIDsQuery(w, params.Query, params.Watch, params.Playable, params.Tag)
	if !ok {
		return
	}
	ids, missingTagIDs, err := s.library.LibraryIDs(r.Context(), query)
	if err != nil {
		s.internalError(w, "idを取得できませんでした", err)
		return
	}
	s.writeVideoIDs(w, ids, missingTagIDs)
}

// GetFolderGroup はフォルダのグループ1件を返す（GET /api/folders/{rootId}/group、
// contracts/library-api.md §3）。
func (s *server) GetFolderGroup(w http.ResponseWriter, r *http.Request, rootID gen.FolderRootId, params gen.GetFolderGroupParams) {
	if s.library == nil {
		s.internalError(w, "グループの問い合わせ先が設定されていません", nil)
		return
	}
	roots, root, rel, ok := s.resolveFolderRootIn(w, r, rootID, params.Path)
	if !ok {
		return
	}
	audience := audienceFrom(r.Context())
	group, err := s.library.FolderGroup(r.Context(), audience, domain.FolderDir(root.Path, rel))
	switch {
	case errors.Is(err, domain.ErrNotFound):
		s.notFound(w, "そのフォルダはグループではありません")
		return
	case err != nil:
		s.folderError(w, r, "グループを取得できませんでした", err)
		return
	}

	payload, ok := s.itemLookups(r.Context(), nil, group.Members).group(r.Context(), audience, roots, group)
	if !ok {
		s.notFound(w, "そのフォルダはグループではありません")
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, payload, s.logger)
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
	item := withTags(withProgress(toAPIVideo(view), l.progress, video.ContentKey), l.tags, video.ContentKey)
	if folder, ok := domain.LocateVideoFolder(roots, video.Path); ok {
		item.Folder = &gen.VideoFolder{RootId: folder.RootID, Path: folder.Path}
	}
	return forAudience(audience, item)
}

// group はグループの項目を応答の形にする。グループのフォルダを登録フォルダから
// 引けなければ false を返す。ゲストには視聴の値を載せず、タグを空にする
// （data-model.md §7）。
func (l itemLookup) group(ctx context.Context, audience domain.Audience, roots []domain.MediaFolder, group domain.LibraryGroup) (gen.LibraryGroup, bool) {
	folder, ok := domain.LocateFolder(roots, group.Path)
	if !ok || len(group.Members) == 0 {
		return gen.LibraryGroup{}, false
	}
	ids := make([]int64, 0, len(group.Members))
	for _, member := range group.Members {
		ids = append(ids, member.ID)
	}
	out := gen.LibraryGroup{
		Folder:      gen.VideoFolder{RootId: folder.RootID, Path: folder.Path},
		Name:        group.Name,
		VideoCount:  len(group.Members),
		DurationMs:  group.DurationMs,
		SizeBytes:   group.SizeBytes,
		AddedAt:     group.AddedAt,
		Previews:    l.s.folderPreviews(ctx, groupPreviewMembers(group.Members)),
		OpenVideoId: group.OpenVideoID,
		VideoIds:    ids,
		Tags:        []gen.VideoTag{},
	}
	if !audience.IsOwner() {
		return out, true
	}
	state := gen.LibraryGroupWatchState(group.WatchState)
	watched := group.WatchedCount
	out.WatchState, out.WatchedCount = &state, &watched
	if group.LastPlayedAt != nil {
		played := *group.LastPlayedAt
		out.LastPlayedAt = &played
	}
	for _, tag := range unionMemberTags(group.Members, l.tags) {
		out.Tags = append(out.Tags, gen.VideoTag{Id: tag.ID, Name: tag.Name, Manual: tag.Manual, FromFolder: tag.FromFolder})
	}
	return out, true
}

// groupPreviewMembers は、グループのカードのフォルダの絵柄に差し込むメンバーを返す。
// サムネイル生成済みのメンバーを並びの順に、MaxGroupPreviews 件まで取る。
func groupPreviewMembers(members []domain.Video) []domain.Video {
	out := make([]domain.Video, 0, domain.MaxGroupPreviews)
	for _, member := range members {
		if len(out) == domain.MaxGroupPreviews {
			break
		}
		if member.HasThumbnail() {
			out = append(out, member)
		}
	}
	return out
}

// unionMemberTags はメンバーのタグの和集合を返す。同じタグは1件にまとめ、出所は
// どれかのメンバーで真なら真にする（contracts/library-api.md §1）。
func unionMemberTags(members []domain.Video, tags map[string][]domain.VideoTag) []domain.VideoTag {
	index := map[int64]int{}
	var out []domain.VideoTag
	for _, member := range members {
		for _, tag := range tags[member.ContentKey] {
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
