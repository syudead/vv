package httpapi

import (
	"net/http"
	"net/url"
	"path"
)

// peerOperations are the only operations the peer credential reaches: the reads that merging and
// forwarding need, media, forwarded edits, operations over a selection, events, health and the
// peer-to-peer operations. Media folders, scans, settings, the account, API tokens, activity,
// favorites, playlists and the pairing screens stay with each node's own owner.
var peerOperations = []string{
	"GET /api/health",
	"GET /api/events",

	"GET /api/videos",
	"GET /api/library",
	"GET /api/videos/{id}",
	"GET /api/videos/{id}/related",
	"GET /api/videos/{id}/group-members",
	"GET /api/videos/{id}/versions",
	"GET /api/videos/{id}/stream",
	"GET /api/videos/{id}/preview",
	"GET /api/videos/{id}/transcode.mp4",
	"GET /api/videos/{id}/transcode-start",
	"GET /api/videos/{id}/subtitles",
	"GET /api/videos/{id}/subtitles/{file}",
	"GET /api/videos/{id}/thumbnail",
	"GET /api/videos/{id}/seek-thumbnail",
	"GET /api/videos/{id}/seek-thumbnail/{sheet}",
	"GET /api/videos/{id}/frame",
	"GET /api/folders",
	"GET /api/folders/{rootId}",
	"GET /api/folders/{rootId}/videos",
	"GET /api/groups",
	"GET /api/groups/{id}",
	"GET /api/groups/{id}/items",
	"GET /api/smart-folders",
	"GET /api/smart-folders/cards",
	"GET /api/smart-folders/{id}",
	"GET /api/smart-folders/{id}/items",
	"GET /api/tags",
	"GET /api/tag-kinds",
	"GET /api/people",
	"GET /api/people/{id}",
	"GET /api/people/{id}/image",
	"GET /api/organizations",
	"GET /api/organizations/{id}",

	"PUT /api/videos/{id}/display-name",
	"PUT /api/videos/{id}/work-info",
	"PUT /api/videos/{id}/details",
	"PUT /api/videos/{id}/thumbnail-position",
	"POST /api/videos/{id}/tags/{tagId}/confirm",
	"POST /api/videos/{id}/tags/{tagId}/reject",
	"POST /api/groups",
	"PATCH /api/groups/{id}",
	"DELETE /api/groups/{id}",
	"POST /api/groups/{id}/items",
	"POST /api/groups/{id}/items/remove",
	"POST /api/groups/{id}/items/{itemId}/move",
	"POST /api/groups/{id}/tag",
	"POST /api/organize/reorder",
	"POST /api/organize/group",
	"POST /api/organize/move",
	"POST /api/organize/merge",
	"POST /api/organize/bundle",
	"POST /api/organize/undo",
	"POST /api/video-bundles",
	"POST /api/smart-folders",
	"PATCH /api/smart-folders/{id}",
	"DELETE /api/smart-folders/{id}",
	"PATCH /api/tags/{id}",
	"DELETE /api/tags/{id}",
	"POST /api/tags/{id}/merge",
	"PATCH /api/people/{id}",
	"DELETE /api/people/{id}",
	"POST /api/people/{id}/merge",
	"PATCH /api/organizations/{id}",
	"DELETE /api/organizations/{id}",
	"POST /api/organizations/{id}/merge",

	"POST /api/video-tags",
	"POST /api/video-tags/summary",
	"PUT /api/video-visibility",
	"POST /api/video-edits",
	"POST /api/video-edits/summary",
	"POST /api/video-code-edits",
	"POST /api/video-code-edits/summary",
	"POST /api/collections/delete",

	"POST /api/peer/pairings",
	"GET /api/peer/handshake",
	"DELETE /api/peer/pairings/{nodeId}",
	"POST /api/peer/related",
	"POST /api/peer/vocabulary",
	"GET /api/peer/videos/{id}/clues",
}

// Match with ServeMux, as accessMux does, so wildcards and GET/HEAD agree with routing.
var peerOperationMux = func() *http.ServeMux {
	mux := http.NewServeMux()
	for _, pattern := range peerOperations {
		mux.Handle(pattern, http.NotFoundHandler())
	}
	return mux
}()

// peerAllowed runs after classifyRequest accepted the path as an API route.
func peerAllowed(r *http.Request) bool {
	return routePattern(peerOperationMux, r) != ""
}

// routePattern is the pattern of mux that r's method and cleaned path match, or empty.
func routePattern(mux *http.ServeMux, r *http.Request) string {
	probe := &http.Request{Method: r.Method, URL: &url.URL{Path: path.Clean("/" + r.URL.Path)}, Host: r.Host}
	_, pattern := mux.Handler(probe)
	return pattern
}
