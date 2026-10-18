package httpapi

import (
	"context"
	"net/http"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

type FolderGroups interface {
	FolderGroupings(ctx context.Context, folderPaths []string) ([]domain.FolderGrouping, error)
	SetFolderGrouping(ctx context.Context, folderPath string, mode domain.FolderGroupMode) (domain.FolderGrouping, error)
}

func (s *server) SetFolderGrouping(w http.ResponseWriter, r *http.Request, rootID gen.FolderRootId, params gen.SetFolderGroupingParams) {
	root, rel, ok := s.resolveFolderRoot(w, r, rootID, params.Path)
	if !ok {
		return
	}
	// Track required mode presence so omission cannot silently become automatic grouping.
	var body struct {
		Mode *gen.FolderGroupingMode `json:"mode"`
	}
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if body.Mode == nil || !body.Mode.Valid() {
		s.invalidRequest(w, "mode must be one of auto, ungroup, or groupDirect.")
		return
	}
	dir, ok := s.existingFolderDir(w, r, root, rel)
	if !ok {
		return
	}
	if s.folderGroups == nil {
		s.internalError(w, "Folder grouping storage is not configured.", nil)
		return
	}

	grouping, err := s.folderGroups.SetFolderGrouping(r.Context(), dir, domainFolderGroupMode(*body.Mode))
	if err != nil {
		s.folderError(w, r, "Could not change the folder grouping.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, apiFolderGrouping(grouping), s.logger)
}

// Roots exist without videos; other folders require indexed locations, matching folder reads.
func (s *server) existingFolderDir(w http.ResponseWriter, r *http.Request, root domain.MediaFolder, rel string) (string, bool) {
	dir := domain.FolderDir(root.Path, rel)
	if rel == "" {
		return dir, true
	}
	found, err := s.folders.HasFolderLocations(r.Context(), audienceFrom(r.Context()), dir)
	if err != nil {
		s.folderError(w, r, "Could not load folders.", err)
		return "", false
	}
	if !found {
		s.notFoundReason(w, reasonFolderNotFound, folderNotFoundMessage)
		return "", false
	}
	return dir, true
}

// Return groupings in folder order for owners only; nil omits grouping for guests or missing storage.
func (s *server) folderGroupings(ctx context.Context, audience domain.Audience, folders []domain.FolderSummary) ([]*gen.FolderGrouping, error) {
	if !audience.IsOwner() || s.folderGroups == nil {
		return nil, nil
	}
	dirs := make([]string, len(folders))
	for i, folder := range folders {
		dirs[i] = domain.FolderDir(folder.RootPath, folder.Path)
	}
	groupings, err := s.folderGroups.FolderGroupings(ctx, dirs)
	if err != nil {
		return nil, err
	}
	out := make([]*gen.FolderGrouping, len(folders))
	for i, grouping := range groupings {
		view := apiFolderGrouping(grouping)
		out[i] = &view
	}
	return out, nil
}

func apiFolderGrouping(grouping domain.FolderGrouping) gen.FolderGrouping {
	mode := gen.FolderGroupingModeAuto
	switch grouping.Mode {
	case domain.FolderGroupUngroup:
		mode = gen.FolderGroupingModeUngroup
	case domain.FolderGroupDirect:
		mode = gen.FolderGroupingModeGroupDirect
	}
	out := gen.FolderGrouping{Mode: mode}
	if grouping.GroupID != 0 {
		out.GroupId = &grouping.GroupID
	}
	return out
}

// auto maps to an empty override.
func domainFolderGroupMode(mode gen.FolderGroupingMode) domain.FolderGroupMode {
	switch mode {
	case gen.FolderGroupingModeUngroup:
		return domain.FolderGroupUngroup
	case gen.FolderGroupingModeGroupDirect:
		return domain.FolderGroupDirect
	default:
		return ""
	}
}
