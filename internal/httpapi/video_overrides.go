package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// 動画の表示名の設定・解除（specs/029-video-overrides/contracts/screen-api.md §1、
// research.md R-8・R-10）。1つの取引で済むので internal/app は通さず、
// internal/store の *OverrideStore を直接呼ぶ。
//
// 代表サムネイルの位置の設定・解除（contracts/screen-api.md §2、research.md R-4・R-8・R-11）は、
// 画像の生成と記録を取り込みの job と同じ生成の錠の中で行うので、internal/app の
// *Ingest（ThumbnailPicker）を呼ぶ。

// OverrideStore は動画の上書きの保存先である。internal/store の *OverrideStore が
// これを満たす。
type OverrideStore interface {
	// SetDisplayName は動画 videoID の表示名を name にし（整えて空なら解除）、反映後の
	// 動画を返す。規則に合わなければ *domain.InvalidDisplayNameError、動画が無いか
	// 登録フォルダの下に所在が無ければ domain.ErrNotFound を返す。
	SetDisplayName(ctx context.Context, videoID int64, name string) (domain.Video, error)
}

// SetVideoDisplayName は動画の表示名を設定・解除する（PUT /api/videos/{id}/display-name）。
func (s *server) SetVideoDisplayName(w http.ResponseWriter, r *http.Request, id gen.VideoId) {
	// gen.DisplayNameUpdate の displayName は string なので、欠けても空（解除）に読めて
	// しまう。欠けた本文で表示名を消さないよう、有無を区別して読む。
	var body struct {
		DisplayName *string `json:"displayName"`
	}
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if body.DisplayName == nil {
		s.invalidRequest(w, "displayName is required.")
		return
	}
	if s.overrides == nil {
		s.internalError(w, "Video override storage is not configured.", nil)
		return
	}

	video, err := s.overrides.SetDisplayName(r.Context(), id, *body.DisplayName)
	switch {
	case errors.Is(err, domain.ErrInvalidDisplayName):
		s.invalidDisplayName(w, err)
		return
	case errors.Is(err, domain.ErrNotFound):
		s.notFoundReason(w, reasonVideoNotFound, "Video not found.")
		return
	case err != nil:
		s.internalError(w, "Could not save the display name.", err)
		return
	}
	s.writeVideoDetail(w, r, video)
}

// invalidDisplayName は表示名の規則違反を reason 付きの 400 にする
// （contracts/screen-api.md §1・§3）。
func (s *server) invalidDisplayName(w http.ResponseWriter, err error) {
	var invalid *domain.InvalidDisplayNameError
	if !errors.As(err, &invalid) {
		s.invalidRequest(w, "The display name cannot be used.")
		return
	}
	switch invalid.Problem {
	case domain.DisplayNameControlCharacters:
		s.invalidRequestReason(w, reasonDisplayNameControlCharacters, "Display names cannot contain control characters.")
	case domain.DisplayNameTooLong:
		s.invalidRequestLimit(w, reasonDisplayNameTooLong, domain.DisplayNameMaxLength,
			"Display names must be at most "+strconv.Itoa(domain.DisplayNameMaxLength)+" characters.")
	default:
		s.invalidRequest(w, "The display name cannot be used.")
	}
}

// ThumbnailPicker は代表サムネイルの位置の設定先である。internal/app の *Ingest が
// これを満たす。
type ThumbnailPicker interface {
	// SetThumbnailPosition は動画 videoID の代表サムネイルを path の positionMs の場面で
	// 作り直して公開し、位置を記録して反映後の動画を返す。positionMs が nil なら解除し、
	// 自動の位置で作り直す。解析前は domain.ErrDurationUnknown、尺の外は
	// domain.ErrThumbnailPositionOutOfRange、生成の失敗は domain.ErrThumbnailFrameUnavailable、
	// 動画が無ければ domain.ErrNotFound を返す。
	SetThumbnailPosition(ctx context.Context, videoID int64, path string, positionMs *int64) (domain.Video, error)
}

