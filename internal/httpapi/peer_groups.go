package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

const (
	remoteGroupPrefix = "/api/groups/"
	// remoteUndoPrefix marks an undo token the peer issued; this node's tokens are base64url and hold no dot.
	remoteUndoPrefix = "peer."
)

// errMixedNodes is a write that names entities of both nodes; a group never holds both nodes' videos.
var errMixedNodes = errors.New("the request names entities of both nodes")

// remoteGroupOperations are the operations under /api/groups/{id} offered for the peer's groups.
var remoteGroupOperations = map[string]remoteOperation{
	"GET /api/groups/{id}":       {rewrite: rewriteAs(rewriteRemoteGroupDetail)},
	"GET /api/groups/{id}/items": {rewrite: rewriteAs(rewriteRemoteGroupItemPage)},
	"PATCH /api/groups/{id}":     {rewrite: rewriteAs(rewriteRemoteGroup), body: remoteBodyAs(peerGroupRename)},
	"DELETE /api/groups/{id}":    {},
	"POST /api/groups/{id}/items": {
		rewrite: rewriteAs(rewriteRemoteGroup), body: remoteBodyAs(peerGroupMembers),
	},
	"POST /api/groups/{id}/items/remove": {
		rewrite: rewriteAs(rewriteRemoteGroup), body: remoteBodyAs(peerGroupItemRemove),
	},
	"POST /api/groups/{id}/items/{itemId}/move": {body: remoteBodyAs(peerGroupItemMove)},
	"POST /api/groups/{id}/tag":                 {rewrite: rewriteAs(rewriteRemoteGroupTag)},
}

var remoteGroupMux = operationMux(remoteGroupOperations)

var (
	remoteGroupAnswer    = remoteOperation{rewrite: rewriteAs(rewriteRemoteGroup)}
	remoteOrganizeAnswer = remoteOperation{rewrite: rewriteAs(rewriteRemoteOrganizeResult)}
)

func (s *server) forwardRemoteGroup(w http.ResponseWriter, r *http.Request, id int64, rest string) {
	op, offered := remoteGroupOperations[routePattern(remoteGroupMux, r)]
	// The peer's own request with a remote id names this node's group; forwarding it back would bounce.
	if !offered || isPeerRequest(r.Context()) {
		s.notFound(w, "The group does not exist.")
		return
	}
	rest, ok := peerGroupItemPath(rest)
	if !ok {
		s.mixedNodes(w)
		return
	}
	query := r.URL.Query()
	if query.Has("trail") {
		query.Set("trail", peerTrail(query.Get("trail")))
	}
	out, err := remoteRequest(r, remoteGroupPrefix+strconv.FormatInt(domain.FromRemoteID(id), 10)+rest, query)
	if err != nil {
		s.invalidRequest(w, "The request cannot be forwarded to the other VVMDM.")
		return
	}
	if op.body != nil && !s.forwardedBody(w, r, out, op.body) {
		return
	}
	s.forward(w, r, out, op)
}

// peerGroupItemPath turns the item id of /items/{itemId}/move into the peer's; false when it names a local item.
func peerGroupItemPath(rest string) (string, bool) {
	parts := strings.Split(rest, "/")
	if len(parts) != 4 || parts[1] != "items" || parts[3] != "move" {
		return rest, true
	}
	id, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return rest, true
	}
	if local, _ := domain.IDNodes(id); local {
		return "", false
	}
	parts[2] = strconv.FormatInt(peerID(id), 10)
	return strings.Join(parts, "/"), true
}

// peerTrail keeps the peer's groups of a trail; the trail of a remote group is the peer's own.
func peerTrail(raw string) string {
	var kept []string
	for value := range strings.SplitSeq(raw, ",") {
		if id, err := strconv.ParseInt(value, 10, 64); err == nil && domain.IsRemoteID(id) {
			kept = append(kept, strconv.FormatInt(domain.FromRemoteID(id), 10))
		}
	}
	return strings.Join(kept, ",")
}

// forwardedBody gives out r's body in the peer's ids, answering the request itself when it cannot.
func (s *server) forwardedBody(w http.ResponseWriter, r *http.Request, out *http.Request, convert remoteBody) bool {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, groupsBodyLimit))
	if err == nil {
		raw, err = convert(raw)
	}
	switch {
	case errors.Is(err, errMixedNodes):
		s.mixedNodes(w)
		return false
	case err != nil:
		s.invalidRequest(w, "The request body is not valid JSON.")
		return false
	}
	out.Body, out.ContentLength = io.NopCloser(bytes.NewReader(raw)), int64(len(raw))
	out.Header.Set("Content-Type", "application/json")
	return true
}

func remoteBodyAs[T any](toPeer func(*T) bool) remoteBody {
	return func(body []byte) ([]byte, error) {
		var value T
		if err := json.Unmarshal(body, &value); err != nil {
			return nil, err
		}
		if !toPeer(&value) {
			return nil, errMixedNodes
		}
		return json.Marshal(value)
	}
}

