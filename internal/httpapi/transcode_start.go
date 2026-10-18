package httpapi

import (
	"context"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/syudead/vv/internal/httpapi/gen"
)

// Retain finished offsets briefly so reload-triggered reports can still retrieve them; tests can shorten the duration.
var transcodeStartRetention = 60 * time.Second

var transcodeAttemptPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func validTranscodeAttempt(attempt string) bool {
	return transcodeAttemptPattern.MatchString(attempt)
}

// Include video ID in the key so attempts cannot be read through another video's route.
type transcodeStartKey struct {
	videoID int64
	attempt string
}

// Close and replace changed on every state change to wake reporting requests.
type transcodeStartSlot struct {
	active  int
	waiters int
	// A later failed request must not erase an already-resolved offset.
	resolved bool
	failed   bool
	startMs  int64
	// Increment generation on each start to invalidate old cleanup timers.
	generation uint64
	changed    chan struct{}
}

type transcodeStarts struct {
	mu    sync.Mutex
	slots map[transcodeStartKey]*transcodeStartSlot
}

func newTranscodeStarts() *transcodeStarts {
	return &transcodeStarts{slots: map[transcodeStartKey]*transcodeStartSlot{}}
}

// Caller must hold mu.
func (l *transcodeStarts) slotLocked(key transcodeStartKey) *transcodeStartSlot {
	slot, ok := l.slots[key]
	if !ok {
		slot = &transcodeStartSlot{changed: make(chan struct{})}
		l.slots[key] = slot
	}
	return slot
}

func (slot *transcodeStartSlot) notifyLocked() {
	close(slot.changed)
	slot.changed = make(chan struct{})
}

// begin returns resolve/finish callbacks; finishing before resolution wakes waiters with failure.
func (l *transcodeStarts) begin(key transcodeStartKey) (resolve func(startMs int64), end func()) {
	l.mu.Lock()
	slot := l.slotLocked(key)
	slot.active++
	slot.generation++
	slot.notifyLocked()
	l.mu.Unlock()

	resolved := false
	resolve = func(startMs int64) {
		l.mu.Lock()
		defer l.mu.Unlock()
		resolved = true
		slot.resolved, slot.failed, slot.startMs = true, false, startMs
		slot.notifyLocked()
	}
	end = func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		slot.active--
		if !resolved && !slot.resolved {
			slot.failed = true
		}
		slot.notifyLocked()
		if slot.active > 0 {
			return
		}
		generation := slot.generation
		time.AfterFunc(transcodeStartRetention, func() {
			l.mu.Lock()
			defer l.mu.Unlock()
			if l.slots[key] == slot && slot.active == 0 && slot.generation == generation {
				delete(l.slots, key)
			}
		})
	}
	return resolve, end
}

// wait also waits for not-yet-registered attempts and returns false on failure, timeout, or cancellation.
func (l *transcodeStarts) wait(ctx context.Context, key transcodeStartKey, timeout time.Duration) (int64, bool) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	l.mu.Lock()
	slot := l.slotLocked(key)
	slot.waiters++
	defer func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		slot.waiters--
		// Remove report-only slots when no waiters remain.
		if slot.waiters == 0 && slot.active == 0 && slot.generation == 0 && l.slots[key] == slot {
			delete(l.slots, key)
		}
	}()
	for {
		if slot.resolved {
			startMs := slot.startMs
			l.mu.Unlock()
			return startMs, true
		}
		if slot.failed && slot.active == 0 {
			l.mu.Unlock()
			return 0, false
		}
		changed := slot.changed
		l.mu.Unlock()
		select {
		case <-changed:
		case <-timer.C:
			return 0, false
		case <-ctx.Done():
			return 0, false
		}
		l.mu.Lock()
	}
}

func (s *server) GetTranscodeStart(w http.ResponseWriter, r *http.Request, id gen.VideoId, params gen.GetTranscodeStartParams) {
	// Apply the same guest visibility checks as the transcode route.
	video, r, release, ok := s.lookupServedVideo(w, r, id)
	if !ok {
		return
	}
	defer release()
	if !validTranscodeAttempt(params.Attempt) {
		s.invalidRequest(w, "Invalid attempt.")
		return
	}
	startMs, ok := s.transcodeStarts.wait(r.Context(),
		transcodeStartKey{videoID: video.ID, attempt: params.Attempt}, transcodeStartupTimeout)
	if !ok {
		if r.Context().Err() != nil {
			return
		}
		s.notFound(w, "No start position for this transcode.")
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, gen.TranscodeStart{StartMs: startMs}, s.logger)
}
