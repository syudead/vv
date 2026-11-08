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
	_ "image/jpeg"
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

// Send this exact English instruction for a painted mask with no user instruction, regardless of
// locale.
const DefaultInstruction = "Remove any text, logo or object in the marked area and restore the background behind it so it matches its surroundings."

const requestTimeout = 2 * time.Minute

// Queue responses include complete pending workflows.
const maxQueueBytes = 32 << 20

const maxOutputBytes = 64 << 20

// Node titles filled by the application.
const (
	titleFrame     = "vv:frame"
	titlePrompt    = "vv:prompt"
	titleMask      = "vv:mask"
	titleReference = "vv:reference"
	titleOutput    = "vv:output"
)

// Start fills only vv:* inputs, selecting the reference-image workflow when needed and supplying a
// white mask or DefaultInstruction as appropriate. Uploaded inputs and outputs remain on the
// provider; Progress uses the same client ID.
func (c *Client) Start(ctx context.Context, conn domain.ComfyUISettings, input domain.ImageEditInput) (domain.ImageEditJob, error) {
	target, err := c.target(conn)
	if err != nil {
		return domain.ImageEditJob{}, err
	}
	p := presetFor(conn.EffectiveModel())
	name := p.edit
	if input.Reference != nil {
		name = p.editReference
	}
	prompt, err := loadPrompt(name)
	if err != nil {
		return domain.ImageEditJob{}, err
	}
	if override := c.diffusionModelFor(conn); override != "" {
		if err := prompt.setLoaderInput(diffusionLoader, override); err != nil {
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
	if err := prompt.setPrompt(instruction); err != nil {
		return domain.ImageEditJob{}, err
	}
	clientID, err := newClientID()
	if err != nil {
		return domain.ImageEditJob{}, err
	}
	// Wait briefly for socket registration before submitting to avoid losing early progress.
	watch := c.startProgress(ctx, target, clientID)
	watch.waitReady(ctx)
	id, err := target.submit(ctx, prompt, clientID)
	if err != nil {
		watch.stop()
		return domain.ImageEditJob{}, err
	}
	c.keepProgress(id, watch)
	return domain.ImageEditJob{Connection: conn, ID: id, Output: output}, nil
}

// State maps provider states, preserves previous for unknown values, and closes progress sockets on
// completion.
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

// Cancel refreshes provider state before choosing queue deletion or interruption, rechecking if a
// queued job starts meanwhile; never interrupt completed Cloud jobs because Cloud interrupts all
// active account jobs. Close progress only after successful cancellation.
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

// Output fetches the first vv:output image; follow Cloud's signed redirect without the API key, and
// report no_output when absent.
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

type target struct {
	client *Client
	base   string
	apiKey string
	cloud  bool
}

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

// Map status failures without reading response bodies.
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

func (t *target) request(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, t.base+path, body)
	if err != nil {
		return nil, fmt.Errorf("cannot build the request to %s: %w", t.name(), err)
	}
	return req, nil
}

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

// upload returns the LoadImage filename, including any subfolder, after uploading with
// overwrite=true and type=input.
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

// submit shares clientID with the progress socket and maps rejection details to workflow_rejected,
// missing_models, or insufficient_credits.
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

// Follow at most one redirect and never forward X-API-Key.
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

func (t *target) state(ctx context.Context, job domain.ImageEditJob, previous domain.ThumbnailEditState) (domain.ThumbnailEditState, *domain.ImageEditError, error) {
	if t.cloud {
		return t.cloudState(ctx, job, previous)
	}
	return t.selfHostedState(ctx, job, previous)
}

// Self-hosted interrupt targets prompt_id; Cloud cannot target an individual run.
func (t *target) interrupt(ctx context.Context, id string) error {
	var body any = map[string]string{"prompt_id": id}
	if t.cloud {
		body = struct{}{}
	}
	return t.postJSON(ctx, "/interrupt", body, nil)
}

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
			// Cloud success may precede history publication; preserve the previous state and retry until
			// output appears or the session deadline expires.
			return previous, nil, nil
		}
		return succeededOrNoOutput(entry, job.Output)
	case domain.ThumbnailEditFailed:
		failure := &domain.ImageEditError{Reason: domain.ImageEditRunFailed, Message: status.ErrorMessage}
		// Prefer execution errors from history, falling back to the status response.
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

// cloudState additionally requires an output image before accepting success.
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

func succeededOrNoOutput(entry *historyEntry, output string) (domain.ThumbnailEditState, *domain.ImageEditError, error) {
	if entry != nil {
		if _, ok := entry.firstImage(output); ok {
			return domain.ThumbnailEditSucceeded, nil, nil
		}
	}
	return domain.ThumbnailEditFailed, &domain.ImageEditError{Reason: domain.ImageEditNoOutput}, nil
}

// Queue items are [number, prompt_id, prompt, extra_data, outputs].
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

type imageRef struct {
	Filename  string `json:"filename"`
	Subfolder string `json:"subfolder"`
	Type      string `json:"type"`
}

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

// Both history endpoints normally return an ID-keyed map; absent entries return nil.
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
		// Also accept providers returning the entry directly.
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

func mentionsCredits(kind string) bool {
	return strings.Contains(strings.ToLower(kind), "credit")
}

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

// Older ComfyUI versions describe missing node classes in free text.
var missingNodePattern = regexp.MustCompile(`node (\S+) does not exist`)

var nodeIDPattern = regexp.MustCompile(`Node ID '#([^']+)'`)

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

// Compare numeric IDs numerically and other IDs lexically.
func nodeIDLess(a, b string) bool {
	if len(a) != len(b) && isDigits(a) && isDigits(b) {
		return len(a) < len(b)
	}
	return a < b
}

func isDigits(s string) bool {
	return s != "" && strings.Trim(s, "0123456789") == ""
}

// Keep generic nodes to preserve embedded workflow values, including number representations.
type promptGraph map[string]map[string]any

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

func (g promptGraph) nodeID(title string) (string, error) {
	for id, node := range g {
		meta, _ := node["_meta"].(map[string]any)
		if meta != nil && meta["title"] == title {
			return id, nil
		}
	}
	return "", fmt.Errorf("the bundled workflow has no node titled %s", title)
}

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

// The instruction input name depends on the node class.
func (g promptGraph) setPrompt(instruction string) error {
	id, err := g.nodeID(titlePrompt)
	if err != nil {
		return err
	}
	input, ok := promptInputs[g.classType(id)]
	if !ok {
		return fmt.Errorf("the bundled workflow's %s node has an unknown type %s", titlePrompt, g.classType(id))
	}
	return g.setInput(titlePrompt, input, instruction)
}

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

func (g promptGraph) classType(id string) string {
	class, _ := g[id]["class_type"].(string)
	return class
}

func (g promptGraph) stringInput(id, input string) string {
	inputs, _ := g[id]["inputs"].(map[string]any)
	value, _ := inputs[input].(string)
	return value
}

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

func uploadPrefix() (string, error) {
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return "vv-" + hex.EncodeToString(random[:]), nil
}

// Unknown image formats default to .png.
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

func routeOf(path string) string {
	return strings.SplitN(path, "?", 2)[0]
}
