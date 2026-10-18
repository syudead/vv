package domain

import "errors"

// Callers may mask these errors uniformly to avoid disclosing path existence.
var (
	ErrMediaFileUnavailable = errors.New("media file is unavailable")
	// ErrMediaFileOutsideRoot also matches ErrMediaFileUnavailable.
	ErrMediaFileOutsideRoot = errors.New("media file link points outside the media folder")

	ErrInvalidDirectoryPath = errors.New("path must be absolute")
	// ErrDirectoryNotFound includes non-directories and paths traversing symlinks.
	ErrDirectoryNotFound    = errors.New("directory not found")
	ErrDirectoryUnavailable = errors.New("directory is unreadable")
)

type DirectoryEntry struct {
	Name string
	Path string
}

// Root listings have empty CurrentPath and ParentPath; top-level directories have empty ParentPath.
type DirectoryListing struct {
	CurrentPath string
	ParentPath  string
	Directories []DirectoryEntry
}
