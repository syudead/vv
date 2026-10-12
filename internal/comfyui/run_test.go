package comfyui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

// runProvider は自前の ComfyUI か Comfy Cloud の偽物である。経路ごとの応答を差し替えられ、
// 受けた要求を覚える。
type runProvider struct {
	t      *testing.T
	server *httptest.Server
	cloud  bool
	prefix string // Comfy Cloud なら "/api"

	mu        sync.Mutex
	uploads   map[string][]byte // 上げた名前ごとの中身
	fields    []map[string]string
	prompts   []map[string]any
	clientIDs []string // POST /prompt の client_id
	posts     []recorded
	keys      []string // 受けた要求の X-API-Key
	handlers  map[string]http.HandlerFunc
}

type recorded struct {
	path string
	body string
}

func newRunProvider(t *testing.T, cloud bool) *runProvider {
	t.Helper()
	f := &runProvider{t: t, cloud: cloud, uploads: map[string][]byte{}, handlers: map[string]http.HandlerFunc{}}
	if cloud {
		f.prefix = "/api"
	}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	return f
}

// conn はこの偽物への接続先である。
func (f *runProvider) conn() domain.ComfyUISettings {
	if f.cloud {
		return domain.ComfyUISettings{Provider: domain.ComfyUICloud, APIKey: "secret", URL: "http://unused"}
	}
	return domain.ComfyUISettings{Provider: domain.ComfyUISelfHosted, URL: f.server.URL, APIKey: "kept-for-cloud"}
}

func (f *runProvider) client() *Client {
	return NewWithOptions(fixedSettings{}, Options{CloudBaseURL: f.server.URL + "/api"})
}

// handle は経路 path（/api を除いた形）の応答を差し替える。
func (f *runProvider) handle(path string, handler http.HandlerFunc) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.handlers[path] = handler
}

func (f *runProvider) serve(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, f.prefix)
	if path == "/ws" {
		// WebSocket は X-API-Key ではなく token の問い合わせでキーを渡すので、キーの記録に入れない。
		// 差し替えが無ければ接続を拒む。
		f.mu.Lock()
		handler, ok := f.handlers[path]
		f.mu.Unlock()
		if !ok {
			handler = statusHandler(http.StatusNotFound, "")
		}
		handler(w, r)
		return
	}
	f.mu.Lock()
	f.keys = append(f.keys, r.Header.Get("X-API-Key"))
	handler, ok := f.handlers[path]
	f.mu.Unlock()
	if ok {
		handler(w, r)
		return
	}
	switch {
	case r.Method == http.MethodPost && path == "/upload/image":
		f.upload(w, r)
	case r.Method == http.MethodPost && path == "/prompt":
		var body struct {
			Prompt   map[string]any `json:"prompt"`
			ClientID string         `json:"client_id"`
		}
		decoder := json.NewDecoder(r.Body)
		decoder.UseNumber()
		if err := decoder.Decode(&body); err != nil {
			f.t.Errorf("prompt body: %v", err)
		}
		f.mu.Lock()
		f.prompts = append(f.prompts, body.Prompt)
		f.clientIDs = append(f.clientIDs, body.ClientID)
		f.mu.Unlock()
		_, _ = w.Write([]byte(`{"prompt_id":"run-1","number":1,"node_errors":{}}`))
	case r.Method == http.MethodPost:
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.posts = append(f.posts, recorded{path: path, body: string(body)})
		f.mu.Unlock()
	default:
		f.t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}
}

