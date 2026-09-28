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

// つないだ直後に今のスキャンを1回送り、その後は知らせがあったときだけ送る。
// 切れていた間の変化は、つなぎ直した直後の送信で取り戻せる。processing の event は
// 送らない（specs/024-import-progress/contracts/scan-api.md §1・§4）。
func TestStreamEventsSendsCurrentStateThenOnlyChanges(t *testing.T) {
	events := NewEvents()
	scans := &lockedScans{current: domain.Scan{ID: 4, State: domain.ScanRunning}}
	handler := newTestServer(t, Options{Scans: scans, Events: events})

	stream, resp := openEvents(t, handler)
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}

	scanEvent := nextEvent(t, stream)
	if scanEvent.name != "scan" {
		t.Fatalf("最初のイベント = %q, want scan", scanEvent.name)
	}
	var scan gen.Scan
	if err := json.Unmarshal([]byte(scanEvent.data), &scan); err != nil {
		t.Fatal(err)
	}
	if scan.Id != 4 {
		t.Errorf("scan = %+v", scan)
	}

	// 知らせが無ければ何も送らない。
	expectNoEvent(t, stream)

	scans.setImport(domain.ImportProgress{Status: domain.ImportRunning, Counted: true, Total: 10, Settled: 3})
	events.VideoChanged(9)
	events.ProcessingChanged()

	// 仕事の成否で済みの本数が変わるので、scan を送る。
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
	scan = gen.Scan{}
	if err := json.Unmarshal([]byte(got["scan"]), &scan); err != nil {
		t.Fatalf("ProcessingChanged で scan が送られない: %v (%v)", err, got)
	}
	if scan.Videos == nil || scan.Videos.Settled != 3 {
		t.Errorf("scan.videos = %+v, want settled 3（送る直前の値）", scan.Videos)
	}
	if _, ok := got["processing"]; ok {
		t.Errorf("processing の event が送られた: %v", got)
	}
	expectNoEvent(t, stream)
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

	scan, videos := sub.take()
	if !scan {
		t.Error("scan = false, want true")
	}
	if len(videos) != 2 || videos[0] != 1 || videos[1] != 2 {
		t.Errorf("videos = %v, want [1 2]", videos)
	}
}

// まだ一度も取り込んでいなければ、scan は送らない。
func TestStreamEventsSkipsScanBeforeFirstScan(t *testing.T) {
	events := NewEvents()
	handler := newTestServer(t, Options{Scans: &fakeScans{}, Events: events})
	stream, _ := openEvents(t, handler)
	expectNoEvent(t, stream)
	// 動画の変化は送る（接続は生きている）。
	events.VideoChanged(3)
	if event := nextEvent(t, stream); event.name != "video" {
		t.Fatalf("イベント = %q, want video", event.name)
	}
}

// 停止時に Close すると、接続は終わる。終わらない応答が HTTP サーバーの停止を
// 猶予時間いっぱいまで待たせない。
func TestStreamEventsEndsOnClose(t *testing.T) {
	events := NewEvents()
	handler := newTestServer(t, Options{Scans: &fakeScans{hasScan: true}, Events: events})
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

// /api/processing はなくなった（specs/024-import-progress/contracts/scan-api.md §1）。
func TestProcessingRouteIsGone(t *testing.T) {
	handler := newTestServer(t, Options{})
	rec := do(t, handler, http.MethodGet, "/api/processing")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body)
	}
}

// 状態の変化は、知らせ（scan・video）に置き換わる。知らせる対象ではない変化は無視する。
func TestEventsHandleMapsDomainEvents(t *testing.T) {
	events := NewEvents()
	sub, unsubscribe := events.subscribe()
	defer unsubscribe()
	sub.take() // つないだ直後の送信を除く。

	events.Handle(domain.JobsQueued{Kinds: domain.JobKinds})
	events.Handle(domain.ContentUnreferenced{ContentKeys: []string{"k"}})
	if scan, videos := sub.take(); scan || len(videos) != 0 {
		t.Fatalf("知らせない変化で知らせた: scan=%v videos=%v", scan, videos)
	}

	events.Handle(domain.ScanChanged{})
	if scan, _ := sub.take(); !scan {
		t.Fatal("ScanChanged: scan=false")
	}
	events.Handle(domain.ProcessingChanged{})
	if scan, _ := sub.take(); !scan {
		t.Fatal("ProcessingChanged: scan=false")
	}
	// 同じ動画の続けての変化は1つにまとまる。
	events.Handle(domain.VideoIngestChanged{VideoID: 5, Stage: domain.JobProbe})
	events.Handle(domain.VideoIngestChanged{VideoID: 5, Stage: domain.JobThumbnail})
	events.Handle(domain.VideoIngestChanged{VideoID: 6})
	if _, videos := sub.take(); !slices.Equal(videos, []int64{5, 6}) {
		t.Fatalf("videos = %v, want [5 6]", videos)
	}
}

