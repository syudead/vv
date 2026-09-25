package httpapi

import (
	"net/http"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// GetVideoPreview serves only the persisted, content-keyed preview MP4.
func (s *server) GetVideoPreview(
	w http.ResponseWriter, r *http.Request, id gen.VideoId, _ gen.GetVideoPreviewParams,
) {
	// ゲストとして処理する要求は、非公開にされたら打ち切れるよう台帳に載せる
	// （contracts/guest-api.md §6）。
	video, r, release, ok := s.lookupServedVideo(w, r, id)
	if !ok {
		return
	}
	defer release()

	if video.PreviewState != domain.PreviewStateDone || s.artifacts == nil {
		s.notFound(w, "プレビューはまだ生成されていません")
		return
	}

	// 無い・manifest と合わない・生成途中のものは、置き場が「無い」と答える。
	file, digest, err := s.artifacts.PreviewFile(video.ContentKey)
	if err != nil {
		s.notFound(w, "プレビューはまだ生成されていません")
		return
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		s.notFound(w, "プレビューはまだ生成されていません")
		return
	}

	// 版の有無によらず、使うたびに確かめさせる（contracts/guest-api.md §5）。
	// If-None-Match が一致すれば http.ServeContent が 304 を返す。更新時刻は渡さない。
	// 渡すと If-Modified-Since だけの要求に更新時刻で 304 を返し、同じ秒に作り直した
	// プレビューが古いまま残る。確かめは内容のダイジェストの ETag だけで行う。
	setRevalidate(w, digestETag("preview", digest))
	w.Header().Set("Content-Type", "video/mp4")
	http.ServeContent(w, r, info.Name(), time.Time{}, file)
}
