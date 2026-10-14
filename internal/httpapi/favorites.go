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

// favoritesBodyLimit は PUT /api/favorites が読む本文の上限（バイト）である。videoIds と
// groupIds の合計の上限 20000 件が収まるよう、外部連携 API の externalBodyLimit と同じ大きさにする。
const favoritesBodyLimit = externalBodyLimit

// UpdateFavorites は動画とグループのお気に入りを付け外しする（PUT /api/favorites）。
// videoIds と groupIds の合計（重複は 1 つ）の上限はタグの付け外しと同じにする。
// 無い動画やグループの id は誤りにせず、保存層が数えない（specs/048-groups/research.md R-10）。
// 確定後の /api/events の知らせは出さない（research.md R-6）。
func (s *server) UpdateFavorites(w http.ResponseWriter, r *http.Request) {
	// gen.FavoritesRequest の favorite は bool なので、欠けても false に読めてしまう。
	// 欠けた本文でお気に入りを外さないよう、有無を区別して読む。
	var body struct {
		VideoIds []int64 `json:"videoIds"`
		GroupIds []int64 `json:"groupIds"`
		Favorite *bool   `json:"favorite"`
	}
	if !s.readLargeJSONBody(w, r, favoritesBodyLimit, &body) {
		return
	}
	videoIDs := uniqueInt64s(body.VideoIds)
	groupIDs := uniqueInt64s(body.GroupIds)
	if total := len(videoIDs) + len(groupIDs); total < 1 || total > maxVideoTagsIDs {
		s.invalidRequestLimit(w, reasonTooManyVideos, maxVideoTagsIDs,
			fmt.Sprintf("videoIds and groupIds must contain between 1 and %d items in total.", maxVideoTagsIDs))
		return
	}
	if body.Favorite == nil {
		s.invalidRequest(w, "favorite is required.")
		return
	}
	if s.favorites == nil {
		s.internalError(w, "Favorite storage is not configured.", nil)
		return
	}

	applied, err := s.favorites.SetFavorites(r.Context(), domain.FavoriteChange{
		VideoIDs: videoIDs, GroupIDs: groupIDs, Favorite: *body.Favorite,
	})
	if err != nil {
		s.internalError(w, "Could not change the favorites.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.FavoritesResponse{
		AppliedVideos: applied.Videos, AppliedGroups: applied.Groups,
	}, s.logger)
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
