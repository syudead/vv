package domain

import "errors"

var ErrInvalidRescanScope = errors.New("invalid force rescan scope")

// RescanScopeKind names what a force rescan reads again.
type RescanScopeKind string

const (
	RescanLibrary RescanScopeKind = "library"
	RescanFolder  RescanScopeKind = "folder"
	RescanVideo   RescanScopeKind = "video"
)

// RescanScope is the target of a force rescan. A folder scope carries FolderID and the folder's Path
// at start; a video scope carries VideoID; a library scope carries neither.
type RescanScope struct {
	Kind     RescanScopeKind
	FolderID int64
	Path     string
	VideoID  int64
}

// CarriesIssue reports whether an issue of the previous scan stays when this scope starts: the scope
// rediscovers only the issues of what it reads again.
func (s RescanScope) CarriesIssue(videoID int64, path string) bool {
	switch s.Kind {
	case RescanFolder:
		return !PathWithinRoot(s.Path, path)
	case RescanVideo:
		return videoID != s.VideoID
	default:
		return false
	}
}

// Partial reports whether the scope reads less than the whole library, so its scan cannot stand in
// for a full scan.
func (s RescanScope) Partial() bool {
	return s.Kind == RescanFolder || s.Kind == RescanVideo
}
