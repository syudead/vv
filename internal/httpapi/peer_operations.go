package httpapi

import (
	"net/http"
	"net/url"
	"path"
	"slices"
)

// peerNonEntityOperations are the operations outside the entity collections that the peer credential reaches: the
// lists merging needs, creation, operations over a selection, events, health and the peer-to-peer operations.
var peerNonEntityOperations = []string{
	"GET /api/health",
	"GET /api/events",

	"GET /api/videos",
	"GET /api/library",
	"GET /api/folders",
	"GET /api/folders/{rootId}",
	"GET /api/folders/{rootId}/videos",
	"GET /api/groups",
	"GET /api/smart-folders",
	"GET /api/smart-folders/cards",
	"GET /api/smart-folders/{id}",
	"GET /api/smart-folders/{id}/items",
	"GET /api/tags",
	"GET /api/tag-kinds",
	"GET /api/people",
	"GET /api/organizations",

	"POST /api/groups",
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
	"POST /api/people",
	"POST /api/organizations",

	"POST /api/video-tags",
	"POST /api/video-tags/summary",
	"PUT /api/video-visibility",
	"POST /api/video-edits",
	"POST /api/video-edits/summary",
	"POST /api/video-code-edits",
	"POST /api/video-code-edits/summary",
	"POST /api/work-info-fill-targets",
	"POST /api/collections/delete",

	"POST /api/peer/pairings",
	"GET /api/peer/handshake",
	"DELETE /api/peer/pairings/{nodeId}",
	"POST /api/peer/related",
	"POST /api/peer/vocabulary",
	"GET /api/peer/videos/{id}/clues",
	"POST /api/peer/rule-counts",
	"POST /api/peer/credit-matches",
	"POST /api/peer/videos/{id}/fetched-work-info",
	"POST /api/peer/people/{id}/fetched-person",
}

// peerOperations are the only operations the peer credential reaches: the non-entity ones above and the entity
// operations forwarded for a remote id or called on the peer by this node. Media folders, scans, settings, the
// account, API tokens, activity, favorites, playlists and the pairing screens stay with each node's own owner.
var peerOperations = slices.Concat(peerNonEntityOperations, remoteRoutes.peer)

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