func (f *runProvider) upload(w http.ResponseWriter, r *http.Request) {
	file, header, err := r.FormFile("image")
	if err != nil {
		f.t.Errorf("upload: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	data, _ := io.ReadAll(file)
	f.mu.Lock()
	f.uploads[header.Filename] = data
	f.fields = append(f.fields, map[string]string{"overwrite": r.FormValue("overwrite"), "type": r.FormValue("type")})
	f.mu.Unlock()
	_ = json.NewEncoder(w).Encode(map[string]string{"name": header.Filename, "subfolder": "", "type": "input"})
}

// pngOf は w×h の単色の PNG を作る。
func pngOf(t *testing.T, w, h int, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, c)
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// embedded は埋め込んだワークフロー name を、送った形と比べられるように読む。
func embedded(t *testing.T, name string) map[string]any {
	t.Helper()
	data, err := workflowFiles.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var out map[string]any
	if err := decoder.Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

// inputsOf はワークフロー w の _meta.title が title のノードの入力を返す。
func inputsOf(t *testing.T, w map[string]any, title string) map[string]any {
	t.Helper()
	for _, node := range w {
		n := node.(map[string]any)
		if meta, ok := n["_meta"].(map[string]any); ok && meta["title"] == title {
			return n["inputs"].(map[string]any)
		}
	}
	t.Fatalf("no node titled %s", title)
	return nil
}

func hasTitle(w map[string]any, title string) bool {
	for _, node := range w {
		if meta, ok := node.(map[string]any)["_meta"].(map[string]any); ok && meta["title"] == title {
			return true
		}
	}
	return false
}

// 送るプロンプトは埋め込んだ版から vv:* の入力だけを変えたもので、参考画像があれば参考画像ありの
// 版を、なければもう一方を使う。上げるファイルは vv- と乱数で始まり、overwrite=true、type=input である。
func TestStartSubmitsBundledWorkflowWithOnlyVVInputs(t *testing.T) {
	for _, cloud := range []bool{false, true} {
		f := newRunProvider(t, cloud)
		frame := pngOf(t, 8, 6, color.RGBA{R: 200, A: 255})
		mask := pngOf(t, 8, 6, color.White)
		reference := pngOf(t, 4, 4, color.RGBA{B: 200, A: 255})
		cases := []struct {
			name      string
			input     domain.ImageEditInput
			workflow  string
			reference bool
		}{
			{"参考画像なし", domain.ImageEditInput{Frame: frame, Mask: mask, Instruction: "空を青くする"}, workflowEdit, false},
			{"参考画像あり", domain.ImageEditInput{Frame: frame, Mask: mask, Reference: reference, Instruction: "空を青くする"}, workflowEditReference, true},
		}
		for _, c := range cases {
			job, err := f.client().Start(context.Background(), f.conn(), c.input)
			if err != nil {
				t.Fatalf("%s: Start = %v", c.name, err)
			}
			if job.ID != "run-1" || job.Output != "99" || job.Connection != f.conn() {
				t.Errorf("%s: job = %+v", c.name, job)
			}
			sent := f.prompts[len(f.prompts)-1]
			want := embedded(t, c.workflow)
			for _, title := range []string{titleFrame, titleMask, titleReference} {
				if !hasTitle(want, title) {
					continue
				}
				uploaded, _ := inputsOf(t, sent, title)["image"].(string)
				if !strings.HasPrefix(uploaded, "vv-") {
					t.Errorf("%s: %s image = %q, want a vv- name", c.name, title, uploaded)
				}
				if _, ok := f.uploads[uploaded]; !ok {
					t.Errorf("%s: %s names %q, which was not uploaded", c.name, title, uploaded)
				}
				inputsOf(t, want, title)["image"] = uploaded
			}
			inputsOf(t, want, titlePrompt)["text"] = "空を青くする"
			if !reflect.DeepEqual(sent, want) {
				t.Errorf("%s: the prompt differs from the bundled workflow beyond vv:* inputs", c.name)
			}
			if hasTitle(sent, titleReference) != c.reference {
				t.Errorf("%s: reference node present = %v", c.name, !c.reference)
			}
			if c.reference && !bytes.Equal(f.uploads[inputsOf(t, sent, titleReference)["image"].(string)], reference) {
				t.Errorf("%s: the reference upload is not the reference image", c.name)
			}
			if !bytes.Equal(f.uploads[inputsOf(t, sent, titleFrame)["image"].(string)], frame) {
				t.Errorf("%s: the frame upload is not the frame", c.name)
			}
		}
		for _, fields := range f.fields {
			if fields["overwrite"] != "true" || fields["type"] != "input" {
				t.Errorf("upload fields = %v", fields)
			}
		}
		assertKeys(t, f)
	}
}

// assertKeys は Comfy Cloud への要求がすべてキーを持ち、自前への要求がどれも持たないことを確かめる。
func assertKeys(t *testing.T, f *runProvider) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, key := range f.keys {
		if f.cloud && key != "secret" || !f.cloud && key != "" {
			t.Errorf("X-API-Key = %q on a %v request", key, map[bool]string{true: "cloud", false: "self-hosted"}[f.cloud])
		}
	}
}

// DiffusionModel を渡すと、自前の ComfyUI へは UNETLoader の unet_name だけをそのファイルに代えて
// 送り、Comfy Cloud へは埋め込んだワークフローのまま送る。
func TestStartWithDiffusionModelReplacesOnlyTheUNETLoaderFile(t *testing.T) {
	cloud := newRunProvider(t, true)
	cloudClient := NewWithOptions(fixedSettings{}, Options{CloudBaseURL: cloud.server.URL + "/api", DiffusionModel: "klein-4b-finetune.safetensors"})
	if _, err := cloudClient.Start(context.Background(), cloud.conn(), domain.ImageEditInput{Frame: pngOf(t, 8, 6, color.White)}); err != nil {
		t.Fatalf("Start on Cloud = %v", err)
	}
	for _, node := range cloud.prompts[len(cloud.prompts)-1] {
		if n := node.(map[string]any); n["class_type"] == "UNETLoader" && n["inputs"].(map[string]any)["unet_name"] != "flux-2-klein-4b-fp8.safetensors" {
			t.Errorf("Cloud unet_name = %v, want the bundled file", n["inputs"].(map[string]any)["unet_name"])
		}
	}

	f := newRunProvider(t, false)
	client := NewWithOptions(fixedSettings{}, Options{DiffusionModel: " klein-4b-finetune.safetensors "})
	frame := pngOf(t, 8, 6, color.RGBA{R: 200, A: 255})
	if _, err := client.Start(context.Background(), f.conn(), domain.ImageEditInput{Frame: frame}); err != nil {
		t.Fatalf("Start = %v", err)
	}
	sent := f.prompts[len(f.prompts)-1]
	want := embedded(t, workflowEdit)
	for id, node := range sent {
		n := node.(map[string]any)
		if n["class_type"] != "UNETLoader" {
			continue
		}
		inputs := n["inputs"].(map[string]any)
		if inputs["unet_name"] != "klein-4b-finetune.safetensors" {
			t.Errorf("unet_name = %v", inputs["unet_name"])
		}
		want[id].(map[string]any)["inputs"].(map[string]any)["unet_name"] = "klein-4b-finetune.safetensors"
	}
	for _, title := range []string{titleFrame, titleMask} {
		inputsOf(t, want, title)["image"] = inputsOf(t, sent, title)["image"]
	}
	if !reflect.DeepEqual(sent, want) {
		t.Errorf("the prompt differs from the bundled workflow beyond vv:* inputs and unet_name")
	}
}

// マスクが無ければ、コマと同じ大きさの真っ白な PNG を上げる。
func TestStartWithoutMaskUploadsWhiteMaskOfFrameSize(t *testing.T) {
	f := newRunProvider(t, false)
	if _, err := f.client().Start(context.Background(), f.conn(), domain.ImageEditInput{Frame: pngOf(t, 7, 5, color.Black), Instruction: "x"}); err != nil {
		t.Fatal(err)
	}
	name := inputsOf(t, f.prompts[0], titleMask)["image"].(string)
	img, err := png.Decode(bytes.NewReader(f.uploads[name]))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds() != image.Rect(0, 0, 7, 5) {
		t.Fatalf("mask bounds = %v", img.Bounds())
	}
	for y := range 5 {
		for x := range 7 {
			if r, g, b, a := img.At(x, y).RGBA(); r != 0xffff || g != 0xffff || b != 0xffff || a != 0xffff {
				t.Fatalf("mask pixel (%d,%d) = %v", x, y, img.At(x, y))
			}
		}
	}
}

// 塗ったマスクと空の指示では契約の既定の指示を一字一句送る。塗っていなければ空のまま送る。
func TestStartInstruction(t *testing.T) {
	const contract = "Remove any text, logo or object in the marked area and restore the background behind it so it matches its surroundings."
	frame := pngOf(t, 4, 4, color.Black)
	mask := pngOf(t, 4, 4, color.White)
	cases := []struct {
		name  string
		input domain.ImageEditInput
		want  string
	}{
		{"塗って空", domain.ImageEditInput{Frame: frame, Mask: mask}, contract},
		{"塗って空白だけ", domain.ImageEditInput{Frame: frame, Mask: mask, Instruction: "  \n"}, contract},
		{"塗って指示あり", domain.ImageEditInput{Frame: frame, Mask: mask, Instruction: "ロゴを消す"}, "ロゴを消す"},
		{"塗らずに空", domain.ImageEditInput{Frame: frame}, ""},
	}
	for _, c := range cases {
		f := newRunProvider(t, false)
		if _, err := f.client().Start(context.Background(), f.conn(), c.input); err != nil {
			t.Fatal(err)
		}
		if got := inputsOf(t, f.prompts[0], titlePrompt)["text"]; got != c.want {
			t.Errorf("%s: text = %q, want %q", c.name, got, c.want)
		}
	}
}

func jsonHandler(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }
}

func statusHandler(code int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	}
}

