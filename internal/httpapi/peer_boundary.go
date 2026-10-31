package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// remoteCollection is an entity collection whose path id names the peer's entity when it carries the offset.
type remoteCollection struct {
	prefix string
	noun   string
	// query, when set, turns the query of a forwarded request into the peer's and may replace op's rewrite.
	query func(pattern string, query url.Values, op *remoteOperation)
}

var remoteCollections = []remoteCollection{
	{prefix: remoteVideoPrefix, noun: "video"},
	{prefix: remoteGroupPrefix, noun: "group", query: peerGroupQuery},
	{prefix: "/api/tags/", noun: "tag"},
	{prefix: "/api/tag-kinds/", noun: "tag kind"},
	{prefix: "/api/people/", noun: "person", query: peerPersonQuery},
	{prefix: "/api/organizations/", noun: "organization"},
	{prefix: "/api/face-appearances/", noun: "face appearance"},
	{prefix: "/api/virtual-videos/", noun: "video"},
}

// remoteIDSegment is the index of the entity id in a path split at its slashes: "", "api", collection, id.
const remoteIDSegment = 3

// keptOperation is an entity operation that is not forwarded for a remote id, with the reason.
type keptOperation struct {
	reason string
	// here is set when this node answers the remote id itself; otherwise the remote id answers 404 not_found.
	here bool
	// peer is set when this node calls the operation on the peer in the peer's own ids, so the credential reaches it.
	peer bool
}

var (
	keptAnalysis      = keptOperation{reason: "Starts an analysis on the peer from here."}
	keptPlugin        = keptOperation{reason: "The plugin and its kept result are this node's.", here: true}
	keptVocabulary    = keptOperation{reason: "Applied on both nodes by name, whichever node the id names.", here: true, peer: true}
	keptThumbnailEdit = keptOperation{reason: "This node's ComfyUI edits a thumbnail; it does not read the peer's image."}
)

// keptOperations name every entity operation that is not forwarded for a remote id; any other is forwarded.
var keptOperations = map[string]keptOperation{
	"POST /api/videos/{id}/open":     {reason: "Opens the file in this node's default app."},
	"POST /api/videos/{id}/probe":    keptAnalysis,
	"POST /api/videos/{id}/auto-tag": keptAnalysis,
	"PUT /api/videos/{id}/progress":  {reason: "A playback position stays with the node that played it."},
	"GET /api/videos/{id}/related":   {reason: "Answered from both nodes' videos.", here: true, peer: true},

	"POST /api/videos/{id}/thumbnail-edits":       keptThumbnailEdit,
	"POST /api/videos/{id}/thumbnail-edits/image": keptThumbnailEdit,

	"POST /api/videos/{id}/work-info-fetches":                               keptPlugin,
	"POST /api/videos/{id}/work-info-fetches/{fetchId}/apply":               keptPlugin,
	"GET /api/videos/{id}/work-info-fetches/{fetchId}/covers/{index}":       keptPlugin,
	"GET /api/videos/{id}/work-info-fetches/{fetchId}/people/{index}/image": keptPlugin,
	"POST /api/people/{id}/plugin-fetches":                                  keptPlugin,
	"POST /api/people/{id}/plugin-fetches/{fetchId}/apply":                  keptPlugin,
	"GET /api/people/{id}/plugin-fetches/{fetchId}/images/{index}":          keptPlugin,

	"DELETE /api/tags/{id}":              keptVocabulary,
	"POST /api/tags/{id}/merge":          keptVocabulary,
	"DELETE /api/people/{id}":            keptVocabulary,
	"POST /api/people/{id}/merge":        keptVocabulary,
	"DELETE /api/organizations/{id}":     keptVocabulary,
	"POST /api/organizations/{id}/merge": keptVocabulary,
}

// slowRemoteOperations wait on the peer extracting a frame or fitting an image, so they have no response timeout,
// like media.
var slowRemoteOperations = map[string]bool{
	"PUT /api/videos/{id}/thumbnail-position": true,
	"POST /api/videos/{id}/thumbnail-image":   true,
	"DELETE /api/videos/{id}/thumbnail-image": true,
	"PUT /api/people/{id}/image":              true,
}

