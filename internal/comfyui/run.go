package comfyui

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // 参考画像とコマの大きさを読む。
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// DefaultInstruction は、範囲を塗って指示を空にした実行で送る既定の指示である。持ち主の言語に
// かかわらずこの英文を一字一句送る（contracts/workflow.md の Workflows、research.md R-3）。
const DefaultInstruction = "Remove any text, logo or object in the marked area and restore the background behind it so it matches its surroundings."

// requestTimeout は実行の 1 つの要求の上限時間である。
const requestTimeout = 2 * time.Minute

// maxQueueBytes は GET /queue の応答の上限である。待っているワークフローを丸ごと返す。
const maxQueueBytes = 32 << 20

// maxOutputBytes は出力の画像の上限である。
const maxOutputBytes = 64 << 20

// vv が埋めるノードの _meta.title である（contracts/workflow.md の Workflows）。
const (
	titleFrame     = "vv:frame"
	titlePrompt    = "vv:prompt"
	titleMask      = "vv:mask"
	titleReference = "vv:reference"
	titleOutput    = "vv:output"
)

// Start は入力を上げ、vv のワークフローの vv:* の入力だけを書き換えて POST /prompt で送る。
// 参考画像があれば参考画像ありの版を、なければもう一方を使う。マスクが無ければコマの大きさの
// 真っ白なマスクを、塗ったマスクで指示が空なら DefaultInstruction を送る（research.md R-3）。
// 上げたファイルと出力は ComfyUI から消さない（contracts/workflow.md の Run）。送る前に、
// ステップの進み具合を読む WebSocket を同じ client_id で開き始める（Progress）。
func (c *Client) Start(ctx context.Context, conn domain.ComfyUISettings, input domain.ImageEditInput) (domain.ImageEditJob, error) {
	target, err := c.target(conn)
	if err != nil {
		return domain.ImageEditJob{}, err
	}
	name := workflowEdit
	if input.Reference != nil {
		name = workflowEditReference
	}
	prompt, err := loadPrompt(name)
	if err != nil {
		return domain.ImageEditJob{}, err
	}
	if c.diffusionModel != "" && conn.Provider == domain.ComfyUISelfHosted {
		if err := prompt.setLoaderInput(diffusionLoader, c.diffusionModel); err != nil {
			return domain.ImageEditJob{}, err
		}
	}
	output, err := prompt.nodeID(titleOutput)
	if err != nil {
		return domain.ImageEditJob{}, err
	}
	mask := input.Mask
	if mask == nil {
		if mask, err = whiteMask(input.Frame); err != nil {
			return domain.ImageEditJob{}, err
		}
	}
	instruction := input.Instruction
	if strings.TrimSpace(instruction) == "" {
		instruction = ""
		if input.Mask != nil {
			instruction = DefaultInstruction
		}
	}
	prefix, err := uploadPrefix()
	if err != nil {
		return domain.ImageEditJob{}, err
	}
	uploads := []struct {
		title string
		name  string
		data  []byte
	}{
		{titleFrame, prefix + "-frame.png", input.Frame},
		{titleMask, prefix + "-mask.png", mask},
	}
	if input.Reference != nil {
		uploads = append(uploads, struct {
			title string
			name  string
			data  []byte
		}{titleReference, prefix + "-reference" + imageExtension(input.Reference), input.Reference})
	}
	for _, u := range uploads {
		uploaded, err := target.upload(ctx, u.name, u.data)
		if err != nil {
			return domain.ImageEditJob{}, err
		}
		if err := prompt.setInput(u.title, "image", uploaded); err != nil {
			return domain.ImageEditJob{}, err
		}
	}
	if err := prompt.setInput(titlePrompt, "text", instruction); err != nil {
		return domain.ImageEditJob{}, err
	}
	clientID, err := newClientID()
	if err != nil {
		return domain.ImageEditJob{}, err
	}
	// 最初の progress を取りこぼさないよう、送る前に WebSocket を開き、提供元に登録されるのを
	// 上限つきで待つ（research.md R-5）。
	watch := c.watchProgress(ctx, target, clientID)
	watch.waitReady(ctx)
	id, err := target.submit(ctx, prompt, clientID)
	if err != nil {
		watch.stop()
		return domain.ImageEditJob{}, err
	}
	c.keepProgress(id, watch)
	return domain.ImageEditJob{Connection: conn, ID: id, Output: output}, nil
}

