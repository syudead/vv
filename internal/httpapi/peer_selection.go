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
// the peer's part in one request with the peer's own ids. A smart folder expands on both nodes, the other node's
// matches read as a list by the folder's rule in names, so the limit counts the union.

// selectionPageLimit is the page size used to read the ids matching a smart folder's rule.
const selectionPageLimit = 200

// selection is the targets of an operation over a selection split by node.
type selection struct {
	local []int64
	// peer holds the peer's own video ids.
	peer []int64
	// peerFolders are the peer's own smart folders whose rule has no form by name; the peer resolves them.
	peerFolders []int64
}

func (sel selection) hasPeer() bool { return len(sel.peer)+len(sel.peerFolders) > 0 }

// peerTargets is the peer's part as the request fields the peer takes.
func (sel selection) peerTargets() ([]int64, *[]int64) {
	ids := sel.peer
	if ids == nil {
		ids = []int64{}
	}
	if len(sel.peerFolders) == 0 {
		return ids, nil
	}
	return ids, &sel.peerFolders
}

// resolveSelection resolves the targets of an operation over a selection on both nodes and applies limit to their
// union. A request of the peer, or one naming no remote id while unpaired, resolves on this node alone. It writes
// the error response when it returns false.
func (s *server) resolveSelection(
	w http.ResponseWriter, r *http.Request, videoIDs []int64, smartFolderIDs *[]int64, limit int,
) (selection, bool) {
	ctx := r.Context()
	folders := deref(smartFolderIDs)
	if isPeerRequest(ctx) ||
		(!slices.ContainsFunc(slices.Concat(videoIDs, folders), domain.IsRemoteID) && !s.mergesLists(ctx, domain.VideoQuery{})) {
		ids, ok := s.resolveVideoTargets(w, r, videoIDs, smartFolderIDs, limit)
		return selection{local: ids}, ok
	}
	if !validVideoTargetCounts(videoIDs, smartFolderIDs, limit) {
		s.tooManyVideos(w, limit)
		return selection{}, false
	}
	var sel selection
	sel.local, sel.peer = domain.SplitRemoteIDs(videoIDs)
	localFolders, peerFolders := domain.SplitRemoteIDs(folders)
	if !s.expandLocalFolders(w, r, &sel, localFolders, limit) || !s.expandPeerFolders(w, r, &sel, peerFolders, limit) {
		return selection{}, false
	}
	sel.local, sel.peer = uniqueInt64s(sel.local), uniqueInt64s(sel.peer)
	// Folders that resolve to nothing leave no target, which would otherwise create an unused tag by name.
	if total := len(sel.local) + len(sel.peer); total > limit || total+len(sel.peerFolders) < 1 {
		s.tooManyVideos(w, limit)
		return selection{}, false
	}
	return sel, true
}

// expandLocalFolders adds this node's folders' members and, while paired, the peer's videos matching their rules;
// a peer that fails to list is left out, as from a merged smart folder page.
func (s *server) expandLocalFolders(w http.ResponseWriter, r *http.Request, sel *selection, folders []int64, limit int) bool {
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
	if !s.mergesLists(ctx, domain.VideoQuery{}) {
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
		ids, err := s.peerMatchIDs(r, *rule, limit-len(sel.peer))
		if err != nil {
			s.logger.Info("the selection leaves out the other VVMDM's matches", slog.Int64("smartFolder", id), slog.Any("error", err))
			return true
		}
		sel.peer = append(sel.peer, ids...)
	}
	return true
}

// localFolderRuleByName is this node's folder's rule with its ancestors' in names, or nil when the peer matches
// nothing for it: a missing folder or entity, a favorite rule, or a rule with no form by name.
func (s *server) localFolderRuleByName(ctx context.Context, id int64) (*gen.SmartRuleByName, error) {
	contents, err := s.smartFolders.SmartFolderContents(ctx, domain.AudienceOwner, id, "", 1, domain.VideoQuery{})
	if errors.Is(err, domain.ErrNotFound) || (err == nil && len(contents.Missing) > 0) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	q, ok := domain.SmartPathQuery(contents.PathRules)
	if !ok || q.FavoriteOnly {
		return nil, nil
	}
	return s.smartRuleByName(ctx, q)
}

