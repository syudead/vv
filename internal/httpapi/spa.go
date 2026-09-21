package httpapi

import (
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"
)

// 応答ヘッダは contracts/http-routes.md の表に従う。
const (
	// assetsPrefix 配下は内容ハッシュ付きの名前なので長期キャッシュして良い。
	assetsPrefix = "assets/"
	// assetsCacheControl は /assets/* に付ける値。
	assetsCacheControl = "public, max-age=31536000, immutable"
	// indexCacheControl は index.html に付ける値。更新したビルドが即座に反映される
	// ようにするため、内容の再確認を必ず行わせる。
	indexCacheControl = "no-cache"
	// indexFileName は SPA の入口である。
	indexFileName = "index.html"
)

// spaHandler は埋め込んだ SPA のビルド成果物を配信する。
type spaHandler struct {
	assets fs.FS
	logger *slog.Logger
}

// newSPAHandler は SPA 配信のハンドラを返す。assets は web/dist に相当する
// ファイルシステムで、通常は embed.FS から渡る。
func newSPAHandler(assets fs.FS, logger *slog.Logger) http.Handler {
	return &spaHandler{assets: assets, logger: logger}
}

// ServeHTTP は次のように振り分ける（contracts/http-routes.md）。
//
//   - 実在するファイル → そのまま配信する
//   - /assets/ 配下で実在しない → 404（index.html へ落とさない）
//   - それ以外 → index.html（クライアント側ルーティングのためのフォールバック）
func (h *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.assets == nil {
		h.logger.Error("SPA のビルド成果物が設定されていません")
		http.Error(w, "SPA のビルド成果物がありません", http.StatusInternalServerError)
		return
	}

	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")

	if name != "" && h.isRegularFile(name) {
		h.setCacheControl(w, name)
		http.ServeFileFS(w, r, h.assets, name)
		return
	}

	// 資産の取り違えを index.html の 200 で隠してしまうと、ビルドの不整合に
	// 気付けなくなる。/assets/ 配下は素直に 404 を返す。
	if strings.HasPrefix(name, assetsPrefix) {
		http.NotFound(w, r)
		return
	}

	h.serveIndex(w, r)
}

// serveIndex は SPA の入口を返す。
func (h *spaHandler) serveIndex(w http.ResponseWriter, r *http.Request) {
	if !h.isRegularFile(indexFileName) {
		h.logger.Error("index.html が埋め込まれていません", slog.String("path", r.URL.Path))
		http.Error(w, "index.html がありません（task build を実行してください）",
			http.StatusInternalServerError)
		return
	}

	w.Header().Set("Cache-Control", indexCacheControl)
	http.ServeFileFS(w, r, h.assets, indexFileName)
}

// setCacheControl は配信するファイルに応じたキャッシュ指示を付ける。
func (h *spaHandler) setCacheControl(w http.ResponseWriter, name string) {
	switch {
	case strings.HasPrefix(name, assetsPrefix):
		w.Header().Set("Cache-Control", assetsCacheControl)
	case name == indexFileName:
		w.Header().Set("Cache-Control", indexCacheControl)
	}
}

// isRegularFile は名前が通常のファイルとして存在するかを返す。
func (h *spaHandler) isRegularFile(name string) bool {
	info, err := fs.Stat(h.assets, name)
	return err == nil && info.Mode().IsRegular()
}
