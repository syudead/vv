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

// maxProgressBody は再生位置の本文に許す大きさである。数十バイトで足りる
// ものなので、これを越える要求は読み切らずに断る。
const maxProgressBody = 1 << 10

// PutVideoProgress は再生位置を記録する（PUT /api/videos/{id}/progress）。
//
// 視聴済みの判定はサーバー側で行い、クライアントの申告は採らない。
// クライアントごとに判定が揺れると、一覧の表示と再生画面が食い違う。
//
// 呼び出し間隔はクライアントの責務で、サーバー側で頻度制限はしない。
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

	// 記録の鍵は利用者データの鍵（videos.id ではない）。ファイルを移動・改名・
	// 置き直しても再生位置が引き継がれ、束ねた動画では集まりの再生位置になる
	// （specs/030-video-versions/data-model.md §3）。尺は再生したバージョンのもの。
	saved, err := s.playback.SaveProgress(
		r.Context(), video.UserKey, domain.EvaluateProgress(update.positionMs, durationMs), playOf(video, update))
	if err != nil {
		s.internalError(w, "Could not save playback progress.", err)
		return
	}

	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, toAPIProgress(saved), s.logger)
}

// acceptsProgressBody は本文の型が受け付けられるかを確かめる。
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

// progressUpdate は再生位置の本文から読み取った値である。playbackID は送られなければ空。
type progressUpdate struct {
	positionMs int64
	playbackID string
}

// readProgressUpdate は本文から位置と視聴の識別子を読み取る。
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

	// positionMs と playbackId だけを読む。completed のような申告があっても採らない。
	// playbackId は生のまま受け取り、明示の null を「送られていない」と区別して断る。
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
		// 16 進の大文字と小文字は同じ識別子なので、小文字にそろえてから一意の鍵に渡す。
		// そろえないと、大小だけ違う同じ識別子で履歴が 2 件になる。
		out.playbackID = strings.ToLower(playbackID)
	}
	return out, true
}

// playOf は保存が視聴履歴に書く値を返す。識別子が無いか、動画の content_key が空なら nil で、
// 履歴を書かない（specs/043-watch-history/contracts/screen-api.md）。鍵は再生したバージョンの
// content_key で、集まりの鍵（UserKey）ではない。題名はいま動画のページが見せている有効な題名である。
func playOf(video domain.Video, update progressUpdate) *domain.Play {
	if update.playbackID == "" || video.ContentKey == "" {
		return nil
	}
	return &domain.Play{PlaybackID: update.playbackID, ContentKey: video.ContentKey, Title: video.Title}
}

// toAPIProgress は domain.Progress を契約の形へ写す。
func toAPIProgress(progress domain.Progress) gen.Progress {
	return gen.Progress{
		PositionMs: progress.PositionMs,
		Completed:  progress.Completed,
		UpdatedAt:  progress.UpdatedAt,
	}
}