// State は実行の状態を読む。自前の ComfyUI は GET /queue の後に GET /history/{id}、Comfy Cloud は
// GET /job/{id}/status を読み、contracts/workflow.md の Run states の表で vv の状態に対応させる。
// どちらの表にも無い値なら previous を返す（research.md R-5）。終わった実行の WebSocket は閉じる。
func (c *Client) State(ctx context.Context, job domain.ImageEditJob, previous domain.ThumbnailEditState) (domain.ThumbnailEditState, *domain.ImageEditError, error) {
	target, err := c.target(job.Connection)
	if err != nil {
		return previous, nil, err
	}
	state, failure, err := target.state(ctx, job, previous)
	if err == nil && state.Terminal() {
		c.dropProgress(job.ID, nil)
	}
	return state, failure, err
}

// Cancel は実行を中止する。前に読んだ状態は古いことがあるので、送る要求を選ぶ直前に提供元の
// 今の状態を読み直す。previous は最後に読んだ状態で、提供元が表に無い値を返したときに使う。
// 終わった実行（succeeded・failed・cancelled）には何も送らない。Comfy Cloud の POST /interrupt は
// アカウントの実行中の仕事をすべて止めるので、終わった実行のために送ると別の実行を止めてしまう。
// 実行中なら自前の ComfyUI に POST /interrupt {"prompt_id":id}、Comfy Cloud に POST /interrupt を
// 送る。それより前なら POST /queue {"delete":[id]} で消し、読んでから消すまでに実行が
// 始まっていれば、もう一度読んで実行中なら interrupt を送る（research.md R-5）。進み具合を
// 読む WebSocket は中止が済んだときだけ閉じ、失敗すれば実行は続きうるので開いたままにする。
func (c *Client) Cancel(ctx context.Context, job domain.ImageEditJob, previous domain.ThumbnailEditState) error {
	target, err := c.target(job.Connection)
	if err != nil {
		return err
	}
	if err := c.cancel(ctx, target, job, previous); err != nil {
		return err
	}
	c.dropProgress(job.ID, nil)
	return nil
}

func (c *Client) cancel(ctx context.Context, target *target, job domain.ImageEditJob, previous domain.ThumbnailEditState) error {
	state, _, err := target.state(ctx, job, previous)
	if err != nil {
		return err
	}
	switch state {
	case domain.ThumbnailEditSucceeded, domain.ThumbnailEditFailed, domain.ThumbnailEditCancelled:
		return nil
	case domain.ThumbnailEditRunning:
		return target.interrupt(ctx, job.ID)
	}
	if err := target.postJSON(ctx, "/queue", map[string][]string{"delete": {job.ID}}, nil); err != nil {
		return err
	}
	after, _, err := target.state(ctx, job, state)
	if err != nil {
		return err
	}
	if after == domain.ThumbnailEditRunning {
		return target.interrupt(ctx, job.ID)
	}
	return nil
}

// Output は vv:output の最初の画像を GET /view で取る。Comfy Cloud は署名付き URL への転送を
// 返すので、転送先は X-API-Key を付けずに取る。画像が無ければ no_output の誤りを返す。
func (c *Client) Output(ctx context.Context, job domain.ImageEditJob) ([]byte, error) {
	target, err := c.target(job.Connection)
	if err != nil {
		return nil, err
	}
	entry, err := target.history(ctx, job.ID)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, &domain.ImageEditError{Reason: domain.ImageEditNoOutput, Message: "the run has no history entry"}
	}
	ref, ok := entry.firstImage(job.Output)
	if !ok {
		return nil, &domain.ImageEditError{Reason: domain.ImageEditNoOutput}
	}
	query := url.Values{"filename": {ref.Filename}, "subfolder": {ref.Subfolder}, "type": {ref.Type}}
	return target.view(ctx, "/view?"+query.Encode())
}

