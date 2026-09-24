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

// eventsKeepAlive は、変化が無いあいだに接続を保つためのコメント行を送る間隔
// である。途中の中継や OS が無通信の接続を切るのを防ぐだけで、状態は送らない。
const eventsKeepAlive = 30 * time.Second

// eventsRetryMs は、切れたときにブラウザがつなぎ直すまでの待ち時間である。
const eventsRetryMs = 3000

// Processing は段階ごとの残りの問い合わせ先である。internal/store の *DB が
// これを満たす。
type Processing interface {
	Processing(ctx context.Context) (domain.Processing, error)
}

// Events は画面へ送る変化の知らせを配る。
//
// 知らせるのは「何が変わったか」だけで、送る内容は送る直前に読み直す。
// 知らせが続けて届いても、まだ送っていない分は1つにまとめるので、遅い
// 接続のために知らせを捨てたり、送る側を待たせたりしない。
type Events struct {
	mu     sync.Mutex
	subs   map[*eventSubscriber]struct{}
	closed chan struct{}
	once   sync.Once
}

// NewEvents は配り先の無い Events を返す。
func NewEvents() *Events {
	return &Events{subs: map[*eventSubscriber]struct{}{}, closed: make(chan struct{})}
}

// eventSubscriber は1本の接続が、まだ送っていない変化を持つ。
type eventSubscriber struct {
	mu         sync.Mutex
	scan       bool
	processing bool
	videos     map[int64]struct{}
	// ready は送るものがあることを知らせる。容量1で、重なった知らせはまとまる。
	ready chan struct{}
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

// take はまだ送っていない変化を取り出して空にする。
func (s *eventSubscriber) take() (scan, processing bool, videos []int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	scan, processing = s.scan, s.processing
	for id := range s.videos {
		videos = append(videos, id)
	}
	slices.Sort(videos)
	s.scan, s.processing = false, false
	clear(s.videos)
	return scan, processing, videos
}

func (e *Events) subscribe() (*eventSubscriber, func()) {
	sub := &eventSubscriber{videos: map[int64]struct{}{}, ready: make(chan struct{}, 1)}
	// つないだ直後に今の状態を1回送る。切れていた間の変化を取り戻すためである。
	sub.mark(func(s *eventSubscriber) {
		s.scan = true
		s.processing = true
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

// ScanChanged は直近のスキャンが変わったことを知らせる。
func (e *Events) ScanChanged() {
	e.publish(func(s *eventSubscriber) { s.scan = true })
}

// ProcessingChanged は段階ごとの残りが変わったかもしれないことを知らせる。
func (e *Events) ProcessingChanged() {
	e.publish(func(s *eventSubscriber) { s.processing = true })
}

// VideoChanged は動画の状態が変わったことを知らせる。
func (e *Events) VideoChanged(id int64) {
	e.publish(func(s *eventSubscriber) { s.videos[id] = struct{}{} })
}

// Close はすべての接続を終わらせる。停止時に呼ぶ。流れ続ける応答が残ると、
// HTTP サーバーの停止が猶予時間いっぱいまで待たされる。
func (e *Events) Close() {
	e.once.Do(func() { close(e.closed) })
}

// StreamEvents は変化を Server-Sent Events で送る（GET /api/events）。
func (s *server) StreamEvents(w http.ResponseWriter, r *http.Request) {
	if s.events == nil {
		s.internalError(w, "変化の知らせの経路が設定されていません", nil)
		return
	}
	controller := http.NewResponseController(w)

	header := w.Header()
	header.Set("Content-Type", "text/event-stream; charset=utf-8")
	header.Set("Cache-Control", cacheNoStore)
	// 途中の中継がまとめて送ろうと溜め込むのを止める。
	header.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	if _, err := fmt.Fprintf(w, "retry: %d\n\n", eventsRetryMs); err != nil {
		return
	}
	if err := controller.Flush(); err != nil {
		s.logger.Warn("変化の知らせを送れません", slog.Any("error", err))
		return
	}

	sub, unsubscribe := s.events.subscribe()
	defer unsubscribe()

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
			if err := s.writePendingEvents(ctx, w, sub); err != nil {
				if ctx.Err() == nil {
					s.logger.Warn("変化の知らせを送れません", slog.Any("error", err))
				}
				return
			}
		}
		if err := controller.Flush(); err != nil {
			return
		}
	}
}

// writePendingEvents はまだ送っていない変化を、今の内容で書き出す。
func (s *server) writePendingEvents(ctx context.Context, w http.ResponseWriter, sub *eventSubscriber) error {
	scan, processing, videos := sub.take()
	if scan && s.scans != nil {
		current, err := s.scans.CurrentScan(ctx)
		switch {
		case errors.Is(err, domain.ErrNotFound):
		case err != nil:
			return fmt.Errorf("スキャンの状態を読めません: %w", err)
		default:
			if err := writeEvent(w, "scan", toAPIScan(current)); err != nil {
				return err
			}
		}
	}
	if processing && s.processing != nil {
		remaining, err := s.processing.Processing(ctx)
		if err != nil {
			return err
		}
		if err := writeEvent(w, "processing", toAPIProcessing(remaining)); err != nil {
			return err
		}
	}
	for _, id := range videos {
		if err := writeEvent(w, "video", gen.VideoChanged{Id: id}); err != nil {
			return err
		}
	}
	return nil
}

func writeEvent(w http.ResponseWriter, name string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, data)
	return err
}

// GetProcessing は段階ごとの残りを返す（GET /api/processing）。
func (s *server) GetProcessing(w http.ResponseWriter, r *http.Request) {
	if s.processing == nil {
		s.internalError(w, "取り込みの残りの経路が設定されていません", nil)
		return
	}
	remaining, err := s.processing.Processing(r.Context())
	if err != nil {
		s.internalError(w, "取り込みの残りを取得できませんでした", err)
		return
	}
	w.Header().Set("Cache-Control", cacheNoStore)
	writeJSON(w, http.StatusOK, toAPIProcessing(remaining), s.logger)
}

func toAPIProcessing(p domain.Processing) gen.Processing {
	return gen.Processing{Probe: p.Probe, Thumbnail: p.Thumbnail, Preview: p.Preview}
}
