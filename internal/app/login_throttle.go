package app

import (
	"container/list"
	"net/netip"
	"sync"
	"time"
)

const (
	loginFailureWindow = 5 * time.Minute
	// Count active verification reservations together with failures.
	loginFailureLimit         = 5
	defaultMaxThrottleSources = 10000
	ipv6SourcePrefix          = 64
)

// Reserve before verifying, then fail/succeed/cancel exactly once; counting reservations prevents
// concurrent requests exceeding the limit. Evict least-recently-used sources only when they have no
// active reservations.
type loginThrottle struct {
	mu         sync.Mutex
	maxSources int
	sources    map[string]*list.Element
	// Newest sources first.
	order *list.List
}

type throttleSource struct {
	key     string
	pending int
	// Oldest failures first, capped to loginFailureLimit.
	failures []time.Time
}

func newLoginThrottle(maxSources int) *loginThrottle {
	if maxSources <= 0 {
		maxSources = defaultMaxThrottleSources
	}
	return &loginThrottle{maxSources: maxSources, sources: map[string]*list.Element{}, order: list.New()}
}

// Normalize mapped IPv4 to per-address buckets, IPv6 to /64, and invalid addresses to one shared
// bucket.
func throttleKey(addr netip.Addr) string {
	addr = addr.Unmap()
	switch {
	case addr.Is4():
		return addr.String()
	case addr.Is6():
		prefix, err := addr.WithZone("").Prefix(ipv6SourcePrefix)
		if err != nil {
			return "invalid"
		}
		return prefix.String()
	default:
		return "invalid"
	}
}

// Return a retry estimate without reserving when active reservations plus failures reach the limit.
func (t *loginThrottle) reserve(key string, now time.Time) (time.Duration, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	source := t.touch(key)
	source.prune(now)
	if source.pending+len(source.failures) >= loginFailureLimit {
		if len(source.failures) > 0 {
			return source.failures[0].Add(loginFailureWindow).Sub(now), false
		}
		// Only active reservations occupy the bucket; their imminent completion warrants a short retry
		// estimate.
		return time.Second, false
	}
	source.pending++
	return 0, true
}

func (t *loginThrottle) fail(key string, now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	source := t.touch(key)
	source.release()
	source.prune(now)
	source.failures = append(source.failures, now)
	if over := len(source.failures) - loginFailureLimit; over > 0 {
		source.failures = append([]time.Time(nil), source.failures[over:]...)
	}
}

// Keep other in-flight reservations while clearing this attempt and failure history.
func (t *loginThrottle) succeed(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	element, ok := t.sources[key]
	if !ok {
		return
	}
	source := element.Value.(*throttleSource)
	source.release()
	source.failures = nil
	if source.pending == 0 {
		t.order.Remove(element)
		delete(t.sources, key)
	}
}

func (t *loginThrottle) cancel(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	element, ok := t.sources[key]
	if !ok {
		return
	}
	source := element.Value.(*throttleSource)
	source.release()
	if source.pending == 0 && len(source.failures) == 0 {
		t.order.Remove(element)
		delete(t.sources, key)
	}
}

// Caller holds t.mu; creating a record may evict old sources beyond capacity.
func (t *loginThrottle) touch(key string) *throttleSource {
	if element, ok := t.sources[key]; ok {
		t.order.MoveToFront(element)
		return element.Value.(*throttleSource)
	}
	source := &throttleSource{key: key}
	t.sources[key] = t.order.PushFront(source)
	for element := t.order.Back(); len(t.sources) > t.maxSources && element != nil; {
		prev := element.Prev()
		old := element.Value.(*throttleSource)
		if old.pending == 0 && old != source {
			t.order.Remove(element)
			delete(t.sources, old.key)
		}
		element = prev
	}
	return source
}

func (s *throttleSource) release() {
	if s.pending > 0 {
		s.pending--
	}
}

func (s *throttleSource) prune(now time.Time) {
	cutoff := now.Add(-loginFailureWindow)
	keep := 0
	for keep < len(s.failures) && !s.failures[keep].After(cutoff) {
		keep++
	}
	if keep > 0 {
		s.failures = append([]time.Time(nil), s.failures[keep:]...)
	}
}
