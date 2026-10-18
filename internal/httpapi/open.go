package httpapi

import (
	"net"
	"net/http"
	"strings"

	"github.com/syudead/vv/internal/httpapi/gen"
)

// Open only the server-selected representative path; precedence is not-found, unavailable environment, forbidden source, missing file, then internal failure.
func (s *server) OpenVideoFile(w http.ResponseWriter, r *http.Request, id gen.VideoId) {
	video, ok := s.lookupVideo(w, r, id)
	if !ok {
		return
	}
	if s.opener == nil || !s.opener.Available() {
		s.writeError(w, http.StatusConflict, codeOpenUnavailable, "This server cannot open files.")
		return
	}
	if !s.loopbackRequest(r) {
		s.writeReasonError(w, http.StatusForbidden, codeForbidden, reasonOpenNotLocal,
			"Files can only be opened from the computer running the server.")
		return
	}
	if s.files == nil {
		s.internalError(w, "Media file access is not configured.", nil)
		return
	}
	path, ok, err := s.openablePath(r, video.Path)
	if err != nil {
		s.internalError(w, "Could not load media folders.", err)
		return
	}
	if !ok {
		s.writeError(w, http.StatusConflict, codeFileMissing, "File not found. It may have been moved or deleted.")
		return
	}
	if err := s.opener.Open(path); err != nil {
		s.internalError(w, "Could not open the file.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	w.WriteHeader(http.StatusNoContent)
}

// Reuse streaming's symlink/root checks, mask outside-root paths as missing, and pass the validated resolved path to the opener.
func (s *server) openablePath(r *http.Request, path string) (string, bool, error) {
	roots, err := s.mediaFolderPaths(r)
	if err != nil {
		return "", false, err
	}
	resolved, err := s.files.ResolveMediaFile(roots, path)
	if err != nil {
		return "", false, nil
	}
	return resolved, true, nil
}

func (s *server) canOpen(r *http.Request) bool {
	return s.opener != nil && s.opener.Available() && s.loopbackRequest(r)
}

// Require loopback source, direct peer, and Host to block DNS rebinding and LAN proxies spoofing loopback X-Forwarded-For.
func (s *server) loopbackRequest(r *http.Request) bool {
	if !parseForwardedAddr(r.RemoteAddr).IsLoopback() {
		return false
	}
	source := s.clientOrigin(r).source
	if !source.IsValid() || !source.IsLoopback() {
		return false
	}
	return loopbackHostName(r.Host)
}

// Accept localhost, 127.0.0.1, or [::1] regardless of port.
func loopbackHostName(hostHeader string) bool {
	name := hostHeader
	if host, _, err := net.SplitHostPort(hostHeader); err == nil {
		name = host
	} else {
		name = strings.TrimSuffix(strings.TrimPrefix(name, "["), "]")
	}
	switch strings.ToLower(name) {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}
