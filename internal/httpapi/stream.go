package httpapi

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// Media responses must not be shared across users even for identical content.
const cacheStream = "private, max-age=0, must-revalidate"

// Use extensions for Content-Type because ffprobe conflates MOV/MP4; playability uses the same rule.
var streamContentTypes = map[string]string{
	".mp4":  "video/mp4",
	".m4v":  "video/mp4",
	".webm": "video/webm",
}

const defaultStreamContentType = "application/octet-stream"

// Delegate Range handling to ServeContent and preserve ReaderFrom/sendfile through wrappers; streaming remains available for browser-incompatible formats.
func (s *server) StreamVideo(w http.ResponseWriter, r *http.Request, id gen.VideoId) {
	// Register guest streams for cancellation when visibility becomes private.
	video, r, release, ok := s.lookupServedVideo(w, r, id)
	if !ok {
		return
	}
	defer release()

	if s.files == nil {
		s.internalError(w, "Media file access is not configured.", nil)
		return
	}
	file, info, path, ok := s.openMediaFile(r, video)
	if !ok {
		// Mask missing, outside-root, and nonregular targets identically to avoid leaking path existence.
		s.notFoundReason(w, reasonFileUnavailable, "Cannot open this video's file.")
		return
	}
	defer func() { _ = file.Close() }()

	w.Header().Set("Content-Type", streamContentType(path))
	w.Header().Set("Cache-Control", cacheStream)

	http.ServeContent(w, r, filepath.Base(path), info.ModTime(), file)
}

// openMediaFile returns the unresolved location path for filename/Content-Type; MediaFiles validates the opened target.
func (s *server) openMediaFile(r *http.Request, video domain.Video) (*os.File, os.FileInfo, string, bool) {
	var (
		file *os.File
		info os.FileInfo
	)
	path, ok := s.firstMediaLocation(r, video, func(roots []string, path string) error {
		var err error
		file, info, err = s.files.OpenMediaFile(roots, path)
		return err
	})
	if !ok {
		return nil, nil, "", false
	}
	return file, info, path, true
}

// resolveMediaFile returns the validated symlink target for subprocesses plus the original location for later ownership revalidation.
func (s *server) resolveMediaFile(r *http.Request, video domain.Video) (location, resolved string, ok bool) {
	location, ok = s.firstMediaLocation(r, video, func(roots []string, path string) error {
		var err error
		resolved, err = s.files.ResolveMediaFile(roots, path)
		return err
	})
	if !ok {
		return "", "", false
	}
	return location, resolved, true
}

func (s *server) firstMediaLocation(
	r *http.Request, video domain.Video, try func(roots []string, path string) error,
) (string, bool) {
	locations, err := s.videos.VideoLocations(r.Context(), video.ID)
	if err != nil {
		return "", false
	}
	roots, err := s.mediaFolderPaths(r)
	if err != nil {
		return "", false
	}
	for _, location := range locations {
		err := try(roots, location.Path)
		if err == nil {
			return location.Path, true
		}
		if errors.Is(err, domain.ErrMediaFileOutsideRoot) {
			s.logger.Warn("link target is outside the media folders",
				slog.Int64("video", video.ID), slog.String("path", location.Path))
		} else {
			s.logger.Debug("cannot open the media file", slog.Int64("video", video.ID), slog.Any("error", err))
		}
	}
	return "", false
}

func (s *server) mediaFolderPaths(r *http.Request) ([]string, error) {
	folders, err := s.videos.ListMediaFolders(r.Context())
	if err != nil {
		return nil, err
	}
	roots := make([]string, 0, len(folders))
	for _, folder := range folders {
		roots = append(roots, folder.Path)
	}
	return roots, nil
}

func streamContentType(path string) string {
	if contentType, ok := streamContentTypes[strings.ToLower(filepath.Ext(path))]; ok {
		return contentType
	}
	return defaultStreamContentType
}