// remoteRewrites turn a forwarded answer into this node's ids by its response schema; a nil entry keeps an answer
// that holds no id as the peer sent it.
var remoteRewrites = map[string]remoteRewrite{
	"Video":                   rewriteAs(rewriteRemoteVideo),
	"VideoVersions":           rewriteAs(rewriteRemoteVersions),
	"GroupMemberPage":         rewriteAs(rewriteRemoteGroupMembers),
	"VideoFaceAppearanceList": rewriteAs(rewriteRemoteFaceAppearances),
	"SeekThumbnailSprite":     rewriteAs(rewriteRemoteSprite),
	"SubtitleTrackList":       nil,
	"TranscodeStart":          nil,
	"GroupDetail":             rewriteAs(rewriteRemoteGroupDetail),
	"GroupItemPage":           rewriteAs(rewriteRemoteGroupItemPage),
	"LibraryGroup":            rewriteAs(rewriteRemoteGroup),
	"GroupTagResult":          rewriteAs(rewriteRemoteGroupTag),
	"Tag":                     rewriteAs(rewriteRemoteTag),
	"TagKind":                 rewriteAs(rewriteRemoteTagKind),
	"Person":                  rewriteAs(rewriteRemotePerson),
	"Organization":            rewriteAs(rewriteRemoteOrganizationEntity),

	"FaceAppearanceList":        rewriteAs(rewriteRemotePersonAppearances),
	"FaceAppearanceMove":        rewriteAs(rewriteRemoteFaceMove),
	"FaceCandidateList":         rewriteAs(rewriteRemoteFaceCandidates),
	"FaceCandidateConfirmation": rewriteAs(rewriteRemoteFaceConfirmation),
	"SimilarPersonList":         rewriteAs(rewriteRemoteSimilarPeople),
}

// remoteRequestTranslator turns a request body into the peer's ids: body from the body alone, request with this
// node's and the peer's data, answering r and reporting false when it cannot.
type remoteRequestTranslator struct {
	body    remoteBody
	request func(s *server, w http.ResponseWriter, r *http.Request) bool
}

// remoteRequests translate a forwarded request body by its request schema. A schema with no id needs none.
var remoteRequests = map[string]remoteRequestTranslator{
	"WorkInfoInput": {request: peerWorkInfoBody("", func(body **gen.WorkInfoInput) *gen.WorkInfoInput { return *body })},
	"VideoDetailsInput": {
		request: peerWorkInfoBody("workInfo", func(body *remoteDetailsInput) *gen.WorkInfoInput { return body.WorkInfo }),
	},
	"GroupRename":          {body: remoteBodyAs(peerGroupRename)},
	"GroupMembers":         {body: remoteBodyAs(peerGroupMembers)},
	"GroupItemRemove":      {body: remoteBodyAs(peerGroupItemRemove)},
	"GroupItemMove":        {body: remoteBodyAs(peerGroupItemMove)},
	"VirtualVideoMerge":    {body: remoteBodyAs(peerVirtualVideoMerge)},
	"RenameTagRequest":     {request: (*server).checkRemoteTagRename},
	"CreditPatch":          {request: (*server).checkRemoteCreditPatch},
	"AddTagSynonymRequest": {request: (*server).peerTagSynonymBody},

	"FaceAppearanceMoveRequest":  {request: (*server).peerFaceMoveBody},
	"FaceAppearanceSplitRequest": {body: remoteBodyAs(peerFaceSplit)},
}

// remotePathParam turns one path segment after the entity id into the peer's, or answers r and reports false.
type remotePathParam func(s *server, w http.ResponseWriter, r *http.Request, segment string) (string, bool)

// remotePathParams translate a path parameter after the entity id by its name; a nil entry names no entity.
var remotePathParams = map[string]remotePathParam{
	"tagId":        (*server).peerVideoTagSegment,
	"itemId":       (*server).peerOffsetSegment,
	"appearanceId": (*server).peerOffsetSegment,
	"otherId":      (*server).peerFaceCandidateSegment,
	"sheet":        nil,
	"file":         nil,
}

// remoteRoute is how one entity operation answers a remote id: op is set when it is forwarded, kept when it is not.
type remoteRoute struct {
	op   *remoteOperation
	kept *keptOperation
}

type remoteRouteTable struct {
	routes map[string]remoteRoute
	mux    *http.ServeMux
	// peer are the entity operations the peer credential reaches.
	peer []string
	// problems name what keeps an operation from being forwarded, or a kept entry naming no operation; such an
	// operation answers 404 not_found.
	problems []string
}

var remoteRoutes = buildRemoteRoutes(gen.ScreenOperations, keptOperations)

