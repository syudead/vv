package httpapi

import (
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// withPeerRoots adds the peer's media-folder roots to local in name order, local first on equal names; a peer
// that does not answer leaves the local roots alone.
func (s *server) withPeerRoots(r *http.Request, local []gen.FolderSummary) []gen.FolderSummary {
	var answer gen.RootFolderListing
	if err := s.peerGet(r, "/api/folders", url.Values{}, &answer); err != nil {
		s.logger.Info("the folder list leaves out the other VVMDM", slog.Any("error", err))
		return local
	}
	for i := range answer.Folders {
		rewriteRemoteFolderSummary(&answer.Folders[i])
	}
	folders := append(local, answer.Folders...)
	slices.SortStableFunc(folders, func(a, b gen.FolderSummary) int {
		if order := domain.CompareNatural(a.Name, b.Name); order != 0 {
			return order
		}
		return strings.Compare(a.Name, b.Name)
	})
	return folders
}

// forwardRemoteFolder forwards a read under one of the peer's media-folder roots; a folder belongs to one node's
// disk, so its listing is the peer's alone.
func (s *server) forwardRemoteFolder(
	w http.ResponseWriter, r *http.Request, rootID int64, suffix string, query url.Values, rewrite remoteRewrite,
) {
	// The peer's own request with a remote id names this node's folder; forwarding it back would bounce.
	if isPeerRequest(r.Context()) {
		s.notFoundReason(w, reasonFolderNotFound, folderNotFoundMessage)
		return
	}
	out, err := remoteRequest(r, "/api/folders/"+strconv.FormatInt(domain.FromRemoteID(rootID), 10)+suffix, query)
	if err != nil {
		s.invalidRequest(w, "The request cannot be forwarded to the other VVMDM.")
		return
	}
	s.forward(w, r, out, remoteOperation{rewrite: rewrite})
}

// listRemoteFolderVideos forwards the videos of the peer's folder with the filters in the peer's terms and the
// peer's own cursor.
func (s *server) listRemoteFolderVideos(w http.ResponseWriter, r *http.Request, rootID int64, params gen.ListFolderVideosParams) {
	query, ok := s.parseVideoQuery(w, audienceFrom(r.Context()), folderVideoQueryParams(params))
	if !ok {
		return
	}
	filters, err := s.splitPeerFilters(r.Context(), query)
	if err != nil {
		s.internalError(w, "Could not load the folder's videos.", err)
		return
	}
	if filters.peerEmpty && !isPeerRequest(r.Context()) {
		w.Header().Set("Cache-Control", cacheNoStore)
		writeJSON(w, http.StatusOK, gen.VideoPage{Items: []gen.Video{}}, s.logger)
		return
	}
	s.forwardRemoteFolder(w, r, rootID, "/videos", peerQuery(r.URL.Query(), query.Sort, filters.values),
		rewriteAs(rewriteRemoteVideoPage))
}