// lockedScans は、送信の goroutine が読むあいだにテストが今の処理を差し替える。
type lockedScans struct {
	mu      sync.Mutex
	current domain.Scan
}

func (f *lockedScans) StartScan(ctx context.Context) (domain.Scan, bool, error) {
	scan, err := f.CurrentScan(ctx)
	return scan, false, err
}

func (f *lockedScans) CurrentScan(context.Context) (domain.Scan, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.current, nil
}

func (f *lockedScans) ListScanIssues(context.Context, string, int) (domain.ScanIssuePage, error) {
	return domain.ScanIssuePage{}, nil
}

func (f *lockedScans) setImport(progress domain.ImportProgress) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.current.Import = progress
}

func (f *lockedScans) setActivity(activity domain.ScanActivity) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.current.Activity = activity
}

// 今の処理が変わると scan を送り、activity に種類とファイル名とフォルダが入る。
// 何も動いていなければ activity を省く
// （specs/024-import-progress/contracts/scan-api.md §2・§4）。
func TestStreamEventsSendsScanWhenActivityChanges(t *testing.T) {
	events := NewEvents()
	scans := &lockedScans{current: domain.Scan{ID: 4, State: domain.ScanRunning}}
	handler := newTestServer(t, Options{Scans: scans, Events: events})

	stream, _ := openEvents(t, handler)
	var scan gen.Scan
	if err := json.Unmarshal([]byte(nextEvent(t, stream).data), &scan); err != nil {
		t.Fatal(err)
	}
	if scan.Activity != nil {
		t.Errorf("何も動いていないのに activity がある: %+v", scan.Activity)
	}

	scans.setActivity(domain.ScanActivity{
		Kind: domain.ActivityThumbnail, VideoID: 12, Path: "/media/a/movie.mp4",
		Folder: domain.VideoFolder{RootID: 3, Path: "a"}, RootName: "media", Located: true,
	})
	events.Handle(domain.ScanActivityChanged{})

	event := nextEvent(t, stream)
	if event.name != "scan" {
		t.Fatalf("イベント = %q, want scan", event.name)
	}
	scan = gen.Scan{}
	if err := json.Unmarshal([]byte(event.data), &scan); err != nil {
		t.Fatal(err)
	}
	activity := scan.Activity
	if activity == nil {
		t.Fatalf("activity が無い: %s", event.data)
	}
	if activity.Kind != gen.Thumbnail || activity.FileName != "movie.mp4" {
		t.Errorf("activity = %+v, want thumbnail の movie.mp4", activity)
	}
	if activity.VideoId == nil || *activity.VideoId != 12 {
		t.Errorf("activity.videoId = %v, want 12", activity.VideoId)
	}
	if activity.Folder == nil || activity.Folder.RootId != 3 || activity.Folder.Path != "a" ||
		activity.Folder.RootName == nil || *activity.Folder.RootName != "media" {
		t.Errorf("activity.folder = %+v, want 3 の a (media)", activity.Folder)
	}
	expectNoEvent(t, stream)

	// 何も動かなくなれば、activity を省いた scan を送る。
	scans.setActivity(domain.ScanActivity{})
	events.Handle(domain.ScanActivityChanged{})
	scan = gen.Scan{}
	if err := json.Unmarshal([]byte(nextEvent(t, stream).data), &scan); err != nil {
		t.Fatal(err)
	}
	if scan.Activity != nil {
		t.Errorf("終わったのに activity がある: %+v", scan.Activity)
	}
}

// 今の処理の変化は scan だけを記録する。1ファイルごとに届いても、送る前の分は
// 1回にまとまる。
func TestScanActivityChangedMarksOnlyScan(t *testing.T) {
	events := NewEvents()
	sub, unsubscribe := events.subscribe()
	defer unsubscribe()
	sub.take()

	for range 20 {
		events.Handle(domain.ScanActivityChanged{})
	}
	scan, videos := sub.take()
	if !scan || len(videos) != 0 {
		t.Errorf("scan = %v, videos = %v, want scan だけ", scan, videos)
	}
	if scan, _ := sub.take(); scan {
		t.Error("まとめたあとにも scan が残っている")
	}
}
