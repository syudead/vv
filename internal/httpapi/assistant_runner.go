package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/syudead/vv/internal/domain"
)

// ActionRunner calls screen API handlers in process, as the owner, the way /mcp calls the external API mux: the
// write rows of the assistant catalog, the read tools, and the images read tools refer to. It implements
// app.ActionRunner. Each operation's method, path and schemas come from the generated tool registry.
type ActionRunner struct {
	// api is the generated mux without the auth and mutation boundaries: the caller is the server itself.
	api http.Handler
}

// assistantActionKeys are body fields whose objects, or array elements, take the row's action as their key (add
// or remove); a key of another action is refused. Other rows with an action send it as the body's action field.
var assistantActionKeys = map[string][]string{"applyVideoEdits": {"tags", "credits"}}

// Tools lists the assistant's tools: every owner read and excluded operation and every catalog row.
func (r *ActionRunner) Tools() []domain.AssistantTool { return loadAssistantRegistry().tools() }

// Run implements app.ActionRunner.
func (r *ActionRunner) Run(ctx context.Context, call domain.AssistantCall) (domain.AssistantCallResult, error) {
	row, ok := domain.LookupAssistantAction(call.ActionID)
	if !ok {
		return domain.AssistantCallResult{}, fmt.Errorf("%w: %q", domain.ErrAssistantActionUnknown, call.ActionID)
	}
	operation := row.Operation
	if operation == "" {
		operation = call.Operation
	}
	if !slices.Contains(row.Operations(), operation) || (row.Operation != "" && call.Operation != "" && call.Operation != row.Operation) {
		return domain.AssistantCallResult{}, fmt.Errorf("%w: %q does not call %q", domain.ErrAssistantActionUnknown, row.ID, call.Operation)
	}
	op, ok := loadAssistantRegistry().byID[operation]
	if !ok {
		return domain.AssistantCallResult{}, fmt.Errorf("%w: %q has no operation", domain.ErrAssistantActionUnknown, row.ID)
	}
	target, err := requestTarget(op, call.Path, queryValues(call.Query))
	if err != nil {
		return domain.AssistantCallResult{}, fmt.Errorf("assistant action %s: %w", row.ID, err)
	}
	if !op.Body {
		return r.send(ctx, op.Method, target, nil)
	}
	body, err := r.rowBody(ctx, row, op, target, call.Body)
	if err != nil {
		return domain.AssistantCallResult{}, fmt.Errorf("assistant action %s: %w", row.ID, err)
	}
	return r.send(ctx, op.Method, target, body)
}

// rowBody builds the request body of a row: the current settings for a settings row, the call's fields, and the
// row's action; a schema row's body must match the operation's schema.
func (r *ActionRunner) rowBody(
	ctx context.Context, row domain.AssistantAction, op *registryOperation, target string, fields map[string]any,
) (map[string]any, error) {
	body := map[string]any{}
	if row.ReadsCurrent {
		current, err := r.current(ctx, op, target)
		if err != nil {
			return nil, err
		}
		body = current
	}
	maps.Copy(body, fields)
	if row.Schema {
		if err := validateBody(op, body); err != nil {
			return nil, err
		}
	}
	keys, keyed := assistantActionKeys[row.Operation]
	if row.Action != "" && !keyed {
		body["action"] = row.Action
	}
	if err := checkActionKeys(keys, row.Action, body); err != nil {
		return nil, err
	}
	return body, nil
}

func validateBody(op *registryOperation, body map[string]any) error {
	_, schema, err := op.schemas()
	if err != nil {
		return err
	}
	if schema == nil {
		return fmt.Errorf("%w: %s takes no body", domain.ErrAssistantInvalidArguments, op.ID)
	}
	// Round trip through JSON so typed Go values validate as the decoded JSON a handler receives.
	var instance any
	if err := roundTrip(body, &instance); err != nil {
		return err
	}
	if err := schema.Validate(instance); err != nil {
		return fmt.Errorf("%w: %w", domain.ErrAssistantInvalidArguments, err)
	}
	return nil
}

