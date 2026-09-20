package httpapi

import (
	"encoding/json"
	"errors"
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
		s.internalError(w, "メディアフォルダの経路が設定されていません", nil)
		return
	}
	folders, err := s.mediaFolders.ListMediaFolders(r.Context())
	if err != nil {
		s.internalError(w, "メディアフォルダを取得できませんでした", err)
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
		s.invalidRequest(w, "pathを指定してください")
		return
	}
	if s.mediaFolders == nil {
		s.internalError(w, "メディアフォルダの経路が設定されていません", nil)
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
		s.invalidRequest(w, "id、path、versionを正しく指定してください")
		return
	}
	if s.mediaFolders == nil {
		s.internalError(w, "メディアフォルダの経路が設定されていません", nil)
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
		s.invalidRequest(w, "idとversionを正しく指定してください")
		return
	}
	if s.mediaFolders == nil {
		s.internalError(w, "メディアフォルダの経路が設定されていません", nil)
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
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		s.invalidRequest(w, "Content-Typeはapplication/jsonを指定してください")
		return false
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		s.invalidRequest(w, "JSONを解釈できません")
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		s.invalidRequest(w, "JSONは1件だけ指定してください")
		return false
	}
	return true
}

func (s *server) acceptsSameOrigin(w http.ResponseWriter, r *http.Request) bool {
	if strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site") {
		s.writeError(w, http.StatusForbidden, codeForbidden, "same-originの操作だけを受け付けます")
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	expectedScheme := "http"
	if r.TLS != nil {
		expectedScheme = "https"
	}
	if err != nil || parsed.Scheme != expectedScheme || !strings.EqualFold(parsed.Host, r.Host) {
		s.writeError(w, http.StatusForbidden, codeForbidden, "same-originの操作だけを受け付けます")
		return false
	}
	return true
}

func (s *server) writeMediaFolderError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidMediaFolder):
		s.writeError(w, http.StatusBadRequest, codeInvalidMediaDirectory, "選択したディレクトリを利用できません")
	case errors.Is(err, domain.ErrUnsupportedMediaFolder):
		s.writeError(w, http.StatusBadRequest, codeUnsupportedMediaDirectory, "そのディレクトリは選択できません")
	case errors.Is(err, domain.ErrNotFound):
		s.writeError(w, http.StatusNotFound, codeMediaFolderNotFound, "メディアフォルダが見つかりません")
	case errors.Is(err, domain.ErrFolderConflict):
		s.writeError(w, http.StatusConflict, codeOverlappingMediaDirectories, "登録済みフォルダと重複または包含しています")
	case errors.Is(err, domain.ErrScanRunning):
		s.writeError(w, http.StatusConflict, codeScanInProgress, "取り込み中はメディアフォルダを変更できません")
	case errors.Is(err, domain.ErrVersionConflict):
		s.writeError(w, http.StatusConflict, codeConflict, "メディアフォルダが別の操作で変更されました")
	default:
		s.internalError(w, "メディアフォルダを変更できませんでした", err)
	}
}