// peerGroupRename keeps only an undo token the peer issued, since a local token cannot join the peer's undo.
func peerGroupRename(body *gen.GroupRename) bool {
	if body.UndoToken != nil {
		token, ok := strings.CutPrefix(*body.UndoToken, remoteUndoPrefix)
		body.UndoToken = nil
		if ok {
			body.UndoToken = &token
		}
	}
	return true
}

func peerGroupMembers(body *gen.GroupMembers) bool {
	return onlyRemote(optionalIDs(body.VideoIds), optionalIDs(body.GroupIds)) && toPeer(body.VideoIds, body.GroupIds)
}

func peerGroupItemRemove(body *gen.GroupItemRemove) bool {
	return onlyRemote(body.ItemIds) && toPeer(&body.ItemIds)
}

func peerGroupItemMove(body *gen.GroupItemMove) bool {
	return onlyRemote([]int64{optionalID(body.After)}) && toPeerID(body.After)
}

// onlyRemote reports whether no positive id names this node's entity.
func onlyRemote(groups ...[]int64) bool {
	local, _ := domain.IDNodes(slices.Concat(groups...)...)
	return !local
}

// toPeer turns the remote ids of each list into the peer's in place and reports true, for chaining.
func toPeer(lists ...*[]int64) bool {
	for _, ids := range lists {
		if ids != nil {
			domain.ToPeerIDs(*ids)
		}
	}
	return true
}

// toPeerID turns each set remote id into the peer's in place and reports true, for chaining.
func toPeerID(ids ...*int64) bool {
	for _, id := range ids {
		if id != nil {
			*id = peerID(*id)
		}
	}
	return true
}

func peerID(id int64) int64 {
	if domain.IsRemoteID(id) {
		return domain.FromRemoteID(id)
	}
	return id
}

func (s *server) mixedNodes(w http.ResponseWriter) {
	s.invalidRequestReason(w, gen.ErrorReasonMixedNodes,
		"Videos and groups of both VVMDMs cannot be organized together. Select those of one VVMDM.")
}

// writesLocally reports whether a write over ids runs on this node. A write whose ids are all the peer's is
// forwarded by forward, and one naming both nodes is refused with mixed_nodes; the peer's own request is
// always local, so a forwarded write never comes back.
func (s *server) writesLocally(w http.ResponseWriter, r *http.Request, forward func(), ids ...int64) bool {
	if isPeerRequest(r.Context()) {
		return true
	}
	local, remote := domain.IDNodes(ids...)
	switch {
	case !remote:
		return true
	case local:
		s.mixedNodes(w)
	default:
		forward()
	}
	return false
}

// forwardJSON sends body, already in the peer's ids, to path on the peer with r's method.
func (s *server) forwardJSON(w http.ResponseWriter, r *http.Request, path string, body any, op remoteOperation) {
	raw, err := json.Marshal(body)
	if err != nil {
		s.internalError(w, "Could not forward the request to the other VVMDM.", err)
		return
	}
	out, err := http.NewRequestWithContext(r.Context(), r.Method, path, bytes.NewReader(raw))
	if err != nil {
		s.internalError(w, "Could not forward the request to the other VVMDM.", err)
		return
	}
	out.Header.Set("Content-Type", "application/json")
	s.forward(w, r, out, op)
}

func (s *server) createsGroupLocally(w http.ResponseWriter, r *http.Request, body *gen.GroupCreate) bool {
	return s.writesLocally(w, r, func() {
		toPeer(body.VideoIds, body.GroupIds)
		s.forwardJSON(w, r, "/api/groups", body, remoteGroupAnswer)
	}, slices.Concat(optionalIDs(body.VideoIds), optionalIDs(body.GroupIds))...)
}

// addsGroupItemsLocally refuses the peer's videos or groups for a local group; a remote group was forwarded
// before routing.
func (s *server) addsGroupItemsLocally(w http.ResponseWriter, r *http.Request, id int64, body *gen.GroupMembers) bool {
	return s.writesLocally(w, r, func() { s.mixedNodes(w) },
		slices.Concat([]int64{id}, optionalIDs(body.VideoIds), optionalIDs(body.GroupIds))...)
}

func (s *server) bundlesLocally(w http.ResponseWriter, r *http.Request, body *gen.VideoBundleRequest) bool {
	return s.writesLocally(w, r, func() {
		toPeer(&body.VideoIds)
		toPeerID(&body.RepresentativeId)
		s.forwardJSON(w, r, "/api/video-bundles", body, remoteOperation{rewrite: rewriteAs(rewriteRemoteVersions)})
	}, append([]int64{body.RepresentativeId}, body.VideoIds...)...)
}

func (s *server) reordersLocally(w http.ResponseWriter, r *http.Request, body *gen.OrganizeReorder) bool {
	return s.writesLocally(w, r, func() {
		toPeer(&body.ItemIds)
		toPeerID(&body.GroupId, body.After)
		s.forwardJSON(w, r, "/api/organize/reorder", body, remoteOrganizeAnswer)
	}, append([]int64{body.GroupId, optionalID(body.After)}, body.ItemIds...)...)
}