var job = domain.ImageEditJob{ID: "run-1", Output: "99"}

const historyWithImage = `{"run-1":{"outputs":{"99":{"images":[{"filename":"vv-thumbnail_00001_.png","subfolder":"","type":"output"}]}},"status":{"status_str":"success","completed":true,"messages":[]}}}`

const emptyQueue = `{"queue_running":[],"queue_pending":[]}`

// 自前の ComfyUI の Run states の表の各行が、その vv の状態になる。
func TestStateSelfHosted(t *testing.T) {
	cases := []struct {
		name    string
		queue   string
		history string
		want    domain.ThumbnailEditState
		reason  domain.ImageEditFailure
		message string
	}{
		{"待ち行列", `{"queue_running":[],"queue_pending":[[2,"run-1",{},{},["99"]]]}`, "", domain.ThumbnailEditQueued, "", ""},
		{"実行中", `{"queue_running":[[1,"run-1",{},{},["99"]]],"queue_pending":[[2,"other",{},{},[]]]}`, "", domain.ThumbnailEditRunning, "", ""},
		{"成功", emptyQueue, historyWithImage, domain.ThumbnailEditSucceeded, "", ""},
		{"成功したが画像が無い", emptyQueue, `{"run-1":{"outputs":{"99":{"images":[]}},"status":{"status_str":"success","completed":true}}}`, domain.ThumbnailEditFailed, domain.ImageEditNoOutput, ""},
		{"失敗", emptyQueue, `{"run-1":{"outputs":{},"status":{"status_str":"error","completed":false,"messages":[["execution_start",{}],["execution_error",{"node_id":"50","exception_type":"torch.OutOfMemoryError","exception_message":"CUDA out of memory"}]]}}}`, domain.ThumbnailEditFailed, domain.ImageEditRunFailed, "CUDA out of memory"},
		{"履歴なしで待ち行列から消えた", emptyQueue, `{}`, domain.ThumbnailEditFailed, domain.ImageEditRunFailed, "the run left the ComfyUI queue without a result"},
		{"状態の分からない履歴", emptyQueue, `{"run-1":{"outputs":{}}}`, domain.ThumbnailEditPreparing, "", ""},
	}
	for _, c := range cases {
		f := newRunProvider(t, false)
		f.handle("/queue", jsonHandler(c.queue))
		f.handle("/history/run-1", jsonHandler(c.history))
		j := job
		j.Connection = f.conn()
		got, failure, err := f.client().State(context.Background(), j, domain.ThumbnailEditPreparing)
		if err != nil {
			t.Fatalf("%s: State = %v", c.name, err)
		}
		checkState(t, c.name, got, failure, c.want, c.reason, c.message)
	}
}

