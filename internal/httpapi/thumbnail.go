package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/syudead/vv/internal/httpapi/gen"
)

// GetVideoThumbnail はサムネイル画像を返す（GET /api/videos/{id}/thumbnail）。
//
// 未生成の場合は 404 を返し、クライアントは枠だけを描く（FR-010）。壊れた
// 応答を返すより、未生成と同じ扱いにする方が利用者の損失が小さい。
func (s *server) GetVideoThumbnail(
	w http.ResponseWriter, r *http.Request, id gen.VideoId, params gen.GetVideoThumbnailParams,
) {
	video, ok := s.lookupVideo(w, r, id)
	if !ok {
		return
	}

	if s.thumbnailsDir == "" || !video.HasThumbnail() {
		s.notFound(w, "サムネイルはまだ生成されていません")
		return
	}

	path := thumbnailFilePath(s.thumbnailsDir, video.ContentKey)
	file, err := os.Open(path)
	if err != nil {
		// 状態が done でも実体が無いことはある（利用者が消した、生成中に
		// 停止した）。存在を漏らさないためにも 404 に揃える。
		s.notFound(w, "サムネイルはまだ生成されていません")
		return
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		s.notFound(w, "サムネイルはまだ生成されていません")
		return
	}

	// 版付きの要求だけ長期キャッシュを許す。版は内容由来なので、内容が
	// 変われば URL も変わり、古い画像が残らない（R-112）。版の無い要求に
	// 1年のキャッシュを付けると、差し替えても更新されなくなる。
	if params.V != nil && *params.V != "" {
		w.Header().Set("Cache-Control", cacheImmutable)
	} else {
		w.Header().Set("Cache-Control", cacheNoStore)
	}
	w.Header().Set("Content-Type", "image/jpeg")

	http.ServeContent(w, r, filepath.Base(path), info.ModTime(), file)
}

// thumbnailFilePath は content_key から保存先を組み立てる。
//
// internal/media の ThumbnailPath と同じ規則である。配信のためだけに
// internal/media へ依存するより、規則を写す方が依存の向き（ARCHITECTURE.md）
// を保てる。規則を変えるときは両方を直す。
func thumbnailFilePath(thumbnailsDir, contentKey string) string {
	safe := strings.NewReplacer(":", "_", "/", "_", `\`, "_").Replace(contentKey)

	prefix := safe
	if len(prefix) > 2 {
		prefix = prefix[:2]
	}
	return filepath.Join(thumbnailsDir, prefix, safe+".jpg")
}
