package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

const peerExcludedFoldersHeader = "X-VV-Peer-Excluded-Folders"

// peerExcludedFolders reads the reader's switched-off folders from X-VV-Peer-Excluded-Folders: none when absent,
// false when the value is not one comma-separated list of positive ids.
func peerExcludedFolders(r *http.Request) ([]int64, bool) {
	values := r.Header.Values(peerExcludedFoldersHeader)
	switch len(values) {
	case 0:
		return nil, true
	case 1:
	default:
		return nil, false
	}
	parts := strings.Split(values[0], ",")
	ids := make([]int64, len(parts))
	for i, part := range parts {
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err != nil || id < 1 {
			return nil, false
		}
		ids[i] = id
	}
	return ids, true
}

// ListPeerMediaFolders answers the reading node the folders it keeps switches for; the usage and edits of
// listMediaFolders stay with this node's owner.
func (s *server) ListPeerMediaFolders(w http.ResponseWriter, r *http.Request) {
	if !s.requirePeerCaller(w, r) {
		return
	}
	folders, ok := s.loadMediaFolders(w, r)
	if !ok {
		return
	}
	out := make([]gen.PeerFolder, len(folders))
	for i, folder := range folders {
		out[i] = gen.PeerFolder{Id: folder.ID, Path: folder.Path, Version: folder.Version}
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, out, s.logger)
}

func (s *server) ListPeerLinkMediaFolders(w http.ResponseWriter, r *http.Request, nodeID string) {
	if !s.requirePairing(w) {
		return
	}
	folders, err := s.pairing.LinkFolders(r.Context(), nodeID)
	if errors.Is(err, domain.ErrNotFound) {
		s.notFound(w, "This VVMDM does not read that one.")
		return
	}
	if err != nil {
		s.internalError(w, "Could not read the media folders of the read VVMDM.", err)
		return
	}
	body := gen.PeerLinkMediaFolders{
		State: gen.PeerLinkMediaFoldersState(folders.State), Folders: make([]gen.PeerMediaFolder, len(folders.Folders)),
	}
	if folders.Reason != "" {
		reason := gen.PeerLinkMediaFoldersReason(folders.Reason)
		body.Reason = &reason
	}
	for i, folder := range folders.Folders {
		body.Folders[i] = peerMediaFolderBody(folder)
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, body, s.logger)
}

func (s *server) SetPeerLinkMediaFolderEnabled(w http.ResponseWriter, r *http.Request, nodeID string, folderID int64) {
	enabled, ok := s.readSwitchBody(w, r)
	if !ok || !s.requirePairing(w) {
		return
	}
	folder, err := s.pairing.SetLinkFolderEnabled(r.Context(), nodeID, folderID, enabled)
	if errors.Is(err, domain.ErrNotFound) {
		s.notFound(w, "The read VVMDM has no such media folder.")
		return
	}
	if err != nil {
		s.internalError(w, "Could not switch the media folder of the read VVMDM.", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, peerMediaFolderBody(folder), s.logger)
}

func peerMediaFolderBody(folder domain.PeerMediaFolder) gen.PeerMediaFolder {
	return gen.PeerMediaFolder{
		Id: folder.FolderID, Path: folder.Path, Enabled: folder.Enabled, RefreshedAt: folder.RefreshedAt.UTC(),
	}
}
