package comfyui

import (
	"context"
	"image/color"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/syudead/vv/internal/domain"
)

// wsProgress は偽物の /ws で、接続を受けると messages を順に送り、release が閉じるまで開いておく。
// 受けた接続の問い合わせを connected に送る。
func wsProgress(t *testing.T, connected chan<- map[string]string, release <-chan struct{}, messages ...string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept: %v", err)
			return
		}
		defer func() { _ = conn.CloseNow() }()
		connected <- map[string]string{"clientId": r.URL.Query().Get("clientId"), "token": r.URL.Query().Get("token")}
		// 送る前に POST /prompt が済むのを待つ必要はない。進み具合は id で絞って持つ。
		_ = conn.Write(r.Context(), websocket.MessageBinary, []byte{1, 2, 3, 4})
		for _, message := range messages {
			if err := conn.Write(r.Context(), websocket.MessageText, []byte(message)); err != nil {
				return
			}
		}
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}
}

// waitProgress は c が job の進み具合 want を返すまで待つ。
func waitProgress(t *testing.T, c *Client, j domain.ImageEditJob, want domain.ImageEditProgress) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if got := c.Progress(j); got != nil && *got == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("Progress = %+v, want %+v", c.Progress(j), want)
}

// その実行の id への progress は value / max として読め、他の id への progress と progress 以外の
// メッセージは無視する。WebSocket は POST /prompt と同じ client_id で接続し、Comfy Cloud は
// キーを token の問い合わせで渡す。実行が終われば進み具合は無くなる。
func TestProgressFromWebSocket(t *testing.T) {
	for _, cloud := range []bool{false, true} {
		f := newRunProvider(t, cloud)
		connected := make(chan map[string]string, 1)
		release := make(chan struct{})
		// 偽物の server を閉じる前に、開いたままの WebSocket の handler を終わらせる。
		t.Cleanup(func() { close(release) })
		f.handle("/ws", wsProgress(t, connected, release,
			`{"type":"status","data":{"status":{"exec_info":{"queue_remaining":1}}}}`,
			`{"type":"progress","data":{"value":2,"max":4,"prompt_id":"run-1","node":"50"}}`,
			`{"type":"progress","data":{"value":9,"max":10,"prompt_id":"other","node":"50"}}`,
			`{"type":"executing","data":{"node":"50","prompt_id":"run-1"}}`,
			`{"type":"progress","data":{"value":3,"max":4,"prompt_id":"run-1","node":"50"}}`,
			`{"type":"progress","data":{"value":10,"max":10,"prompt_id":"another","node":"50"}}`,
		))
		client := f.client()
		j, err := client.Start(context.Background(), f.conn(), domain.ImageEditInput{Frame: pngOf(t, 4, 4, color.Black), Instruction: "x"})
		if err != nil {
			t.Fatal(err)
		}
		query := <-connected
		waitProgress(t, client, j, domain.ImageEditProgress{Value: 3, Max: 4})
		// 他の id のメッセージを読んだ後でも、その実行の進み具合のままである。
		time.Sleep(50 * time.Millisecond)
		if got := client.Progress(j); got == nil || *got != (domain.ImageEditProgress{Value: 3, Max: 4}) {
			t.Errorf("cloud=%v: Progress = %+v after another id's message", cloud, got)
		}
		if other := client.Progress(domain.ImageEditJob{ID: "other"}); other != nil {
			t.Errorf("cloud=%v: Progress(other) = %+v, want nil", cloud, other)
		}
		f.mu.Lock()
		clientID := f.clientIDs[0]
		f.mu.Unlock()
		if query["clientId"] == "" || query["clientId"] != clientID {
			t.Errorf("cloud=%v: WebSocket clientId = %q, POST /prompt client_id = %q", cloud, query["clientId"], clientID)
		}
		wantToken := map[bool]string{true: "secret", false: ""}[cloud]
		if query["token"] != wantToken {
			t.Errorf("cloud=%v: token = %q, want %q", cloud, query["token"], wantToken)
		}
		assertKeys(t, f)

		if cloud {
			f.handle("/job/run-1/status", jsonHandler(`{"status":"success"}`))
			f.handle("/history_v2/run-1", jsonHandler(historyWithImage))
		} else {
			f.handle("/queue", jsonHandler(emptyQueue))
			f.handle("/history/run-1", jsonHandler(historyWithImage))
		}
		state, _, err := client.State(context.Background(), j, domain.ThumbnailEditRunning)
		if err != nil || state != domain.ThumbnailEditSucceeded {
			t.Fatalf("cloud=%v: State = %s, %v", cloud, state, err)
		}
		if got := client.Progress(j); got != nil {
			t.Errorf("cloud=%v: Progress after succeeded = %+v, want nil", cloud, got)
		}
	}
}

