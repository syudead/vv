package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

const (
	peerRelatedPath = "/api/peer/related"
	// maxPeerRelatedBody holds the clues with a fingerprint and the largest exclude list.
	maxPeerRelatedBody = 4 << 20
)

// mergedRelatedVideos answers the related list of this node's video with the videos of both nodes
// (docs/design-docs/related-videos.md).
func (s *server) mergedRelatedVideos(w http.ResponseWriter, r *http.Request, id int64, params gen.GetRelatedVideosParams, limit int) {
	page, ok := s.mergedRelatedPage(w, params, limit)
	if !ok {
		return
	}
	video, ok := s.lookupVideo(w, r, id)
	if !ok || !s.requireCatalog(w) {
		return
	}
	audience := audienceFrom(r.Context())
	peer := s.relatedPeer(r)
	related, err := s.catalog.MergedRelatedVideos(r.Context(), audience, video, page, peer)
	if err != nil {
		s.internalError(w, "Could not load related videos.", err)
		return
	}
	present := s.presenter(r.Context(), audience, withGroupMembers(related.Local, related.Group))
	items := mergedRelatedItems(related, present(r.Context(), related.Local), peer.videos)
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, relatedBody(items, presentGroup(r.Context(), present, related.Group),
		mergedCursorText(related.NextCursor), related.NextID, related.PrevID), s.logger)
}

// remoteRelatedVideos answers the related list of the peer's video: its clues and group come from the peer,
// this node scores its own videos and the peer its own.
func (s *server) remoteRelatedVideos(w http.ResponseWriter, r *http.Request, id int64, params gen.GetRelatedVideosParams, limit int) {
	page, ok := s.mergedRelatedPage(w, params, limit)
	if !ok || !s.requireCatalog(w) {
		return
	}
	if s.peerLink == nil {
		s.peerUnreachable(w)
		return
	}
	ctx := r.Context()
	peer := s.relatedPeer(r)
	peerID := domain.FromRemoteID(id)
	var body gen.RelatedClues
	if err := peer.call(ctx, http.MethodGet, peerCluesPath(peerID), nil, &body); err != nil {
		s.writePeerCallError(w, err)
		return
	}
	clues, err := cluesFrom(body)
	if err != nil {
		s.internalError(w, "The other VVMDM answered in an unexpected form.", err)
		return
	}
	opened := url.Values{}
	keepRemoteContext(r.URL.Query(), opened)
	head, exclude, err := peer.group(ctx, peerID, opened, page.Cursor.Start())
	if err != nil {
		s.writePeerCallError(w, err)
		return
	}
	related, err := s.catalog.RemoteRelatedVideos(ctx, audienceFrom(ctx), clues, exclude, page, peer)
	if err != nil {
		s.internalError(w, "Could not load related videos.", err)
		return
	}
	present := s.presenter(ctx, audienceFrom(ctx), related.Local)
	items := mergedRelatedItems(related, present(ctx, related.Local), peer.videos)
	payload := relatedBody(items, nil, mergedCursorText(related.NextCursor), 0, 0)
	switch {
	case head.Group != nil:
		payload.Group, payload.NextId, payload.PrevId = head.Group, head.NextId, head.PrevId
	case page.Cursor.Start() && len(related.IDs) > 0:
		payload.NextId = &related.IDs[0]
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, payload, s.logger)
}

func (s *server) mergedRelatedPage(w http.ResponseWriter, params gen.GetRelatedVideosParams, limit int) (domain.MergedRelatedPage, bool) {
	opened := videoContext(params.Group, params.SmartFolder)
	page := domain.MergedRelatedPage{Limit: limit, GroupID: opened.GroupID, SmartFolderID: opened.SmartFolderID}
	if params.Cursor != nil {
		cursor, err := domain.ParseMergedRelatedCursor(*params.Cursor)
		if err != nil {
			s.invalidCursor(w)
			return domain.MergedRelatedPage{}, false
		}
		page.Cursor = cursor
	}
	return page, true
}

func mergedCursorText(cursor *domain.MergedRelatedCursor) string {
	if cursor == nil {
		return ""
	}
	return cursor.String()
}

// mergedRelatedItems puts the presented videos of this node and the rewritten ones of the peer in the
// merged order, leaving out a video that went away between ranking and reading.
func mergedRelatedItems(related domain.MergedRelated, local []gen.Video, remote map[int64]gen.Video) []gen.Video {
	byID := make(map[int64]gen.Video, len(local))
	for _, video := range local {
		byID[video.Id] = video
	}
	items := make([]gen.Video, 0, len(related.IDs))
	for _, id := range related.IDs {
		video, ok := byID[id]
		if !ok {
			video, ok = remote[id]
		}
		if ok {
			items = append(items, video)
		}
	}
	return items
}

func (s *server) writePeerCallError(w http.ResponseWriter, err error) {
	var status *peerStatusError
	switch {
	case errors.Is(err, domain.ErrPeerUnreachable):
		s.peerUnreachable(w)
	case errors.As(err, &status) && status.code == http.StatusNotFound:
		s.notFoundReason(w, reasonVideoNotFound, "Video not found.")
	default:
		s.internalError(w, "Could not read the related videos of the other VVMDM.", err)
	}
}

