package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

func (s *server) ListMediaFolders(w http.ResponseWriter, r *http.Request) {
	if s.mediaFolders == nil {
		s.internalError(w, "Media folder storage is not configured.", nil)
		return
	}
	folders, err := s.mediaFolders.ListMediaFolders(r.Context())
	if err != nil {
		s.internalError(w, "Could not load media folders.", err)
		return
	}
	out := make([]gen.MediaFolder, 0, len(folders))
	for _, folder := range folders {
		out = append(out, toAPIMediaFolder(folder))
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, out, s.logger)
}

func (s *server) CreateMediaFolder(w http.ResponseWriter, r *http.Request) {
	var body gen.CreateMediaFolderRequest
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Path) == "" {
		s.invalidRequest(w, "path is required.")
		return
	}
	if s.mediaFolders == nil {
		s.internalError(w, "Media folder storage is not configured.", nil)
		return
	}
	folder, err := s.mediaFolders.AddMediaFolder(r.Context(), body.Path)
	if err != nil {
		s.writeMediaFolderError(w, err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusCreated, toAPIMediaFolder(folder), s.logger)
}

func (s *server) UpdateMediaFolder(w http.ResponseWriter, r *http.Request, id gen.MediaFolderId) {
	var body gen.UpdateMediaFolderRequest
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if id < 1 || body.Version < 1 || strings.TrimSpace(body.Path) == "" {
		s.invalidRequest(w, "Specify a valid id, path, and version.")
		return
	}
	if s.mediaFolders == nil {
		s.internalError(w, "Media folder storage is not configured.", nil)
		return
	}
	folder, err := s.mediaFolders.ReplaceMediaFolder(r.Context(), id, body.Version, body.Path)
	if err != nil {
		s.writeMediaFolderError(w, err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, toAPIMediaFolder(folder), s.logger)
}

func (s *server) DeleteMediaFolder(w http.ResponseWriter, r *http.Request, id gen.MediaFolderId, params gen.DeleteMediaFolderParams) {
	if id < 1 || params.Version < 1 {
		s.invalidRequest(w, "Specify a valid id and version.")
		return
	}
	if s.mediaFolders == nil {
		s.internalError(w, "Media folder storage is not configured.", nil)
		return
	}
	if err := s.mediaFolders.DeleteMediaFolder(r.Context(), id, params.Version); err != nil {
		s.writeMediaFolderError(w, err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	w.WriteHeader(http.StatusNoContent)
}

func toAPIMediaFolder(folder domain.MediaFolder) gen.MediaFolder {
	return gen.MediaFolder{
		Id: folder.ID, Path: folder.Path, Version: folder.Version,
		CreatedAt: folder.CreatedAt, UpdatedAt: folder.UpdatedAt,
	}
}

func (s *server) readJSONBody(w http.ResponseWriter, r *http.Request, target any) bool {
	return s.decodeJSONBody(w, r, io.LimitReader(r.Body, 1<<20), target)
}

// readLargeJSONBody は readJSONBody と同じく本文を読むが、上限を limit バイトにする。
// 上限を超えた本文は、途中で切れた JSON の誤りとしてではなく、上限を超えたことを示して断る。
func (s *server) readLargeJSONBody(w http.ResponseWriter, r *http.Request, limit int64, target any) bool {
	return s.decodeJSONBody(w, r, http.MaxBytesReader(w, r.Body, limit), target)
}

func (s *server) decodeJSONBody(w http.ResponseWriter, r *http.Request, body io.Reader, target any) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		s.invalidRequest(w, "Content-Type must be application/json.")
		return false
	}
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		s.invalidBody(w, err, "Cannot parse the JSON body.")
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		s.invalidBody(w, err, "The body must contain exactly one JSON value.")
		return false
	}
	return true
}

// invalidBody は本文を読めなかったときの 400 invalid_request を書く。
func (s *server) invalidBody(w http.ResponseWriter, err error, message string) {
	if tooLarge := (*http.MaxBytesError)(nil); errors.As(err, &tooLarge) {
		message = fmt.Sprintf("The body must be at most %d bytes. Split the items into smaller requests.", tooLarge.Limit)
	}
	s.invalidRequest(w, message)
}

func (s *server) acceptsSameOrigin(w http.ResponseWriter, r *http.Request) bool {
	if strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site") {
		s.writeReasonError(w, http.StatusForbidden, codeForbidden, reasonCrossOrigin, "Only same-origin requests are accepted.")
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	// 期待するスキームは Cookie と同じ判定から取る（contracts/auth-api.md §8）。TLS を
	// 終端する信頼するプロキシの後ろでは、X-Forwarded-Proto で https になる。
	expectedScheme := "http"
	if s.clientOrigin(r).https {
		expectedScheme = "https"
	}
	if err != nil || parsed.Scheme != expectedScheme || !strings.EqualFold(parsed.Host, r.Host) {
		s.writeReasonError(w, http.StatusForbidden, codeForbidden, reasonCrossOrigin, "Only same-origin requests are accepted.")
		return false
	}
	return true
}

func (s *server) writeMediaFolderError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidMediaFolder):
		s.writeError(w, http.StatusBadRequest, codeInvalidMediaDirectory, "The selected directory cannot be used.")
	case errors.Is(err, domain.ErrUnsupportedMediaFolder):
		s.writeError(w, http.StatusBadRequest, codeUnsupportedMediaDirectory, "That directory cannot be selected.")
	case errors.Is(err, domain.ErrNotFound):
		s.writeError(w, http.StatusNotFound, codeMediaFolderNotFound, "Media folder not found.")
	case errors.Is(err, domain.ErrFolderConflict):
		s.writeError(w, http.StatusConflict, codeOverlappingMediaDirectories, "The folder overlaps with or contains an existing media folder.")
	case errors.Is(err, domain.ErrScanRunning):
		s.writeError(w, http.StatusConflict, codeScanInProgress, "Media folders cannot be changed while a scan is running.")
	case errors.Is(err, domain.ErrVersionConflict):
		s.conflictReason(w, reasonMediaFoldersChanged, "Media folders were changed by another operation.")
	default:
		s.internalError(w, "Could not change media folders.", err)
	}
}
