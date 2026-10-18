package comfyui

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/syudead/vv/internal/domain"
)

// HTTP determines execution state; WebSockets supply optional progress, so connection failures must
// not fail the run.

const progressDialTimeout = 30 * time.Second

// Wait for the first socket message before submitting, since ComfyUI registers the client before
// sending status; timeout allows execution even if early progress is lost.
const progressReadyTimeout = 3 * time.Second

// Outlive the ten-minute edit session but eventually close sockets whose completion was never
// reported.
const progressLifetime = 15 * time.Minute

// Limit text messages; discard binary previews without reading them.
const maxProgressMessageBytes = 1 << 20

// The socket opens before a run ID exists, so filter cached progress by ID when reading.
type progressWatch struct {
	stop context.CancelFunc
	// Closes on the first message or connection failure.
	ready     chan struct{}
	readyOnce sync.Once

	mu     sync.Mutex
	steps  map[string]domain.ImageEditProgress
	closed bool
}

func (w *progressWatch) markReady() {
	w.readyOnce.Do(func() { close(w.ready) })
}

func (w *progressWatch) waitReady(ctx context.Context) {
	timer := time.NewTimer(progressReadyTimeout)
	defer timer.Stop()
	select {
	case <-w.ready:
	case <-timer.C:
	case <-ctx.Done():
	}
}

func (w *progressWatch) record(id string, step domain.ImageEditProgress) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.closed {
		w.steps[id] = step
	}
}

// Discard stale progress on disconnect because no further updates can arrive.
func (w *progressWatch) disconnect() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
	clear(w.steps)
}

func (w *progressWatch) step(id string) (domain.ImageEditProgress, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	step, ok := w.steps[id]
	return step, ok
}

// Execution outlives the HTTP request, so do not inherit request cancellation; stop or
// progressLifetime closes the socket.
func (c *Client) watchProgress(ctx context.Context, t *target, clientID string) *progressWatch {
	ctx, stop := context.WithTimeout(context.WithoutCancel(ctx), progressLifetime)
	w := &progressWatch{stop: stop, ready: make(chan struct{}), steps: map[string]domain.ImageEditProgress{}}
	go c.readProgress(ctx, t.wsURL(clientID), w)
	return w
}

func (c *Client) readProgress(ctx context.Context, wsURL string, w *progressWatch) {
	defer w.markReady()
	defer w.disconnect()
	dialCtx, cancel := context.WithTimeout(ctx, progressDialTimeout)
	conn, resp, err := websocket.Dial(dialCtx, wsURL, &websocket.DialOptions{HTTPClient: c.http})
	cancel()
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return
	}
	defer func() { _ = conn.CloseNow() }()
	// Apply size limits after identifying the message type instead of using the library-wide limit.
	conn.SetReadLimit(-1)
	for {
		kind, r, err := conn.Reader(ctx)
		if err != nil {
			return
		}
		// ComfyUI sends its first status message only after registering this client ID.
		w.markReady()
		if kind == websocket.MessageText {
			if id, step, ok := progressOf(io.LimitReader(r, maxProgressMessageBytes)); ok {
				w.record(id, step)
			}
		}
		if _, err := io.Copy(io.Discard, r); err != nil {
			return
		}
	}
}

// Expected shape: {"type":"progress","data":{"value":3,"max":20,"prompt_id":"...","node":"..."}}.
func progressOf(r io.Reader) (string, domain.ImageEditProgress, bool) {
	var message struct {
		Type string `json:"type"`
		Data struct {
			Value    int    `json:"value"`
			Max      int    `json:"max"`
			PromptID string `json:"prompt_id"`
		} `json:"data"`
	}
	if json.NewDecoder(r).Decode(&message) != nil || message.Type != "progress" {
		return "", domain.ImageEditProgress{}, false
	}
	d := message.Data
	if d.PromptID == "" || d.Max <= 0 || d.Value < 0 || d.Value > d.Max {
		return "", domain.ImageEditProgress{}, false
	}
	return d.PromptID, domain.ImageEditProgress{Value: d.Value, Max: d.Max}, true
}

func (c *Client) keepProgress(id string, w *progressWatch) {
	c.progressMu.Lock()
	if previous, ok := c.progress[id]; ok {
		previous.stop()
	}
	c.progress[id] = w
	c.progressMu.Unlock()
	time.AfterFunc(progressLifetime, func() { c.dropProgress(id, w) })
}

// If w is non-nil, remove the entry only while id still refers to that socket.
func (c *Client) dropProgress(id string, w *progressWatch) {
	c.progressMu.Lock()
	defer c.progressMu.Unlock()
	current, ok := c.progress[id]
	if !ok || w != nil && current != w {
		return
	}
	current.stop()
	delete(c.progress, id)
}

// Progress returns nil when disconnected, before the first update, or after execution ends.
func (c *Client) Progress(job domain.ImageEditJob) *domain.ImageEditProgress {
	c.progressMu.Lock()
	w, ok := c.progress[job.ID]
	c.progressMu.Unlock()
	if !ok {
		return nil
	}
	step, ok := w.step(job.ID)
	if !ok {
		return nil
	}
	return &step
}

// Cloud sockets use /ws outside the API prefix and authenticate via the token query parameter;
// self-hosted sockets use the saved base URL.
func (t *target) wsURL(clientID string) string {
	base := t.base
	query := url.Values{"clientId": {clientID}}
	if t.cloud {
		base = strings.TrimSuffix(base, "/api")
		query.Set("token", t.apiKey)
	}
	return base + "/ws?" + query.Encode()
}

// Random client ID shared by the socket and POST /prompt.
func newClientID() (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(random[:]), nil
}
