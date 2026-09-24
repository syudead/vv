package eventbus

import (
	"io"
	"log/slog"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

func newTestBus() *Bus {
	return New(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// recorder は受け取った変化を記録する。
type recorder struct {
	mu     sync.Mutex
	events []domain.Event
	got    chan struct{}
}

func newRecorder() *recorder {
	return &recorder{got: make(chan struct{}, 100)}
}

func (r *recorder) handle(event domain.Event) {
	r.mu.Lock()
	r.events = append(r.events, event)
	r.mu.Unlock()
	r.got <- struct{}{}
}

func (r *recorder) snapshot() []domain.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.events)
}

func (r *recorder) wait(t *testing.T, n int) {
	t.Helper()
	for range n {
		select {
		case <-r.got:
		case <-time.After(5 * time.Second):
			t.Fatalf("変化が届かない: %v", r.snapshot())
		}
	}
}

// 購読者は発行された順に受け取る。On は指定した種類だけを渡す。
func TestPublishDeliversInOrderAndOnFiltersByType(t *testing.T) {
	bus := newTestBus()
	defer bus.Close()
	all := newRecorder()
	bus.Subscribe("all", all.handle)
	scans := newRecorder()
	On(bus, "scans", func(event domain.ScanChanged) { scans.handle(event) })

	bus.Publish(domain.ScanChanged{}, domain.ProcessingChanged{})
	bus.Publish(domain.VideoIngestChanged{VideoID: 3, Stage: domain.JobProbe})

	all.wait(t, 3)
	want := []domain.Event{
		domain.ScanChanged{}, domain.ProcessingChanged{},
		domain.VideoIngestChanged{VideoID: 3, Stage: domain.JobProbe},
	}
	if got := all.snapshot(); !slices.Equal(got, want) {
		t.Fatalf("受け取り = %v, want %v", got, want)
	}
	scans.wait(t, 1)
	bus.Close()
	if got := scans.snapshot(); !slices.Equal(got, []domain.Event{domain.ScanChanged{}}) {
		t.Fatalf("種類を絞った受け取り = %v", got)
	}
}

// 遅い購読者も panic した購読者も、発行する側と他の購読者を止めない。
func TestSlowOrPanickingSubscriberDoesNotBlockPublisher(t *testing.T) {
	bus := newTestBus()
	release := make(chan struct{})
	bus.Subscribe("slow", func(domain.Event) { <-release })
	panics := 0
	var panicsMu sync.Mutex
	bus.Subscribe("panicking", func(domain.Event) {
		panicsMu.Lock()
		panics++
		panicsMu.Unlock()
		panic("購読者の失敗")
	})
	fast := newRecorder()
	bus.Subscribe("fast", fast.handle)

	published := make(chan struct{})
	go func() {
		for range 3 {
			bus.Publish(domain.ProcessingChanged{})
		}
		close(published)
	}()
	select {
	case <-published:
	case <-time.After(5 * time.Second):
		t.Fatal("遅い購読者が発行を止めた")
	}
	fast.wait(t, 3)

	close(release)
	bus.Close()
	panicsMu.Lock()
	defer panicsMu.Unlock()
	if panics != 3 {
		t.Fatalf("panic のあとも渡し続けるはず: %d 回, want 3", panics)
	}
}

// 購読をやめた後は渡さない。やめる関数は、呼び出し中の処理が終わってから戻る。
func TestUnsubscribeStopsDeliveryAndWaitsForHandler(t *testing.T) {
	bus := newTestBus()
	defer bus.Close()
	entered := make(chan struct{})
	release := make(chan struct{})
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

	bus.Publish(domain.ScanChanged{}, domain.ScanChanged{})
	<-entered
	stopped := make(chan struct{})
	go func() {
		unsubscribe()
		close(stopped)
	}()
	select {
	case <-stopped:
		t.Fatal("呼び出し中の処理を待たずに戻った")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	<-stopped
	bus.Publish(domain.ScanChanged{})
	time.Sleep(20 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Fatalf("やめた後に渡した: %d 回, want 1", calls)
	}
}

// Close は積んである分を渡し終えるまで待ち、その後の発行は渡さない。
func TestCloseDrainsPendingAndDropsLaterEvents(t *testing.T) {
	bus := newTestBus()
	release := make(chan struct{})
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
	go func() {
		bus.Close()
		close(closed)
	}()
	close(release)
	<-closed
	bus.Publish(domain.ContentUnreferenced{ContentKeys: []string{"c"}})

	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(keys, []string{"a", "b"}) {
		t.Fatalf("渡した = %v, want [a b]", keys)
	}
}
