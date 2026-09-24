package eventbus

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/syudead/vv/internal/domain"
)

// Bus は発行された変化を購読者へ配る。
//
// 購読者ごとに goroutine と順番を保つ待ち行列を持つ。発行は待ち行列に積む
// だけなので、遅い購読者や panic した購読者が、発行する側（取引の確定、
// ワーカー、走査）を止めることはない。
type Bus struct {
	logger *slog.Logger

	mu     sync.Mutex
	subs   map[*subscription]struct{}
	closed bool
}

// New は購読者の無い Bus を返す。logger が nil なら slog の既定を使う。
func New(logger *slog.Logger) *Bus {
	if logger == nil {
		logger = slog.Default()
	}
	return &Bus{logger: logger, subs: map[*subscription]struct{}{}}
}

// subscription は1つの購読者と、まだ渡していない変化である。
type subscription struct {
	name   string
	handle func(domain.Event)

	mu      sync.Mutex
	pending []domain.Event
	// closing は Bus を閉じたことを表す。積んである分を渡し終えたら止まる。
	closing bool
	// ready は渡すものがあることを知らせる。容量1で、重なった知らせはまとまる。
	ready chan struct{}
	// stop は購読をやめたことを表す。積んである分は捨てる。
	stop     chan struct{}
	stopOnce sync.Once
	done     chan struct{}
}

// Publish は変化を購読者へ配る。購読者の処理は待たない。閉じた後は何もしない。
//
// 全購読者の待ち行列へ積み終えるまで Bus の錠を持つ。積むのは待ち行列への
// 追加だけなので短い。錠を持つので、並行した発行が購読者ごとに違う順番で
// 届くことはなく、始まった発行の途中に Close が割り込んで取りこぼすこともない。
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

// Subscribe は、発行された変化をすべて handle へ渡す購読者を登録する。name は
// 記録に出す名前である。handle は1つの goroutine から、発行された順に呼ばれる。
//
// 返す関数で購読をやめる。やめた後は handle を呼ばず、呼び出し中のものが
// あれば終わるのを待ってから戻る。handle の中からは呼ばないこと。
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

// On は E の変化だけを handle へ渡す購読者を登録する。Subscribe と同じく、
// 返す関数で購読をやめる。
func On[E domain.Event](b *Bus, name string, handle func(E)) (unsubscribe func()) {
	return b.Subscribe(name, func(event domain.Event) {
		if typed, ok := event.(E); ok {
			handle(typed)
		}
	})
}

// Close は新しい発行を受け付けなくし、すでに積んである変化を購読者へ渡し
// 終えるのを待つ。停止時に、発行する側を止めてから呼ぶ。購読をやめた購読者の
// 分は待たない。
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

// next は次に渡す変化を取り出す。無ければ ok は false で、closing は閉じた
// あとで積んである分も無いことを表す。
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

// deliver は1件を渡す。購読者が panic しても、次の変化は渡し続ける。
func (s *subscription) deliver(event domain.Event, logger *slog.Logger) {
	defer func() {
		if recovered := recover(); recovered != nil {
			logger.Error("変化の購読者が panic しました",
				slog.String("subscriber", s.name),
				slog.String("event", fmt.Sprintf("%T", event)),
				slog.Any("panic", recovered))
		}
	}()
	s.handle(event)
}
