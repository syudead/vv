package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// プレイリストから再生しているときの位置と次の動画（specs/044-playlists/contracts/screen-api.md
// 「GET /api/playlists/{id}/playback」、research.md R-6）。

// PlaylistPlaybackReader はプレイリストの連続再生の読み出し先である。internal/store の
// *PlaylistStore がこれを満たす。1 つの読み取りで済むので、internal/app は通さない。
type PlaylistPlaybackReader interface {
	// PlaylistPlayback はプレイリスト id を項目 itemID の動画 videoID から再生しているときの
	// 位置と次に開く動画を返す（data-model.md「Playback order」）。プレイリストが無ければ
	// domain.ErrPlaylistNotFound、項目が無ければ domain.ErrPlaylistItemNotFound を返す。
	PlaylistPlayback(ctx context.Context, id, itemID, videoID int64) (domain.PlaylistPlayback, error)
}

// GetPlaylistPlayback はプレイリストから再生しているときの位置と次の動画を返す
// （GET /api/playlists/{id}/playback）。所有者だけで、ゲストは /api/* の既定で 401 になる
// （research.md R-8）。item か video が無いか整数でなければ、生成された経路が 400 を返す。
func (s *server) GetPlaylistPlayback(w http.ResponseWriter, r *http.Request, id gen.PlaylistId, params gen.GetPlaylistPlaybackParams) {
	if s.playlistPlayback == nil {
		s.internalError(w, "Playlist storage is not configured.", nil)
		return
	}
	playback, err := s.playlistPlayback.PlaylistPlayback(r.Context(), id, params.Item, params.Video)
	switch {
	case errors.Is(err, domain.ErrPlaylistNotFound):
		s.notFound(w, "Playlist not found.")
		return
	case errors.Is(err, domain.ErrPlaylistItemNotFound):
		s.notFound(w, "Playlist item not found.")
		return
	case err != nil:
		s.internalError(w, "Could not load the playlist playback.", err)
		return
	}

	steps := []*domain.PlaylistStep{playback.Next, playback.PreviousItem, playback.NextItem}
	var videos []domain.Video
	for _, step := range steps {
		if step != nil {
			videos = append(videos, step.Video)
		}
	}
	audience := audienceFrom(r.Context())
	presented := s.presenter(r.Context(), audience, videos)(r.Context(), videos)
	out := make([]*gen.PlaylistStep, len(steps))
	next := 0
	for i, step := range steps {
		if step == nil {
			continue
		}
		out[i] = &gen.PlaylistStep{ItemId: step.ItemID, Video: presented[next]}
		next++
	}

	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.PlaylistPlayback{
		Playlist:     toAPIPlaylistSummary(playback.Playlist),
		ItemId:       playback.ItemID,
		Position:     playback.Position,
		Count:        playback.Count,
		Next:         out[0],
		PreviousItem: out[1],
		NextItem:     out[2],
	}, s.logger)
}

// toAPIPlaylistSummary はプレイリストの要約を契約の形にする。
func toAPIPlaylistSummary(playlist domain.Playlist) gen.PlaylistSummary {
	return gen.PlaylistSummary{
		Id:        playlist.ID,
		Name:      playlist.Name,
		ItemCount: playlist.ItemCount,
		UpdatedAt: playlist.UpdatedAt.UTC(),
	}
}
