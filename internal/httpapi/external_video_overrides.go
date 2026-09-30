package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/extgen"
)

// 外部連携 API の表示名とサムネイルの位置の一括操作
// （specs/029-video-overrides/contracts/external-api.md §1・§2、research.md R-9）。
// 動画の指定・index・「全件を確かめてから反映する」形は POST /api/v1/video-tags と同じにする。

// maxVideoDisplayNames は POST /api/v1/video-display-names の items の上限である。
const maxVideoDisplayNames = 20000

// maxVideoThumbnails は POST /api/v1/video-thumbnails の items の上限である。1 件ごとに
// ffmpeg を走らせるので、1 つの要求に収まるよう小さく区切る（R-9）。
const maxVideoThumbnails = 20

// displayNameItem・thumbnailItem は本文の要素の JSON の形である。知らない項目は誤りにする。
// displayName・positionMs が欠けた要素を null（解除）と取り違えないよう、有無を区別して読む。
type (
	displayNameItem struct {
		Video       extgen.VideoRef `json:"video"`
		DisplayName json.RawMessage `json:"displayName"`
	}
	thumbnailItem struct {
		Video      extgen.VideoRef `json:"video"`
		PositionMs json.RawMessage `json:"positionMs"`
	}
)

// UpdateVideoDisplayNames は複数の動画の表示名を設定・解除する
// （POST /api/v1/video-display-names）。全件を 1 つの取引で行う保存先
// （OverrideStore.SetDisplayNames）に任せ、internal/app は通さない。
func (e *externalServer) UpdateVideoDisplayNames(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Items []displayNameItem `json:"items"`
	}
	if !e.readJSONBody(w, r, &body) {
		return
	}
	if len(body.Items) < 1 || len(body.Items) > maxVideoDisplayNames {
		e.invalidRequestLimit(w, extgen.TooManyVideos, maxVideoDisplayNames,
			fmt.Sprintf("items must contain between 1 and %d items.", maxVideoDisplayNames))
		return
	}
	changes := make([]domain.DisplayNameChange, 0, len(body.Items))
	for i, item := range body.Items {
		ref, ok := externalVideoRef(item.Video)
		if !ok {
			e.invalidItem(w, i, "Each video must have exactly one of id, contentKey and path.")
			return
		}
		if len(item.DisplayName) == 0 {
			e.invalidItem(w, i, "displayName is required.")
			return
		}
		var name *string
		if err := json.Unmarshal(item.DisplayName, &name); err != nil {
			e.invalidItem(w, i, "displayName must be a string or null.")
			return
		}
		change := domain.DisplayNameChange{Video: ref}
		if name != nil {
			change.DisplayName = *name
		}
		changes = append(changes, change)
	}
	if e.s.overrides == nil {
		e.internalError(w, "Video override storage is not configured.", nil)
		return
	}

	videos, err := e.s.overrides.SetDisplayNames(r.Context(), changes)
	if err != nil {
		e.writeDisplayNamesError(w, err)
		return
	}
	out := extgen.VideoDisplayNamesResponse{Items: make([]extgen.VideoDisplayNamesItem, 0, len(videos))}
	for _, video := range videos {
		out.Items = append(out.Items, extgen.VideoDisplayNamesItem{
			Video:       extgen.VideoTagsVideo{Id: video.ID, ContentKey: video.ContentKey},
			Title:       video.Title,
			FileTitle:   video.FileTitle,
			DisplayName: optionalString(video.DisplayName),
		})
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, out, e.s.logger)
}