// buildRemoteRoutes forwards every entity operation of operations that kept does not name.
func buildRemoteRoutes(operations []gen.ScreenOperation, kept map[string]keptOperation) remoteRouteTable {
	table := remoteRouteTable{routes: map[string]remoteRoute{}, mux: http.NewServeMux()}
	for _, operation := range operations {
		if !isEntityOperation(operation.Path) {
			continue
		}
		pattern := operation.Method + " " + operation.Path
		table.mux.Handle(pattern, http.NotFoundHandler())
		if keep, ok := kept[pattern]; ok {
			table.routes[pattern] = remoteRoute{kept: &keep}
			if keep.peer {
				table.peer = append(table.peer, pattern)
			}
			continue
		}
		op, missing := forwardedOperation(pattern, operation)
		for _, what := range missing {
			table.problems = append(table.problems, pattern+": no "+what)
		}
		if len(missing) == 0 {
			table.routes[pattern] = remoteRoute{op: &op}
			table.peer = append(table.peer, pattern)
		}
	}
	for pattern := range kept {
		if _, ok := table.routes[pattern]; !ok {
			table.problems = append(table.problems, pattern+": kept but not an entity operation")
		}
	}
	slices.Sort(table.problems)
	return table
}

func isEntityOperation(path string) bool {
	return slices.ContainsFunc(remoteCollections, func(c remoteCollection) bool {
		return strings.HasPrefix(path, c.prefix+"{id}")
	})
}

// forwardedOperation is how operation is forwarded, and what it lacks to be.
func forwardedOperation(pattern string, operation gen.ScreenOperation) (remoteOperation, []string) {
	op := remoteOperation{media: operation.Streams || slowRemoteOperations[pattern]}
	var missing []string
	if operation.Response != "" && !operation.Streams {
		rewrite, ok := remoteRewrites[operation.Response]
		if !ok {
			missing = append(missing, "rewriter for "+operation.Response)
		}
		op.rewrite = rewrite
	}
	if translator, ok := remoteRequests[operation.Request]; ok && operation.Request != "" {
		op.body, op.request = translator.body, translator.request
	} else if len(operation.BodyIDs) > 0 {
		missing = append(missing, "translator for "+operation.Request+" ("+strings.Join(operation.BodyIDs, ", ")+")")
	}
	segments := strings.Split(operation.Path, "/")
	for _, name := range operation.PathParams {
		translate, ok := remotePathParams[name]
		switch {
		case !ok:
			missing = append(missing, "translator for path parameter "+name)
		case translate != nil:
			op.params = append(op.params, remoteParam{index: slices.Index(segments, "{"+name+"}"), translate: translate})
		}
	}
	return op, missing
}

