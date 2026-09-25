package httpapi

import (
	"net/http"

	"github.com/syudead/vv/internal/httpapi/gen"
)

// GetVideoThumbnail はサムネイル画像を返す（GET /api/videos/{id}/thumbnail）。
//
// 未生成の場合は 404 を返し、クライアントは枠だけを描く。壊れた
// 応答を返すより、未生成と同じ扱いにする方が利用者の損失が小さい。
func (s *server) GetVideoThumbnail(
	w http.ResponseWriter, r *http.Request, id gen.VideoId, _ gen.GetVideoThumbnailParams,
) {
	video, ok := s.lookupVideo(w, r, id)
	if !ok {
		return
	}

	if s.artifacts == nil || !video.HasThumbnail() {
		s.notFound(w, "サムネイルはまだ生成されていません")
		return
	}

	file, err := s.artifacts.ThumbnailFile(video.ContentKey)
	if err != nil {
		// 状態が done でも実体が無いことはある（利用者が消した、生成中に
		// 停止した）。存在を漏らさないためにも 404 に揃える。
		s.notFound(w, "サムネイルはまだ生成されていません")
		return
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		s.notFound(w, "サムネイルはまだ生成されていません")
		return
	}

	// 版の有無によらず、使うたびに確かめさせる（contracts/guest-api.md §5）。
	// If-None-Match が一致すれば http.ServeContent が 304 を返す。ETag は内容から
	// 作る。作り直しで大きさと更新時刻が前と同じになっても、古い画像を使わせない。
	etag, err := readerETag(file)
	if err != nil {
		s.notFound(w, "サムネイルはまだ生成されていません")
		return
	}
	setRevalidate(w, etag)
	w.Header().Set("Content-Type", "image/jpeg")

	http.ServeContent(w, r, info.Name(), info.ModTime(), file)
}
