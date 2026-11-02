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
	"sync"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// readsPeer reports whether a request reaches a link as well as this node: this node reads another one and the
// request is not the peer's own, which names this node's entities only.
func (s *server) readsPeer(ctx context.Context) bool {
	return s.listSlot(ctx) != 0
}

type linkSlotKey struct{}

// withLinkSlot is ctx for a request to the link in slot, which requestSlot then answers.
func withLinkSlot(ctx context.Context, slot int64) context.Context {
	return context.WithValue(ctx, linkSlotKey{}, slot)
}

// requestSlot is the slot of the link r reaches: the link whose entity r's path names, set at the remote
// boundary, or else listSlot's.
func (s *server) requestSlot(r *http.Request) int64 {
	if slot, ok := r.Context().Value(linkSlotKey{}).(int64); ok {
		return slot
	}
	return s.listSlot(r.Context())
}

// listSlot is the slot of the link that the vocabulary and the operations naming no remote id reach: the first
// link in slot order, or 0 when the request reaches no link. They reach one link until they fan out to every link.
func (s *server) listSlot(ctx context.Context) int64 {
	links := s.readLinks(ctx)
	if len(links) == 0 {
		return 0
	}
	return links[0].Slot
}

// readLinks are the links a merged read reaches, in slot order, with whether each is connected now; the peer's own
// request reaches none, since it names this node's entities only.
func (s *server) readLinks(ctx context.Context) []domain.LinkReach {
	if s.pairing == nil || isPeerRequest(ctx) {
		return nil
	}
	state, err := s.pairing.State(ctx)
	if err != nil {
		s.logger.Warn("could not read the pairing state", slog.Any("error", err))
		return nil
	}
	links := make([]domain.LinkReach, len(state.Links))
	for i, link := range state.Links {
		links[i] = domain.LinkReach{Slot: link.Link.Slot, Connected: link.State == domain.PeerConnected}
	}
	return links
}

// connectedSlots are the slots of the connected links a read without a cursor merges; a link that is not
// connected is left out without a request, so the read does not wait for it.
func (s *server) connectedSlots(ctx context.Context) []int64 {
	var slots []int64
	for _, link := range s.readLinks(ctx) {
		if link.Connected {
			slots = append(slots, link.Slot)
		}
	}
	return slots
}

// eachLink calls read for every slot concurrently, so the slowest link bounds the wait instead of their sum, and
// returns the values and errors in the order of slots.
func eachLink[T any](slots []int64, read func(slot int64) (T, error)) ([]T, []error) {
	values := make([]T, len(slots))
	errs := make([]error, len(slots))
	var wg sync.WaitGroup
	for i, slot := range slots {
		wg.Go(func() { values[i], errs[i] = read(slot) })
	}
	wg.Wait()
	return values, errs
}

// linkAnswer is one link's answer, in that link's own ids.
type linkAnswer[T any] struct {
	slot  int64
	value T
}

// linkAnswers reads path from every connected link concurrently and returns the answers in slot order; a link that
// does not answer is left out.
func linkAnswers[T any](s *server, r *http.Request, path string, query url.Values) []linkAnswer[T] {
	slots := s.connectedSlots(r.Context())
	values, errs := eachLink(slots, func(slot int64) (T, error) {
		var answer T
		err := s.peerGet(r, slot, path, query, &answer)
		return answer, err
	})
	answers := make([]linkAnswer[T], 0, len(slots))
	for i, slot := range slots {
		if errs[i] != nil {
			s.logger.Info("the list leaves out a linked VVMDM", slog.String("path", path), slog.Int64("slot", slot),
				slog.Any("error", errs[i]))
			continue
		}
		answers = append(answers, linkAnswer[T]{slot: slot, value: values[i]})
	}
	return answers
}