// WebSocket が接続を拒んでも、進み具合は無いまま、実行は HTTP の状態で succeeded に達する。
func TestProgressAbsentWhenWebSocketRefused(t *testing.T) {
	for _, cloud := range []bool{false, true} {
		f := newRunProvider(t, cloud)
		refused := make(chan struct{}, 1)
		f.handle("/ws", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			refused <- struct{}{}
		})
		client := f.client()
		j, err := client.Start(context.Background(), f.conn(), domain.ImageEditInput{Frame: pngOf(t, 4, 4, color.Black), Instruction: "x"})
		if err != nil {
			t.Fatalf("cloud=%v: Start = %v", cloud, err)
		}
		<-refused
		if cloud {
			f.sequence("/job/run-1/status", `{"status":"executing"}`, `{"status":"success"}`)
			f.handle("/history_v2/run-1", jsonHandler(historyWithImage))
		} else {
			f.sequence("/queue", runningQueue, emptyQueue)
			f.handle("/history/run-1", jsonHandler(historyWithImage))
		}
		state, failure, err := client.State(context.Background(), j, domain.ThumbnailEditQueued)
		if err != nil || failure != nil || state != domain.ThumbnailEditRunning {
			t.Fatalf("cloud=%v: first State = %s, %v, %v", cloud, state, failure, err)
		}
		if got := client.Progress(j); got != nil {
			t.Errorf("cloud=%v: Progress while running = %+v, want nil", cloud, got)
		}
		state, failure, err = client.State(context.Background(), j, state)
		if err != nil || failure != nil || state != domain.ThumbnailEditSucceeded {
			t.Fatalf("cloud=%v: second State = %s, %v, %v", cloud, state, failure, err)
		}
		if got := client.Progress(j); got != nil {
			t.Errorf("cloud=%v: Progress = %+v, want nil", cloud, got)
		}
	}
}

// progress でない、または値の合わないメッセージは進み具合にしない。
func TestProgressOfIgnoresOtherMessages(t *testing.T) {
	cases := map[string]bool{
		`{"type":"progress","data":{"value":1,"max":20,"prompt_id":"p"}}`:  true,
		`{"type":"progress","data":{"value":0,"max":20,"prompt_id":"p"}}`:  true,
		`{"type":"progress","data":{"value":1,"max":20}}`:                  false,
		`{"type":"progress","data":{"value":1,"max":0,"prompt_id":"p"}}`:   false,
		`{"type":"progress","data":{"value":21,"max":20,"prompt_id":"p"}}`: false,
		`{"type":"executing","data":{"node":"1","prompt_id":"p"}}`:         false,
		`not json`: false,
	}
	for message, want := range cases {
		_, _, ok := progressOf(strings.NewReader(message))
		if ok != want {
			t.Errorf("progressOf(%s) ok = %v, want %v", message, ok, want)
		}
	}
}

