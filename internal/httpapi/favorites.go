package httpapi

import (
	"context"
	"fmt"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// 動画とグループのお気に入りの付け外し（specs/035-favorites/contracts/screen-api.md §1）。

// Favorites はお気に入りの保存先である。internal/store の *FavoriteStore がこれを満たす。
// 1 つの取引で済むので、internal/app は通さない。
type Favorites interface {
	// SetFavorites は change の動画とグループのお気に入りを change.Favorite にそろえ、
	// 反映した数を返す（specs/035-favorites/data-model.md §4）。
	SetFavorites(ctx context.Context, change domain.FavoriteChange) (domain.FavoriteApplied, error)
}

// favoritesBodyLimit は PUT /api/favorites が読む本文の上限（バイト）である。folders の上限
// 20000 件を深いパスで指定しても収まるよう、外部連携 API の externalBodyLimit と同じ大きさにする
// （1 件あたり 1.6 KB 程度）。画面の API の既定の 1 MiB では、上限内の件数でも収まらない。
const favoritesBodyLimit = externalBodyLimit

// UpdateFavorites は動画とグループのお気に入りを付け外しする（PUT /api/favorites）。
// videoIds と folders の合計（重複は 1 つ）の上限はタグの付け外しと同じにする。
// 登録フォルダに無い rootId のフォルダは誤りにせず、保存層へ渡さない（数えない）。
// 確定後の /api/events の知らせは出さない（research.md R-6）。
func (s *server) UpdateFavorites(w http.ResponseWriter, r *http.Request) {
	// gen.FavoritesRequest の favorite は bool なので、欠けても false に読めてしまう。
	// 欠けた本文でお気に入りを外さないよう、有無を区別して読む。
	var body struct {
		VideoIds []int64           `json:"videoIds"`
		Folders  []gen.VideoFolder `json:"folders"`
		Favorite *bool             `json:"favorite"`
	}
	if !s.readLargeJSONBody(w, r, favoritesBodyLimit, &body) {
		return
	}
	videoIDs := uniqueInt64s(body.VideoIds)
	folders := uniqueVideoFolders(body.Folders)
	if total := len(videoIDs) + len(folders); total < 1 || total > maxVideoTagsIDs {
		s.invalidRequestLimit(w, reasonTooManyVideos, maxVideoTagsIDs,
			fmt.Sprintf("videoIds and folders must contain between 1 and %d items in total.", maxVideoTagsIDs))
		return
	}
	for _, folder := range folders {
		if err := domain.ValidateFolderPath(folder.Path); err != nil {
			s.invalidRequestReason(w, reasonInvalidFolderPath,
				"Invalid folder path. Empty segments, leading or trailing /, ., and .. are not allowed.")
			return
		}
	}
	if body.Favorite == nil {
		s.invalidRequest(w, "favorite is required.")
		return
	}
	if s.favorites == nil {
		s.internalError(w, "Favorite storage is not configured.", nil)
		return
	}

	paths, ok := s.favoriteFolderPaths(w, r, folders)
	if !ok {
		return
	}
	applied, err := s.favorites.SetFavorites(r.Context(), domain.FavoriteChange{
		VideoIDs: videoIDs, FolderPaths: paths, Favorite: *body.Favorite,
	})
	if err != nil {
		s.internalError(w, "Could not change the favorites.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.FavoritesResponse{
		AppliedVideos: applied.Videos, AppliedFolders: applied.Folders,
	}, s.logger)
}

// favoriteFolderPaths は folders を GET /api/folders/{rootId}/group と同じく絶対パスにする。
// rootId が登録フォルダに無いものは落とす。登録フォルダを引けなければ 500 を書いて false を返す。
func (s *server) favoriteFolderPaths(w http.ResponseWriter, r *http.Request, folders []gen.VideoFolder) ([]string, bool) {
	if len(folders) == 0 {
		return nil, true
	}
	if s.folders == nil {
		s.internalError(w, "Folder queries are not configured.", nil)
		return nil, false
	}
	roots, err := s.folders.ListMediaFolders(r.Context())
	if err != nil {
		s.folderError(w, r, "Could not load folders.", err)
		return nil, false
	}
	rootPaths := make(map[int64]string, len(roots))
	for _, root := range roots {
		rootPaths[root.ID] = root.Path
	}
	paths := make([]string, 0, len(folders))
	for _, folder := range folders {
		if rootPath, ok := rootPaths[folder.RootId]; ok {
			paths = append(paths, domain.FolderDir(rootPath, folder.Path))
		}
	}
	return paths, true
}

// uniqueInt64s は最初に現れた順で重複を除く。
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

// uniqueVideoFolders は rootId と path の組で、最初に現れた順に重複を除く。
// rootName は同一性に含めない（値が同じでも別のポインタになり、別のものに数えてしまう）。
func uniqueVideoFolders(folders []gen.VideoFolder) []gen.VideoFolder {
	type folderKey struct {
		rootID int64
		path   string
	}
	seen := make(map[folderKey]struct{}, len(folders))
	out := make([]gen.VideoFolder, 0, len(folders))
	for _, folder := range folders {
		key := folderKey{rootID: folder.RootId, path: folder.Path}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, folder)
	}
	return out
}
