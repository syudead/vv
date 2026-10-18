package eventbus

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/syudead/vv/internal/domain"
)

// Bus gives each subscriber an ordered queue and goroutine, isolating publishers from slow or
// panicking handlers.
type Bus struct {
	logger *slog.Logger

	mu     sync.Mutex
	subs   map[*subscription]struct{}
	closed bool
}

// New uses slog.Default when logger is nil.
func New(logger *slog.Logger) *Bus {
	if logger == nil {
		logger = slog.Default()
	}
	return &Bus{logger: logger, subs: map[*subscription]struct{}{}}
}

type subscription struct {
	name   string
	handle func(domain.Event)

	mu      sync.Mutex
	pending []domain.Event
	// Drain queued events before stopping.
	closing bool
	// Capacity one coalesces readiness notifications.
	ready chan struct{}
	// Unsubscription discards queued events.
	stop     chan struct{}
	stopOnce sync.Once
	done     chan struct{}
}

// Publish holds the bus lock through enqueueing for every subscriber, preserving a common order and
// preventing Close from interrupting delivery; handlers run asynchronously and closed buses ignore
// events.
func (b *Bus) Publish(events ...domain.Event) {
	if len(events) == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	for sub := range b.subs {
		sub.enqueue(events)
	}
}

// Subscribe invokes handle serially in publication order; unsubscribe waits for any active call and
// prevents later calls. Never unsubscribe from inside handle.
func (b *Bus) Subscribe(name string, handle func(domain.Event)) (unsubscribe func()) {
	sub := &subscription{
		name:   name,
		handle: handle,
		ready:  make(chan struct{}, 1),
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return func() {}
	}
	b.subs[sub] = struct{}{}
	b.mu.Unlock()

	go sub.run(b.logger)

	return func() {
		b.mu.Lock()
		delete(b.subs, sub)
		b.mu.Unlock()
		sub.stopOnce.Do(func() { close(sub.stop) })
		<-sub.done
	}
}

// On filters events by type E and returns an unsubscribe function.
func On[E domain.Event](b *Bus, name string, handle func(E)) (unsubscribe func()) {
	return b.Subscribe(name, func(event domain.Event) {
		if typed, ok := event.(E); ok {
			handle(typed)
		}
	})
}

// Close rejects new events and drains active subscribers; call it after stopping publishers. It
// does not wait for unsubscribed handlers.
func (b *Bus) Close() {
	b.mu.Lock()
	b.closed = true
	subs := make([]*subscription, 0, len(b.subs))
	for sub := range b.subs {
		subs = append(subs, sub)
	}
	b.mu.Unlock()
	for _, sub := range subs {
		sub.mu.Lock()
		sub.closing = true
		sub.mu.Unlock()
		sub.signal()
	}
	for _, sub := range subs {
		<-sub.done
	}
}

func (s *subscription) enqueue(events []domain.Event) {
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		return
	}
	s.pending = append(s.pending, events...)
	s.mu.Unlock()
	s.signal()
}

func (s *subscription) signal() {
	select {
	case s.ready <- struct{}{}:
	default:
	}
}

// closing means closed and fully drained, not merely temporarily empty.
func (s *subscription) next() (event domain.Event, ok, closing bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.pending) == 0 {
		s.pending = nil
		return nil, false, s.closing
	}
	event = s.pending[0]
	s.pending[0] = nil
	s.pending = s.pending[1:]
	return event, true, false
}

func (s *subscription) run(logger *slog.Logger) {
	defer close(s.done)
	for {
		select {
		case <-s.stop:
			return
		case <-s.ready:
		}
		for {
			select {
			case <-s.stop:
				return
			default:
			}
			event, ok, closing := s.next()
			if closing {
				return
			}
			if !ok {
				break
			}
			s.deliver(event, logger)
		}
	}
}

// Recover handler panics so later events still arrive.
func (s *subscription) deliver(event domain.Event, logger *slog.Logger) {
	defer func() {
		if recovered := recover(); recovered != nil {
			logger.Error("event subscriber panicked",
				slog.String("subscriber", s.name),
				slog.String("event", fmt.Sprintf("%T", event)),
				slog.Any("panic", recovered))
		}
	}()
	s.handle(event)
}
