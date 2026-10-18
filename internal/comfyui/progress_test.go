package comfyui

import (
	"context"
	"encoding/json"
	"image/color"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/coder/websocket"

	"github.com/syudead/vv/internal/domain"
)

type progressTransport func(*http.Request) (*http.Response, error)

func (f progressTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func progressHTTP(handler http.HandlerFunc) *http.Client {
	return &http.Client{Transport: progressTransport(func(r *http.Request) (*http.Response, error) {
		w := httptest.NewRecorder()
		handler(w, r)
		return w.Result(), nil
	})}
}

func newProgressWatch() *progressWatch {
	return &progressWatch{ready: make(chan struct{}), steps: map[string]domain.ImageEditProgress{}, stop: func() {}}
}

func TestProgressWatchKeepsLatestStepForEachJob(t *testing.T) {
	c := New(fixedSettings{})
	w := newProgressWatch()
	c.progress["run-1"] = w
	w.record("run-1", domain.ImageEditProgress{Value: 2, Max: 4})
	w.record("other", domain.ImageEditProgress{Value: 9, Max: 10})
	w.record("run-1", domain.ImageEditProgress{Value: 3, Max: 4})
	if got := c.Progress(domain.ImageEditJob{ID: "run-1"}); got == nil || *got != (domain.ImageEditProgress{Value: 3, Max: 4}) {
		t.Fatalf("progress = %+v", got)
	}
	if got := c.Progress(domain.ImageEditJob{ID: "other"}); got != nil {
		t.Fatalf("unregistered job progress = %+v", got)
	}
	w.disconnect()
	w.record("run-1", domain.ImageEditProgress{Value: 4, Max: 4})
	if got := c.Progress(domain.ImageEditJob{ID: "run-1"}); got != nil {
		t.Fatalf("disconnected progress = %+v", got)
	}
}

func TestProgressWaitReady(t *testing.T) {
	for _, event := range []string{"ready", "cancel", "timeout"} {
		t.Run(event, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				w := newProgressWatch()
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				done := make(chan struct{})
				go func() { w.waitReady(ctx); close(done) }()
				synctest.Wait()
				select {
				case <-done:
					t.Fatal("returned before registration, cancellation or timeout")
				default:
				}
				switch event {
				case "ready":
					w.markReady()
				case "cancel":
					cancel()
				case "timeout":
					time.Sleep(progressReadyTimeout - time.Nanosecond)
					synctest.Wait()
					select {
					case <-done:
						t.Fatal("registration wait timed out early")
					default:
					}
					time.Sleep(time.Nanosecond)
				}
				synctest.Wait()
				select {
				case <-done:
				default:
					t.Fatal("registration wait did not finish")
				}
			})
		})
	}
}

// Start's caller wiring is checked without opening a socket or waiting in real time.
func TestStartWaitsForWebSocketRegistration(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w := newProgressWatch()
		var watchID, promptID string
		c := NewWithOptions(fixedSettings{}, Options{HTTPClient: progressHTTP(func(rw http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/upload/image":
				_, _ = rw.Write([]byte(`{"name":"frame.png"}`))
			case "/prompt":
				var body struct {
					ClientID string `json:"client_id"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				promptID = body.ClientID
				select {
				case <-w.ready:
				default:
					t.Error("prompt submitted before registration")
				}
				_, _ = rw.Write([]byte(`{"prompt_id":"run-1"}`))
			default:
				t.Errorf("unexpected request: %s", r.URL.Path)
				rw.WriteHeader(http.StatusNotFound)
			}
		})})
		c.startProgress = func(_ context.Context, target *target, id string) *progressWatch {
			if target.base != "http://provider" {
				t.Errorf("progress target = %s", target.base)
			}
			watchID = id
			return w
		}
		type result struct {
			job domain.ImageEditJob
			err error
		}
		done := make(chan result, 1)
		frame := pngOf(t, 1, 1, color.Black)
		go func() {
			j, err := c.Start(context.Background(), domain.ComfyUISettings{Provider: domain.ComfyUISelfHosted, URL: "http://provider"}, domain.ImageEditInput{Frame: frame, Instruction: "x"})
			done <- result{j, err}
		}()
		synctest.Wait()
		if watchID == "" || promptID != "" {
			t.Fatalf("before ready: watch=%q prompt=%q", watchID, promptID)
		}
		w.markReady()
		synctest.Wait()
		got := <-done
		if got.err != nil || got.job.ID != "run-1" {
			t.Fatalf("Start = %+v", got)
		}
		if promptID != watchID {
			t.Fatalf("prompt client_id = %q, watch = %q", promptID, watchID)
		}
		w.record("run-1", domain.ImageEditProgress{Value: 1, Max: 4})
		if got := c.Progress(got.job); got == nil || got.Value != 1 || got.Max != 4 {
			t.Fatalf("progress = %+v", got)
		}
		c.dropProgress("run-1", nil)
	})
}

// A refused stream must not prevent submission. This uses the constructor's real
// watch factory and an in-process HTTP transport, including the websocket dial.
func TestProgressAbsentWhenWebSocketRefused(t *testing.T) {
	for _, cloud := range []bool{false, true} {
		var connectedID, promptID, token string
		prefix := ""
		settings := domain.ComfyUISettings{Provider: domain.ComfyUISelfHosted, URL: "http://provider", APIKey: "secret"}
		if cloud {
			prefix = "/api"
			settings.Provider = domain.ComfyUICloud
		}
		c := NewWithOptions(fixedSettings{}, Options{CloudBaseURL: "http://provider/api", HTTPClient: progressHTTP(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/ws":
				connectedID = r.URL.Query().Get("clientId")
				token = r.URL.Query().Get("token")
				w.WriteHeader(http.StatusForbidden)
			case prefix + "/upload/image":
				_, _ = w.Write([]byte(`{"name":"frame.png"}`))
			case prefix + "/prompt":
				var body struct {
					ClientID string `json:"client_id"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				promptID = body.ClientID
				_, _ = w.Write([]byte(`{"prompt_id":"run-1"}`))
			default:
				t.Errorf("unexpected request: %s", r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
			}
		})})
		j, err := c.Start(context.Background(), settings, domain.ImageEditInput{Frame: pngOf(t, 1, 1, color.Black), Instruction: "x"})
		if err != nil || j.ID != "run-1" {
			t.Fatalf("Start = %+v, %v", j, err)
		}
		defer c.dropProgress(j.ID, nil)
		if got := c.Progress(j); got != nil {
			t.Fatalf("progress = %+v", got)
		}
		wantToken := ""
		if cloud {
			wantToken = "secret"
		}
		if connectedID == "" || connectedID != promptID || token != wantToken {
			t.Fatalf("cloud=%v: websocket client=%q token=%q; prompt client=%q", cloud, connectedID, token, promptID)
		}
	}
}

