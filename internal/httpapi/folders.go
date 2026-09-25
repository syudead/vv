package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// Folders はフォルダ画面の問い合わせ先である。フォルダは保存されておらず、
// 所在のパスから導く。所在と動画を返す読み出しは見る人（domain.Audience）を取る
// （Library と同じく、今はすべて所有者として読む）。
type Folders interface {
	ListMediaFolders(ctx context.Context) ([]domain.MediaFolder, error)
	FolderLocations(ctx context.Context, audience domain.Audience, dir string) ([]domain.FolderLocation, error)
	HasFolderLocations(ctx context.Context, audience domain.Audience, dir string) (bool, error)
	ListFolderVideos(ctx context.Context, audience domain.Audience, q domain.FolderVideoQuery) (domain.VideoPage, error)
}

const folderNotFoundMessage = "そのフォルダは見つかりません"

// ListRootFolders は登録済みメディアフォルダの集計を返す（GET /api/folders）。
func (s *server) ListRootFolders(w http.ResponseWriter, r *http.Request) {
	if s.folders == nil {
		s.internalError(w, "フォルダの問い合わせ先が設定されていません", nil)
		return
	}
	roots, err := s.folders.ListMediaFolders(r.Context())
	if err != nil {
		s.folderError(w, r, "フォルダを取得できませんでした", err)
		return
	}

	summaries := make([]domain.FolderSummary, 0, len(roots))
	for _, root := range roots {
		locations, err := s.folders.FolderLocations(r.Context(), audienceFrom(r.Context()), root.Path)
		if err != nil {
			s.folderError(w, r, "フォルダを取得できませんでした", err)
			return
		}
		listing, _ := domain.SummarizeFolder(root, "", locations)
		summaries = append(summaries, listing.Folder)
	}
	domain.SortFolders(summaries)

	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.RootFolderListing{Folders: toAPIFolders(summaries)}, s.logger)
}

// GetFolder はフォルダ1件と直下の子フォルダを返す（GET /api/folders/{rootId}）。
func (s *server) GetFolder(w http.ResponseWriter, r *http.Request, rootID gen.FolderRootId, params gen.GetFolderParams) {
	root, rel, ok := s.resolveFolderRoot(w, r, rootID, params.Path)
	if !ok {
		return
	}
	locations, err := s.folders.FolderLocations(r.Context(), audienceFrom(r.Context()), domain.FolderDir(root.Path, rel))
	if err != nil {
		s.folderError(w, r, "フォルダを取得できませんでした", err)
		return
	}
	listing, found := domain.SummarizeFolder(root, rel, locations)
	if !found {
		s.notFound(w, folderNotFoundMessage)
		return
	}

	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.FolderListing{
		Folder:  toAPIFolder(listing.Folder),
		Folders: toAPIFolders(listing.Folders),
	}, s.logger)
}

// ListFolderVideos はフォルダの動画を返す（GET /api/folders/{rootId}/videos）。
// 範囲は scope で直下（既定）か配下すべてかを選ぶ。ページング・絞り込み・並び順は
// ListVideos と同じである。フォルダが無ければ scope・query に関係なく 404 を返す。
func (s *server) ListFolderVideos(w http.ResponseWriter, r *http.Request, rootID gen.FolderRootId, params gen.ListFolderVideosParams) {
	roots, root, rel, ok := s.resolveFolderRootIn(w, r, rootID, params.Path)
	if !ok {
		return
	}

	query := domain.FolderVideoQuery{
		Dir: domain.FolderDir(root.Path, rel), Scope: domain.FolderScopeDirect,
		Sort: domain.SortAddedDesc, Limit: domain.DefaultLimit,
	}
	// フォルダの有無は条件の検査より先に確かめる。無いフォルダは、条件の値に
	// 関係なく 404 にする（contracts/list-api.md §5）。
	if rel != "" {
		found, err := s.folders.HasFolderLocations(r.Context(), audienceFrom(r.Context()), query.Dir)
		if err != nil {
			s.folderError(w, r, "フォルダの動画を取得できませんでした", err)
			return
		}
		if !found {
			s.notFound(w, folderNotFoundMessage)
			return
		}
	}
	if params.Scope != nil {
		scope := domain.FolderScope(*params.Scope)
		if !scope.Valid() {
			s.invalidRequest(w, "検索の範囲の値が不明です")
			return
		}
		query.Scope = scope
	}
	if query.Query, ok = s.parseSearchQuery(w, params.Query); !ok {
		return
	}
	filters, ok := s.parseListFilters(w, listFilterParams{
		watch: params.Watch, playable: params.Playable, sort: params.Sort, seed: params.Seed,
	})
	if !ok {
		return
	}
	query.Watch, query.PlayableOnly, query.Sort, query.Seed = filters.watch, filters.playableOnly, filters.sort, filters.seed
	if params.Limit != nil {
		if *params.Limit < 1 {
			s.invalidRequest(w, "1ページの件数は 1 以上を指定してください")
			return
		}
		query.Limit = min(*params.Limit, domain.MaxLimit)
	}
	if params.Cursor != nil {
		query.Cursor = *params.Cursor
	}

	page, err := s.folders.ListFolderVideos(r.Context(), audienceFrom(r.Context()), query)
	switch {
	case errors.Is(err, domain.ErrInvalidCursor):
		s.invalidRequest(w, "読み込み位置を解釈できません。フォルダを開き直してください")
		return
	case err != nil:
		s.folderError(w, r, "フォルダの動画を取得できませんでした", err)
		return
	}

	s.writeVideoPage(w, r, page, roots)
}

