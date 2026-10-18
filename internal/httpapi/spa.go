package httpapi

import (
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"
)

const (
	// Content-hashed asset names permit long-lived caching.
	assetsPrefix       = "assets/"
	assetsCacheControl = "public, max-age=31536000, immutable"
	// Always revalidate index.html so new builds become visible immediately.
	indexCacheControl = "no-cache"
	indexFileName     = "index.html"
)

type spaHandler struct {
	assets fs.FS
	logger *slog.Logger
}

func newSPAHandler(assets fs.FS, logger *slog.Logger) http.Handler {
	return &spaHandler{assets: assets, logger: logger}
}

// Serve existing files, return 404 for missing /assets/ files, and use index.html elsewhere for client routing.
func (h *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.assets == nil {
		h.logger.Error("SPA build output is not configured")
		http.Error(w, "The SPA build output is missing.", http.StatusInternalServerError)
		return
	}

	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")

	if name != "" && h.isRegularFile(name) {
		h.setCacheControl(w, name)
		http.ServeFileFS(w, r, h.assets, name)
		return
	}

	// Never hide missing build assets behind a successful HTML fallback.
	if strings.HasPrefix(name, assetsPrefix) {
		http.NotFound(w, r)
		return
	}

	h.serveIndex(w, r)
}

func (h *spaHandler) serveIndex(w http.ResponseWriter, r *http.Request) {
	if !h.isRegularFile(indexFileName) {
		h.logger.Error("index.html is not embedded", slog.String("path", r.URL.Path))
		http.Error(w, "index.html is missing (run task build).",
			http.StatusInternalServerError)
		return
	}

	w.Header().Set("Cache-Control", indexCacheControl)
	http.ServeFileFS(w, r, h.assets, indexFileName)
}

func (h *spaHandler) setCacheControl(w http.ResponseWriter, name string) {
	switch {
	case strings.HasPrefix(name, assetsPrefix):
		w.Header().Set("Cache-Control", assetsCacheControl)
	case name == indexFileName:
		w.Header().Set("Cache-Control", indexCacheControl)
	}
}

func (h *spaHandler) isRegularFile(name string) bool {
	info, err := fs.Stat(h.assets, name)
	return err == nil && info.Mode().IsRegular()
}
