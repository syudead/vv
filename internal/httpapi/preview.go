package httpapi

import (
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// GetVideoPreview serves only the persisted, content-keyed preview MP4.
func (s *server) GetVideoPreview(
	w http.ResponseWriter, r *http.Request, id gen.VideoId, params gen.GetVideoPreviewParams,
) {
	video, ok := s.lookupVideo(w, r, id)
	if !ok {
		return
	}

	versioned := params.V != nil && *params.V == video.ContentKey && *params.V != ""
	if !versioned {
		w.Header().Set("Cache-Control", cacheNoStore)
	}
	if video.PreviewState != domain.PreviewStateDone || s.artifacts == nil {
		s.notFound(w, "プレビューはまだ生成されていません")
		return
	}

	// 無い・manifest と合わない・生成途中のものは、置き場が「無い」と答える。
	file, err := s.artifacts.PreviewFile(video.ContentKey)
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

	if versioned {
		w.Header().Set("Cache-Control", cacheImmutable)
	} else {
		w.Header().Set("Cache-Control", cacheNoStore)
	}
	w.Header().Set("Content-Type", "video/mp4")
	http.ServeContent(w, r, info.Name(), info.ModTime(), file)
}