// SetVideoThumbnailPosition は動画の代表サムネイルの位置を設定・解除する
// （PUT /api/videos/{id}/thumbnail-position）。応答は画像の生成が終わってから返す。
func (s *server) SetVideoThumbnailPosition(w http.ResponseWriter, r *http.Request, id gen.VideoId) {
	// gen.ThumbnailPositionUpdate の positionMs は *int64 なので、欠けても null（解除）に
	// 読めてしまう。欠けた本文で位置を消さないよう、有無を区別して読む。
	var body struct {
		PositionMs json.RawMessage `json:"positionMs"`
	}
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if len(body.PositionMs) == 0 {
		s.invalidRequest(w, "positionMs is required.")
		return
	}
	var positionMs *int64
	if err := json.Unmarshal(body.PositionMs, &positionMs); err != nil {
		s.invalidRequest(w, "positionMs must be an integer or null.")
		return
	}
	if s.thumbnails == nil {
		s.internalError(w, "Thumbnail generation is not configured.", nil)
		return
	}

	video, ok := s.lookupVideo(w, r, id)
	if !ok {
		return
	}
	// 所在を開く前に位置を確かめる。解析前や尺の外の指定で、読めない所在の 404 を返さない。
	// 生成の錠の中でも internal/app が確かめ直す。
	if positionMs != nil {
		if err := domain.CheckThumbnailPosition(video, *positionMs); err != nil {
			s.thumbnailPositionError(w, video, err)
			return
		}
	}
	if s.files == nil {
		s.internalError(w, "Media file access is not configured.", nil)
		return
	}
	// 読む元は配信（getVideoStream）と同じ規則で決め、symlink を辿った先のパスを渡す。
	// 辿る前のパスを渡すと、確かめた後に symlink を差し替えられたとき、ffmpeg が
	// 登録フォルダの外や別の動画を読みうる。
	path, ok := s.resolveMediaFile(r, video)
	if !ok {
		s.notFoundReason(w, reasonFileUnavailable, "Cannot open this video's file.")
		return
	}

	saved, err := s.thumbnails.SetThumbnailPosition(r.Context(), id, path, positionMs)
	if err != nil {
		s.thumbnailPositionError(w, video, err)
		return
	}
	s.writeVideoDetail(w, r, saved)
}

// thumbnailPositionError は代表サムネイルの位置の設定の失敗を応答にする
// （contracts/screen-api.md §2・§3）。尺の外の limit には video の尺を載せる。
func (s *server) thumbnailPositionError(w http.ResponseWriter, video domain.Video, err error) {
	switch {
	case errors.Is(err, domain.ErrDurationUnknown):
		s.conflictReason(w, reasonDurationUnknown, "The video's duration is not known yet.")
	case errors.Is(err, domain.ErrThumbnailPositionOutOfRange):
		if video.DurationMs == nil {
			s.invalidRequestReason(w, reasonThumbnailPositionOutOfRange, "The position is outside the video.")
			return
		}
		s.invalidRequestLimit(w, reasonThumbnailPositionOutOfRange, int(*video.DurationMs),
			"The position must be at least 0 and less than "+strconv.FormatInt(*video.DurationMs, 10)+" ms.")
	case errors.Is(err, domain.ErrThumbnailFrameUnavailable):
		// 生成の失敗の理由（ffmpeg の出力など）は記録にだけ残す。
		s.logger.Warn("cannot extract a frame for the thumbnail",
			slog.Int64("video", video.ID), slog.Any("error", err))
		s.conflictReason(w, reasonThumbnailFrameUnavailable, "Cannot make a thumbnail from this position.")
	case errors.Is(err, domain.ErrNotFound):
		s.notFoundReason(w, reasonVideoNotFound, "Video not found.")
	default:
		s.internalError(w, "Could not change the thumbnail.", err)
	}
}