// expandPeerFolders adds the peer's folders' matches on both nodes from the rule the peer gives. A folder with
// favorite matches this node's favorites only, and a rule with no form by name is left to the peer.
func (s *server) expandPeerFolders(w http.ResponseWriter, r *http.Request, sel *selection, folders []int64, limit int) bool {
	for _, id := range folders {
		var detail gen.SmartFolderDetail
		err := s.peerGet(r, remoteSmartFolderPrefix+strconv.FormatInt(id, 10), url.Values{"limit": {"1"}}, &detail)
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
			if !rule.Favorite {
				sel.peerFolders = append(sel.peerFolders, id)
			}
			continue
		}
		if !rule.Favorite {
			ids, err := s.peerMatchIDs(r, *rule.Match, limit-len(sel.peer))
			if err != nil {
				s.peerUnreachable(w)
				return false
			}
			sel.peer = append(sel.peer, ids...)
		}
		q := queryByName(*rule.Match)
		q.FavoriteOnly = rule.Favorite
		ids, err := s.localMatchIDs(r.Context(), q, limit-len(sel.local))
		if err != nil {
			s.internalError(w, "Could not read the smart folder videos.", err)
			return false
		}
		sel.local = append(sel.local, ids...)
	}
	return true
}

// peerMatchIDs lists the peer's videos matching rule, stopping once more than limit are read.
func (s *server) peerMatchIDs(r *http.Request, rule gen.SmartRuleByName, limit int) ([]int64, error) {
	values := url.Values{"limit": {strconv.Itoa(selectionPageLimit)}}
	setNameFilters(values, queryByName(rule))
	ids := []int64{}
	for {
		var page gen.VideoPage
		if err := s.peerGet(r, "/api/videos", values, &page); err != nil {
			return nil, err
		}
		for _, video := range page.Items {
			ids = append(ids, video.Id)
		}
		if page.NextCursor == nil || len(ids) > limit {
			return ids, nil
		}
		values.Set("cursor", *page.NextCursor)
	}
}

// localMatchIDs lists this node's videos matching q, stopping once more than limit are read.
func (s *server) localMatchIDs(ctx context.Context, q domain.VideoQuery, limit int) ([]int64, error) {
	if s.videos == nil {
		return nil, nil
	}
	q.Limit = selectionPageLimit
	ids := []int64{}
	for {
		page, err := s.videos.ListVideos(ctx, domain.AudienceOwner, q)
		if err != nil {
			return nil, err
		}
		for _, video := range page.Items {
			ids = append(ids, video.ID)
		}
		if page.NextCursor == "" || len(ids) > limit {
			return ids, nil
		}
		q.Cursor = page.NextCursor
	}
}

// setNameFilters sets q's search and its filters by name as list parameters.
func setNameFilters(values url.Values, q domain.VideoQuery) {
	if q.Query != "" {
		values.Set("query", q.Query)
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

// remoteSelectionResult reports the peer's part: what it applied, or, when it failed, every requested peer id
// with the peer's reason.
func (s *server) remoteSelectionResult(applied int, peerIDs []int64, err error) *gen.RemoteSelectionResult {
	if err == nil {
		return &gen.RemoteSelectionResult{Applied: applied, FailedVideoIds: []int64{}}
	}
	s.logger.Info("the other VVMDM did not apply its part of the selection", slog.Any("error", err))
	failed := make([]int64, len(peerIDs))
	for i, id := range peerIDs {
		failed[i] = domain.ToRemoteID(id)
	}
	reason := remoteFailureReason(err)
	return &gen.RemoteSelectionResult{FailedVideoIds: failed, Reason: &reason}
}

// peerSelectionRead reads a summary of the peer's part into out; a peer that fails is left out of the summary,
// as from a merged list.
func (s *server) peerSelectionRead(r *http.Request, path string, sel selection, out any) bool {
	ids, folders := sel.peerTargets()
	err := s.peerCall(r, http.MethodPost, path, nil, gen.VideoTagsSummaryRequest{VideoIds: ids, SmartFolderIds: folders}, out)
	if err != nil {
		s.logger.Info("the summary leaves out the other VVMDM", slog.String("path", path), slog.Any("error", err))
		return false
	}
	return true
}