func checkState(t *testing.T, name string, got domain.ThumbnailEditState, failure *domain.ImageEditError, want domain.ThumbnailEditState, reason domain.ImageEditFailure, message string) {
	t.Helper()
	if got != want {
		t.Errorf("%s: state = %s, want %s", name, got, want)
	}
	if reason == "" {
		if failure != nil {
			t.Errorf("%s: failure = %v", name, failure)
		}
		return
	}
	if failure == nil || failure.Reason != reason || failure.Message != message {
		t.Errorf("%s: failure = %+v, want %s %q", name, failure, reason, message)
	}
}

// Comfy Cloud の Run states の表の各行が、その vv の状態になる。表に無い値は前の状態を保つ。
func TestStateCloud(t *testing.T) {
	rows := map[domain.ThumbnailEditState][]string{
		domain.ThumbnailEditQueued:    {"submitted", "preprocessing", "queued_waiting", "queued_limited", "allocated", "waiting_to_dispatch", "pending"},
		domain.ThumbnailEditPreparing: {"preparing", "pending_execution"},
		domain.ThumbnailEditRunning:   {"executing", "in_progress", "executed"},
		domain.ThumbnailEditCancelled: {"cancel_requested", "cancelled"},
	}
	for want, statuses := range rows {
		for _, status := range statuses {
			f := newRunProvider(t, true)
			f.handle("/job/run-1/status", jsonHandler(`{"status":"`+status+`"}`))
			got, failure, err := f.client().State(context.Background(), cloudJob(f), domain.ThumbnailEditReady)
			if err != nil {
				t.Fatalf("%s: %v", status, err)
			}
			checkState(t, status, got, failure, want, "", "")
			assertKeys(t, f)
		}
	}
	cases := []struct {
		status  string
		history string
		want    domain.ThumbnailEditState
		reason  domain.ImageEditFailure
		message string
	}{
		{"success", historyWithImage, domain.ThumbnailEditSucceeded, "", ""},
		{"completed", historyWithImage, domain.ThumbnailEditSucceeded, "", ""},
		{"success", `{"run-1":{"outputs":{}}}`, domain.ThumbnailEditFailed, domain.ImageEditNoOutput, ""},
		// 状態が success でも履歴の項目がまだ無ければ、失敗にせず前の状態を保つ。
		{"success", `{}`, domain.ThumbnailEditRunning, "", ""},
		{"error", `{"run-1":{"outputs":{},"status":{"status_str":"error","messages":[["execution_error",{"exception_type":"RuntimeError","exception_message":"boom"}]]}}}`, domain.ThumbnailEditFailed, domain.ImageEditRunFailed, "boom"},
		{"non_retryable_error", `{}`, domain.ThumbnailEditFailed, domain.ImageEditRunFailed, ""},
		{"lost", `{}`, domain.ThumbnailEditFailed, domain.ImageEditRunFailed, ""},
		{"something_new", "", domain.ThumbnailEditRunning, "", ""},
	}
	for _, c := range cases {
		f := newRunProvider(t, true)
		f.handle("/job/run-1/status", jsonHandler(`{"status":"`+c.status+`"}`))
		f.handle("/history_v2/run-1", jsonHandler(c.history))
		got, failure, err := f.client().State(context.Background(), cloudJob(f), domain.ThumbnailEditRunning)
		if err != nil {
			t.Fatalf("%s: %v", c.status, err)
		}
		checkState(t, c.status, got, failure, c.want, c.reason, c.message)
		assertKeys(t, f)
	}
}