// target は 1 つの接続先への要求の送り先である。
type target struct {
	client *Client
	base   string
	apiKey string
	cloud  bool
}

// target は接続先 conn の送り先を作る。
func (c *Client) target(conn domain.ComfyUISettings) (*target, error) {
	switch conn.Provider {
	case domain.ComfyUISelfHosted:
		return &target{client: c, base: strings.TrimRight(conn.URL, "/")}, nil
	case domain.ComfyUICloud:
		if conn.APIKey == "" {
			return nil, domain.ErrComfyUIAPIKeyMissing
		}
		return &target{client: c, base: c.cloud, apiKey: conn.APIKey, cloud: true}, nil
	default:
		return nil, errors.New("no ComfyUI connection is saved; choose a provider and save it first")
	}
}

func (t *target) name() string {
	if t.cloud {
		return "Comfy Cloud"
	}
	return "ComfyUI at " + t.base
}

// do は要求を送る。つながらなければ unreachable の誤りを返す。
func (t *target) do(req *http.Request, withKey bool) (*http.Response, error) {
	if withKey && t.apiKey != "" {
		req.Header.Set("X-API-Key", t.apiKey)
	}
	resp, err := t.client.http.Do(req)
	if err != nil {
		if ctxErr := req.Context().Err(); ctxErr != nil && errors.Is(err, ctxErr) {
			return nil, err
		}
		return nil, &domain.ImageEditError{Reason: domain.ImageEditUnreachable, Message: fmt.Sprintf("cannot reach %s", t.name())}
	}
	return resp, nil
}

// statusError は 2xx でない応答 resp を失敗の理由に対応させる（contracts/workflow.md の
// Failure reasons）。応答の本文は読まない。
func (t *target) statusError(route string, resp *http.Response) *domain.ImageEditError {
	switch {
	case t.cloud && (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden):
		return &domain.ImageEditError{Reason: domain.ImageEditUnauthorized, Message: "the API key was rejected by Comfy Cloud"}
	case t.cloud && resp.StatusCode == http.StatusPaymentRequired:
		return &domain.ImageEditError{Reason: domain.ImageEditInsufficientCredits, Message: "Comfy Cloud has no credits left"}
	default:
		return &domain.ImageEditError{Reason: domain.ImageEditUnreachable, Message: fmt.Sprintf("%s answered %d to %s", t.name(), resp.StatusCode, route)}
	}
}

// request は基底 URL からの path への要求を作る。
func (t *target) request(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, t.base+path, body)
	if err != nil {
		return nil, fmt.Errorf("cannot build the request to %s: %w", t.name(), err)
	}
	return req, nil
}

// getJSON は path を GET し、200 の JSON を最大 limit バイトまで out に読む。
func (t *target) getJSON(ctx context.Context, path string, limit int64, out any) error {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := t.request(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	resp, err := t.do(req, true)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	route := routeOf(path)
	if resp.StatusCode != http.StatusOK {
		return t.statusError(route, resp)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, limit)).Decode(out); err != nil {
		return &domain.ImageEditError{Reason: domain.ImageEditUnreachable, Message: fmt.Sprintf("%s answered %s with something other than JSON", t.name(), route)}
	}
	return nil
}

