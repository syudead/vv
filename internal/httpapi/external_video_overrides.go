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
		e.videoNotFoundAt(w, notFound.Index)
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

// thumbnailTarget は検証を通ったサムネイルの位置の指定 1 件である。
type thumbnailTarget struct {
	video      domain.Video
	path       string
	positionMs *int64
}

// UpdateVideoThumbnails は複数の動画の代表サムネイルの位置を設定・解除する
// （POST /api/v1/video-thumbnails）。先に全件の引き当て・位置の検証・所在の解決を行い、
// 誤りがあれば何も反映しない。通ったら items の順に 1 件ずつ、画面の
// PUT /api/videos/{id}/thumbnail-position と同じ ThumbnailPicker で画像を作って記録する。
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
		target, ok := e.thumbnailTarget(w, r, i, ref, positions[i])
		if !ok {
			return
		}
		targets = append(targets, target)
	}

	out := extgen.VideoThumbnailsResponse{Items: make([]extgen.VideoThumbnailsItem, 0, len(targets))}
	for i, target := range targets {
		saved, err := e.s.thumbnails.SetThumbnailPosition(r.Context(), target.video.ID, target.path, target.positionMs)
		if err != nil {
			e.writeThumbnailError(w, i, target.video, err)
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

// thumbnailTarget は items の index 番目を引き当て、位置を確かめ、読む元の所在を決める。
// 誤りなら応答を書いて ok = false を返す。順序は画面の経路と同じで、位置は所在を開く前に
// 確かめる（解析前や尺の外の指定で file_unavailable を返さない）。
func (e *externalServer) thumbnailTarget(
	w http.ResponseWriter, r *http.Request, index int, ref domain.VideoRef, positionMs *int64,
) (thumbnailTarget, bool) {
	found, err := e.s.externalVideos.LookupExternalVideo(r.Context(), ref)
	if errors.Is(err, domain.ErrNotFound) {
		e.videoNotFoundAt(w, index)
		return thumbnailTarget{}, false
	}
	if err != nil {
		e.internalError(w, "Could not look up the video.", err)
		return thumbnailTarget{}, false
	}
	video, err := e.s.videos.GetVideo(r.Context(), domain.AudienceOwner, found.Video.ID)
	if errors.Is(err, domain.ErrNotFound) {
		e.videoNotFoundAt(w, index)
		return thumbnailTarget{}, false
	}
	if err != nil {
		e.internalError(w, "Could not load the video.", err)
		return thumbnailTarget{}, false
	}
	if positionMs != nil {
		if err := domain.CheckThumbnailPosition(video, *positionMs); err != nil {
			e.writeThumbnailError(w, index, video, err)
			return thumbnailTarget{}, false
		}
	}
	// 読む元は配信と同じ規則で決め、symlink を辿った先のパスを渡す（画面の経路と同じ）。
	path, ok := e.s.resolveMediaFile(r, video)
	if !ok {
		e.writeItemError(w, http.StatusNotFound, extgen.ErrorCodeNotFound, extgen.FileUnavailable, index, nil,
			"Item "+strconv.Itoa(index)+": Cannot open this video's file. Nothing was changed.")
		return thumbnailTarget{}, false
	}
	return thumbnailTarget{video: video, path: path, positionMs: positionMs}, true
}

// writeThumbnailError は items の index 番目の位置の設定の失敗を応答にする。尺の外の limit
// には video の尺を載せる。
func (e *externalServer) writeThumbnailError(w http.ResponseWriter, index int, video domain.Video, err error) {
	prefix := "Item " + strconv.Itoa(index) + ": "
	switch {
	case errors.Is(err, domain.ErrDurationUnknown):
		e.writeItemError(w, http.StatusConflict, extgen.ErrorCodeConflict, extgen.DurationUnknown, index, nil,
			prefix+"The video's duration is not known yet.")
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
			extgen.ThumbnailPositionOutOfRange, index, limit, message)
	case errors.Is(err, domain.ErrThumbnailFrameUnavailable):
		// 生成の失敗の理由（ffmpeg の出力など）は記録にだけ残す。
		e.s.logger.Warn("cannot extract a frame for the thumbnail",
			slog.Int64("video", video.ID), slog.Int("index", index), slog.Any("error", err))
		e.writeItemError(w, http.StatusConflict, extgen.ErrorCodeConflict, extgen.ThumbnailFrameUnavailable, index, nil,
			prefix+"Cannot make a thumbnail from this position. The items before it were changed.")
	case errors.Is(err, domain.ErrNotFound):
		e.videoNotFoundAt(w, index)
	default:
		e.internalError(w, "Could not change the thumbnail.", err)
	}
}

// videoNotFoundAt は items の index 番目の動画を引けなかったときの 404 を返す。
func (e *externalServer) videoNotFoundAt(w http.ResponseWriter, index int) {
	e.writeItemError(w, http.StatusNotFound, extgen.ErrorCodeNotFound, extgen.VideoNotFound, index, nil,
		"Video "+strconv.Itoa(index)+" is not in the library. Nothing was changed.")
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
