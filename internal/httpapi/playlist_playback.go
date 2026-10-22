package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

type PlaylistPlaybackReader interface {
	PlaylistPlayback(ctx context.Context, id, itemID, videoID int64) (domain.PlaylistPlayback, error)
}

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
		if step.SmartFolderID != 0 {
			out[i].SmartFolderId = &step.SmartFolderID
		}
		next++
	}

	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.PlaylistPlayback{
		Playlist:      toAPIPlaylistSummary(playback.Playlist),
		ItemId:        playback.ItemID,
		Position:      playback.Position,
		Count:         playback.Count,
		PlayableCount: playback.PlayableCount,
		Next:          out[0],
		PreviousItem:  out[1],
		NextItem:      out[2],
	}, s.logger)
}

func toAPIPlaylistSummary(playlist domain.Playlist) gen.PlaylistSummary {
	return gen.PlaylistSummary{
		Id:        playlist.ID,
		Name:      playlist.Name,
		ItemCount: playlist.ItemCount,
		UpdatedAt: playlist.UpdatedAt.UTC(),
	}
}