// postJSON は body を JSON で path へ POST する。out が nil でなければ 200 の応答を読む。
func (t *target) postJSON(ctx context.Context, path string, body, out any) error {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := t.request(ctx, http.MethodPost, path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.do(req, true)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		return t.statusError(path, resp)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(out); err != nil {
		return &domain.ImageEditError{Reason: domain.ImageEditUnreachable, Message: fmt.Sprintf("%s answered %s with something other than JSON", t.name(), path)}
	}
	return nil
}

// upload は data を POST /upload/image で name の名前、overwrite=true、type=input で上げ、
// LoadImage に渡す名前（サブフォルダがあれば「サブフォルダ/名前」）を返す。
func (t *target) upload(ctx context.Context, name string, data []byte) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("image", name)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(data); err != nil {
		return "", err
	}
	for field, value := range map[string]string{"overwrite": "true", "type": "input"} {
		if err := form.WriteField(field, value); err != nil {
			return "", err
		}
	}
	if err := form.Close(); err != nil {
		return "", err
	}
	req, err := t.request(ctx, http.MethodPost, "/upload/image", &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", form.FormDataContentType())
	resp, err := t.do(req, true)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", t.statusError("/upload/image", resp)
	}
	var uploaded struct {
		Name      string `json:"name"`
		Subfolder string `json:"subfolder"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&uploaded); err != nil || uploaded.Name == "" {
		return "", &domain.ImageEditError{Reason: domain.ImageEditUnreachable, Message: fmt.Sprintf("%s did not name the uploaded image", t.name())}
	}
	if uploaded.Subfolder != "" {
		return uploaded.Subfolder + "/" + uploaded.Name, nil
	}
	return uploaded.Name, nil
}

// submit は prompt を POST /prompt で送り、prompt_id を返す。clientID は進み具合を読む WebSocket の
// clientId で、ComfyUI はその実行のメッセージをこの接続へ送る。400 はノードの誤りから
// workflow_rejected か missing_models に、クレジットを名指す誤りは insufficient_credits にする。
func (t *target) submit(ctx context.Context, prompt promptGraph, clientID string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	data, err := json.Marshal(map[string]any{"prompt": prompt, "client_id": clientID})
	if err != nil {
		return "", err
	}
	req, err := t.request(ctx, http.MethodPost, "/prompt", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.do(req, true)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	var reply promptReply
	decodeErr := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&reply)
	if resp.StatusCode == http.StatusOK && decodeErr == nil && reply.PromptID != "" {
		return reply.PromptID, nil
	}
	if decodeErr == nil && reply.Error != nil && mentionsCredits(reply.Error.Type) {
		return "", &domain.ImageEditError{Reason: domain.ImageEditInsufficientCredits, Message: reply.Error.Message}
	}
	if resp.StatusCode == http.StatusBadRequest && decodeErr == nil {
		return "", reply.rejection(prompt)
	}
	if resp.StatusCode == http.StatusOK {
		return "", &domain.ImageEditError{Reason: domain.ImageEditUnreachable, Message: fmt.Sprintf("%s did not return a prompt_id", t.name())}
	}
	return "", t.statusError("/prompt", resp)
}

// view は path の画像を取る。転送が返れば、転送先を X-API-Key を付けずに 1 度だけ取る。
func (t *target) view(ctx context.Context, path string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := t.request(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := t.do(req, true)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 == 3 {
		location, err := resp.Location()
		if err != nil {
			return nil, &domain.ImageEditError{Reason: domain.ImageEditUnreachable, Message: fmt.Sprintf("%s redirected /view without a location", t.name())}
		}
		next, err := http.NewRequestWithContext(ctx, http.MethodGet, location.String(), nil)
		if err != nil {
			return nil, err
		}
		redirected, err := t.do(next, false)
		if err != nil {
			return nil, err
		}
		defer func() { _ = redirected.Body.Close() }()
		resp = redirected
	}
	if resp.StatusCode != http.StatusOK {
		return nil, t.statusError("/view", resp)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxOutputBytes+1))
	if err != nil {
		return nil, &domain.ImageEditError{Reason: domain.ImageEditUnreachable, Message: fmt.Sprintf("cannot read the output image from %s", t.name())}
	}
	if len(data) > maxOutputBytes {
		return nil, &domain.ImageEditError{Reason: domain.ImageEditNoOutput, Message: "the output image is too large"}
	}
	if len(data) == 0 {
		return nil, &domain.ImageEditError{Reason: domain.ImageEditNoOutput}
	}
	return data, nil
}

// state は実行の状態を接続先に合う経路で読む。
func (t *target) state(ctx context.Context, job domain.ImageEditJob, previous domain.ThumbnailEditState) (domain.ThumbnailEditState, *domain.ImageEditError, error) {
	if t.cloud {
		return t.cloudState(ctx, job, previous)
	}
	return t.selfHostedState(ctx, job, previous)
}

// interrupt は実行中の実行 id を止める。自前の ComfyUI は prompt_id で名指し、その実行が
// 実行中のときだけ止まる。Comfy Cloud は実行を名指せない。
func (t *target) interrupt(ctx context.Context, id string) error {
	var body any = map[string]string{"prompt_id": id}
	if t.cloud {
		body = struct{}{}
	}
	return t.postJSON(ctx, "/interrupt", body, nil)
}

// selfHostedState は自前の ComfyUI の実行の状態を読む。待ち行列を先に見て、無ければ履歴を見る。
func (t *target) selfHostedState(ctx context.Context, job domain.ImageEditJob, previous domain.ThumbnailEditState) (domain.ThumbnailEditState, *domain.ImageEditError, error) {
	var queue struct {
		Running []json.RawMessage `json:"queue_running"`
		Pending []json.RawMessage `json:"queue_pending"`
	}
	if err := t.getJSON(ctx, "/queue", maxQueueBytes, &queue); err != nil {
		return previous, nil, err
	}
	if queueHas(queue.Pending, job.ID) {
		return domain.ThumbnailEditQueued, nil, nil
	}
	if queueHas(queue.Running, job.ID) {
		return domain.ThumbnailEditRunning, nil, nil
	}
	entry, err := t.history(ctx, job.ID)
	if err != nil {
		return previous, nil, err
	}
	if entry == nil {
		return domain.ThumbnailEditFailed, &domain.ImageEditError{
			Reason: domain.ImageEditRunFailed, Message: "the run left the ComfyUI queue without a result",
		}, nil
	}
	switch {
	case entry.Status.StatusStr == "error":
		return domain.ThumbnailEditFailed, entry.failure(), nil
	case entry.Status.Completed:
		return succeededOrNoOutput(entry, job.Output)
	default:
		return previous, nil, nil
	}
}

// cloudState は Comfy Cloud の実行の状態を GET /job/{id}/status から読む。
func (t *target) cloudState(ctx context.Context, job domain.ImageEditJob, previous domain.ThumbnailEditState) (domain.ThumbnailEditState, *domain.ImageEditError, error) {
	var status struct {
		Status       string `json:"status"`
		ErrorMessage string `json:"error_message"`
	}
	if err := t.getJSON(ctx, "/job/"+url.PathEscape(job.ID)+"/status", maxResponseBytes, &status); err != nil {
		return previous, nil, err
	}
	state, known := cloudStates[status.Status]
	if !known {
		return previous, nil, nil
	}
	switch state {
	case domain.ThumbnailEditSucceeded:
		entry, err := t.history(ctx, job.ID)
		if err != nil {
			return previous, nil, err
		}
		if entry == nil {
			// 状態が success になってから履歴の項目ができるまでの間は、出力がまだ読めない。
			// 失敗にせず前の状態を保ち、次の読み取りで確かめる（終わらなければ時間の上限が止める）。
			return previous, nil, nil
		}
		return succeededOrNoOutput(entry, job.Output)
	case domain.ThumbnailEditFailed:
		failure := &domain.ImageEditError{Reason: domain.ImageEditRunFailed, Message: status.ErrorMessage}
		// 実行の誤りの文は履歴にある。読めなければ状態の応答の文で失敗にする。
		if entry, err := t.history(ctx, job.ID); err == nil && entry != nil {
			if fromHistory := entry.failure(); fromHistory.Message != "" || fromHistory.Reason != domain.ImageEditRunFailed {
				failure = fromHistory
			}
		}
		return state, failure, nil
	default:
		return state, nil, nil
	}
}

// cloudStates は Comfy Cloud の仕事の状態から vv の状態への対応である（contracts/workflow.md の
// Run states）。succeeded は出力の画像があるときだけで、cloudState が確かめる。
var cloudStates = map[string]domain.ThumbnailEditState{
	"submitted":           domain.ThumbnailEditQueued,
	"preprocessing":       domain.ThumbnailEditQueued,
	"queued_waiting":      domain.ThumbnailEditQueued,
	"queued_limited":      domain.ThumbnailEditQueued,
	"allocated":           domain.ThumbnailEditQueued,
	"waiting_to_dispatch": domain.ThumbnailEditQueued,
	"pending":             domain.ThumbnailEditQueued,
	"preparing":           domain.ThumbnailEditPreparing,
	"pending_execution":   domain.ThumbnailEditPreparing,
	"executing":           domain.ThumbnailEditRunning,
	"in_progress":         domain.ThumbnailEditRunning,
	"executed":            domain.ThumbnailEditRunning,
	"success":             domain.ThumbnailEditSucceeded,
	"completed":           domain.ThumbnailEditSucceeded,
	"error":               domain.ThumbnailEditFailed,
	"non_retryable_error": domain.ThumbnailEditFailed,
	"lost":                domain.ThumbnailEditFailed,
	"cancel_requested":    domain.ThumbnailEditCancelled,
	"cancelled":           domain.ThumbnailEditCancelled,
}

// succeededOrNoOutput は成功した実行を、出力の画像があれば succeeded、無ければ no_output の
// failed にする。
func succeededOrNoOutput(entry *historyEntry, output string) (domain.ThumbnailEditState, *domain.ImageEditError, error) {
	if entry != nil {
		if _, ok := entry.firstImage(output); ok {
			return domain.ThumbnailEditSucceeded, nil, nil
		}
	}
	return domain.ThumbnailEditFailed, &domain.ImageEditError{Reason: domain.ImageEditNoOutput}, nil
}

// queueHas は GET /queue の項目の一覧 items に id があるかを返す。項目は
// [番号, prompt_id, prompt, extra_data, outputs] の配列である。
func queueHas(items []json.RawMessage, id string) bool {
	for _, item := range items {
		var fields []json.RawMessage
		if json.Unmarshal(item, &fields) != nil || len(fields) < 2 {
			continue
		}
		var itemID string
		if json.Unmarshal(fields[1], &itemID) == nil && itemID == id {
			return true
		}
	}
	return false
}

// imageRef は出力の画像の 1 つである。
type imageRef struct {
	Filename  string `json:"filename"`
	Subfolder string `json:"subfolder"`
	Type      string `json:"type"`
}

// historyEntry は 1 つの実行の履歴である。
type historyEntry struct {
	Outputs map[string]struct {
		Images []imageRef `json:"images"`
	} `json:"outputs"`
	Status struct {
		StatusStr string            `json:"status_str"`
		Completed bool              `json:"completed"`
		Messages  []json.RawMessage `json:"messages"`
	} `json:"status"`
}

// history は実行 id の履歴を読む。自前の ComfyUI は GET /history/{id}、Comfy Cloud は
// GET /history_v2/{id} で、どちらも id をキーにした対応を返す。履歴が無ければ nil を返す。
func (t *target) history(ctx context.Context, id string) (*historyEntry, error) {
	route := "/history/"
	if t.cloud {
		route = "/history_v2/"
	}
	var entries map[string]json.RawMessage
	if err := t.getJSON(ctx, route+url.PathEscape(id), maxQueueBytes, &entries); err != nil {
		return nil, err
	}
	raw, ok := entries[id]
	if !ok {
		// 履歴の項目そのものを返す提供元にも合わせる。
		if _, hasOutputs := entries["outputs"]; !hasOutputs {
			return nil, nil
		}
		whole, err := json.Marshal(entries)
		if err != nil {
			return nil, err
		}
		raw = whole
	}
	var entry historyEntry
	if err := json.Unmarshal(raw, &entry); err != nil {
		return nil, &domain.ImageEditError{Reason: domain.ImageEditUnreachable, Message: fmt.Sprintf("%s answered %s with an unreadable history", t.name(), strings.TrimSuffix(route, "/"))}
	}
	return &entry, nil
}

// firstImage は出力のノード output の最初の画像を返す。
func (e *historyEntry) firstImage(output string) (imageRef, bool) {
	images := e.Outputs[output].Images
	if len(images) == 0 || images[0].Filename == "" {
		return imageRef{}, false
	}
	ref := images[0]
	if ref.Type == "" {
		ref.Type = "output"
	}
	return ref, true
}

// failure は失敗した実行の理由を履歴の execution_error から作る。誤りの種類がクレジットを
// 名指していれば insufficient_credits、それ以外は run_failed で、実行の誤りの文を渡す。
func (e *historyEntry) failure() *domain.ImageEditError {
	for _, message := range e.Status.Messages {
		var pair []json.RawMessage
		if json.Unmarshal(message, &pair) != nil || len(pair) != 2 {
			continue
		}
		var kind string
		if json.Unmarshal(pair[0], &kind) != nil || kind != "execution_error" {
			continue
		}
		var detail struct {
			ExceptionType    string `json:"exception_type"`
			ExceptionMessage string `json:"exception_message"`
		}
		if json.Unmarshal(pair[1], &detail) != nil {
			continue
		}
		reason := domain.ImageEditRunFailed
		if mentionsCredits(detail.ExceptionType) {
			reason = domain.ImageEditInsufficientCredits
		}
		return &domain.ImageEditError{Reason: reason, Message: strings.TrimSpace(detail.ExceptionMessage)}
	}
	return &domain.ImageEditError{Reason: domain.ImageEditRunFailed}
}

// mentionsCredits は誤りの種類 kind がクレジットを名指すかを返す。
func mentionsCredits(kind string) bool {
	return strings.Contains(strings.ToLower(kind), "credit")
}

// promptReply は POST /prompt の応答である。
type promptReply struct {
	PromptID string `json:"prompt_id"`
	Error    *struct {
		Type      string `json:"type"`
		Message   string `json:"message"`
		Details   string `json:"details"`
		ExtraInfo struct {
			ClassType string `json:"class_type"`
		} `json:"extra_info"`
	} `json:"error"`
	NodeErrors map[string]struct {
		ClassType string `json:"class_type"`
		Errors    []struct {
			Type      string `json:"type"`
			Message   string `json:"message"`
			Details   string `json:"details"`
			ExtraInfo struct {
				InputName string `json:"input_name"`
			} `json:"extra_info"`
		} `json:"errors"`
	} `json:"node_errors"`
}

// missingNodePattern は古い ComfyUI が無いノードの型を述べる文である。
var missingNodePattern = regexp.MustCompile(`node (\S+) does not exist`)

// nodeIDPattern は誤りの details が名指すノードの id である。
var nodeIDPattern = regexp.MustCompile(`Node ID '#([^']+)'`)