// RankPeerRelated scores this node's videos for the peer, which sends the clues of a video it holds.
func (s *server) RankPeerRelated(w http.ResponseWriter, r *http.Request) {
	if !s.requirePeerCaller(w, r) || !s.requireCatalog(w) {
		return
	}
	var body gen.PeerRelatedRequest
	if !s.readLargeJSONBody(w, r, maxPeerRelatedBody, &body) {
		return
	}
	limit, ok := s.relatedLimit(w, body.Limit)
	if !ok {
		return
	}
	var after *domain.RelatedCursor
	if body.Cursor != nil {
		cursor, err := domain.ParseRelatedCursor(*body.Cursor)
		if err != nil {
			s.invalidCursor(w)
			return
		}
		after = &cursor
	}
	clues, err := cluesFrom(body.Clues)
	if err != nil {
		s.invalidRequest(w, "clues.fingerprint is not a valid fingerprint.")
		return
	}
	var exclude []int64
	if body.Exclude != nil {
		exclude = *body.Exclude
	}
	audience := audienceFrom(r.Context())
	scored, err := s.catalog.PeerRelated(r.Context(), audience, clues, exclude, after, limit)
	if err != nil {
		s.internalError(w, "Could not score related videos.", err)
		return
	}
	videos := make([]domain.Video, 0, len(scored.Items))
	for _, item := range scored.Items {
		videos = append(videos, item.Video)
	}
	presented := s.presenter(r.Context(), audience, videos)(r.Context(), videos)
	payload := gen.PeerRelatedPage{Items: make([]gen.PeerRelatedItem, 0, len(presented))}
	for i, video := range presented {
		payload.Items = append(payload.Items, gen.PeerRelatedItem{Score: int64(scored.Items[i].Score), Video: video})
	}
	if scored.NextCursor != nil {
		cursor := scored.NextCursor.String()
		payload.NextCursor = &cursor
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, payload, s.logger)
}

// GetPeerRelatedClues gives the peer the clues of this node's video it shows.
func (s *server) GetPeerRelatedClues(w http.ResponseWriter, r *http.Request, id gen.VideoId) {
	if !s.requirePeerCaller(w, r) || !s.requireCatalog(w) {
		return
	}
	clues, found, err := s.catalog.RelatedClues(r.Context(), audienceFrom(r.Context()), id)
	if err != nil {
		s.internalError(w, "Could not read the related video clues.", err)
		return
	}
	if !found {
		s.notFoundReason(w, reasonVideoNotFound, "Video not found.")
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, cluesBody(clues), s.logger)
}

// requirePeerCaller keeps the peer's operations from the screens, which would read tag names a guest may
// not see.
func (s *server) requirePeerCaller(w http.ResponseWriter, r *http.Request) bool {
	if !isPeerRequest(r.Context()) {
		s.notFound(w, "This operation is for the paired VVMDM.")
		return false
	}
	return true
}

func cluesBody(clues domain.RelatedClues) gen.RelatedClues {
	body := gen.RelatedClues{Tags: clues.TagNames, Words: clues.Words, AddedAt: clues.AddedAt}
	if body.Tags == nil {
		body.Tags = []string{}
	}
	if body.Words == nil {
		body.Words = []string{}
	}
	if fingerprint := clues.Fingerprint; fingerprint != nil {
		body.Fingerprint = &gen.RelatedFingerprint{
			Version: fingerprint.Version, IntervalMs: fingerprint.IntervalMs, Frames: fingerprint.Encode(),
		}
	}
	return body
}

func cluesFrom(body gen.RelatedClues) (domain.RelatedClues, error) {
	clues := domain.RelatedClues{TagNames: body.Tags, Words: body.Words, AddedAt: body.AddedAt}
	if body.Fingerprint != nil {
		// Scoring compares every frame with every candidate's, so a longer fingerprint from the peer is refused.
		if len(body.Fingerprint.Frames) > domain.MaxFingerprintBytes {
			return domain.RelatedClues{}, fmt.Errorf("%w: %d bytes", domain.ErrInvalidFingerprint, len(body.Fingerprint.Frames))
		}
		fingerprint, err := domain.DecodeFingerprint(body.Fingerprint.Frames, body.Fingerprint.Version, body.Fingerprint.IntervalMs)
		if err != nil {
			return domain.RelatedClues{}, err
		}
		clues.Fingerprint = &fingerprint
	}
	return clues, nil
}

// relatedPeer is domain.RelatedPeer over the peer link for one screen request, in its audience. It keeps the
// peer's videos it read, rewritten and by remote id, for the answer.
type relatedPeer struct {
	link   PeerLink
	guest  bool
	videos map[int64]gen.Video
}

func (s *server) relatedPeer(r *http.Request) *relatedPeer {
	return &relatedPeer{
		link: s.peerLink, guest: audienceFrom(r.Context()) != domain.AudienceOwner, videos: map[int64]gen.Video{},
	}
}

