package httpapi

import (
	"net/http"
	"os"
	"path/filepath"

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
	if video.PreviewState != domain.PreviewStateDone || s.thumbnailsDir == "" {
		s.notFound(w, "プレビューはまだ生成されていません")
		return
	}

	path := previewFilePath(s.thumbnailsDir, video.ContentKey)
	file, err := os.Open(path)
	if err != nil {
		s.notFound(w, "プレビューはまだ生成されていません")
		return
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		s.notFound(w, "プレビューはまだ生成されていません")
		return
	}

	if versioned {
		w.Header().Set("Cache-Control", cacheImmutable)
	} else {
		w.Header().Set("Cache-Control", cacheNoStore)
	}
	w.Header().Set("Content-Type", "video/mp4")
	http.ServeContent(w, r, filepath.Base(path), info.ModTime(), file)
}