func roundTrip(value any, out any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode assistant arguments: %w", err)
	}
	return json.Unmarshal(encoded, out)
}

var assistantActionNames = []string{"add", "remove"}

func checkActionKeys(fields []string, action string, body map[string]any) error {
	for _, field := range fields {
		var edits []any
		switch value := body[field].(type) {
		case []any:
			edits = value
		case nil:
		default:
			edits = []any{value}
		}
		for _, edit := range edits {
			values, _ := edit.(map[string]any)
			for _, key := range assistantActionNames {
				if _, ok := values[key]; ok && key != action {
					return fmt.Errorf("%s holds %s, which is not the action %s", field, key, action)
				}
			}
		}
	}
	return nil
}

func queryValues(query map[string]string) url.Values {
	values := url.Values{}
	for key, value := range query {
		values.Set(key, value)
	}
	return values
}

// requestTarget fills the path parameters, escaping each value, and appends the query.
func requestTarget(op *registryOperation, path map[string]string, query url.Values) (string, error) {
	target := op.Path
	for _, name := range op.PathParams {
		value := path[name]
		if value == "" {
			return "", fmt.Errorf("path parameter %s is missing", name)
		}
		target = strings.Replace(target, "{"+name+"}", url.PathEscape(value), 1)
	}
	if len(query) == 0 {
		return target, nil
	}
	return target + "?" + query.Encode(), nil
}

// current reads the settings and keeps the fields the update request takes.
func (r *ActionRunner) current(ctx context.Context, op *registryOperation, target string) (map[string]any, error) {
	result, err := r.send(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	var settings map[string]json.RawMessage
	if err := json.Unmarshal(result.Body, &settings); err != nil {
		return nil, fmt.Errorf("read the current settings at %s: %w", target, err)
	}
	fields, err := op.bodyFields()
	if err != nil {
		return nil, err
	}
	body := map[string]any{}
	for _, field := range fields {
		if value, ok := settings[field]; ok {
			body[field] = value
		}
	}
	return body, nil
}

func (r *ActionRunner) send(ctx context.Context, method, target string, body map[string]any) (domain.AssistantCallResult, error) {
	var reader io.Reader = http.NoBody
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return domain.AssistantCallResult{}, fmt.Errorf("encode the request of %s %s: %w", method, target, err)
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(withAudience(ctx, domain.AudienceOwner), method, target, reader)
	if err != nil {
		return domain.AssistantCallResult{}, fmt.Errorf("build %s %s: %w", method, target, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res := &capturedResponse{header: http.Header{}}
	r.api.ServeHTTP(res, req)
	if res.status >= 200 && res.status < 300 {
		return domain.AssistantCallResult{
			Status: res.status, ContentType: res.header.Get("Content-Type"), Body: res.body.Bytes(),
		}, nil
	}
	return domain.AssistantCallResult{}, callError(res)
}

// callError keeps the screen API's code and reason so the proposal row can show them.
func callError(res *capturedResponse) *domain.AssistantCallError {
	failure := &domain.AssistantCallError{Status: res.status, Code: string(codeInternal)}
	var payload struct {
		Code    string `json:"code"`
		Reason  string `json:"reason"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(res.body.Bytes(), &payload); err != nil || payload.Code == "" {
		failure.Message = strings.TrimSpace(res.body.String())
		return failure
	}
	failure.Code, failure.Reason, failure.Message = payload.Code, payload.Reason, payload.Message
	return failure
}

// capturedResponse records a handler's response; the runner reads it once the handler returns.
type capturedResponse struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (c *capturedResponse) Header() http.Header { return c.header }

func (c *capturedResponse) WriteHeader(status int) {
	if c.status == 0 {
		c.status = status
	}
}

func (c *capturedResponse) Write(p []byte) (int, error) {
	c.WriteHeader(http.StatusOK)
	return c.body.Write(p)
}