// rejection は 400 の応答を、足りないノードの型かモデルファイルがあれば missing_models に、
// それ以外は最初のノードの誤りの文を持つ workflow_rejected にする。
func (r promptReply) rejection(prompt promptGraph) *domain.ImageEditError {
	var missing []string
	if r.Error != nil {
		class := r.Error.ExtraInfo.ClassType
		if class == "" {
			if m := missingNodePattern.FindStringSubmatch(r.Error.Message); m != nil {
				class = strings.TrimSuffix(m[1], ".")
			}
		}
		if class == "" && r.Error.Type == "missing_node_type" {
			if m := nodeIDPattern.FindStringSubmatch(r.Error.Details); m != nil {
				class = prompt.classType(m[1])
			}
		}
		if class != "" {
			missing = append(missing, class)
		}
	}
	ids := make([]string, 0, len(r.NodeErrors))
	for id := range r.NodeErrors {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return nodeIDLess(ids[i], ids[j]) })
	first := ""
	for _, id := range ids {
		node := r.NodeErrors[id]
		class := node.ClassType
		if class == "" {
			class = prompt.classType(id)
		}
		for _, e := range node.Errors {
			if first == "" {
				first = e.Message
				if e.Details != "" {
					first += ": " + e.Details
				}
			}
			input, loader := modelLoaders[class]
			if loader && e.Type == "value_not_in_list" && (e.ExtraInfo.InputName == "" || e.ExtraInfo.InputName == input) {
				if file := prompt.stringInput(id, input); file != "" && !slices.Contains(missing, file) {
					missing = append(missing, file)
				}
			}
		}
	}
	if len(missing) > 0 {
		return &domain.ImageEditError{Reason: domain.ImageEditMissingModels, Message: strings.Join(missing, ", ")}
	}
	if first == "" && r.Error != nil {
		first = r.Error.Message
	}
	return &domain.ImageEditError{Reason: domain.ImageEditWorkflowRejected, Message: first}
}