func TestProgressLifetimeFollowsStateAndCancel(t *testing.T) {
	for _, tc := range []struct {
		name        string
		cancel      bool
		status      int
		queue       string
		wantStopped bool
		wantState   domain.ThumbnailEditState
	}{
		{name: "running", queue: runningQueue, wantState: domain.ThumbnailEditRunning},
		{name: "completed", queue: emptyQueue, wantStopped: true, wantState: domain.ThumbnailEditSucceeded},
		{name: "cancel failed", cancel: true, status: http.StatusInternalServerError},
		{name: "cancel succeeded", cancel: true, queue: runningQueue, wantStopped: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := NewWithOptions(fixedSettings{}, Options{HTTPClient: progressHTTP(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/queue":
					if tc.status != 0 {
						w.WriteHeader(tc.status)
					} else {
						_, _ = w.Write([]byte(tc.queue))
					}
				case "/history/run-1":
					_, _ = w.Write([]byte(historyWithImage))
				case "/interrupt":
				default:
					t.Errorf("unexpected request: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			})})
			w := newProgressWatch()
			stopped := false
			w.stop = func() { stopped = true }
			w.record("run-1", domain.ImageEditProgress{Value: 2, Max: 4})
			c.progress["run-1"] = w
			j := domain.ImageEditJob{ID: "run-1", Output: "99", Connection: domain.ComfyUISettings{Provider: domain.ComfyUISelfHosted, URL: "http://provider"}}
			var err error
			if tc.cancel {
				err = c.Cancel(context.Background(), j, domain.ThumbnailEditRunning)
			} else {
				var state domain.ThumbnailEditState
				state, _, err = c.State(context.Background(), j, domain.ThumbnailEditRunning)
				if state != tc.wantState {
					t.Fatalf("state = %s, want %s", state, tc.wantState)
				}
			}
			if (err != nil) != (tc.status != 0) {
				t.Fatalf("error = %v", err)
			}
			if stopped != tc.wantStopped {
				t.Fatalf("stopped = %v, want %v", stopped, tc.wantStopped)
			}
			got := c.Progress(j)
			if tc.wantStopped {
				if got != nil {
					t.Fatalf("terminal progress = %+v", got)
				}
			} else if got == nil || *got != (domain.ImageEditProgress{Value: 2, Max: 4}) {
				t.Fatalf("retained progress = %+v", got)
			}
		})
	}
}

// This adapter test retains only the actual websocket boundary. A ping after the
// messages acknowledges that the client consumed them; no progress polling is needed.
func TestReadProgressFromWebSocket(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	closeSocket := make(chan struct{})
	acknowledged := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(rw, r, nil)
		if err != nil {
			acknowledged <- err
			return
		}
		defer func() { _ = conn.CloseNow() }()
		readCtx := conn.CloseRead(ctx)
		messages := []struct {
			kind websocket.MessageType
			body string
		}{
			{websocket.MessageText, `{"type":"progress","data":{"value":3,"max":4,"prompt_id":"run-1"}}`},
			{websocket.MessageBinary, `{"type":"progress","data":{"value":4,"max":4,"prompt_id":"run-1"}}`},
			{websocket.MessageText, `{"type":"progress","data":{"value":9,"max":10,"prompt_id":"other"}}`},
			{websocket.MessageText, `{"type":"executing","data":{"prompt_id":"run-1"}}`},
		}
		for _, message := range messages {
			if err := conn.Write(readCtx, message.kind, []byte(message.body)); err != nil {
				acknowledged <- err
				return
			}
		}
		acknowledged <- conn.Ping(readCtx)
		select {
		case <-closeSocket:
		case <-ctx.Done():
		}
	}))
	defer server.Close()
	defer close(closeSocket)
	c := New(fixedSettings{})
	w := newProgressWatch()
	done := make(chan struct{})
	go func() { c.readProgress(ctx, server.URL, w); close(done) }()
	if err := <-acknowledged; err != nil {
		t.Fatal(err)
	}
	if got, ok := w.step("run-1"); !ok || got != (domain.ImageEditProgress{Value: 3, Max: 4}) {
		t.Fatalf("progress = %+v, %v", got, ok)
	}
	select {
	case <-w.ready:
	default:
		t.Fatal("first message did not mark ready")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("reader did not stop")
	}
	if _, ok := w.step("run-1"); ok {
		t.Fatal("disconnected progress retained")
	}
}
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