func cloudJob(f *runProvider) domain.ImageEditJob {
	j := job
	j.Connection = f.conn()
	return j
}

// sequence は GET に bodies を順に返し（最後のものを繰り返す）、POST は偽物の記録に回す。
func (f *runProvider) sequence(path string, bodies ...string) {
	var mu sync.Mutex
	f.handle(path, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			f.mu.Lock()
			f.posts = append(f.posts, recorded{path: path, body: string(body)})
			f.mu.Unlock()
			return
		}
		mu.Lock()
		body := bodies[0]
		if len(bodies) > 1 {
			bodies = bodies[1:]
		}
		mu.Unlock()
		_, _ = w.Write([]byte(body))
	})
}

const (
	pendingQueue = `{"queue_running":[],"queue_pending":[[2,"run-1",{},{},["99"]]]}`
	runningQueue = `{"queue_running":[[1,"run-1",{},{},["99"]]],"queue_pending":[]}`
)

// 中止は提供元の今の状態で要求を選ぶ。待ち行列にある間は待ち行列から消し、実行中は自前が
// prompt_id を付けて interrupt し、Cloud は POST /interrupt を送る。前に読んだ状態が古くても、
// 終わった実行には何も送らず（Cloud の interrupt が別の実行を止めないため）、消す間に始まった
// 実行は interrupt で止める。
func TestCancel(t *testing.T) {
	cases := []struct {
		name     string
		cloud    bool
		previous domain.ThumbnailEditState
		states   []string // 自前は GET /queue、Cloud は GET /job/{id}/status の応答の順
		history  string
		want     []recorded
	}{
		{"自前の待ち行列", false, domain.ThumbnailEditQueued, []string{pendingQueue, emptyQueue}, `{}`,
			[]recorded{{"/queue", `{"delete":["run-1"]}`}}},
		{"自前の実行中", false, domain.ThumbnailEditRunning, []string{runningQueue}, "",
			[]recorded{{"/interrupt", `{"prompt_id":"run-1"}`}}},
		{"自前で消す間に始まった", false, domain.ThumbnailEditQueued, []string{pendingQueue, runningQueue}, "",
			[]recorded{{"/queue", `{"delete":["run-1"]}`}, {"/interrupt", `{"prompt_id":"run-1"}`}}},
		{"自前で前の状態より先に始まっていた", false, domain.ThumbnailEditQueued, []string{runningQueue}, "",
			[]recorded{{"/interrupt", `{"prompt_id":"run-1"}`}}},
		{"自前で前の状態より先に終わっていた", false, domain.ThumbnailEditRunning, []string{emptyQueue}, historyWithImage, nil},
		{"Cloud の待ち行列", true, domain.ThumbnailEditQueued, []string{`{"status":"queued_waiting"}`, `{"status":"cancelled"}`}, "",
			[]recorded{{"/queue", `{"delete":["run-1"]}`}}},
		{"Cloud の準備中", true, domain.ThumbnailEditPreparing, []string{`{"status":"preparing"}`, `{"status":"cancelled"}`}, "",
			[]recorded{{"/queue", `{"delete":["run-1"]}`}}},
		{"Cloud の実行中", true, domain.ThumbnailEditRunning, []string{`{"status":"executing"}`}, "",
			[]recorded{{"/interrupt", `{}`}}},
		{"Cloud で消す間に始まった", true, domain.ThumbnailEditPreparing, []string{`{"status":"preparing"}`, `{"status":"executing"}`}, "",
			[]recorded{{"/queue", `{"delete":["run-1"]}`}, {"/interrupt", `{}`}}},
		{"Cloud で前の状態より先に終わっていた", true, domain.ThumbnailEditRunning, []string{`{"status":"success"}`}, historyWithImage, nil},
		{"Cloud で前の状態より先に失敗していた", true, domain.ThumbnailEditRunning, []string{`{"status":"error"}`}, `{}`, nil},
		{"Cloud の表に無い値は前の状態で選ぶ", true, domain.ThumbnailEditRunning, []string{`{"status":"something_new"}`}, "",
			[]recorded{{"/interrupt", `{}`}}},
	}
	for _, c := range cases {
		f := newRunProvider(t, c.cloud)
		if c.cloud {
			f.sequence("/job/run-1/status", c.states...)
			f.handle("/history_v2/run-1", jsonHandler(c.history))
		} else {
			f.sequence("/queue", c.states...)
			f.handle("/history/run-1", jsonHandler(c.history))
		}
		if err := f.client().Cancel(context.Background(), cloudJob(f), c.previous); err != nil {
			t.Fatalf("%s: Cancel = %v", c.name, err)
		}
		if !reflect.DeepEqual(f.posts, c.want) {
			t.Errorf("%s: posts = %+v, want %+v", c.name, f.posts, c.want)
		}
		assertKeys(t, f)
	}
}

