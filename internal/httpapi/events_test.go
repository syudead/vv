package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/httpapi/gen"
)

// fakeProcessing は段階ごとの残りを差し替える。
type fakeProcessing struct {
	mu      sync.Mutex
	current domain.Processing
}

func (f *fakeProcessing) Processing(context.Context) (domain.Processing, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.current, nil
}

func (f *fakeProcessing) set(p domain.Processing) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.current = p
}

// sseEvent は受け取ったイベント1件である。
type sseEvent struct {
	name string
	data string
}

// openEvents は /api/events へつなぎ、受け取ったイベントを流すチャネルを返す。
func openEvents(t *testing.T, handler http.Handler) (<-chan sseEvent, *http.Response) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	events := make(chan sseEvent, 32)
	go func() {
		defer close(events)
		scanner := bufio.NewScanner(resp.Body)
		var current sseEvent
		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case line == "":
				if current.name != "" {
					events <- current
				}
				current = sseEvent{}
			case strings.HasPrefix(line, "event: "):
				current.name = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				current.data = strings.TrimPrefix(line, "data: ")
			}
		}
	}()
	return events, resp
}

func nextEvent(t *testing.T, events <-chan sseEvent) sseEvent {
	t.Helper()
	select {
	case event, ok := <-events:
		if !ok {
			t.Fatal("イベントの流れが終わった")
		}
		return event
	case <-time.After(2 * time.Second):
		t.Fatal("イベントが届かない")
	}
	return sseEvent{}
}

func expectNoEvent(t *testing.T, events <-chan sseEvent) {
	t.Helper()
	select {
	case event := <-events:
		t.Fatalf("知らせていないのにイベントが届いた: %+v", event)
	case <-time.After(100 * time.Millisecond):
	}
}

// つないだ直後に今のスキャンと残りを1回ずつ送り、その後は知らせがあった
// ときだけ送る。切れていた間の変化は、つなぎ直した直後の送信で取り戻せる。
func TestStreamEventsSendsCurrentStateThenOnlyChanges(t *testing.T) {
	events := NewEvents()
	processing := &fakeProcessing{current: domain.Processing{Probe: 3, Thumbnail: 2, Preview: 1}}
	scans := &fakeScans{hasScan: true, current: domain.Scan{ID: 4, State: domain.ScanRunning, Total: 10, Completed: 2}}
	handler := newTestServer(t, Options{Scans: scans, Processing: processing, Events: events})

	stream, resp := openEvents(t, handler)
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}

	// 残りをスキャンより先に送る。画面はスキャンの完了を受けた時点の残りで
	// 完了を知らせるかどうかを決める。
	processingEvent := nextEvent(t, stream)
	if processingEvent.name != "processing" {
		t.Fatalf("最初のイベント = %q, want processing", processingEvent.name)
	}
	var remaining gen.Processing
	if err := json.Unmarshal([]byte(processingEvent.data), &remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != (gen.Processing{Probe: 3, Thumbnail: 2, Preview: 1}) {
		t.Errorf("processing = %+v", remaining)
	}

	scanEvent := nextEvent(t, stream)
	if scanEvent.name != "scan" {
		t.Fatalf("2つ目のイベント = %q, want scan", scanEvent.name)
	}
	var scan gen.Scan
	if err := json.Unmarshal([]byte(scanEvent.data), &scan); err != nil {
		t.Fatal(err)
	}
	if scan.Id != 4 || scan.Completed != 2 {
		t.Errorf("scan = %+v", scan)
	}

	// 知らせが無ければ何も送らない。
	expectNoEvent(t, stream)

	processing.set(domain.Processing{Probe: 2, Thumbnail: 2, Preview: 1})
	events.VideoChanged(9)
	events.ProcessingChanged()

	got := map[string]string{}
	for range 2 {
		event := nextEvent(t, stream)
		got[event.name] = event.data
	}
	var changed gen.VideoChanged
	if err := json.Unmarshal([]byte(got["video"]), &changed); err != nil {
		t.Fatalf("video イベントを読めない: %v (%v)", err, got)
	}
	if changed.Id != 9 {
		t.Errorf("video.id = %d, want 9", changed.Id)
	}
	if err := json.Unmarshal([]byte(got["processing"]), &remaining); err != nil {
		t.Fatal(err)
	}
	if remaining.Probe != 2 {
		t.Errorf("processing.probe = %d, want 2（送る直前の値）", remaining.Probe)
	}
}

