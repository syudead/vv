package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// An operation over a selection splits its targets by node: the local part is applied in one transaction, then
// each link's part in one request with that link's own ids, in slot order. A smart folder expands on every node
// it reaches, the other nodes' matches read as a list by the folder's rule in names, so the limit counts the
// union.

// selectionPageLimit is the page size used to read the ids matching a smart folder's rule.
const selectionPageLimit = 200

// selection is the targets of an operation over a selection split by node.
type selection struct {
	local []int64
	// links are the links' parts in slot order; a link without a part is left out.
	links []selectionLink
}

// selectionLink is the part of a selection on the link in slot.
type selectionLink struct {
	slot int64
	// peer holds the link's own video ids.
	peer []int64
	// peerFolders are the link's own smart folders whose rule has no form by name; the link resolves them.
	peerFolders []int64
}

// peerTargets is the link's part as the request fields the link takes.
func (link selectionLink) peerTargets() ([]int64, *[]int64) {
	ids := link.peer
	if ids == nil {
		ids = []int64{}
	}
	if len(link.peerFolders) == 0 {
		return ids, nil
	}
	return ids, &link.peerFolders
}

// selectionParts collects the links' parts of a selection while it resolves, by slot.
type selectionParts map[int64]*selectionLink

func (parts selectionParts) of(slot int64) *selectionLink {
	if parts[slot] == nil {
		parts[slot] = &selectionLink{slot: slot}
	}
	return parts[slot]
}

// resolveSelection resolves the targets of an operation over a selection on every node and applies limit to their
// union. A request of the peer, or one naming no remote id while this node reads none, resolves on this node
// alone. It writes the error response when it returns false.
func (s *server) resolveSelection(
	w http.ResponseWriter, r *http.Request, videoIDs []int64, smartFolderIDs *[]int64, limit int,
) (selection, bool) {
	ctx := r.Context()
	folders := deref(smartFolderIDs)
	if isPeerRequest(ctx) ||
		(!slices.ContainsFunc(slices.Concat(videoIDs, folders), domain.IsRemoteID) && !s.readsPeer(ctx)) {
		ids, ok := s.resolveVideoTargets(w, r, videoIDs, smartFolderIDs, limit)
		return selection{local: ids}, ok
	}
	if !validVideoTargetCounts(videoIDs, smartFolderIDs, limit) {
		s.tooManyVideos(w, limit)
		return selection{}, false
	}
	var sel selection
	parts := selectionParts{}
	local, videoLinks := domain.SlotIDs(videoIDs)
	sel.local = local
	for slot, ids := range videoLinks {
		parts.of(slot).peer = ids
	}
	localFolders, folderLinks := domain.SlotIDs(folders)
	if !s.expandLocalFolders(w, r, &sel, parts, localFolders, limit) {
		return selection{}, false
	}
	for _, slot := range sortedSlots(folderLinks) {
		if !s.expandPeerFolders(w, r, &sel, parts.of(slot), folderLinks[slot], limit) {
			return selection{}, false
		}
	}
	return s.limitSelection(w, sel, parts, limit)
}

// limitSelection gathers the links' parts in slot order without duplicates and refuses a union over limit. Folders
// that resolve to nothing leave no target, which would otherwise create an unused tag by name.
func (s *server) limitSelection(w http.ResponseWriter, sel selection, parts selectionParts, limit int) (selection, bool) {
	sel.local = uniqueInt64s(sel.local)
	total, folders := len(sel.local), 0
	for _, slot := range sortedSlots(parts) {
		link := parts[slot]
		link.peer = uniqueInt64s(link.peer)
		if len(link.peer)+len(link.peerFolders) == 0 {
			continue
		}
		total, folders = total+len(link.peer), folders+len(link.peerFolders)
		sel.links = append(sel.links, *link)
	}
	if total > limit || total+folders < 1 {
		s.tooManyVideos(w, limit)
		return selection{}, false
	}
	return sel, true
}