// 自前の ComfyUI は vv:output の最初の画像を GET /view で取る。
func TestOutputSelfHosted(t *testing.T) {
	f := newRunProvider(t, false)
	f.handle("/history/run-1", jsonHandler(historyWithImage))
	f.handle("/view", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("filename") != "vv-thumbnail_00001_.png" || q.Get("type") != "output" || !q.Has("subfolder") {
			t.Errorf("view query = %q", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte("png-bytes"))
	})
	data, err := f.client().Output(context.Background(), cloudJob(f))
	if err != nil || string(data) != "png-bytes" {
		t.Fatalf("Output = %q, %v", data, err)
	}
}

// Comfy Cloud の出力は署名付き URL への転送先から X-API-Key なしで取る。
func TestOutputCloudFollowsRedirectWithoutKey(t *testing.T) {
	signed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "" {
			t.Errorf("the signed URL received the API key")
		}
		if r.URL.Query().Get("sig") != "abc" {
			t.Errorf("signed query = %q", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte("cloud-png"))
	}))
	t.Cleanup(signed.Close)
	f := newRunProvider(t, true)
	f.handle("/history_v2/run-1", jsonHandler(historyWithImage))
	f.handle("/view", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, signed.URL+"/out.png?sig=abc", http.StatusFound)
	})
	data, err := f.client().Output(context.Background(), cloudJob(f))
	if err != nil || string(data) != "cloud-png" {
		t.Fatalf("Output = %q, %v", data, err)
	}
	assertKeys(t, f)
}