// nodeIDLess はノードの id を、数なら数の順に、そうでなければ文字の順に比べる。
func nodeIDLess(a, b string) bool {
	if len(a) != len(b) && isDigits(a) && isDigits(b) {
		return len(a) < len(b)
	}
	return a < b
}

func isDigits(s string) bool {
	return s != "" && strings.Trim(s, "0123456789") == ""
}

// promptGraph は送るワークフローである。埋め込んだファイルの値を数の書き方まで保つよう、
// ノードを汎用の対応のまま持つ。
type promptGraph map[string]map[string]any

// loadPrompt は埋め込んだワークフロー name を送る形で読む。
func loadPrompt(name string) (promptGraph, error) {
	data, err := workflowFiles.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("cannot read the bundled workflow %s: %w", name, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var graph promptGraph
	if err := decoder.Decode(&graph); err != nil {
		return nil, fmt.Errorf("the bundled workflow %s is not valid JSON: %w", name, err)
	}
	return graph, nil
}

// nodeID は _meta.title が title のノードの id を返す。
func (g promptGraph) nodeID(title string) (string, error) {
	for id, node := range g {
		meta, _ := node["_meta"].(map[string]any)
		if meta != nil && meta["title"] == title {
			return id, nil
		}
	}
	return "", fmt.Errorf("the bundled workflow has no node titled %s", title)
}

// setInput は _meta.title が title のノードの入力 input を value にする。
func (g promptGraph) setInput(title, input string, value any) error {
	id, err := g.nodeID(title)
	if err != nil {
		return err
	}
	inputs, ok := g[id]["inputs"].(map[string]any)
	if !ok {
		return fmt.Errorf("the bundled workflow's %s node has no inputs", title)
	}
	inputs[input] = value
	return nil
}

// setLoaderInput は型 class のローダーノードすべての、ファイル名を受ける入力を file にする。
func (g promptGraph) setLoaderInput(class, file string) error {
	input := modelLoaders[class]
	found := false
	for id := range g {
		if g.classType(id) != class {
			continue
		}
		inputs, ok := g[id]["inputs"].(map[string]any)
		if !ok {
			return fmt.Errorf("the bundled workflow's %s node has no inputs", class)
		}
		inputs[input] = file
		found = true
	}
	if !found {
		return fmt.Errorf("the bundled workflow has no %s node", class)
	}
	return nil
}

// classType はノード id の型を返す。無ければ空である。
func (g promptGraph) classType(id string) string {
	class, _ := g[id]["class_type"].(string)
	return class
}

// stringInput はノード id の文字列の入力 input を返す。
func (g promptGraph) stringInput(id, input string) string {
	inputs, _ := g[id]["inputs"].(map[string]any)
	value, _ := inputs[input].(string)
	return value
}

// whiteMask はコマ frame と同じ大きさの、不透明な真っ白の PNG を作る（research.md R-3）。
func whiteMask(frame []byte) ([]byte, error) {
	config, _, err := image.DecodeConfig(bytes.NewReader(frame))
	if err != nil {
		return nil, fmt.Errorf("cannot read the frame's size: %w", err)
	}
	mask := image.NewGray(image.Rect(0, 0, config.Width, config.Height))
	for i := range mask.Pix {
		mask.Pix[i] = 0xff
	}
	var out bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := encoder.Encode(&out, mask); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// uploadPrefix は上げるファイルの名前の頭で、vv- と乱数である（contracts/workflow.md の Run）。
func uploadPrefix() (string, error) {
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return "vv-" + hex.EncodeToString(random[:]), nil
}

// imageExtension は画像 data の形式に合う拡張子を返す。分からなければ .png にする。
func imageExtension(data []byte) string {
	switch http.DetectContentType(data) {
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		return ".png"
	}
}

// routeOf は path から問い合わせの部分を除く。
func routeOf(path string) string {
	return strings.SplitN(path, "?", 2)[0]
}