// readLinkParts reads the part of every link a merged listing holds, concurrently, after that link's side of
// cursor. A skipped link is not asked, a link that fails is skipped for the rest of the listing, and a done link's
// items are left out after read gave its total. Each part's side carries its slot.
func readLinkParts[T any](s *server, what string, cursor domain.MergedCursor,
	read func(slot int64, side domain.SideCursor) (listPart[T], error),
) ([]listPart[T], domain.MergedCursor) {
	var asked []int64
	for _, slot := range cursor.Links() {
		if !cursor.Side(slot).Skipped {
			asked = append(asked, slot)
		}
	}
	answers, errs := eachLink(asked, func(slot int64) (listPart[T], error) { return read(slot, cursor.Side(slot)) })
	// A skipped link keeps its place in the cursor and is done, so the merge needs no side of it.
	parts := make([]listPart[T], 0, len(asked))
	for i, slot := range asked {
		part := answers[i]
		if errs[i] != nil {
			s.logger.Info("the merged list leaves out a linked VVMDM", slog.String("path", what), slog.Int64("slot", slot),
				slog.Any("error", errs[i]))
			part, cursor = listPart[T]{}, cursor.Skip(slot)
		}
		if cursor.Side(slot).Done {
			part.side = domain.MergeSide[T]{}
		}
		part.side.Slot = slot
		parts = append(parts, part)
	}
	return parts, cursor
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

// peerListFilters are a query's filters split between the nodes. A filter by a link's entity matches no local
// video and no other link's; one by a local entity goes to each link by name, and one by an entity without a
// name leaves every link's part empty.
type peerListFilters struct {
	localEmpty bool
	// unnamed is set when a local filter's entity has no name to send.
	unnamed bool
	// remoteSlots are the slots of the links whose entities the filters name.
	remoteSlots []int64
	// local is the query with the local IDs only.
	local domain.VideoQuery
	// values are a link's filter parameters.
	values url.Values
}

// linkEmpty reports whether the link in slot can match no video of the filters.
func (f peerListFilters) linkEmpty(slot int64) bool {
	return f.unnamed || slices.ContainsFunc(f.remoteSlots, func(other int64) bool { return other != slot })
}

// peerFilterParams are the filter parameters a request to the peer carries in its own terms.
var peerFilterParams = []string{
	"tag", "tagKind", "person", "organization", "personRole", "tagName", "tagKindName", "personName", "organizationName",
}

// splitPeerFilters splits q's filters between this node and the links.
func (s *server) splitPeerFilters(ctx context.Context, q domain.VideoQuery) (peerListFilters, error) {
	out := peerListFilters{local: q, values: url.Values{}}
	out.local.TagIDs = nil
	remote := func(id int64) {
		out.localEmpty = true
		if slot := domain.RemoteSlot(id); !slices.Contains(out.remoteSlots, slot) {
			out.remoteSlots = append(out.remoteSlots, slot)
		}
	}
	for _, id := range q.TagIDs {
		if domain.IsRemoteID(id) {
			out.values.Add("tag", strconv.FormatInt(domain.FromRemoteID(id), 10))
			remote(id)
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
			remote(*filter.id)
			*filter.id = 0
		}
	}
	named, found, err := s.videos.NameFilters(ctx, out.local)
	if err != nil {
		return peerListFilters{}, err
	}
	out.unnamed = !found
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

// peerQuery is the request's query for the peer: its own filters and the request's order.
func peerQuery(original url.Values, sort domain.VideoSort, filters url.Values) url.Values {
	out := maps.Clone(original)
	for _, name := range peerFilterParams {
		delete(out, name)
	}
	maps.Copy(out, filters)
	if sort != "" {
		out.Set("sort", string(sort))
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

// peerGet reads path from the node of the link in slot as the viewer and decodes a 200 answer into out.
func (s *server) peerGet(r *http.Request, slot int64, path string, query url.Values, out any) error {
	return s.peerCall(r, slot, http.MethodGet, path, query, nil, out)
}

// peerCall sends body, when set, as JSON to path on the node of the link in slot as the viewer and decodes a 200
// or 201 answer into out, when set; a 204 answers nothing, and any other answer is a *peerRefusal.
func (s *server) peerCall(r *http.Request, slot int64, method, path string, query url.Values, body, out any) error {
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
	answer, err := s.peerSend(req, slot, !audienceFrom(r.Context()).IsOwner(), false)
	if err != nil || out == nil || answer == nil {
		return err
	}
	return json.Unmarshal(answer, out)
}

// peerSend sends req to the node of the link in slot, as the guest when guest is set and without a response
// timeout when media is, and reads a 200 or 201 answer of at most maxForwardedJSON bytes; a 204 reads as a nil
// answer, and any other answer is a *peerRefusal.
func (s *server) peerSend(req *http.Request, slot int64, guest, media bool) ([]byte, error) {
	if s.peerLink == nil {
		return nil, domain.ErrPeerUnreachable
	}
	resp, err := s.peerLink.Forward(req, slot, guest, media)
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

// mergedRead is one merged list: how this node and each link read their parts.
type mergedRead[T any] struct {
	path  string
	local func(q domain.VideoQuery) (listPart[T], error)
	// peer decodes and rewrites the answer of the link in slot into this node's id space.
	peer func(r *http.Request, slot int64, query url.Values) (listPart[T], error)
}

// mergedPage is one page of a merged list with the cursor that continues it, empty at the end.
type mergedPage[T any] struct {
	items   []T
	next    string
	total   int
	missing []int64
}

// readMerged reads this node's part and every link's after the composite cursor and merges them. A side that is
// done is still read for its total; a link that is not connected on the first page, or fails, is left out for the
// rest of the listing.
func readMerged[T any](s *server, r *http.Request, q domain.VideoQuery, read mergedRead[T]) (mergedPage[T], error) {
	cursor, err := domain.ParseMergedCursor(q.Cursor)
	if err != nil {
		return mergedPage[T]{}, err
	}
	cursor = cursor.Begin(s.readLinks(r.Context()))
	filters, err := s.splitPeerFilters(r.Context(), q)
	if err != nil {
		return mergedPage[T]{}, err
	}
	limit := q.Limit
	var local listPart[T]
	if !filters.localEmpty {
		side := cursor.Side(0)
		query := filters.local
		query.Cursor, query.Limit = side.Cursor, sideLimit(limit, side.Done)
		if local, err = read.local(query); err != nil {
			return mergedPage[T]{}, err
		}
		if side.Done {
			local.side = domain.MergeSide[T]{}
		}
	}
	parts, cursor := readLinkParts(s, read.path, cursor, func(slot int64, side domain.SideCursor) (listPart[T], error) {
		if filters.linkEmpty(slot) {
			return listPart[T]{}, nil
		}
		query := peerQuery(r.URL.Query(), q.Sort, filters.values)
		query.Set("limit", strconv.Itoa(sideLimit(limit, side.Done)))
		query.Del("cursor")
		if side.Cursor != "" {
			query.Set("cursor", side.Cursor)
		}
		return read.peer(r, slot, query)
	})
	page := mergedPage[T]{total: local.total, missing: local.missing}
	sides := []domain.MergeSide[T]{local.side}
	for _, part := range parts {
		sides = append(sides, part.side)
		page.total += part.total
		for _, id := range part.missing {
			if remote := domain.RemoteID(part.side.Slot, id); !slices.Contains(page.missing, remote) {
				page.missing = append(page.missing, remote)
			}
		}
	}
	items, next, more := domain.MergePages(q.Sort, cursor, sides, limit)
	page.items = items
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
	join, err := s.vocabularyJoin(ctx, 0)
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
		peer: func(r *http.Request, slot int64, query url.Values) (listPart[gen.Video], error) {
			var answer gen.VideoPage
			if err := s.peerGet(r, slot, "/api/videos", query, &answer); err != nil {
				return listPart[gen.Video]{}, err
			}
			rewriteRemoteVideos(answer.Items, join.InSlot(slot))
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
	join, err := s.vocabularyJoin(ctx, 0)
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
		peer: func(r *http.Request, slot int64, query url.Values) (listPart[gen.LibraryItem], error) {
			var answer gen.LibraryPage
			if err := s.peerGet(r, slot, "/api/library", query, &answer); err != nil {
				return listPart[gen.LibraryItem]{}, err
			}
			for i := range answer.Items {
				rewriteRemoteLibraryItem(&answer.Items[i], join.InSlot(slot))
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