func reasonOf(t *testing.T, name string, err error) *domain.ImageEditError {
	t.Helper()
	var failure *domain.ImageEditError
	if !errors.As(err, &failure) {
		t.Fatalf("%s: err = %v, want an ImageEditError", name, err)
	}
	return failure
}

// Failure reasons の表の各行（timed_out は編集のセッションが数える）が、その接続先の応答から作られる。
func TestFailureReasons(t *testing.T) {
	frame := pngOf(t, 4, 4, color.Black)
	start := func(f *runProvider) error {
		_, err := f.client().Start(context.Background(), f.conn(), domain.ImageEditInput{Frame: frame, Instruction: "x"})
		return err
	}
	closed := newRunProvider(t, false)
	closed.server.Close()

	cases := []struct {
		name    string
		cloud   bool
		setup   func(f *runProvider)
		run     func(f *runProvider) error
		reason  domain.ImageEditFailure
		message string
	}{
		{"つながらない", false, nil, func(*runProvider) error { return start(closed) }, domain.ImageEditUnreachable, ""},
		{"5xx", false, func(f *runProvider) {
			f.handle("/upload/image", statusHandler(http.StatusBadGateway, "proxy"))
		}, start, domain.ImageEditUnreachable, ""},
		{"Cloud の 401", true, func(f *runProvider) {
			f.handle("/upload/image", statusHandler(http.StatusUnauthorized, ""))
		}, start, domain.ImageEditUnauthorized, ""},
		{"Cloud の 403", true, func(f *runProvider) {
			f.handle("/job/run-1/status", statusHandler(http.StatusForbidden, ""))
		}, func(f *runProvider) error {
			_, _, err := f.client().State(context.Background(), cloudJob(f), domain.ThumbnailEditQueued)
			return err
		}, domain.ImageEditUnauthorized, ""},
		{"Cloud の 402", true, func(f *runProvider) {
			f.handle("/prompt", statusHandler(http.StatusPaymentRequired, `{"error":{"type":"payment_required","message":"pay"}}`))
		}, start, domain.ImageEditInsufficientCredits, ""},
		{"クレジットを名指す誤り", true, func(f *runProvider) {
			f.handle("/prompt", statusHandler(http.StatusBadRequest, `{"error":{"type":"insufficient_credits","message":"Not enough credits"},"node_errors":{}}`))
		}, start, domain.ImageEditInsufficientCredits, "Not enough credits"},
		{"ノードの誤り", false, func(f *runProvider) {
			f.handle("/prompt", statusHandler(http.StatusBadRequest, `{"error":{"type":"prompt_outputs_failed_validation","message":"Prompt outputs failed validation"},"node_errors":{
				"45":{"class_type":"Flux2Scheduler","errors":[{"type":"value_smaller_than_min","message":"Value 0 smaller than min of 1","details":"steps"}]},
				"100":{"class_type":"SaveImage","errors":[{"type":"other","message":"later"}]}}}`))
		}, start, domain.ImageEditWorkflowRejected, "Value 0 smaller than min of 1: steps"},
		{"モデルファイルが無い", false, func(f *runProvider) {
			f.handle("/prompt", statusHandler(http.StatusBadRequest, `{"error":{"type":"prompt_outputs_failed_validation","message":"Prompt outputs failed validation"},"node_errors":{
				"1":{"class_type":"UNETLoader","errors":[{"type":"value_not_in_list","message":"Value not in list","details":"unet_name: 'flux-2-klein-4b-fp8.safetensors' not in []","extra_info":{"input_name":"unet_name"}}]},
				"3":{"class_type":"VAELoader","errors":[{"type":"value_not_in_list","message":"Value not in list","extra_info":{"input_name":"vae_name"}}]}}}`))
		}, start, domain.ImageEditMissingModels, "flux-2-klein-4b-fp8.safetensors, flux2-vae.safetensors"},
		{"ノードの型が無い", true, func(f *runProvider) {
			f.handle("/prompt", statusHandler(http.StatusBadRequest, `{"error":{"type":"missing_node_type","message":"Node 'Work size' not found. The custom node may not be installed.","details":"Node ID '#15'","extra_info":{"node_id":"15","class_type":"GetImageSize"}},"node_errors":{}}`))
		}, start, domain.ImageEditMissingModels, "GetImageSize"},
		{"古い ComfyUI のノードの型が無い", false, func(f *runProvider) {
			f.handle("/prompt", statusHandler(http.StatusBadRequest, `{"error":{"type":"invalid_prompt","message":"Cannot execute because node Flux2Scheduler does not exist.","details":"Node ID '#45'"},"node_errors":{}}`))
		}, start, domain.ImageEditMissingModels, "Flux2Scheduler"},
		{"実行の誤り", false, func(f *runProvider) {
			f.handle("/queue", jsonHandler(emptyQueue))
			f.handle("/history/run-1", jsonHandler(`{"run-1":{"outputs":{},"status":{"status_str":"error","messages":[["execution_error",{"exception_type":"ValueError","exception_message":"bad image"}]]}}}`))
		}, stateFailure, domain.ImageEditRunFailed, "bad image"},
		{"実行中のクレジット切れ", true, func(f *runProvider) {
			f.handle("/job/run-1/status", jsonHandler(`{"status":"error"}`))
			f.handle("/history_v2/run-1", jsonHandler(`{"run-1":{"outputs":{},"status":{"status_str":"error","messages":[["execution_error",{"exception_type":"InsufficientCreditsError","exception_message":"out of credits"}]]}}}`))
		}, stateFailure, domain.ImageEditInsufficientCredits, "out of credits"},
		{"出力が無い", false, func(f *runProvider) {
			f.handle("/history/run-1", jsonHandler(`{"run-1":{"outputs":{"99":{"images":[]}},"status":{"status_str":"success","completed":true}}}`))
		}, func(f *runProvider) error {
			_, err := f.client().Output(context.Background(), cloudJob(f))
			return err
		}, domain.ImageEditNoOutput, ""},
	}
	for _, c := range cases {
		f := newRunProvider(t, c.cloud)
		if c.setup != nil {
			c.setup(f)
		}
		failure := reasonOf(t, c.name, c.run(f))
		if failure.Reason != c.reason {
			t.Errorf("%s: reason = %s, want %s (%v)", c.name, failure.Reason, c.reason, failure)
		}
		if c.message != "" && failure.Message != c.message {
			t.Errorf("%s: message = %q, want %q", c.name, failure.Message, c.message)
		}
		if strings.Contains(failure.Error(), "proxy") {
			t.Errorf("%s: the reply body leaked: %v", c.name, failure)
		}
	}
}

// stateFailure は実行の状態を読み、failed の理由を誤りとして返す。
func stateFailure(f *runProvider) error {
	state, failure, err := f.client().State(context.Background(), cloudJob(f), domain.ThumbnailEditRunning)
	if err != nil {
		return err
	}
	if state != domain.ThumbnailEditFailed || failure == nil {
		return errors.New("the run did not fail")
	}
	return failure
}