func (p *relatedPeer) Related(ctx context.Context, clues domain.RelatedClues, exclude []int64, after *domain.RelatedCursor, limit int) (domain.RelatedSidePage, error) {
	body := gen.PeerRelatedRequest{Clues: cluesBody(clues), Limit: &limit}
	if len(exclude) > 0 {
		body.Exclude = &exclude
	}
	if after != nil {
		cursor := after.String()
		body.Cursor = &cursor
	}
	var answer gen.PeerRelatedPage
	if err := p.call(ctx, http.MethodPost, peerRelatedPath, body, &answer); err != nil {
		return domain.RelatedSidePage{}, err
	}
	page := domain.RelatedSidePage{Items: make([]domain.RankedRelated, 0, len(answer.Items)), More: answer.NextCursor != nil}
	for _, item := range answer.Items {
		video := item.Video
		page.Items = append(page.Items, domain.RankedRelated{
			Score: domain.RelatedScore(item.Score), AddedAt: video.AddedAt.Unix(), VideoID: video.Id,
		})
		// The vocabulary is not joined by name yet, as in the forwarded answers.
		rewriteRemoteVideo(&video, domain.VocabularyJoin{})
		p.videos[video.Id] = video
	}
	return page, nil
}

// remoteGroupPage is the largest page listVideoGroupMembers answers.
const remoteGroupPage = domain.MaxGroupMemberPage

// group reads the group of the peer's video peerID in the opened context. On the first page head holds the
// peer's group window and neighbours, rewritten. exclude is the video, its bundle and every member of its
// group, in the peer's ids, which the related list leaves out.
func (p *relatedPeer) group(ctx context.Context, peerID int64, opened url.Values, first bool) (head gen.RelatedVideos, exclude []int64, err error) {
	exclude, err = p.bundle(ctx, peerID)
	if err != nil {
		return gen.RelatedVideos{}, nil, err
	}
	if first {
		query := cloneValues(opened)
		query.Set("limit", "1")
		if err := p.call(ctx, http.MethodGet, peerVideoPath(peerID, "/related", query), nil, &head); err != nil {
			return gen.RelatedVideos{}, nil, err
		}
		rewriteRemoteRelated(&head, domain.VocabularyJoin{})
		if head.Group == nil {
			return gen.RelatedVideos{}, exclude, nil
		}
	}
	for offset := 0; ; {
		query := cloneValues(opened)
		query.Set("offset", strconv.Itoa(offset))
		query.Set("limit", strconv.Itoa(remoteGroupPage))
		var members gen.GroupMemberPage
		err := p.call(ctx, http.MethodGet, peerVideoPath(peerID, "/group-members", query), nil, &members)
		var status *peerStatusError
		if errors.As(err, &status) && status.code == http.StatusNotFound {
			return head, exclude, nil
		}
		if err != nil {
			return gen.RelatedVideos{}, nil, err
		}
		for _, member := range members.Items {
			exclude = append(exclude, member.Id)
		}
		offset += len(members.Items)
		if len(members.Items) == 0 || offset >= members.Total {
			return head, exclude, nil
		}
	}
}

// bundle is the peer's video peerID and the other versions of its bundle, in the peer's ids. The peer scores
// with no reference video, so it cannot leave out the bundle's representative itself.
func (p *relatedPeer) bundle(ctx context.Context, peerID int64) ([]int64, error) {
	var versions gen.VideoVersions
	if err := p.call(ctx, http.MethodGet, peerVideoPath(peerID, "/versions", nil), nil, &versions); err != nil {
		return nil, err
	}
	ids := []int64{peerID, versions.RepresentativeId}
	for _, version := range versions.Items {
		ids = append(ids, version.Id)
	}
	return ids, nil
}

func cloneValues(values url.Values) url.Values {
	out := make(url.Values, len(values))
	for name, value := range values {
		out[name] = append([]string(nil), value...)
	}
	return out
}

// peerVideoPath is the peer's screen path of its video peerID.
func peerVideoPath(peerID int64, suffix string, query url.Values) string {
	target := remoteVideoPrefix + strconv.FormatInt(peerID, 10) + suffix
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	return target
}

func peerCluesPath(peerID int64) string {
	return "/api/peer/videos/" + strconv.FormatInt(peerID, 10) + "/clues"
}

// peerStatusError is a peer answer other than 200.
type peerStatusError struct {
	code int
}

func (e *peerStatusError) Error() string {
	return fmt.Sprintf("the other VVMDM answered %d", e.code)
}

// call sends one request with a JSON body, when in is set, and decodes a 200 answer into out.
func (p *relatedPeer) call(ctx context.Context, method, target string, in, out any) error {
	var body io.Reader
	if in != nil {
		encoded, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", contentTypeJSON)
	}
	resp, err := p.link.Forward(req, p.guest, false)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return &peerStatusError{code: resp.StatusCode}
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxForwardedJSON+1))
	if err != nil {
		return err
	}
	if len(raw) > maxForwardedJSON {
		return errors.New("the answer of the other VVMDM is too large")
	}
	return json.Unmarshal(raw, out)
}
