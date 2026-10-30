package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/syudead/vv/internal/domain"
)

// Read runs a read tool: the arguments must match the operation's schema, and an operation that is not a read
// tool is refused before any call. An image operation answers a reference that Image reads back.
func (r *ActionRunner) Read(ctx context.Context, read domain.AssistantRead) (domain.AssistantReadResult, error) {
	op, err := readOperation(read.Operation)
	if err != nil {
		return domain.AssistantReadResult{}, err
	}
	args, err := readArguments(op, read.Arguments)
	if err != nil {
		return domain.AssistantReadResult{}, err
	}
	target, err := requestTarget(op, pathValues(op, args), queryArguments(op, args))
	if err != nil {
		return domain.AssistantReadResult{}, fmt.Errorf("%w: %w", domain.ErrAssistantInvalidArguments, err)
	}
	var body map[string]any
	if op.Body {
		body, _ = args[assistantBodyArgument].(map[string]any)
	}
	result, err := r.send(ctx, op.Method, target, op.BodyType, body)
	if err != nil {
		return domain.AssistantReadResult{}, err
	}
	if op.Response == responseImage {
		return imageReference(op, args, result.ContentType)
	}
	return domain.AssistantReadResult{Body: maskURLCredentials(result.Body)}, nil
}

// Image reads the image a reference names, such as "getVideoFrame?atMs=1000&id=7".
func (r *ActionRunner) Image(ctx context.Context, reference string) (domain.AssistantImage, error) {
	name, rawQuery, _ := strings.Cut(reference, "?")
	op, err := readOperation(name)
	if err != nil {
		return domain.AssistantImage{}, err
	}
	if op.Response != responseImage {
		return domain.AssistantImage{}, fmt.Errorf("%w: %s answers no image", domain.ErrAssistantInvalidArguments, name)
	}
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return domain.AssistantImage{}, fmt.Errorf("%w: %w", domain.ErrAssistantInvalidArguments, err)
	}
	path := map[string]string{}
	for _, param := range op.PathParams {
		path[param] = values.Get(param)
	}
	query := url.Values{}
	for _, param := range op.Query {
		if list := values[param.Name]; len(list) > 0 {
			query[param.Name] = queryList(param, list)
		}
	}
	target, err := requestTarget(op, path, query)
	if err != nil {
		return domain.AssistantImage{}, fmt.Errorf("%w: %w", domain.ErrAssistantInvalidArguments, err)
	}
	result, err := r.send(ctx, http.MethodGet, target, "", nil)
	if err != nil {
		return domain.AssistantImage{}, err
	}
	return domain.AssistantImage{Bytes: result.Body, ContentType: result.ContentType}, nil
}

// readOperation finds an owner operation classified as a read tool.
func readOperation(name string) (*registryOperation, error) {
	op, ok := loadAssistantRegistry().byID[name]
	if !ok || !op.Owner {
		return nil, fmt.Errorf("%w: %q", domain.ErrAssistantNotReadTool, name)
	}
	class, err := op.classify()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrAssistantNotReadTool, err)
	}
	switch class.Kind {
	case domain.AssistantToolRead:
		return op, nil
	case domain.AssistantToolExcluded:
		return nil, &domain.AssistantExcludedError{Exclusion: *class.Exclusion}
	}
	return nil, fmt.Errorf("%w: %s is a write", domain.ErrAssistantNotReadTool, name)
}

func readArguments(op *registryOperation, raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	var instance any
	if err := json.Unmarshal(raw, &instance); err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrAssistantInvalidArguments, err)
	}
	schema, _, err := op.schemas()
	if err != nil {
		return nil, err
	}
	if err := schema.Validate(instance); err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrAssistantInvalidArguments, err)
	}
	args, ok := instance.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: the arguments are not an object", domain.ErrAssistantInvalidArguments)
	}
	return args, nil
}

func pathValues(op *registryOperation, args map[string]any) map[string]string {
	path := map[string]string{}
	for _, name := range op.PathParams {
		if value, ok := args[name]; ok {
			path[name] = argumentText(value)
		}
	}
	return path
}

func queryArguments(op *registryOperation, args map[string]any) url.Values {
	query := url.Values{}
	for _, param := range op.Query {
		value, ok := args[param.Name]
		if !ok {
			continue
		}
		var list []string
		if items, isList := value.([]any); isList {
			for _, item := range items {
				list = append(list, argumentText(item))
			}
		} else {
			list = []string{argumentText(value)}
		}
		query[param.Name] = queryList(param, list)
	}
	return query
}

// queryList joins an array parameter with commas when the spec sets explode to false.
func queryList(param registryQuery, list []string) []string {
	if param.Explode || len(list) < 2 {
		return list
	}
	return []string{strings.Join(list, ",")}
}

func argumentText(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	}
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

// imageReference names the image by its operation and parameters, which Image calls again; the model never
// holds the bytes or a URL.
func imageReference(op *registryOperation, args map[string]any, contentType string) (domain.AssistantReadResult, error) {
	values := url.Values{}
	for name, value := range pathValues(op, args) {
		values.Set(name, value)
	}
	for name, list := range queryArguments(op, args) {
		values[name] = list
	}
	reference := op.ID
	if len(values) > 0 {
		reference += "?" + values.Encode()
	}
	body, err := json.Marshal(map[string]string{"image": reference, "contentType": contentType})
	if err != nil {
		return domain.AssistantReadResult{}, fmt.Errorf("encode the image reference: %w", err)
	}
	return domain.AssistantReadResult{Body: body}, nil
}

// urlCredentials matches the userinfo of a URL, such as user:pass@ in socks5://user:pass@host:1080.
var urlCredentials = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.\-]*://)[^\s/?#@"\\]+@`)

// maskURLCredentials keeps credentials in saved URLs away from the model; marking the operation secret is the
// first guard, and this catches URLs the marks miss.
func maskURLCredentials(body []byte) json.RawMessage {
	return urlCredentials.ReplaceAll(body, []byte("${1}***@"))
}
