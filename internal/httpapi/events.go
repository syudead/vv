package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// Keepalive comments prevent idle intermediaries from closing SSE connections without transmitting state.
const eventsKeepAlive = 30 * time.Second

const eventsRetryMs = 3000

// Events coalesces unsent changes and rereads state immediately before delivery, avoiding publisher blocking or stale queues on slow connections.
type Events struct {
	mu     sync.Mutex
	subs   map[*eventSubscriber]struct{}
	closed chan struct{}
	once   sync.Once
}

func NewEvents() *Events {
	return &Events{subs: map[*eventSubscriber]struct{}{}, closed: make(chan struct{})}
}

type eventSubscriber struct {
	mu           sync.Mutex
	scan         bool
	candidates   bool
	activity     bool
	videos       map[int64]struct{}
	groups       map[int64]struct{}
	smartFolders map[int64]struct{}
	onboarding   bool
	assistant    bool
	peer         bool
	ready        chan struct{}
}

func (s *eventSubscriber) mark(update func(*eventSubscriber)) {
	s.mu.Lock()
	update(s)
	s.mu.Unlock()
	select {
	case s.ready <- struct{}{}:
	default:
	}
}

type pendingEvents struct {
	scan         bool
	candidates   bool
	activity     bool
	videos       []int64
	groups       []int64
	smartFolders []int64
	onboarding   bool
	assistant    bool
	peer         bool
}

func (s *eventSubscriber) take() pendingEvents {
	s.mu.Lock()
	defer s.mu.Unlock()
	pending := pendingEvents{
		scan: s.scan, candidates: s.candidates, activity: s.activity, videos: sortedIDs(s.videos),
		groups: sortedIDs(s.groups), smartFolders: sortedIDs(s.smartFolders), onboarding: s.onboarding,
		assistant: s.assistant, peer: s.peer,
	}
	s.peer = false
	s.scan = false
	s.candidates = false
	s.activity = false
	s.onboarding = false
	s.assistant = false
	clear(s.videos)
	clear(s.groups)
	clear(s.smartFolders)
	return pending
}

func sortedIDs(set map[int64]struct{}) []int64 {
	var ids []int64
	for id := range set {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

func (e *Events) subscribe() (*eventSubscriber, func()) {
	sub := &eventSubscriber{
		videos: map[int64]struct{}{}, groups: map[int64]struct{}{}, smartFolders: map[int64]struct{}{},
		ready: make(chan struct{}, 1),
	}
	// Send current state on connection to recover changes missed while disconnected.
	sub.mark(func(s *eventSubscriber) {
		s.scan = true
		s.peer = true
	})
	e.mu.Lock()
	e.subs[sub] = struct{}{}
	e.mu.Unlock()
	return sub, func() {
		e.mu.Lock()
		delete(e.subs, sub)
		e.mu.Unlock()
	}
}

func (e *Events) publish(update func(*eventSubscriber)) {
	e.mu.Lock()
	subs := make([]*eventSubscriber, 0, len(e.subs))
	for sub := range e.subs {
		subs = append(subs, sub)
	}
	e.mu.Unlock()
	for _, sub := range subs {
		sub.mark(update)
	}
}

func (e *Events) ScanChanged() {
	e.publish(func(s *eventSubscriber) { s.scan = true })
}

// Job outcomes affect import counts/state, so processing changes send scan events.
func (e *Events) ProcessingChanged() {
	e.publish(func(s *eventSubscriber) { s.scan = true })
}

// Activity changes coalesce into scan notifications rather than queueing stale per-file updates.
func (e *Events) ScanActivityChanged() {
	e.publish(func(s *eventSubscriber) { s.scan = true })
}

func (e *Events) VideoChanged(id int64) {
	e.publish(func(s *eventSubscriber) { s.videos[id] = struct{}{} })
}

// Candidate notifications have no payload; the UI rereads the list.
func (e *Events) CandidatesChanged() {
	e.publish(func(s *eventSubscriber) { s.candidates = true })
}

// Activity notifications have no payload; the UI rereads entries and counts.
func (e *Events) ActivityChanged() {
	e.publish(func(s *eventSubscriber) { s.activity = true })
}

func (e *Events) GroupsChanged(ids []int64) {
	if len(ids) == 0 {
		return
	}
	e.publish(func(s *eventSubscriber) {
		for _, id := range ids {
			s.groups[id] = struct{}{}
		}
	})
}

func (e *Events) SmartFoldersChanged(ids []int64) {
	if len(ids) == 0 {
		return
	}
	e.publish(func(s *eventSubscriber) {
		for _, id := range ids {
			s.smartFolders[id] = struct{}{}
		}
	})
}

func (e *Events) OnboardingChanged() {
	e.publish(func(s *eventSubscriber) { s.onboarding = true })
}

// Assistant notifications have no payload; the panel rereads the proposals it shows.
func (e *Events) AssistantChanged() {
	e.publish(func(s *eventSubscriber) { s.assistant = true })
}

// PeerStateChanged has no payload; delivery reads the pairing state for each audience.
func (e *Events) PeerStateChanged() {
	e.publish(func(s *eventSubscriber) { s.peer = true })
}

// peerChanged forwards the peer's notifications, whose ids already carry the remote offset.
func (e *Events) peerChanged(event domain.PeerChanged) {
	for _, id := range event.VideoIDs {
		e.VideoChanged(id)
	}
	e.GroupsChanged(event.GroupIDs)
	e.SmartFoldersChanged(event.SmartFolderIDs)
}
func (e *Events) Handle(event domain.Event) {
	switch event := event.(type) {
	case domain.ScanChanged:
		e.ScanChanged()
	case domain.ProcessingChanged:
		e.ProcessingChanged()
	case domain.ScanActivityChanged:
		e.ScanActivityChanged()
	case domain.VideoIngestChanged:
		e.VideoChanged(event.VideoID)
	case domain.VideoOverrideChanged:
		e.VideoChanged(event.VideoID)
	case domain.VideoBundleChanged:
		for _, id := range event.VideoIDs {
			e.VideoChanged(id)
		}
	case domain.VideoWorkInfoChanged:
		// Work-info changes reuse per-video notifications rather than adding an SSE kind.
		for _, id := range event.VideoIDs {
			e.VideoChanged(id)
		}
	case domain.AutoTagApplied:
		for _, id := range event.VideoIDs {
			e.VideoChanged(id)
		}
	case domain.VersionCandidatesChanged:
		e.CandidatesChanged()
	case domain.ActivityChanged:
		e.ActivityChanged()
	case domain.GroupsChanged:
		e.GroupsChanged(event.GroupIDs)
	case domain.SmartFoldersChanged:
		e.SmartFoldersChanged(event.IDs)
	case domain.OnboardingChanged:
		// ホームの変化は onboarding の知らせにする。所有者にもゲストにも送る（docs/design-docs/onboarding.md）。
		e.OnboardingChanged()
	case domain.AssistantChanged:
		e.AssistantChanged()
	case domain.PeerStateChanged:
		e.PeerStateChanged()
	case domain.PeerChanged:
		e.peerChanged(event)
	}
}

// Close all SSE streams during shutdown so they cannot exhaust the HTTP grace period.
func (e *Events) Close() {
	e.once.Do(func() { close(e.closed) })
}

func (s *server) StreamEvents(w http.ResponseWriter, r *http.Request) {
	if s.events == nil {
		s.internalError(w, "Change notifications are not configured.", nil)
		return
	}
	// Subscribe before sending headers so the browser's initial reload cannot race missed changes.
	sub, unsubscribe := s.events.subscribe()
	defer unsubscribe()
	// Guests receive group and smartFolder events; other events expose owner activity or private videos.
	guest := !audienceFrom(r.Context()).IsOwner()

	controller := http.NewResponseController(w)

	header := w.Header()
	header.Set("Content-Type", "text/event-stream; charset=utf-8")
	header.Set("Cache-Control", cacheNoStore)
	// Disable buffering by intermediaries.
	header.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	if _, err := fmt.Fprintf(w, "retry: %d\n\n", eventsRetryMs); err != nil {
		return
	}
	if err := controller.Flush(); err != nil {
		s.logger.Warn("cannot send a change notification", slog.Any("error", err))
		return
	}

	keepAlive := time.NewTicker(eventsKeepAlive)
	defer keepAlive.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.events.closed:
			return
		case <-keepAlive.C:
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
				return
			}
		case <-sub.ready:
			if err := s.writePendingEvents(ctx, w, sub, guest); err != nil {
				if ctx.Err() == nil {
					s.logger.Warn("cannot send a change notification", slog.Any("error", err))
				}
				return
			}
		}
		if err := controller.Flush(); err != nil {
			return
		}
	}
}

