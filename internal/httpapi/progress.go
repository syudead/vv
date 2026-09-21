package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// maxProgressBody は再生位置の本文に許す大きさである。数十バイトで足りる
// ものなので、これを越える要求は読み切らずに断る。
const maxProgressBody = 1 << 10

// PutVideoProgress は再生位置を記録する（PUT /api/videos/{id}/progress）。
//
// 視聴済みの判定はサーバー側で行い、クライアントの申告は採らない。
// クライアントごとに判定が揺れると、一覧の表示と再生画面が食い違う（R-111）。
//
// 呼び出し間隔はクライアントの責務で、サーバー側で頻度制限はしない
// （contracts/http-routes.md）。
func (s *server) PutVideoProgress(w http.ResponseWriter, r *http.Request, id gen.VideoId) {
	if s.playback == nil {
		s.internalError(w, "再生位置の保存先が設定されていません", nil)
		return
	}

	if !s.acceptsProgressBody(w, r) {
		return
	}

	video, ok := s.lookupVideo(w, r, id)
	if !ok {
		return
	}

	positionMs, ok := s.readPosition(w, r)
	if !ok {
		return
	}

	var durationMs int64
	if video.DurationMs != nil {
		durationMs = *video.DurationMs
	}

	// 記録の鍵は content_key（videos.id ではない）。ファイルを移動・改名・
	// 置き直しても再生位置が引き継がれる（FR-025／FR-026）。
	saved, err := s.playback.SaveProgress(
		r.Context(), video.ContentKey, domain.EvaluateProgress(positionMs, durationMs))
	if err != nil {
		s.internalError(w, "再生位置を記録できませんでした", err)
		return
	}

	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, toAPIProgress(saved), s.logger)
}

// acceptsProgressBody は本文の型が受け付けられるかを確かめる。
func (s *server) acceptsProgressBody(w http.ResponseWriter, r *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		s.invalidRequest(w, "Content-Type を解釈できません")
		return false
	}
	if mediaType != "application/json" {
		s.invalidRequest(w, "本文は application/json で送ってください")
		return false
	}
	return true
}

// readPosition は本文から位置を読み取る。
func (s *server) readPosition(w http.ResponseWriter, r *http.Request) (int64, bool) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxProgressBody+1))
	if err != nil {
		s.invalidRequest(w, "本文を読み取れません")
		return 0, false
	}
	if len(body) > maxProgressBody {
		s.invalidRequest(w, "本文が大きすぎます")
		return 0, false
	}

	// positionMs だけを読む。completed のような申告があっても採らない。
	var update struct {
		PositionMs *int64 `json:"positionMs"`
	}
	if err := json.Unmarshal(body, &update); err != nil {
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) {
			s.invalidRequest(w, "positionMs は数値で送ってください")
			return 0, false
		}
		s.invalidRequest(w, "本文を JSON として読み取れません")
		return 0, false
	}
	if update.PositionMs == nil {
		s.invalidRequest(w, "positionMs を指定してください")
		return 0, false
	}
	if *update.PositionMs < 0 {
		s.invalidRequest(w, "positionMs は 0 以上で指定してください")
		return 0, false
	}
	return *update.PositionMs, true
}

// toAPIProgress は domain.Progress を契約の形へ写す。
func toAPIProgress(progress domain.Progress) gen.Progress {
	return gen.Progress{
		PositionMs: progress.PositionMs,
		Completed:  progress.Completed,
		UpdatedAt:  progress.UpdatedAt,
	}
}
