package eventbus

import (
	"io"
	"log/slog"
	"slices"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/syudead/vv/internal/domain"
)

func newTestBus() *Bus {
	return New(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// recorder は受け取った変化を記録する。
type recorder struct {
	mu     sync.Mutex
	events []domain.Event
}

func (r *recorder) handle(event domain.Event) {
	r.mu.Lock()
	r.events = append(r.events, event)
	r.mu.Unlock()
}

func (r *recorder) snapshot() []domain.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.events)
}

// 購読者は発行された順に受け取る。On は指定した種類だけを渡す。
func TestPublishDeliversInOrderAndOnFiltersByType(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bus := newTestBus()
		defer bus.Close()
		all := &recorder{}
		bus.Subscribe("all", all.handle)
		scans := &recorder{}
		On(bus, "scans", func(event domain.ScanChanged) { scans.handle(event) })

		bus.Publish(domain.ScanChanged{}, domain.ProcessingChanged{})
		bus.Publish(domain.VideoIngestChanged{VideoID: 3, Stage: domain.JobProbe})

		bus.Close()
		want := []domain.Event{
			domain.ScanChanged{}, domain.ProcessingChanged{},
			domain.VideoIngestChanged{VideoID: 3, Stage: domain.JobProbe},
		}
		if got := all.snapshot(); !slices.Equal(got, want) {
			t.Fatalf("受け取り = %v, want %v", got, want)
		}
		if got := scans.snapshot(); !slices.Equal(got, []domain.Event{domain.ScanChanged{}}) {
			t.Fatalf("種類を絞った受け取り = %v", got)
		}
	})
}

// 遅い購読者も panic した購読者も、発行する側と他の購読者を止めない。
func TestSlowOrPanickingSubscriberDoesNotBlockPublisher(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bus := newTestBus()
		release := make(chan struct{})
		defer func() { close(release); bus.Close() }()
		entered := make(chan struct{}, 2)
		bus.Subscribe("slow", func(domain.Event) { entered <- struct{}{}; <-release })
		panics := 0
		var panicsMu sync.Mutex
		bus.Subscribe("panicking", func(domain.Event) {
			panicsMu.Lock()
			panics++
			panicsMu.Unlock()
			panic("subscriber failed")
		})
		fast := &recorder{}
		bus.Subscribe("fast", fast.handle)
		published := make(chan struct{})
		go func() {
			for range 2 {
				bus.Publish(domain.ProcessingChanged{})
			}
			close(published)
		}()
		synctest.Wait()
		select {
		case <-entered:
		default:
			t.Fatal("slow subscriber did not reach the blocked callback")
		}
		select {
		case <-published:
		default:
			t.Fatal("slow subscriber blocked publisher")
		}
		if got := fast.snapshot(); !slices.Equal(got, []domain.Event{domain.ProcessingChanged{}, domain.ProcessingChanged{}}) {
			t.Fatalf("fast subscriber got %v", got)
		}
		panicsMu.Lock()
		defer panicsMu.Unlock()
		if panics != 2 {
			t.Fatalf("calls after panic = %d, want 2", panics)
		}
	})
}

// 購読をやめた後は渡さない。やめる関数は、呼び出し中の処理が終わってから戻る。
func TestUnsubscribeStopsDeliveryAndWaitsForHandler(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bus := newTestBus()
		defer bus.Close()
		entered := make(chan struct{})
		release := make(chan struct{})
		releaseHandler := sync.OnceFunc(func() { close(release) })
		defer releaseHandler()
		var mu sync.Mutex
		calls := 0
		unsubscribe := bus.Subscribe("sub", func(domain.Event) {
			mu.Lock()
			calls++
			first := calls == 1
			mu.Unlock()
			if first {
				close(entered)
				<-release
			}
		})
		bus.Publish(domain.ScanChanged{})
		<-entered
		// One callback is running and a second event is still queued.
		bus.Publish(domain.ProcessingChanged{})
		stopped := make(chan struct{})
		go func() { unsubscribe(); close(stopped) }()
		synctest.Wait()
		select {
		case <-stopped:
			t.Fatal("unsubscribe returned before the running callback finished")
		default:
		}
		releaseHandler()
		<-stopped
		bus.Publish(domain.VideoIngestChanged{VideoID: 3})
		synctest.Wait()
		mu.Lock()
		defer mu.Unlock()
		if calls != 1 {
			t.Fatalf("calls = %d, want only the running callback", calls)
		}
	})
}

// Close は積んである分を渡し終えるまで待ち、その後の発行は渡さない。
func TestCloseDrainsPendingAndDropsLaterEvents(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bus := newTestBus()
		defer bus.Close()
		release := make(chan struct{})
		releaseHandler := sync.OnceFunc(func() { close(release) })
		defer releaseHandler()
		var mu sync.Mutex
		var keys []string
		On(bus, "release", func(event domain.ContentUnreferenced) {
			<-release
			mu.Lock()
			keys = append(keys, event.ContentKeys...)
			mu.Unlock()
		})
		bus.Publish(domain.ContentUnreferenced{ContentKeys: []string{"a"}})
		bus.Publish(domain.ContentUnreferenced{ContentKeys: []string{"b"}})
		closed := make(chan struct{})
		go func() { bus.Close(); close(closed) }()
		synctest.Wait()
		select {
		case <-closed:
			t.Fatal("Close returned before pending delivery finished")
		default:
		}
		releaseHandler()
		<-closed
		bus.Publish(domain.ContentUnreferenced{ContentKeys: []string{"c"}})
		synctest.Wait()
		mu.Lock()
		defer mu.Unlock()
		if !slices.Equal(keys, []string{"a", "b"}) {
			t.Fatalf("delivered %v, want [a b]", keys)
		}
	})
}

// 並行した発行も、すべての購読者に同じ順番で届く。
func TestConcurrentPublishersKeepOneOrderForEverySubscriber(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		bus := newTestBus()
		first, second := &recorder{}, &recorder{}
		bus.Subscribe("first", first.handle)
		bus.Subscribe("second", second.handle)

		start := make(chan struct{})
		var publishers sync.WaitGroup
		for publisher := range 2 {
			publishers.Go(func() {
				<-start
				for i := range 2 {
					bus.Publish(domain.VideoIngestChanged{VideoID: int64(publisher*2 + i)})
				}
			})
		}
		close(start)
		publishers.Wait()
		bus.Close()

		got := first.snapshot()
		if len(got) != 4 {
			t.Fatalf("受け取り = %d 件, want 4", len(got))
		}
		seen := map[int64]bool{}
		for _, event := range got {
			id := event.(domain.VideoIngestChanged).VideoID
			if id < 0 || id >= 4 || seen[id] {
				t.Fatalf("unexpected or repeated video ID %d", id)
			}
			seen[id] = true
		}
		if !slices.Equal(got, second.snapshot()) {
			t.Fatal("購読者ごとに順番が違う")
		}
	})
}
