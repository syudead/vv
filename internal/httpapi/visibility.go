package httpapi

import (
	"context"
	"net/http"
	"sync"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// Visibility is implemented directly by the store because each operation fits in one transaction.
type Visibility interface {
	// SetVideosPublic updates videos still in the library and returns their content keys.
	SetVideosPublic(ctx context.Context, videoIDs []int64, public bool) ([]string, error)
}

// UpdateVideoVisibility uses tag-mutation rules for ID limits, duplicates, and applied counts. Making videos private terminates their active guest responses.
func (s *server) UpdateVideoVisibility(w http.ResponseWriter, r *http.Request) {
	// Require public presence: the generated bool would decode a missing field as false and accidentally make videos private.
	var body struct {
		VideoIds       []int64  `json:"videoIds"`
		SmartFolderIds *[]int64 `json:"smartFolderIds"`
		Public         *bool    `json:"public"`
	}
	if !s.readJSONBody(w, r, &body) {
		return
	}
	if !validVideoTargetCounts(body.VideoIds, body.SmartFolderIds) {
		s.tooManyVideos(w)
		return
	}
	if body.Public == nil {
		s.invalidRequest(w, "public is required.")
		return
	}
	public := *body.Public
	if s.visibility == nil {
		s.internalError(w, "Visibility storage is not configured.", nil)
		return
	}
	videoIDs, ok := s.resolveVideoTargets(w, r, body.VideoIds, body.SmartFolderIds)
	if !ok {
		return
	}

	// Serialize commit through revocation so a concurrent republication cannot start guest streams that an earlier private transition would then terminate.
	s.guests.lockSwitching()
	defer s.guests.switching.Unlock()
	keys, err := s.visibility.SetVideosPublic(r.Context(), videoIDs, public)
	if err != nil {
		s.internalError(w, "Could not change the visibility.", err)
		return
	}
	// Revoke after commit; revoking earlier would let new guest requests observe the old public state and keep streaming.
	if !public {
		s.guests.revoke(keys)
	}

	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.VideoVisibilityResponse{Applied: len(keys)}, s.logger)
}

// lookupServedVideo tracks guest media requests by content key and returns a cancellable request plus release callback. Owner requests are never tracked or revoked.
func (s *server) lookupServedVideo(
	w http.ResponseWriter, r *http.Request, id int64,
) (domain.Video, *http.Request, func(), bool) {
	if audienceFrom(r.Context()).IsOwner() {
		video, ok := s.lookupVideo(w, r, id)
		return video, r, func() {}, ok
	}
	for {
		// Capture the generation before lookup and retry if revocation occurred before tracking, closing the public-check race.
		since := s.guests.generation()
		video, ok := s.lookupVideo(w, r, id)
		if !ok {
			return domain.Video{}, r, func() {}, false
		}
		ctx, release, tracked := s.guests.track(r.Context(), w, video.ContentKey, since)
		if tracked {
			return video, r.WithContext(ctx), release, true
		}
	}
}

// guestLedger tracks active guest responses by content key. Revocation reuses trackedRequest cancellation and write deadlines from session tracking.
type guestLedger struct {
	// switching serializes commit through revocation separately from mu, allowing streams to enter and leave during the transaction.
	switching sync.Mutex
	// onSwitchWait runs before waiting on another switch; it is a test hook and nil in production.
	onSwitchWait func()
	mu           sync.Mutex
	// gen advances on every revocation.
	gen      uint64
	requests map[string]map[*trackedRequest]struct{}
}

func newGuestLedger() *guestLedger {
	return &guestLedger{requests: map[string]map[*trackedRequest]struct{}{}}
}

// lockSwitching invokes onSwitchWait before blocking so tests can verify contention without timing assumptions.
func (l *guestLedger) lockSwitching() {
	if l.switching.TryLock() {
		return
	}
	if l.onSwitchWait != nil {
		l.onSwitchWait()
	}
	l.switching.Lock()
}

func (l *guestLedger) generation() uint64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.gen
}

// track returns ok=false if revocation occurred after since; callers must reload the video. Call release before the handler returns.
func (l *guestLedger) track(
	ctx context.Context, w http.ResponseWriter, contentKey string, since uint64,
) (context.Context, func(), bool) {
	ctx, cancel := context.WithCancel(ctx)
	// Visibility was checked before tracking, so the response may start immediately.
	req := &trackedRequest{cancel: cancel, controller: http.NewResponseController(w), serving: true}

	l.mu.Lock()
	if l.gen != since {
		l.mu.Unlock()
		cancel()
		return nil, nil, false
	}
	if l.requests[contentKey] == nil {
		l.requests[contentKey] = map[*trackedRequest]struct{}{}
	}
	l.requests[contentKey][req] = struct{}{}
	l.mu.Unlock()

	release := func() {
		req.finish()
		cancel()
		l.mu.Lock()
		delete(l.requests[contentKey], req)
		if len(l.requests[contentKey]) == 0 {
			delete(l.requests, contentKey)
		}
		l.mu.Unlock()
	}
	return ctx, release, true
}

func (l *guestLedger) revoke(contentKeys []string) {
	var requests []*trackedRequest
	l.mu.Lock()
	l.gen++
	for _, key := range contentKeys {
		for req := range l.requests[key] {
			requests = append(requests, req)
		}
	}
	l.mu.Unlock()
	for _, req := range requests {
		req.abort()
	}
}
