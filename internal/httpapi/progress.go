package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

const maxProgressBody = 1 << 10

// Completion is server-derived for consistency; clients control write frequency without server throttling.
func (s *server) PutVideoProgress(w http.ResponseWriter, r *http.Request, id gen.VideoId) {
	if s.playback == nil {
		s.internalError(w, "Playback progress storage is not configured.", nil)
		return
	}

	if !s.acceptsProgressBody(w, r) {
		return
	}

	video, ok := s.lookupVideo(w, r, id)
	if !ok {
		return
	}

	update, ok := s.readProgressUpdate(w, r)
	if !ok {
		return
	}

	var durationMs int64
	if video.DurationMs != nil {
		durationMs = *video.DurationMs
	}

	// Progress uses the shared user-data key across moves/bundles, but duration comes from the played version.
	saved, err := s.playback.SaveProgress(
		r.Context(), video.UserKey, domain.EvaluateProgress(update.positionMs, durationMs), playOf(video, update))
	if err != nil {
		s.internalError(w, "Could not save playback progress.", err)
		return
	}

	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, toAPIProgress(saved), s.logger)
}

func (s *server) acceptsProgressBody(w http.ResponseWriter, r *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		s.invalidRequest(w, "Cannot parse Content-Type.")
		return false
	}
	if mediaType != "application/json" {
		s.invalidRequest(w, "The body must be application/json.")
		return false
	}
	return true
}

type progressUpdate struct {
	positionMs int64
	playbackID string
}

func (s *server) readProgressUpdate(w http.ResponseWriter, r *http.Request) (progressUpdate, bool) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxProgressBody+1))
	if err != nil {
		s.invalidRequest(w, "Cannot read the body.")
		return progressUpdate{}, false
	}
	if len(body) > maxProgressBody {
		s.invalidRequest(w, "The body is too large.")
		return progressUpdate{}, false
	}

	// Ignore client completion claims; distinguish omitted playbackId from invalid explicit null.
	var update struct {
		PositionMs *int64          `json:"positionMs"`
		PlaybackID json.RawMessage `json:"playbackId"`
	}
	if err := json.Unmarshal(body, &update); err != nil {
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) {
			s.invalidRequest(w, "positionMs must be a number.")
			return progressUpdate{}, false
		}
		s.invalidRequest(w, "Cannot parse the body as JSON.")
		return progressUpdate{}, false
	}
	if update.PositionMs == nil {
		s.invalidRequest(w, "positionMs is required.")
		return progressUpdate{}, false
	}
	if *update.PositionMs < 0 {
		s.invalidRequest(w, "positionMs must be 0 or greater.")
		return progressUpdate{}, false
	}
	out := progressUpdate{positionMs: *update.PositionMs}
	if update.PlaybackID != nil {
		var playbackID string
		if string(update.PlaybackID) == "null" || json.Unmarshal(update.PlaybackID, &playbackID) != nil {
			s.invalidRequest(w, "playbackId must be a string.")
			return progressUpdate{}, false
		}
		if err := domain.ValidatePlaybackID(playbackID); err != nil {
			s.invalidRequest(w, "playbackId must be an RFC 4122 id (8-4-4-4-12 hexadecimal).")
			return progressUpdate{}, false
		}
		// Lowercase hexadecimal playback IDs before uniqueness checks to avoid duplicate history entries for case variants.
		out.playbackID = strings.ToLower(playbackID)
	}
	return out, true
}

// History uses the played version's ContentKey, not UserKey; missing playback ID/content key suppresses history.
func playOf(video domain.Video, update progressUpdate) *domain.Play {
	if update.playbackID == "" || video.ContentKey == "" {
		return nil
	}
	return &domain.Play{PlaybackID: update.playbackID, ContentKey: video.ContentKey, Title: video.Title}
}

func toAPIProgress(progress domain.Progress) gen.Progress {
	return gen.Progress{
		PositionMs: progress.PositionMs,
		Completed:  progress.Completed,
		UpdatedAt:  progress.UpdatedAt,
	}
}