// writeDisplayNamesError は表示名の一括操作の保存先の誤りを応答へ写す。
func (e *externalServer) writeDisplayNamesError(w http.ResponseWriter, err error) {
	var notFound *domain.VideoRefNotFoundError
	if errors.As(err, &notFound) {
		e.videoNotFoundAt(w, notFound.Index, "Nothing was changed.")
		return
	}
	var nameAt *domain.DisplayNameAtError
	var invalid *domain.InvalidDisplayNameError
	if errors.As(err, &nameAt) && errors.As(err, &invalid) {
		prefix := "Item " + strconv.Itoa(nameAt.Index) + ": "
		switch invalid.Problem {
		case domain.DisplayNameControlCharacters:
			e.writeItemError(w, http.StatusBadRequest, extgen.ErrorCodeInvalidRequest,
				extgen.DisplayNameControlCharacters, nameAt.Index, nil,
				prefix+"Display names cannot contain control characters.")
		case domain.DisplayNameTooLong:
			limit := domain.DisplayNameMaxLength
			e.writeItemError(w, http.StatusBadRequest, extgen.ErrorCodeInvalidRequest,
				extgen.DisplayNameTooLong, nameAt.Index, &limit,
				prefix+"Display names must be at most "+strconv.Itoa(limit)+" characters.")
		default:
			e.invalidItem(w, nameAt.Index, prefix+"The display name cannot be used.")
		}
		return
	}
	e.internalError(w, "Could not update the display names.", err)
}

// thumbnailTarget は引き当てを通ったサムネイルの位置の指定 1 件である。読む元の所在は
// 持たず、生成の直前に決め直す（前の項目の生成の間に所在が変わりうるため）。
type thumbnailTarget struct {
	videoID    int64
	positionMs *int64
}

// UpdateVideoThumbnails は複数の動画の代表サムネイルの位置を設定・解除する
// （POST /api/v1/video-thumbnails）。先に全件の引き当て・位置の検証・所在の解決を行い、
// 誤りがあれば何も反映しない。通ったら items の順に 1 件ずつ、画面の
// PUT /api/videos/{id}/thumbnail-position と同じ手順（動画を読み直し、位置を確かめ、所在を
// 解決して、ThumbnailPicker で画像を作って記録する）を行う。前の項目の生成の間に動画や所在が
// 変わりうるので、所在は検証のときに決めたものを使わず生成の直前に決め直す。
// 途中の失敗では、それより前の項目は反映済みのまま index で止まった位置を返す。
func (e *externalServer) UpdateVideoThumbnails(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Items []thumbnailItem `json:"items"`
	}
	if !e.readJSONBody(w, r, &body) {
		return
	}
	if len(body.Items) < 1 || len(body.Items) > maxVideoThumbnails {
		e.invalidRequestLimit(w, extgen.TooManyVideos, maxVideoThumbnails,
			fmt.Sprintf("items must contain between 1 and %d items.", maxVideoThumbnails))
		return
	}
	refs := make([]domain.VideoRef, 0, len(body.Items))
	positions := make([]*int64, 0, len(body.Items))
	for i, item := range body.Items {
		ref, ok := externalVideoRef(item.Video)
		if !ok {
			e.invalidItem(w, i, "Each video must have exactly one of id, contentKey and path.")
			return
		}
		if len(item.PositionMs) == 0 {
			e.invalidItem(w, i, "positionMs is required.")
			return
		}
		var positionMs *int64
		if err := json.Unmarshal(item.PositionMs, &positionMs); err != nil {
			e.invalidItem(w, i, "positionMs must be an integer or null.")
			return
		}
		refs = append(refs, ref)
		positions = append(positions, positionMs)
	}
	if e.s.externalVideos == nil || e.s.videos == nil {
		e.internalError(w, "Video storage is not configured.", nil)
		return
	}
	if e.s.thumbnails == nil {
		e.internalError(w, "Thumbnail generation is not configured.", nil)
		return
	}
	if e.s.files == nil {
		e.internalError(w, "Media file access is not configured.", nil)
		return
	}

	targets := make([]thumbnailTarget, 0, len(refs))
	for i, ref := range refs {
		target, ok := e.checkThumbnailTarget(w, r, i, ref, positions[i])
		if !ok {
			return
		}
		targets = append(targets, target)
	}

	out := extgen.VideoThumbnailsResponse{Items: make([]extgen.VideoThumbnailsItem, 0, len(targets))}
	for i, target := range targets {
		step := thumbnailStep{index: i, applying: true}
		video, path, ok := e.thumbnailSource(w, r, step, target)
		if !ok {
			return
		}
		saved, err := e.s.thumbnails.SetThumbnailPosition(r.Context(), video.ID, path, target.positionMs)
		if err != nil {
			e.writeThumbnailError(w, step, video, err)
			return
		}
		out.Items = append(out.Items, extgen.VideoThumbnailsItem{
			Video:               extgen.VideoTagsVideo{Id: saved.ID, ContentKey: saved.ContentKey},
			ThumbnailPositionMs: saved.ThumbnailPositionMs,
		})
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, out, e.s.logger)
}