// POST /prompt は WebSocket が提供元に登録されてから送る。ComfyUI は登録の後に最初の status を
// 送り、登録より前の progress はどこにも届かないので、登録が遅い提供元でも最初の progress を
// 受けられる。
func TestStartWaitsForWebSocketRegistration(t *testing.T) {
	f := newRunProvider(t, false)
	var mu sync.Mutex
	registered := false
	prompted := make(chan struct{})
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	f.handle("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept: %v", err)
			return
		}
		defer func() { _ = conn.CloseNow() }()
		// 接続の登録が遅れる。登録の後の status より前に POST /prompt が届いてはならない。
		time.Sleep(200 * time.Millisecond)
		mu.Lock()
		registered = true
		mu.Unlock()
		if err := conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"status","data":{"status":{"exec_info":{"queue_remaining":0}},"sid":"s"}}`)); err != nil {
			return
		}
		select {
		case <-prompted:
		case <-r.Context().Done():
			return
		}
		_ = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"progress","data":{"value":1,"max":4,"prompt_id":"run-1","node":"50"}}`))
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})
	f.handle("/prompt", func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		ok := registered
		mu.Unlock()
		if !ok {
			t.Error("POST /prompt arrived before the WebSocket was registered")
		}
		close(prompted)
		_, _ = w.Write([]byte(`{"prompt_id":"run-1","number":1,"node_errors":{}}`))
	})
	client := f.client()
	j, err := client.Start(context.Background(), f.conn(), domain.ImageEditInput{Frame: pngOf(t, 4, 4, color.Black), Instruction: "x"})
	if err != nil {
		t.Fatal(err)
	}
	waitProgress(t, client, j, domain.ImageEditProgress{Value: 1, Max: 4})
}

// WebSocket が切れた後は、最後に受けた進み具合を今の値として返さない。
func TestProgressDroppedWhenWebSocketCloses(t *testing.T) {
	f := newRunProvider(t, false)
	closeSocket := make(chan struct{})
	f.handle("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept: %v", err)
			return
		}
		defer func() { _ = conn.CloseNow() }()
		if err := conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"progress","data":{"value":1,"max":4,"prompt_id":"run-1","node":"50"}}`)); err != nil {
			return
		}
		select {
		case <-closeSocket:
		case <-r.Context().Done():
		}
	})
	client := f.client()
	j, err := client.Start(context.Background(), f.conn(), domain.ImageEditInput{Frame: pngOf(t, 4, 4, color.Black), Instruction: "x"})
	if err != nil {
		t.Fatal(err)
	}
	waitProgress(t, client, j, domain.ImageEditProgress{Value: 1, Max: 4})
	close(closeSocket)
	deadline := time.Now().Add(5 * time.Second)
	for client.Progress(j) != nil {
		if time.Now().After(deadline) {
			t.Fatalf("Progress = %+v after the WebSocket closed, want nil", client.Progress(j))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// 中止の要求が失敗すれば実行は続きうるので、進み具合を読む WebSocket は開いたままにする。
// 中止が済めば閉じる。
func TestProgressKeptWhenCancelFails(t *testing.T) {
	f := newRunProvider(t, false)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	connected := make(chan map[string]string, 1)
	f.handle("/ws", wsProgress(t, connected, release,
		`{"type":"progress","data":{"value":2,"max":4,"prompt_id":"run-1","node":"50"}}`,
	))
	client := f.client()
	j, err := client.Start(context.Background(), f.conn(), domain.ImageEditInput{Frame: pngOf(t, 4, 4, color.Black), Instruction: "x"})
	if err != nil {
		t.Fatal(err)
	}
	<-connected
	waitProgress(t, client, j, domain.ImageEditProgress{Value: 2, Max: 4})
	f.handle("/queue", statusHandler(http.StatusInternalServerError, ""))
	if err := client.Cancel(context.Background(), j, domain.ThumbnailEditRunning); err == nil {
		t.Fatal("Cancel = nil, want the provider's error")
	}
	if got := client.Progress(j); got == nil || *got != (domain.ImageEditProgress{Value: 2, Max: 4}) {
		t.Errorf("Progress after a failed cancel = %+v, want 2 of 4", got)
	}
	f.handle("/queue", jsonHandler(runningQueue))
	if err := client.Cancel(context.Background(), j, domain.ThumbnailEditRunning); err != nil {
		t.Fatalf("Cancel = %v", err)
	}
	if got := client.Progress(j); got != nil {
		t.Errorf("Progress after cancel = %+v, want nil", got)
	}
}
