package httpapi

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// PeerLink forwards a screen request for one of the peer's entities; internal/peer implements it.
type PeerLink interface {
	// Forward sends req, whose URL holds the peer's path and query only, as the guest when guest is set; media
	// has no response timeout. A peer that is not connected or gives no response answers
	// domain.ErrPeerUnreachable; any response, whatever its status, is the caller's to close.
	Forward(req *http.Request, guest, media bool) (*http.Response, error)
}

const (
	remoteVideoPrefix = "/api/videos/"
	// maxForwardedJSON bounds an answer the rewrite decodes; a page of related videos stays far below it.
	maxForwardedJSON = 16 << 20
)

// Request headers a forwarded request keeps: the body type and the conditional and range reads of media.
var forwardedRequestHeaders = []string{"Content-Type", "Range", "If-Range", "If-None-Match", "If-Modified-Since"}

// Response headers a passed-through answer keeps; the auth boundary sets the audience header itself.
var forwardedResponseHeaders = []string{
	"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified", "Cache-Control",
	"Content-Disposition",
}

// remoteContextParams name the group or smart folder a video was opened from; only a remote one exists on
// the peer.
var remoteContextParams = []string{"group", "smartFolder"}

// remoteCollection is a collection whose path id names the peer's entity when it carries the offset.
type remoteCollection struct {
	prefix     string
	noun       string
	operations map[string]remoteOperation
	mux        *http.ServeMux
}

var remoteCollections = []remoteCollection{
	newRemoteCollection(remoteVideoPrefix, "video", remoteVideoOperations),
	newRemoteCollection("/api/tags/", "tag", remoteVocabularyOperations),
	newRemoteCollection("/api/people/", "person", remoteVocabularyOperations),
	newRemoteCollection("/api/organizations/", "organization", remoteVocabularyOperations),
}

func newRemoteCollection(prefix, noun string, operations map[string]remoteOperation) remoteCollection {
	mux := http.NewServeMux()
	for pattern := range operations {
		if strings.Contains(pattern, " "+prefix) {
			mux.Handle(pattern, http.NotFoundHandler())
		}
	}
	return remoteCollection{prefix: prefix, noun: noun, operations: operations, mux: mux}
}

// remoteBoundary forwards a request whose path id names the peer's entity to the peer; any other request goes
// to next. An unclean path also goes to next, whose redirect brings it back cleaned.
func (s *server) remoteBoundary(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		escaped := r.URL.EscapedPath()
		for _, collection := range remoteCollections {
			if id, rest, ok := remoteEntityPath(collection.prefix, escaped); ok && path.Clean(escaped) == escaped {
				s.forwardRemoteEntity(w, r, collection, id, rest)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// remoteEntityPath splits prefix{id}... into a remote id and the escaped rest of the path.
func remoteEntityPath(prefix, path string) (id int64, rest string, ok bool) {
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

func (s *server) forwardRemoteEntity(w http.ResponseWriter, r *http.Request, collection remoteCollection, id int64, rest string) {
	op, offered := collection.operations[routePattern(collection.mux, r)]
	// The peer's own request with a remote id names this node's entity; forwarding it back would bounce.
	if !offered || isPeerRequest(r.Context()) {
		s.notFound(w, "This operation is not available for a "+collection.noun+" of the other VVMDM.")
		return
	}
	out, err := remoteRequest(r, collection.prefix+strconv.FormatInt(domain.FromRemoteID(id), 10)+rest, r.URL.Query())
	if err != nil {
		s.invalidRequest(w, "The request cannot be forwarded to the other VVMDM.")
		return
	}
	s.forward(w, r, out, op)
}

// forward sends out, made from r, to the peer as r's viewer and answers with the peer's answer, rewritten by op
// when it succeeded.
func (s *server) forward(w http.ResponseWriter, r *http.Request, out *http.Request, op remoteOperation) {
	if s.peerLink == nil {
		s.peerUnreachable(w)
		return
	}
	guest := audienceFrom(r.Context()) != domain.AudienceOwner
	resp, err := s.peerLink.Forward(out, guest, op.media)
	switch {
	case errors.Is(err, domain.ErrPeerUnreachable):
		s.peerUnreachable(w)
		return
	case err != nil:
		s.internalError(w, "Could not forward the request to the other VVMDM.", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case op.rewrite == nil || resp.StatusCode != http.StatusOK:
		s.passThrough(w, resp)
	case r.Method == http.MethodHead:
		writeRewrittenHead(w, resp)
	default:
		s.writeRewritten(w, r, resp, op.rewrite)
	}
}

// remoteRequest is r for the peer at path with query, keeping only the remote context ids.
func remoteRequest(r *http.Request, path string, query url.Values) (*http.Request, error) {
	for _, name := range remoteContextParams {
		if !query.Has(name) {
			continue
		}
		id, err := strconv.ParseInt(query.Get(name), 10, 64)
		if err != nil || !domain.IsRemoteID(id) {
			query.Del(name)
			continue
		}
		query.Set(name, strconv.FormatInt(domain.FromRemoteID(id), 10))
	}
	target, err := url.Parse(path)
	if err != nil {
		return nil, err
	}
	target.RawQuery = query.Encode()
	var body io.Reader
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		body = r.Body
	}
	out, err := http.NewRequestWithContext(r.Context(), r.Method, target.String(), body)
	if err != nil {
		return nil, err
	}
	out.ContentLength = r.ContentLength
	for _, name := range forwardedRequestHeaders {
		if value := r.Header.Get(name); value != "" {
			out.Header.Set(name, value)
		}
	}
	return out, nil
}

// passThrough keeps the peer's status, error code and reason, and streams media as it arrives.
func (s *server) passThrough(w http.ResponseWriter, resp *http.Response) {
	for _, name := range forwardedResponseHeaders {
		if value := resp.Header.Get(name); value != "" {
			w.Header().Set(name, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_ = http.NewResponseController(w).Flush()
	if _, err := io.Copy(w, resp.Body); err != nil {
		s.logger.Debug("the forwarded response ended early", slog.Any("error", err))
	}
}

// writeRewrittenHead answers a HEAD of a rewritten answer, which has no body to rewrite; the peer's length is
// not the rewritten one, so it is left out.
func writeRewrittenHead(w http.ResponseWriter, resp *http.Response) {
	for _, name := range []string{"Content-Type", "Cache-Control"} {
		if value := resp.Header.Get(name); value != "" {
			w.Header().Set(name, value)
		}
	}
	w.WriteHeader(http.StatusOK)
}

func (s *server) writeRewritten(w http.ResponseWriter, r *http.Request, resp *http.Response, rewrite remoteRewrite) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxForwardedJSON+1))
	if err != nil {
		s.peerUnreachable(w)
		return
	}
	if len(body) > maxForwardedJSON {
		s.internalError(w, "The answer of the other VVMDM is too large.", nil)
		return
	}
	join, err := s.vocabularyJoin(r.Context())
	if err != nil {
		s.internalError(w, "Could not read the names of tags, people and organizations.", err)
		return
	}
	value, err := rewrite(body, join)
	if err != nil {
		s.internalError(w, "The other VVMDM answered in an unexpected form.", err)
		return
	}
	if value := resp.Header.Get("Cache-Control"); value != "" {
		w.Header().Set("Cache-Control", value)
	}
	writeJSON(w, http.StatusOK, value, s.logger)
}

func (s *server) peerUnreachable(w http.ResponseWriter) {
	s.writeReasonError(w, http.StatusServiceUnavailable, gen.ErrorCodeServiceUnavailable, gen.ErrorReasonPeerUnreachable,
		"The other VVMDM is not reachable.")
}