// thumbnailStep は誤りを返すときの items の位置と段階である。applying は反映の段階
// （前の項目を反映済みでありうる）を表す。
type thumbnailStep struct {
	index    int
	applying bool
}

// outcome は誤りの文言の末尾に添える、それまでに何を反映したかの説明である。
func (step thumbnailStep) outcome() string {
	if step.applying && step.index > 0 {
		return "The items before it were changed."
	}
	return "Nothing was changed."
}

// prefix は誤りの文言の先頭に添える項目の位置である。
func (step thumbnailStep) prefix() string {
	return "Item " + strconv.Itoa(step.index) + ": "
}

// checkThumbnailTarget は items の index 番目を検証の段階で引き当て、位置と読む元の所在を
// 確かめる。誤りなら応答を書いて ok = false を返す。
func (e *externalServer) checkThumbnailTarget(
	w http.ResponseWriter, r *http.Request, index int, ref domain.VideoRef, positionMs *int64,
) (thumbnailTarget, bool) {
	step := thumbnailStep{index: index}
	found, err := e.s.externalVideos.LookupExternalVideo(r.Context(), ref)
	if errors.Is(err, domain.ErrNotFound) {
		e.videoNotFoundAt(w, index, step.outcome())
		return thumbnailTarget{}, false
	}
	if err != nil {
		e.internalErrorAt(w, step, "Could not look up the video.", err)
		return thumbnailTarget{}, false
	}
	target := thumbnailTarget{videoID: found.Video.ID, positionMs: positionMs}
	if _, _, ok := e.thumbnailSource(w, r, step, target); !ok {
		return thumbnailTarget{}, false
	}
	return target, true
}

// thumbnailSource は動画を読み直し、位置を確かめ、読む元の所在を決める。誤りなら応答を
// 書いて ok = false を返す。順序は画面の経路と同じで、位置は所在を開く前に確かめる
// （解析前や尺の外の指定で file_unavailable を返さない）。
func (e *externalServer) thumbnailSource(
	w http.ResponseWriter, r *http.Request, step thumbnailStep, target thumbnailTarget,
) (domain.Video, string, bool) {
	video, err := e.s.videos.GetVideo(r.Context(), domain.AudienceOwner, target.videoID)
	if errors.Is(err, domain.ErrNotFound) {
		e.videoNotFoundAt(w, step.index, step.outcome())
		return domain.Video{}, "", false
	}
	if err != nil {
		e.internalErrorAt(w, step, "Could not load the video.", err)
		return domain.Video{}, "", false
	}
	if target.positionMs != nil {
		if err := domain.CheckThumbnailPosition(video, *target.positionMs); err != nil {
			e.writeThumbnailError(w, step, video, err)
			return domain.Video{}, "", false
		}
	}
	// 読む元は配信と同じ規則で決め、symlink を辿った先のパスを渡す（画面の経路と同じ）。
	path, ok := e.s.resolveMediaFile(r, video)
	if !ok {
		e.writeItemError(w, http.StatusNotFound, extgen.ErrorCodeNotFound, extgen.FileUnavailable, step.index, nil,
			step.prefix()+"Cannot open this video's file. "+step.outcome())
		return domain.Video{}, "", false
	}
	return video, path, true
}