func (s *server) organizesGroupLocally(w http.ResponseWriter, r *http.Request, body *gen.OrganizeGroup) bool {
	return s.writesLocally(w, r, func() {
		toPeerRefs(body.Items)
		toPeerID(&body.Target, body.From)
		s.forwardJSON(w, r, "/api/organize/group", body, remoteOrganizeAnswer)
	}, append(refIDs(body.Items), body.Target, optionalID(body.From))...)
}

func (s *server) movesLocally(w http.ResponseWriter, r *http.Request, body *gen.OrganizeMove) bool {
	return s.writesLocally(w, r, func() {
		toPeerRefs(body.Items)
		toPeerID(body.From, body.To)
		s.forwardJSON(w, r, "/api/organize/move", body, remoteOrganizeAnswer)
	}, append(refIDs(body.Items), optionalID(body.From), optionalID(body.To))...)
}

func (s *server) mergesLocally(w http.ResponseWriter, r *http.Request, body *gen.OrganizeMerge) bool {
	return s.writesLocally(w, r, func() {
		toPeerRefs(body.Items)
		toPeerID(&body.Into, body.From)
		s.forwardJSON(w, r, "/api/organize/merge", body, remoteOrganizeAnswer)
	}, append(refIDs(body.Items), body.Into, optionalID(body.From))...)
}

func (s *server) organizesBundleLocally(w http.ResponseWriter, r *http.Request, body *gen.OrganizeBundle) bool {
	return s.writesLocally(w, r, func() {
		toPeer(&body.VideoIds)
		toPeerID(&body.Target)
		s.forwardJSON(w, r, "/api/organize/bundle", body, remoteOrganizeAnswer)
	}, append([]int64{body.Target}, body.VideoIds...)...)
}

// undoesLocally forwards the undo of a token the peer issued.
func (s *server) undoesLocally(w http.ResponseWriter, r *http.Request, body *gen.OrganizeUndo) bool {
	token, remote := strings.CutPrefix(body.Token, remoteUndoPrefix)
	if !remote || isPeerRequest(r.Context()) {
		return true
	}
	body.Token = token
	s.forwardJSON(w, r, "/api/organize/undo", body, remoteOperation{})
	return false
}

func refIDs(refs gen.OrganizeRefs) []int64 {
	ids := make([]int64, 0, len(refs)+2)
	for _, ref := range refs {
		ids = append(ids, ref.Id)
	}
	return ids
}

func toPeerRefs(refs gen.OrganizeRefs) {
	for i := range refs {
		refs[i].Id = peerID(refs[i].Id)
	}
}

// listPeerGroups forwards the group picker's list to the peer with the peer's own cursor.
func (s *server) listPeerGroups(w http.ResponseWriter, r *http.Request) {
	if isPeerRequest(r.Context()) {
		s.invalidRequest(w, "node=peer names the VVMDM that sent the request.")
		return
	}
	query := r.URL.Query()
	query.Del("node")
	out, err := remoteRequest(r, "/api/groups", query)
	if err != nil {
		s.invalidRequest(w, "The request cannot be forwarded to the other VVMDM.")
		return
	}
	s.forward(w, r, out, remoteOperation{rewrite: rewriteAs(rewriteRemoteGroupList)})
}

// deletePeerCollections removes the peer's part of a collections delete, in the peer's own ids, and reports it;
// a failure leaves the local part applied and names every requested remote id.
func (s *server) deletePeerCollections(r *http.Request, del domain.CollectionDelete) (domain.CollectionsDeleted, *gen.RemoteSelectionResult) {
	var answer gen.CollectionsDeleteResponse
	err := s.peerCall(r, http.MethodPost, "/api/collections/delete", nil,
		gen.CollectionsDeleteRequest{GroupIds: &del.GroupIDs, SmartFolderIds: &del.SmartFolderIDs}, &answer)
	if err != nil {
		failed := slices.Concat(del.GroupIDs, del.SmartFolderIDs)
		for i, id := range failed {
			failed[i] = domain.ToRemoteID(id)
		}
		reason := remoteFailureReason(err)
		return domain.CollectionsDeleted{}, &gen.RemoteSelectionResult{FailedVideoIds: failed, Reason: &reason}
	}
	deleted := domain.CollectionsDeleted{DissolvedGroups: answer.DissolvedGroups, DeletedSmartFolders: answer.DeletedSmartFolders}
	return deleted, &gen.RemoteSelectionResult{
		Applied: deleted.DissolvedGroups + deleted.DeletedSmartFolders, FailedVideoIds: []int64{},
	}
}

// remoteFailureReason is the peer's reason for refusing, or peer_unreachable when it gave none.
func remoteFailureReason(err error) gen.ErrorReason {
	var refusal *peerRefusal
	if errors.As(err, &refusal) && refusal.reason != nil {
		return *refusal.reason
	}
	return gen.ErrorReasonPeerUnreachable
}
