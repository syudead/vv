package httpapi

import (
	"errors"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// ListDirectories は設定画面のディレクトリ選択に、サーバーのディレクトリを返す
// （GET /api/directories）。どのディレクトリを並べるかは MediaFiles が決め、
// ここは誤りを応答の状態へ移すだけである。
func (s *server) ListDirectories(w http.ResponseWriter, _ *http.Request, params gen.ListDirectoriesParams) {
	if s.files == nil {
		s.internalError(w, "ディレクトリの読み取りが設定されていません", nil)
		return
	}
	var listing domain.DirectoryListing
	if params.Path == nil {
		listing = s.files.DirectoryRoots()
	} else {
		var err error
		listing, err = s.files.ListDirectories(*params.Path)
		switch {
		case errors.Is(err, domain.ErrInvalidDirectoryPath):
			s.writeError(w, http.StatusBadRequest, codeInvalidRequest, err.Error())
			return
		case errors.Is(err, domain.ErrDirectoryNotFound):
			s.writeError(w, http.StatusNotFound, codeNotFound, err.Error())
			return
		case err != nil:
			s.writeError(w, http.StatusBadRequest, codeDirectoryUnavailable, domain.ErrDirectoryUnavailable.Error())
			return
		}
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, directoryListingResponse(listing), s.logger)
}

func directoryListingResponse(listing domain.DirectoryListing) gen.DirectoryListing {
	directories := make([]gen.DirectoryEntry, 0, len(listing.Directories))
	for _, entry := range listing.Directories {
		directories = append(directories, gen.DirectoryEntry{Name: entry.Name, Path: entry.Path})
	}
	response := gen.DirectoryListing{Directories: directories}
	if listing.CurrentPath != "" {
		response.CurrentPath = &listing.CurrentPath
	}
	if listing.ParentPath != "" {
		response.ParentPath = &listing.ParentPath
	}
	return response
}
