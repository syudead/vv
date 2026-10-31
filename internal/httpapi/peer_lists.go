package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// mergesLists reports whether a list answers with both nodes' items: the node reads a peer, and it is not a
// favorites list, whose favorites stay with each node.
func (s *server) mergesLists(ctx context.Context, q domain.VideoQuery) bool {
	return !q.FavoriteOnly && s.readsPeer(ctx)
}

// readsPeer reports whether a request reaches both nodes: this node is paired and the request is not the peer's
// own, which names this node's entities only.
func (s *server) readsPeer(ctx context.Context) bool {
	if s.pairing == nil || isPeerRequest(ctx) {
		return false
	}
	state, err := s.pairing.State(ctx)
	if err != nil {
		s.logger.Warn("could not read the pairing state", slog.Any("error", err))
		return false
	}
	return state.State != domain.PeerUnpaired
}

// peerItemsFor gives the peer's own list request each item's cursor; no other caller receives them.
func peerItemsFor(ctx context.Context, cursors []domain.ItemCursor) *[]gen.PeerListItem {
	if !isPeerRequest(ctx) {
		return nil
	}
	items := make([]gen.PeerListItem, len(cursors))
	for i, cursor := range cursors {
		items[i] = gen.PeerListItem{Cursor: cursor.Cursor, SortValue: cursor.SortValue}
	}
	return &items
}

// listPart is one node's part of a merged page.
type listPart[T any] struct {
	side    domain.MergeSide[T]
	total   int
	missing []int64
}

// peerListFilters are a query's filters split between the nodes. A filter by a remote-only entity matches no
// local video; one by a local entity goes to the peer by name, and one by an entity no peer name matches
// leaves the peer's part empty.
type peerListFilters struct {
	localEmpty bool
	peerEmpty  bool
	// local is the query with the local IDs only.
	local domain.VideoQuery
	// values are the peer's filter parameters.
	values url.Values
}

// peerFilterParams are the filter parameters a request to the peer carries in its own terms.
var peerFilterParams = []string{
	"tag", "tagKind", "person", "organization", "personRole", "tagName", "tagKindName", "personName", "organizationName",
}

func (s *server) splitPeerFilters(ctx context.Context, q domain.VideoQuery) (peerListFilters, error) {
	out := peerListFilters{local: q, values: url.Values{}}
	out.local.TagIDs = nil
	for _, id := range q.TagIDs {
		if domain.IsRemoteID(id) {
			out.values.Add("tag", strconv.FormatInt(domain.FromRemoteID(id), 10))
			out.localEmpty = true
			continue
		}
		out.local.TagIDs = append(out.local.TagIDs, id)
	}
	for _, filter := range []struct {
		param string
		id    *int64
	}{
		{"tagKind", &out.local.TagKindID}, {"person", &out.local.Person}, {"organization", &out.local.Organization},
	} {
		if domain.IsRemoteID(*filter.id) {
			out.values.Set(filter.param, strconv.FormatInt(domain.FromRemoteID(*filter.id), 10))
			out.localEmpty = true
			*filter.id = 0
		}
	}
	named, found, err := s.videos.NameFilters(ctx, out.local)
	if err != nil {
		return peerListFilters{}, err
	}
	out.peerEmpty = !found
	for _, name := range named.TagNames {
		out.values.Add("tagName", name)
	}
	for param, value := range map[string]string{
		"tagKindName": named.TagKindName, "personName": named.PersonName, "organizationName": named.OrganizationName,
		"personRole": string(named.PersonRole),
	} {
		if value != "" {
			out.values.Set(param, value)
		}
	}
	return out, nil
}

// peerQuery is the request's query for the peer: its own filters and the order the peer lists remote items in.
func peerQuery(original url.Values, sort domain.VideoSort, filters url.Values) url.Values {
	out := maps.Clone(original)
	for _, name := range peerFilterParams {
		delete(out, name)
	}
	maps.Copy(out, filters)
	if sort != "" {
		out.Set("sort", string(domain.PeerSort(sort)))
	}
	return out
}

var errPeerAnswer = errors.New("the other VVMDM did not answer the list")

// peerRefusal is a peer answer other than 200, with the peer's reason and error body when it gave one.
type peerRefusal struct {
	status  int
	reason  *gen.ErrorReason
	problem *gen.Error
}

func (e *peerRefusal) Error() string { return fmt.Sprintf("%v: status %d", errPeerAnswer, e.status) }

func (e *peerRefusal) Unwrap() error { return errPeerAnswer }

// peerGet reads path from the peer as the viewer and decodes a 200 answer into out.
func (s *server) peerGet(r *http.Request, path string, query url.Values, out any) error {
	return s.peerCall(r, http.MethodGet, path, query, nil, out)
}

