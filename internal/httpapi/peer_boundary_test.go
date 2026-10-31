package httpapi

import (
	"slices"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/httpapi/gen"
)

// Every entity operation of openapi.yaml is forwarded, answered here or kept, and a forwarded one has a rewriter for
// its JSON answer and a translator for each id it carries.
func TestRemoteBoundaryDecidesEveryEntityOperation(t *testing.T) {
	for _, problem := range remoteRoutes.problems {
		t.Errorf("remote boundary: %s", problem)
	}
	for _, operation := range gen.ScreenOperations {
		pattern := operation.Method + " " + operation.Path
		route, found := remoteRoutes.routes[pattern]
		if isEntityOperation(operation.Path) && (!found || (route.op == nil && route.kept == nil)) {
			t.Errorf("%s: neither forwarded nor kept", pattern)
		}
	}
}

// The check names each missing rewriter and translator and a stale kept entry, and such an operation is not
// forwarded.
func TestRemoteBoundaryNamesWhatAForwardedOperationLacks(t *testing.T) {
	operations := []gen.ScreenOperation{
		{Method: "GET", Path: "/api/videos/{id}/answer", Response: "Unregistered"},
		{Method: "POST", Path: "/api/videos/{id}/body", Request: "Unregistered", BodyIDs: []string{"videoIds"}},
		{Method: "POST", Path: "/api/videos/{id}/items/{unregistered}", PathParams: []string{"unregistered"}},
		{Method: "PUT", Path: "/api/videos/{id}/display-name", Response: "Video", Request: "DisplayNameUpdate"},
		{Method: "GET", Path: "/api/media-folders/{id}/usage", Response: "Unregistered"},
	}
	kept := map[string]keptOperation{"POST /api/videos/{id}/gone": {reason: "test"}}

	table := buildRemoteRoutes(operations, kept)
	want := []string{
		"GET /api/videos/{id}/answer: no rewriter for Unregistered",
		"POST /api/videos/{id}/body: no translator for Unregistered (videoIds)",
		"POST /api/videos/{id}/gone: kept but not an entity operation",
		"POST /api/videos/{id}/items/{unregistered}: no translator for path parameter unregistered",
	}
	if !slices.Equal(table.problems, want) {
		t.Errorf("problems = %q, want %q", table.problems, want)
	}
	if got := table.peer; !slices.Equal(got, []string{"PUT /api/videos/{id}/display-name"}) {
		t.Errorf("forwarded = %q, want only the complete operation", got)
	}
}

// The peer credential is an allow-list that never reaches what stays with each node's own owner.
func TestPeerCredentialStaysOffEachNodesOwnOperations(t *testing.T) {
	declared := map[string]bool{}
	for _, operation := range gen.ScreenOperations {
		declared[operation.Method+" "+operation.Path] = true
	}
	ownerOnly := []string{
		"/api/media-folders", "/api/scans", "/api/settings", "/api/account", "/api/api-tokens", "/api/playlists",
		"/api/activity",
	}
	for _, pattern := range peerOperations {
		if !declared[pattern] {
			t.Errorf("peerOperations names %q, which openapi.yaml lacks", pattern)
		}
		_, route, _ := strings.Cut(pattern, " ")
		for _, prefix := range ownerOnly {
			if route == prefix || strings.HasPrefix(route, prefix+"/") {
				t.Errorf("peerOperations reaches %s", pattern)
			}
		}
		if kept, ok := keptOperations[pattern]; ok && !kept.peer {
			t.Errorf("peerOperations reaches %s, kept on each node: %s", pattern, kept.reason)
		}
	}
}