// 送る前に重なった知らせは1回にまとめる。遅い接続のために知らせを溜め込まない。
func TestStreamEventsCoalescesPendingChanges(t *testing.T) {
	events := NewEvents()
	sub, unsubscribe := events.subscribe()
	defer unsubscribe()
	sub.take()

	for range 5 {
		events.ProcessingChanged()
		events.VideoChanged(1)
	}
	events.VideoChanged(2)

	scan, processing, videos := sub.take()
	if scan || !processing {
		t.Errorf("scan = %v, processing = %v, want false と true", scan, processing)
	}
	if len(videos) != 2 || videos[0] != 1 || videos[1] != 2 {
		t.Errorf("videos = %v, want [1 2]", videos)
	}
}

// まだ一度も取り込んでいなければ、scan は送らない。
func TestStreamEventsSkipsScanBeforeFirstScan(t *testing.T) {
	handler := newTestServer(t, Options{
		Scans:      &fakeScans{},
		Processing: &fakeProcessing{},
		Events:     NewEvents(),
	})
	stream, _ := openEvents(t, handler)
	if event := nextEvent(t, stream); event.name != "processing" {
		t.Fatalf("最初のイベント = %q, want processing", event.name)
	}
}

// 停止時に Close すると、接続は終わる。終わらない応答が HTTP サーバーの停止を
// 猶予時間いっぱいまで待たせない。
func TestStreamEventsEndsOnClose(t *testing.T) {
	events := NewEvents()
	handler := newTestServer(t, Options{Scans: &fakeScans{}, Processing: &fakeProcessing{}, Events: events})
	stream, _ := openEvents(t, handler)
	nextEvent(t, stream)

	events.Close()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-stream:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("Close しても接続が終わらない")
		}
	}
}

// 段階ごとの残りを返す。
func TestGetProcessing(t *testing.T) {
	handler := newTestServer(t, Options{
		Processing: &fakeProcessing{current: domain.Processing{Probe: 1, Thumbnail: 4, Preview: 7}},
	})
	rec := do(t, handler, http.MethodGet, "/api/processing")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	got := decode[gen.Processing](t, rec)
	if got != (gen.Processing{Probe: 1, Thumbnail: 4, Preview: 7}) {
		t.Errorf("processing = %+v", got)
	}
}

// スキャンの知らせは段階ごとの残りと1回で記録する。別々に記録すると、その間に
// 送信が走り、スキャンだけが古い残りとともに届くことがある。
func TestScanChangedAlsoMarksProcessing(t *testing.T) {
	events := NewEvents()
	sub, unsubscribe := events.subscribe()
	defer unsubscribe()
	sub.take()

	events.ScanChanged()

	scan, processing, _ := sub.take()
	if !scan || !processing {
		t.Errorf("scan = %v, processing = %v, want 両方 true", scan, processing)
	}
}

// 状態の変化は、従来と同じ知らせ（scan・processing・video）に置き換わる。
// 走査の変化は残りも一緒に送る。知らせる対象ではない変化は無視する。
func TestEventsHandleMapsDomainEvents(t *testing.T) {
	events := NewEvents()
	sub, unsubscribe := events.subscribe()
	defer unsubscribe()
	sub.take() // つないだ直後の送信を除く。

	events.Handle(domain.JobsQueued{Kinds: domain.JobKinds})
	events.Handle(domain.ContentUnreferenced{ContentKeys: []string{"k"}})
	if scan, processing, videos := sub.take(); scan || processing || len(videos) != 0 {
		t.Fatalf("知らせない変化で知らせた: scan=%v processing=%v videos=%v", scan, processing, videos)
	}

	events.Handle(domain.ScanChanged{})
	if scan, processing, _ := sub.take(); !scan || !processing {
		t.Fatalf("ScanChanged: scan=%v processing=%v, want 両方", scan, processing)
	}
	events.Handle(domain.ProcessingChanged{})
	if scan, processing, _ := sub.take(); scan || !processing {
		t.Fatalf("ProcessingChanged: scan=%v processing=%v", scan, processing)
	}
	// 同じ動画の続けての変化は1つにまとまる。
	events.Handle(domain.VideoIngestChanged{VideoID: 5, Stage: domain.JobProbe})
	events.Handle(domain.VideoIngestChanged{VideoID: 5, Stage: domain.JobThumbnail})
	events.Handle(domain.VideoIngestChanged{VideoID: 6})
	if _, _, videos := sub.take(); !slices.Equal(videos, []int64{5, 6}) {
		t.Fatalf("videos = %v, want [5 6]", videos)
	}
}