// peerCall sends body, when set, as JSON to path on the peer as the viewer and decodes a 200 or 201 answer into
// out, when set; a 204 answers nothing, and any other answer is a *peerRefusal.
func (s *server) peerCall(r *http.Request, method, path string, query url.Values, body, out any) error {
	if s.peerLink == nil {
		return domain.ErrPeerUnreachable
	}
	var payload io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(raw)
	}
	target := url.URL{Path: path, RawQuery: query.Encode()}
	req, err := http.NewRequestWithContext(r.Context(), method, target.String(), payload)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	answer, err := s.peerSend(req, audienceFrom(r.Context()) != domain.AudienceOwner, false)
	if err != nil || out == nil || answer == nil {
		return err
	}
	return json.Unmarshal(answer, out)
}

// peerSend sends req to the peer, as the guest when guest is set and without a response timeout when media is,
// and reads a 200 or 201 answer of at most maxForwardedJSON bytes; a 204 reads as a nil answer, and any other
// answer is a *peerRefusal.
func (s *server) peerSend(req *http.Request, guest, media bool) ([]byte, error) {
	if s.peerLink == nil {
		return nil, domain.ErrPeerUnreachable
	}
	resp, err := s.peerLink.Forward(req, guest, media)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	answer, err := io.ReadAll(io.LimitReader(resp.Body, maxForwardedJSON+1))
	if err != nil {
		return nil, err
	}
	if (resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated) || len(answer) > maxForwardedJSON {
		refusal := &peerRefusal{status: resp.StatusCode}
		var problem gen.Error
		if json.Unmarshal(answer, &problem) == nil && problem.Code != "" {
			refusal.reason, refusal.problem = problem.Reason, &problem
		}
		return nil, refusal
	}
	return answer, nil
}

// mergedRead is one merged list: how each node reads its part, and the request's filters split between them.
type mergedRead[T any] struct {
	path  string
	local func(q domain.VideoQuery) (listPart[T], error)
	// peer decodes and rewrites the peer's answer into this node's id space.
	peer func(r *http.Request, query url.Values) (listPart[T], error)
}

// mergedPage is one page of a merged list with the cursor that continues it, empty at the end.
type mergedPage[T any] struct {
	items   []T
	next    string
	total   int
	missing []int64
}

// readMerged reads both nodes' parts after the composite cursor and merges them. A side that is done is still
// read for its total; a peer that fails is left out for the rest of the listing.
func readMerged[T any](s *server, r *http.Request, q domain.VideoQuery, read mergedRead[T]) (mergedPage[T], error) {
	cursor, err := domain.ParseMergedCursor(q.Cursor)
	if err != nil {
		return mergedPage[T]{}, err
	}
	filters, err := s.splitPeerFilters(r.Context(), q)
	if err != nil {
		return mergedPage[T]{}, err
	}
	limit := q.Limit
	var local listPart[T]
	if !filters.localEmpty {
		query := filters.local
		query.Cursor, query.Limit = cursor.Local, sideLimit(limit, cursor.LocalDone)
		if local, err = read.local(query); err != nil {
			return mergedPage[T]{}, err
		}
		if cursor.LocalDone {
			local.side = domain.MergeSide[T]{}
		}
	}
	var peer listPart[T]
	if !cursor.PeerSkipped && !filters.peerEmpty {
		query := peerQuery(r.URL.Query(), q.Sort, filters.values)
		query.Set("limit", strconv.Itoa(sideLimit(limit, cursor.PeerDone)))
		query.Del("cursor")
		if cursor.Peer != "" {
			query.Set("cursor", cursor.Peer)
		}
		peer, err = read.peer(r, query)
		if err != nil {
			s.logger.Info("the merged list leaves out the other VVMDM", slog.String("path", read.path), slog.Any("error", err))
			peer, cursor.PeerSkipped, cursor.PeerDone = listPart[T]{}, true, true
		}
		if cursor.PeerDone {
			peer.side = domain.MergeSide[T]{}
		}
	}
	items, next, more := domain.MergePages(q.Sort, cursor, local.side, peer.side, limit)
	page := mergedPage[T]{items: items, total: local.total + peer.total, missing: local.missing}
	for _, id := range peer.missing {
		if remote := domain.ToRemoteID(id); !slices.Contains(page.missing, remote) {
			page.missing = append(page.missing, remote)
		}
	}
	if more {
		page.next = next.Encode()
	}
	return page, nil
}

// sideLimit reads one item of a side that is done, which only its total needs.
func sideLimit(limit int, done bool) int {
	if done {
		return 1
	}
	return limit
}