// expandLocalFolders adds this node's folders' members and every connected link's videos matching their rules; a
// link that fails to list is left out, as from a merged smart folder page.
func (s *server) expandLocalFolders(
	w http.ResponseWriter, r *http.Request, sel *selection, parts selectionParts, folders []int64, limit int,
) bool {
	if len(folders) == 0 {
		return true
	}
	if !s.smartFoldersConfigured(w) {
		return false
	}
	ctx := r.Context()
	members, err := s.smartFolders.SmartFolderVideoIDs(ctx, folders)
	if err != nil {
		s.internalError(w, "Could not read the smart folder videos.", err)
		return false
	}
	sel.local = append(sel.local, members...)
	slots := s.connectedSlots(ctx)
	if len(slots) == 0 {
		return true
	}
	for _, id := range folders {
		rule, err := s.localFolderRuleByName(ctx, id)
		if err != nil {
			s.internalError(w, "Could not read the smart folder videos.", err)
			return false
		}
		if rule == nil {
			continue
		}
		for _, slot := range slots {
			link := parts.of(slot)
			ids, err := s.peerMatchIDs(r, slot, *rule, link.peer, limit)
			if err != nil {
				s.logger.Info("the selection leaves out a read VVMDM's matches", slog.Int64("smartFolder", id),
					slog.Int64("slot", slot), slog.Any("error", err))
				continue
			}
			link.peer = append(link.peer, ids...)
		}
	}
	return true
}

// localFolderRuleByName is this node's folder's rule with its ancestors' in names, or nil when a link matches
// nothing for it: a missing folder or entity, or a rule with no form by name.
func (s *server) localFolderRuleByName(ctx context.Context, id int64) (*gen.SmartRuleByName, error) {
	contents, err := s.smartFolders.SmartFolderContents(ctx, domain.AudienceOwner, id, "", 1, domain.VideoQuery{})
	if errors.Is(err, domain.ErrNotFound) || (err == nil && len(contents.Missing) > 0) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	q, ok := domain.SmartPathQuery(contents.PathRules)
	if !ok {
		return nil, nil
	}
	return s.smartRuleByName(ctx, q)
}

// expandPeerFolders adds the matches of link's folders on that link and here from the rule the link gives, each
// node matching a favorite rule by its own favorites; a rule with no form by name is left to the link.
func (s *server) expandPeerFolders(
	w http.ResponseWriter, r *http.Request, sel *selection, link *selectionLink, folders []int64, limit int,
) bool {
	for _, id := range folders {
		var detail gen.SmartFolderDetail
		err := s.peerGet(r, link.slot, remoteSmartFolderPrefix+strconv.FormatInt(id, 10), url.Values{"limit": {"1"}}, &detail)
		var refusal *peerRefusal
		if errors.As(err, &refusal) && refusal.status == http.StatusNotFound {
			continue
		}
		rule := detail.SmartFolder.PeerRule
		if err != nil || rule == nil {
			s.logger.Info("could not read the other VVMDM's smart folder", slog.Int64("smartFolder", id), slog.Any("error", err))
			s.peerUnreachable(w)
			return false
		}
		if rule.Match == nil {
			link.peerFolders = append(link.peerFolders, id)
			continue
		}
		ids, err := s.peerMatchIDs(r, link.slot, *rule.Match, link.peer, limit)
		if err != nil {
			s.peerUnreachable(w)
			return false
		}
		link.peer = append(link.peer, ids...)
		q := queryByName(*rule.Match)
		q.FavoriteOnly = q.FavoriteOnly || rule.Favorite
		ids, err = s.localMatchIDs(r.Context(), q, sel.local, limit)
		if err != nil {
			s.internalError(w, "Could not read the smart folder videos.", err)
			return false
		}
		sel.local = append(sel.local, ids...)
	}
	return true
}

// distinctOver reports when ids read together with the ids already held name more than limit distinct videos,
// so a page that only repeats held ids does not end the reading early.
type distinctOver struct {
	seen  map[int64]struct{}
	limit int
}

func newDistinctOver(held []int64, limit int) distinctOver {
	seen := make(map[int64]struct{}, len(held))
	for _, id := range held {
		seen[id] = struct{}{}
	}
	return distinctOver{seen: seen, limit: limit}
}

func (d distinctOver) add(id int64) { d.seen[id] = struct{}{} }