// resolveFolderRoot は登録フォルダと相対パスを確かめる。不正な相対パスは 400、
// 登録されていない id は 404 を書いて false を返す。
func (s *server) resolveFolderRoot(w http.ResponseWriter, r *http.Request, rootID int64, path *string) (domain.MediaFolder, string, bool) {
	_, root, rel, ok := s.resolveFolderRootIn(w, r, rootID, path)
	return root, rel, ok
}

// resolveFolderRootIn は resolveFolderRoot と同じで、引いた登録フォルダの一覧も返す。
func (s *server) resolveFolderRootIn(w http.ResponseWriter, r *http.Request, rootID int64, path *string) ([]domain.MediaFolder, domain.MediaFolder, string, bool) {
	if s.folders == nil {
		s.internalError(w, "フォルダの問い合わせ先が設定されていません", nil)
		return nil, domain.MediaFolder{}, "", false
	}
	rel := ""
	if path != nil {
		rel = *path
	}
	if err := domain.ValidateFolderPath(rel); err != nil {
		s.invalidRequest(w, "フォルダの指定が正しくありません。空の段・先頭や末尾の / ・ . ・ .. は使えません")
		return nil, domain.MediaFolder{}, "", false
	}

	roots, err := s.folders.ListMediaFolders(r.Context())
	if err != nil {
		s.folderError(w, r, "フォルダを取得できませんでした", err)
		return nil, domain.MediaFolder{}, "", false
	}
	for _, root := range roots {
		if root.ID == rootID {
			return roots, root, rel, true
		}
	}
	s.notFound(w, folderNotFoundMessage)
	return nil, domain.MediaFolder{}, "", false
}

func toAPIFolders(folders []domain.FolderSummary) []gen.FolderSummary {
	out := make([]gen.FolderSummary, 0, len(folders))
	for _, folder := range folders {
		out = append(out, toAPIFolder(folder))
	}
	return out
}

func toAPIFolder(folder domain.FolderSummary) gen.FolderSummary {
	previews := make([]gen.FolderPreview, 0, len(folder.Previews))
	for _, preview := range folder.Previews {
		previews = append(previews, gen.FolderPreview{
			VideoId:      preview.VideoID,
			ThumbnailUrl: thumbnailURL(domain.Video{ID: preview.VideoID, ContentKey: preview.ContentKey}),
		})
	}
	return gen.FolderSummary{
		RootId:      folder.RootID,
		Path:        folder.Path,
		Name:        folder.Name,
		RootPath:    folder.RootPath,
		VideoCount:  folder.VideoCount,
		FolderCount: folder.FolderCount,
		Previews:    previews,
	}
}

// folderError は保存層の失敗を 500 で返す。要求が打ち切られた（画面が先へ
// 進んだ）ための失敗は、誰も応答を読まないので書かず、ログにも残さない。
func (s *server) folderError(w http.ResponseWriter, r *http.Request, message string, err error) {
	if errors.Is(r.Context().Err(), context.Canceled) {
		return
	}
	s.internalError(w, message, err)
}