func peerSide[T any](items []T, peerItems *[]gen.PeerListItem, next *string) (domain.MergeSide[T], error) {
	if peerItems == nil || len(*peerItems) != len(items) {
		return domain.MergeSide[T]{}, fmt.Errorf("%w: the item cursors do not match the items", errPeerAnswer)
	}
	side := domain.MergeSide[T]{Items: items, Cursors: make([]domain.ItemCursor, len(items))}
	for i, item := range *peerItems {
		side.Cursors[i] = domain.ItemCursor{Cursor: item.Cursor, SortValue: item.SortValue}
	}
	if next != nil {
		side.Next = *next
	}
	return side, nil
}

func optionalIDs(ids *[]int64) []int64 {
	if ids == nil {
		return nil
	}
	return *ids
}

func (s *server) listMergedVideos(w http.ResponseWriter, r *http.Request, q domain.VideoQuery) {
	ctx := r.Context()
	join, err := s.vocabularyJoin(ctx)
	if !s.checkListRead(w, err) {
		return
	}
	page, err := readMerged(s, r, q, mergedRead[gen.Video]{
		path: "/api/videos",
		local: func(q domain.VideoQuery) (listPart[gen.Video], error) {
			page, err := s.videos.ListVideos(ctx, audienceFrom(ctx), q)
			if err != nil {
				return listPart[gen.Video]{}, err
			}
			items := s.listItems(ctx, page.Items, s.registeredRoots(ctx))
			return listPart[gen.Video]{
				side:  domain.MergeSide[gen.Video]{Items: items, Cursors: page.ItemCursors, Next: page.NextCursor},
				total: page.Total, missing: page.MissingTagIDs,
			}, nil
		},
		peer: func(r *http.Request, query url.Values) (listPart[gen.Video], error) {
			var answer gen.VideoPage
			if err := s.peerGet(r, "/api/videos", query, &answer); err != nil {
				return listPart[gen.Video]{}, err
			}
			rewriteRemoteVideos(answer.Items, join)
			side, err := peerSide(answer.Items, answer.PeerItems, answer.NextCursor)
			return listPart[gen.Video]{side: side, total: answer.Total, missing: optionalIDs(answer.MissingTagIds)}, err
		},
	})
	if !s.checkListRead(w, err) {
		return
	}
	payload := gen.VideoPage{Items: page.items, Total: page.total}
	payload.NextCursor, payload.MissingTagIds = optionalPageFields(page.next, page.missing)
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, payload, s.logger)
}

func (s *server) listMergedLibrary(w http.ResponseWriter, r *http.Request, q domain.VideoQuery) {
	ctx := r.Context()
	join, err := s.vocabularyJoin(ctx)
	if !s.checkListRead(w, err) {
		return
	}
	page, err := readMerged(s, r, q, mergedRead[gen.LibraryItem]{
		path: "/api/library",
		local: func(q domain.VideoQuery) (listPart[gen.LibraryItem], error) {
			page, err := s.library.ListLibrary(ctx, audienceFrom(ctx), q)
			if err != nil {
				return listPart[gen.LibraryItem]{}, err
			}
			items, err := s.libraryItems(ctx, page)
			if err != nil {
				return listPart[gen.LibraryItem]{}, err
			}
			return listPart[gen.LibraryItem]{
				side:  domain.MergeSide[gen.LibraryItem]{Items: items, Cursors: page.ItemCursors, Next: page.NextCursor},
				total: page.Total, missing: page.MissingTagIDs,
			}, nil
		},
		peer: func(r *http.Request, query url.Values) (listPart[gen.LibraryItem], error) {
			var answer gen.LibraryPage
			if err := s.peerGet(r, "/api/library", query, &answer); err != nil {
				return listPart[gen.LibraryItem]{}, err
			}
			for i := range answer.Items {
				rewriteRemoteLibraryItem(&answer.Items[i], join)
			}
			side, err := peerSide(answer.Items, answer.PeerItems, answer.NextCursor)
			return listPart[gen.LibraryItem]{side: side, total: answer.Total, missing: optionalIDs(answer.MissingTagIds)}, err
		},
	})
	if !s.checkListRead(w, err) {
		return
	}
	payload := gen.LibraryPage{Items: page.items, Total: page.total}
	payload.NextCursor, payload.MissingTagIds = optionalPageFields(page.next, page.missing)
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, payload, s.logger)
}

func optionalPageFields(next string, missing []int64) (*string, *[]int64) {
	var nextCursor *string
	var missingIDs *[]int64
	if next != "" {
		nextCursor = &next
	}
	if len(missing) > 0 {
		missingIDs = &missing
	}
	return nextCursor, missingIDs
}