// writeThumbnailError は items の index 番目の位置の設定の失敗を応答にする。尺の外の limit
// には video の尺を載せる。
func (e *externalServer) writeThumbnailError(w http.ResponseWriter, step thumbnailStep, video domain.Video, err error) {
	prefix, outcome := step.prefix(), " "+step.outcome()
	switch {
	case errors.Is(err, domain.ErrDurationUnknown):
		e.writeItemError(w, http.StatusConflict, extgen.ErrorCodeConflict, extgen.DurationUnknown, step.index, nil,
			prefix+"The video's duration is not known yet."+outcome)
	case errors.Is(err, domain.ErrThumbnailPositionOutOfRange):
		var limit *int
		message := prefix + "The position is outside the video."
		if video.DurationMs != nil {
			duration := int(*video.DurationMs)
			limit = &duration
			message = prefix + "The position must be at least 0 and less than " +
				strconv.FormatInt(*video.DurationMs, 10) + " ms."
		}
		e.writeItemError(w, http.StatusBadRequest, extgen.ErrorCodeInvalidRequest,
			extgen.ThumbnailPositionOutOfRange, step.index, limit, message+outcome)
	case errors.Is(err, domain.ErrThumbnailFrameUnavailable):
		// 生成の失敗の理由（ffmpeg の出力など）は記録にだけ残す。
		e.s.logger.Warn("cannot extract a frame for the thumbnail",
			slog.Int64("video", video.ID), slog.Int("index", step.index), slog.Any("error", err))
		e.writeItemError(w, http.StatusConflict, extgen.ErrorCodeConflict, extgen.ThumbnailFrameUnavailable, step.index, nil,
			prefix+"Cannot make a thumbnail from this position."+outcome)
	case errors.Is(err, domain.ErrNotFound):
		e.videoNotFoundAt(w, step.index, step.outcome())
	default:
		e.internalErrorAt(w, step, "Could not change the thumbnail.", err)
	}
}

// internalErrorAt は items の index 番目で起きた想定外の失敗を、index とそれまでに何を
// 反映したかを添えた 500 にする。
func (e *externalServer) internalErrorAt(w http.ResponseWriter, step thumbnailStep, message string, err error) {
	e.s.logger.Error(message, slog.Int("index", step.index), slog.Any("error", err))
	index := step.index
	writeExternalError(w, e.s, http.StatusInternalServerError, extgen.Error{
		Code: extgen.ErrorCodeInternal, Index: &index, Message: step.prefix() + message + " " + step.outcome(),
	})
}

// videoNotFoundAt は items の index 番目の動画を引けなかったときの 404 を返す。outcome は
// それまでに何を反映したかの説明である。
func (e *externalServer) videoNotFoundAt(w http.ResponseWriter, index int, outcome string) {
	e.writeItemError(w, http.StatusNotFound, extgen.ErrorCodeNotFound, extgen.VideoNotFound, index, nil,
		"Video "+strconv.Itoa(index)+" is not in the library. "+outcome)
}

// invalidItem は items の index 番目の形の誤りを 400 invalid_request にする。
func (e *externalServer) invalidItem(w http.ResponseWriter, index int, message string) {
	writeExternalError(w, e.s, http.StatusBadRequest, extgen.Error{
		Code: extgen.ErrorCodeInvalidRequest, Index: &index, Message: message,
	})
}

// writeItemError は reason と index（と上限）を添えた誤りを返す。
func (e *externalServer) writeItemError(
	w http.ResponseWriter, status int, code extgen.ErrorCode, reason extgen.ErrorReason,
	index int, limit *int, message string,
) {
	writeExternalError(w, e.s, status, extgen.Error{
		Code: code, Reason: &reason, Index: &index, Limit: limit, Message: message,
	})
}

// optionalString は空を nil にする。
func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
