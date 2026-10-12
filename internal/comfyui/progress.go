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

// ステップの進み具合は ComfyUI の WebSocket の progress のメッセージから取る。状態は HTTP の
// 問い合わせで決め、WebSocket は進み具合だけに使う。つながらなければ進み具合は無いままで、
// 実行は失敗させない（specs/045-comfyui-thumbnails/research.md R-5）。

// progressDialTimeout は WebSocket の接続の上限時間である。
const progressDialTimeout = 30 * time.Second

// progressLifetime は 1 つの実行の WebSocket を開いておく上限時間である。編集のセッションが
// 実行を止める 10 分（research.md R-11）より長くし、終わりを知らされなかった接続も閉じる。
const progressLifetime = 15 * time.Minute

// maxProgressMessageBytes は読む文字のメッセージの上限である。プレビューの画像などのバイナリの
// メッセージは読まずに捨てる。
const maxProgressMessageBytes = 1 << 20

// progressWatch は 1 回の実行のために開いた WebSocket で、受けた progress を実行の id ごとに持つ。
// 接続は実行の id が決まる前に開くので、id で絞るのは読むときである。
type progressWatch struct {
	stop context.CancelFunc

	mu    sync.Mutex
	steps map[string]domain.ImageEditProgress
}

func (w *progressWatch) record(id string, step domain.ImageEditProgress) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.steps[id] = step
}

func (w *progressWatch) step(id string) (domain.ImageEditProgress, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	step, ok := w.steps[id]
	return step, ok
}

// watchProgress は送り先 t の WebSocket に clientID で接続し、progress を読み始める。接続も
// 読み取りも裏で行い、呼び出しを待たせない。実行は要求 ctx より長く続くので、ctx の取り消しは
// 引き継がず、progressLifetime か stop で終わる。
func (c *Client) watchProgress(ctx context.Context, t *target, clientID string) *progressWatch {
	ctx, stop := context.WithTimeout(context.WithoutCancel(ctx), progressLifetime)
	w := &progressWatch{stop: stop, steps: map[string]domain.ImageEditProgress{}}
	go c.readProgress(ctx, t.wsURL(clientID), w)
	return w
}

// readProgress は WebSocket の url に接続し、ctx が終わるか接続が切れるまで progress を w に
// 記録する。接続できなければ何もしない。
func (c *Client) readProgress(ctx context.Context, wsURL string, w *progressWatch) {
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
	// 長さはメッセージの種類を見てから限るので、ライブラリの上限は外す。
	conn.SetReadLimit(-1)
	for {
		kind, r, err := conn.Reader(ctx)
		if err != nil {
			return
		}
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

// progressOf は文字のメッセージ r が progress なら、その実行の id と進み具合を返す。
// {"type":"progress","data":{"value":3,"max":20,"prompt_id":"…","node":"…"}} の形である。
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

// keepProgress は実行 id の進み具合を w から読めるようにする。progressLifetime を過ぎれば捨てる。
func (c *Client) keepProgress(id string, w *progressWatch) {
	c.progressMu.Lock()
	if previous, ok := c.progress[id]; ok {
		previous.stop()
	}
	c.progress[id] = w
	c.progressMu.Unlock()
	time.AfterFunc(progressLifetime, func() { c.dropProgress(id, w) })
}

// dropProgress は実行 id の WebSocket を閉じて進み具合を捨てる。w が nil でなければ、id が
// まだ w を指すときだけ捨てる。
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

// Progress は実行の最新のステップの進み具合を返す。WebSocket がつながらないか、まだ progress が
// 届いていないか、実行が終わっていれば nil である（contracts/workflow.md の Run states）。
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

// wsURL は clientID で接続する WebSocket の URL である。自前の ComfyUI は保存した URL の /ws、
// Comfy Cloud は API の基底から /api を除いた /ws で、キーを token の問い合わせに付ける
// （research.md R-5）。
func (t *target) wsURL(clientID string) string {
	base := t.base
	query := url.Values{"clientId": {clientID}}
	if t.cloud {
		base = strings.TrimSuffix(base, "/api")
		query.Set("token", t.apiKey)
	}
	return base + "/ws?" + query.Encode()
}

// newClientID は WebSocket と POST /prompt の client_id に使う乱数である。
func newClientID() (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(random[:]), nil
}