func (s *server) writePendingEvents(ctx context.Context, w http.ResponseWriter, sub *eventSubscriber, guest bool) error {
	pending := sub.take()
	if guest {
		pending.scan = false
		pending.candidates = false
		pending.activity = false
		pending.assistant = false
		pending.videos = nil
	}
	if pending.scan && s.scans != nil {
		current, err := s.scans.CurrentScan(ctx)
		switch {
		case errors.Is(err, domain.ErrNotFound):
		case err != nil:
			return fmt.Errorf("cannot read the scan status: %w", err)
		default:
			if err := writeEvent(w, "scan", toAPIScan(current)); err != nil {
				return err
			}
		}
	}
	if pending.candidates {
		// EventSource ignores events without data, so payload-free notifications send an empty object.
		if err := writeEvent(w, "candidates", struct{}{}); err != nil {
			return err
		}
	}
	if pending.activity {
		if err := writeEvent(w, "activity", struct{}{}); err != nil {
			return err
		}
	}
	for _, id := range pending.videos {
		if err := writeEvent(w, "video", gen.VideoChanged{Id: id}); err != nil {
			return err
		}
	}
	if len(pending.groups) > 0 {
		// Coalesce IDs without names to avoid leaking guest-inaccessible group names.
		if err := writeEvent(w, "group", gen.GroupChanged{Ids: pending.groups}); err != nil {
			return err
		}
	}
	if len(pending.smartFolders) > 0 {
		// As with groups, coalesce IDs without names.
		if err := writeEvent(w, "smartFolder", gen.SmartFolderChanged{Ids: pending.smartFolders}); err != nil {
			return err
		}
	}
	if pending.assistant {
		if err := writeEvent(w, "assistant", struct{}{}); err != nil {
			return err
		}
	}
	if pending.peer && s.pairing != nil {
		if err := s.writePeerEvent(ctx, w, guest); err != nil {
			return err
		}
	}
	if pending.onboarding && !guest {
		// candidates と同じく中身の無い知らせで、data は空のオブジェクトにする。
		if err := writeEvent(w, "onboarding", struct{}{}); err != nil {
			return err
		}
	}
	return nil
}

func (s *server) writePeerEvent(ctx context.Context, w http.ResponseWriter, guest bool) error {
	state, err := s.pairing.State(ctx)
	if err != nil {
		return fmt.Errorf("cannot read the pairing state: %w", err)
	}
	audience := domain.AudienceOwner
	if guest {
		audience = domain.AudienceGuest
	}
	return writeEvent(w, "peer", peerStateBody(state, audience))
}

func writeEvent(w http.ResponseWriter, name string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, data)
	return err
}