// remoteBoundary forwards a request whose path id names the peer's entity to the peer; any other request, and an
// operation this node answers for a remote id, goes to next. An unclean path also goes to next, whose redirect
// brings it back cleaned.
func (s *server) remoteBoundary(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		escaped := r.URL.EscapedPath()
		if path.Clean(escaped) == escaped {
			for _, collection := range remoteCollections {
				if id, _, ok := remotePath(escaped, collection.prefix); ok {
					s.serveRemote(w, r, next, collection, id, escaped)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *server) serveRemote(w http.ResponseWriter, r *http.Request, next http.Handler, collection remoteCollection, id int64, escaped string) {
	pattern := routePattern(remoteRoutes.mux, r)
	route := remoteRoutes.routes[pattern]
	// The peer's own request with a remote id names this node's entity; forwarding it back would bounce.
	peer := isPeerRequest(r.Context())
	switch {
	case route.kept != nil && route.kept.here && !peer:
		next.ServeHTTP(w, r)
	case route.op == nil || peer:
		s.notFound(w, "This operation is not available for a "+collection.noun+" of the other VVMDM.")
	default:
		s.forwardRemoteEntity(w, r, collection, pattern, *route.op, id, escaped)
	}
}

// remotePath splits prefix{id}... into a remote id and the escaped rest of the path.
func remotePath(path, prefix string) (id int64, rest string, ok bool) {
	tail, found := strings.CutPrefix(path, prefix)
	if !found {
		return 0, "", false
	}
	segment, rest := tail, ""
	if slash := strings.IndexByte(tail, '/'); slash >= 0 {
		segment, rest = tail[:slash], tail[slash:]
	}
	id, err := strconv.ParseInt(segment, 10, 64)
	return id, rest, err == nil && domain.IsRemoteID(id)
}

func (s *server) forwardRemoteEntity(
	w http.ResponseWriter, r *http.Request, collection remoteCollection, pattern string, op remoteOperation, id int64, escaped string,
) {
	if op.request != nil && !op.request(s, w, r) {
		return
	}
	peerPath, ok := s.peerEntityPath(w, r, op, escaped, id)
	if !ok {
		return
	}
	query := r.URL.Query()
	if collection.query != nil {
		collection.query(pattern, query, &op)
	}
	out, err := remoteRequest(r, peerPath, query)
	if err != nil {
		s.invalidRequest(w, "The request cannot be forwarded to the other VVMDM.")
		return
	}
	if op.body != nil && !s.forwardedBody(w, r, out, op.body) {
		return
	}
	s.forward(w, r, out, op)
}

// peerEntityPath is escaped with the peer's entity id and op's path parameters in the peer's ids.
func (s *server) peerEntityPath(w http.ResponseWriter, r *http.Request, op remoteOperation, escaped string, id int64) (string, bool) {
	segments := strings.Split(escaped, "/")
	segments[remoteIDSegment] = strconv.FormatInt(domain.FromRemoteID(id), 10)
	for _, param := range op.params {
		peerSegment, ok := param.translate(s, w, r, segments[param.index])
		if !ok {
			return "", false
		}
		segments[param.index] = peerSegment
	}
	return strings.Join(segments, "/"), true
}

// peerTagID is the peer's id of a tag: a remote tag drops the offset, and a local tag goes by name, since the
// screens show a same-named tag of the peer under the local id. It answers missing with 404 when the peer lacks it.
func (s *server) peerTagID(w http.ResponseWriter, r *http.Request, id int64, missing string) (int64, bool) {
	peerID, err := translateID(s.peerEditVocabulary(r), domain.VocabularyTag, id, "", false)
	switch {
	case errors.Is(err, domain.ErrTagNotFound) || (err == nil && peerID == 0):
		s.notFound(w, missing)
		return 0, false
	case err != nil:
		s.writeSplitEditError(w, err)
		return 0, false
	}
	return peerID, true
}

// peerVideoTagSegment turns the tag id of /tags/{tagId}/{action} into the peer's.
func (s *server) peerVideoTagSegment(w http.ResponseWriter, r *http.Request, segment string) (string, bool) {
	id, err := strconv.ParseInt(segment, 10, 64)
	if err != nil {
		s.invalidRequest(w, "The tag id is not a number.")
		return "", false
	}
	peerID, ok := s.peerTagID(w, r, id, "This tag is not tentatively attached to the video.")
	return strconv.FormatInt(peerID, 10), ok
}

// peerOffsetSegment turns a group item or face appearance id into the peer's; one of this node's is mixed_nodes.
func (s *server) peerOffsetSegment(w http.ResponseWriter, _ *http.Request, segment string) (string, bool) {
	id, err := strconv.ParseInt(segment, 10, 64)
	if err != nil {
		return segment, true
	}
	if local, _ := domain.IDNodes(id); local {
		s.mixedNodes(w)
		return "", false
	}
	return strconv.FormatInt(peerID(id), 10), true
}

// peerTagSynonymBody turns the tag whose merge was approved into the peer's, as a tentative tag's id is.
func (s *server) peerTagSynonymBody(w http.ResponseWriter, r *http.Request) bool {
	var body gen.AddTagSynonymRequest
	raw, ok := s.readJSONBodyRaw(w, r, &body)
	if !ok {
		return false
	}
	if body.MergeTagId != nil {
		peerID, ok := s.peerTagID(w, r, *body.MergeTagId, "The tag to merge does not exist.")
		if !ok {
			return false
		}
		body.MergeTagId = &peerID
		var err error
		if raw, err = json.Marshal(body); err != nil {
			s.internalError(w, "Could not forward the request to the other VVMDM.", err)
			return false
		}
	}
	restoreBody(r, raw)
	return true
}

// checkRemoteCreditPatch refuses the forwarded rename of the peer's person or organization to a name an entry here
// holds, and turns an organization's parent into the peer's.
func (s *server) checkRemoteCreditPatch(w http.ResponseWriter, r *http.Request) bool {
	if strings.HasPrefix(r.URL.Path, "/api/organizations/") {
		return checkRemoteCreditRename(domain.CreditOrganization)(s, w, r) && s.remoteOrganizationPatch(w, r)
	}
	return checkRemoteCreditRename(domain.CreditPerson)(s, w, r)
}

// peerVirtualVideoMerge merges the peer's virtual video into one of the peer's file videos only.
func peerVirtualVideoMerge(body *gen.VirtualVideoMerge) bool {
	return onlyRemote([]int64{body.VideoId}) && toPeerID(&body.VideoId)
}