func (d distinctOver) over() bool { return len(d.seen) > d.limit }

// peerMatchIDs lists the videos of the link in slot matching rule, stopping once they and held name more than
// limit videos.
func (s *server) peerMatchIDs(r *http.Request, slot int64, rule gen.SmartRuleByName, held []int64, limit int) ([]int64, error) {
	values := url.Values{"limit": {strconv.Itoa(selectionPageLimit)}}
	setNameFilters(values, queryByName(rule))
	ids := []int64{}
	count := newDistinctOver(held, limit)
	for {
		var page gen.VideoPage
		if err := s.peerGet(r, slot, "/api/videos", values, &page); err != nil {
			return nil, err
		}
		for _, video := range page.Items {
			ids = append(ids, video.Id)
			count.add(video.Id)
		}
		if page.NextCursor == nil || count.over() {
			return ids, nil
		}
		values.Set("cursor", *page.NextCursor)
	}
}

// localMatchIDs lists this node's videos matching q, stopping once they and held name more than limit videos.
func (s *server) localMatchIDs(ctx context.Context, q domain.VideoQuery, held []int64, limit int) ([]int64, error) {
	if s.videos == nil {
		return nil, nil
	}
	q.Limit = selectionPageLimit
	ids := []int64{}
	count := newDistinctOver(held, limit)
	for {
		page, err := s.videos.ListVideos(ctx, domain.AudienceOwner, q)
		if err != nil {
			return nil, err
		}
		for _, video := range page.Items {
			ids = append(ids, video.ID)
			count.add(video.ID)
		}
		if page.NextCursor == "" || count.over() {
			return ids, nil
		}
		q.Cursor = page.NextCursor
	}
}

// setNameFilters sets q's search, its filters by name and its favorite filter as list parameters.
func setNameFilters(values url.Values, q domain.VideoQuery) {
	if q.Query != "" {
		values.Set("query", q.Query)
	}
	if q.FavoriteOnly {
		values.Set("favorite", "true")
	}
	for _, name := range q.TagNames {
		values.Add("tagName", name)
	}
	for param, value := range map[string]string{
		"tagKindName": q.TagKindName, "personName": q.PersonName, "organizationName": q.OrganizationName,
	} {
		if value != "" {
			values.Set(param, value)
		}
	}
}

// remoteSelectionResult reports the part of the link in slot: what it applied, or, when it failed, every
// requested id of that link with its reason.
func (s *server) remoteSelectionResult(slot int64, applied int, peerIDs []int64, err error) *gen.RemoteSelectionResult {
	if err == nil {
		return &gen.RemoteSelectionResult{Applied: applied, FailedVideoIds: []int64{}}
	}
	s.logger.Info("the other VVMDM did not apply its part of the selection", slog.Any("error", err))
	failed := make([]int64, len(peerIDs))
	for i, id := range peerIDs {
		failed[i] = domain.RemoteID(slot, id)
	}
	reason := remoteFailureReason(err)
	return &gen.RemoteSelectionResult{FailedVideoIds: failed, Reason: &reason}
}

// addRemoteResult adds one link's result to out, the selection's: the applied counts sum and the ids no link changed
// gather, with the reason of the first link that failed.
func addRemoteResult(out **gen.RemoteSelectionResult, part *gen.RemoteSelectionResult) {
	if *out == nil {
		*out = part
		return
	}
	(*out).Applied += part.Applied
	(*out).FailedVideoIds = append((*out).FailedVideoIds, part.FailedVideoIds...)
	if (*out).Reason == nil {
		(*out).Reason = part.Reason
	}
}

// peerSelectionRead reads a summary of link's part into out; a link that fails is left out of the summary, as
// from a merged list.
func (s *server) peerSelectionRead(r *http.Request, path string, link selectionLink, out any) bool {
	ids, folders := link.peerTargets()
	err := s.peerCall(r, link.slot, http.MethodPost, path, nil, gen.VideoTagsSummaryRequest{VideoIds: ids, SmartFolderIds: folders}, out)
	if err != nil {
		s.logger.Info("the summary leaves out the other VVMDM", slog.String("path", path), slog.Any("error", err))
		return false
	}
	return true
}
