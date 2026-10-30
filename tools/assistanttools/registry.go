package main

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// Registry is the generated file's content; internal/httpapi embeds and decodes it.
type Registry struct {
	Operations []Operation `json:"operations"`
}

// Operation is what the assistant needs of one screen API operation.
type Operation struct {
	ID          string `json:"operationId"`
	Method      string `json:"method"`
	Path        string `json:"path"`
	Summary     string `json:"summary"`
	Description string `json:"description,omitempty"`
	// Owner is false for an operation that drops the session requirement (security: []), which no owner tool calls.
	Owner bool `json:"owner"`
	// Response is the 2xx body: json, image, media (another stream or file) or none.
	Response string `json:"response"`
	// Secret marks a response that reaches a credential property.
	Secret     bool         `json:"secret,omitempty"`
	PathParams []string     `json:"pathParams,omitempty"`
	Query      []QueryParam `json:"query,omitempty"`
	// Parameters is the JSON Schema object of a tool's arguments, with every $ref inlined: the path and query
	// parameters by name, and the body as the property named BodyArgument.
	Parameters map[string]any `json:"parameters"`
	// Body is true when the operation takes a JSON body, or a multipart body whose text fields the arguments hold.
	Body bool `json:"body,omitempty"`
	// BodyType is the body's media type, application/json or multipart/form-data.
	BodyType string    `json:"bodyType,omitempty"`
	IDs      []IDField `json:"ids,omitempty"`
}

// QueryParam is a query parameter; an array with Explode false is sent comma-joined.
type QueryParam struct {
	Name    string `json:"name"`
	Explode bool   `json:"explode"`
}

// IDField is an id-like integer of the request with the entity kind it names; Kind is empty when the spec does
// not say, which the coverage check reports for a write tool.
type IDField struct {
	// In is path or body.
	In string `json:"in"`
	// Field is the path parameter's name, or a JSON pointer into the body where "*" stands for every array item.
	Field string `json:"field"`
	Kind  string `json:"kind"`
}

const (
	responseJSON  = "json"
	responseImage = "image"
	responseMedia = "media"
	responseNone  = "none"

	bodyJSON      = "application/json"
	bodyMultipart = "multipart/form-data"

	// BodyArgument is the argument that holds the request body; no path or query parameter takes the name.
	BodyArgument = "body"

	entityExtension = "x-vv-entity"
	secretExtension = "x-vv-secret"
)

// Load reads the spec and builds the registry, sorted by operation id.
func Load(path string) (Registry, error) {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile(path)
	if err != nil {
		return Registry{}, fmt.Errorf("read %s: %w", path, err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		return Registry{}, fmt.Errorf("validate %s: %w", path, err)
	}
	var registry Registry
	for _, route := range doc.Paths.InMatchingOrder() {
		item := doc.Paths.Value(route)
		for method, op := range item.Operations() {
			operation, err := describe(doc, route, method, item, op)
			if err != nil {
				return Registry{}, fmt.Errorf("%s %s: %w", method, route, err)
			}
			registry.Operations = append(registry.Operations, operation)
		}
	}
	slices.SortFunc(registry.Operations, func(a, b Operation) int { return strings.Compare(a.ID, b.ID) })
	return registry, nil
}

func describe(doc *openapi3.T, route, method string, item *openapi3.PathItem, op *openapi3.Operation) (Operation, error) {
	if op.OperationID == "" {
		return Operation{}, fmt.Errorf("the operation has no operationId")
	}
	security := doc.Security
	if op.Security != nil {
		security = *op.Security
	}
	operation := Operation{
		ID: op.OperationID, Method: method, Path: route, Summary: op.Summary, Description: op.Description,
		Owner: len(security) > 0, Response: responseKind(op),
	}
	if err := operation.addParameters(slices.Concat(item.Parameters, op.Parameters)); err != nil {
		return Operation{}, err
	}
	if err := operation.addBody(op.RequestBody); err != nil {
		return Operation{}, err
	}
	secret, err := reachesSecret(op)
	if err != nil {
		return Operation{}, err
	}
	operation.Secret = secret
	return operation, nil
}

func responseKind(op *openapi3.Operation) string {
	kind := responseNone
	for status, response := range op.Responses.Map() {
		if !strings.HasPrefix(status, "2") || response.Value == nil {
			continue
		}
		for contentType := range response.Value.Content {
			switch {
			case contentType == "application/json":
				kind = responseJSON
			case strings.HasPrefix(contentType, "image/"):
				return responseImage
			default:
				return responseMedia
			}
		}
	}
	return kind
}

func (o *Operation) addParameters(params openapi3.Parameters) error {
	properties := map[string]any{}
	required := []string{}
	for _, ref := range params {
		param := ref.Value
		if param.In != openapi3.ParameterInPath && param.In != openapi3.ParameterInQuery {
			continue
		}
		if param.Schema == nil {
			return fmt.Errorf("parameter %s has no schema", param.Name)
		}
		schema, err := inline(param.Schema, nil)
		if err != nil {
			return fmt.Errorf("parameter %s: %w", param.Name, err)
		}
		if param.Description != "" {
			schema["description"] = param.Description
		}
		properties[param.Name] = schema
		if param.Required {
			required = append(required, param.Name)
		}
		if param.In == openapi3.ParameterInPath {
			o.PathParams = append(o.PathParams, param.Name)
		} else {
			o.Query = append(o.Query, QueryParam{Name: param.Name, Explode: param.Explode == nil || *param.Explode})
		}
		if idLike(param.Name) {
			o.IDs = append(o.IDs, IDField{In: "path", Field: param.Name, Kind: parameterKind(ref)})
		}
	}
	o.Parameters = map[string]any{"type": "object", "properties": properties}
	if len(required) > 0 {
		o.Parameters["required"] = required
	}
	return nil
}

// parameterKind reads x-vv-entity first, then the components/parameters name: PersonId names a person.
func parameterKind(ref *openapi3.ParameterRef) string {
	if kind := entity(ref.Value.Extensions); kind != "" {
		return kind
	}
	if kind := entity(ref.Value.Schema.Value.Extensions); kind != "" {
		return kind
	}
	name := ref.Ref[strings.LastIndexByte(ref.Ref, '/')+1:]
	name, ok := strings.CutSuffix(name, "Id")
	if !ok || name == "" {
		return ""
	}
	return strings.ToLower(name[:1]) + name[1:]
}

func (o *Operation) addBody(body *openapi3.RequestBodyRef) error {
	if body == nil || body.Value == nil {
		return nil
	}
	bodyType := bodyJSON
	media := body.Value.Content.Get(bodyType)
	if media == nil {
		bodyType = bodyMultipart
		media = body.Value.Content.Get(bodyType)
	}
	if media == nil || media.Schema == nil {
		return nil
	}
	schema, err := inline(media.Schema, nil)
	if err != nil {
		return fmt.Errorf("request body: %w", err)
	}
	if bodyType == bodyMultipart && !keepTextParts(schema) {
		return nil
	}
	properties, _ := o.Parameters["properties"].(map[string]any)
	if _, taken := properties[BodyArgument]; taken {
		return fmt.Errorf("a parameter is named %s, which holds the request body", BodyArgument)
	}
	properties[BodyArgument] = schema
	if body.Value.Required {
		required, _ := o.Parameters["required"].([]string)
		o.Parameters["required"] = append(required, BodyArgument)
	}
	o.Body, o.BodyType = true, bodyType
	o.IDs = append(o.IDs, bodyIDs(media.Schema, "", "", nil)...)
	return nil
}

// keepTextParts drops a multipart body's file parts, as the assistant holds no file to send, and reports whether a
// text field is left.
func keepTextParts(schema map[string]any) bool {
	properties, _ := schema["properties"].(map[string]any)
	for name, property := range properties {
		if part, _ := property.(map[string]any); part["format"] == "binary" {
			delete(properties, name)
		}
	}
	if required, ok := schema["required"].([]any); ok {
		required = slices.DeleteFunc(required, func(name any) bool {
			field, _ := name.(string)
			_, kept := properties[field]
			return !kept
		})
		schema["required"] = required
		if len(required) == 0 {
			delete(schema, "required")
		}
	}
	return len(properties) > 0
}

// bodyIDs walks the body's objects and arrays for id-like integers. A kind comes from x-vv-entity on the
// property, on an id array's items, or on an enclosing object, which names the kind of that object's own id.
func bodyIDs(ref *openapi3.SchemaRef, pointer, inherited string, seen []string) []IDField {
	if ref == nil || ref.Value == nil || (ref.Ref != "" && slices.Contains(seen, ref.Ref)) {
		return nil
	}
	if ref.Ref != "" {
		seen = append(seen, ref.Ref)
	}
	if kind := refEntity(ref); kind != "" {
		inherited = kind
	}
	schema := ref.Value
	var ids []IDField
	for _, part := range slices.Concat(schema.AllOf, schema.AnyOf, schema.OneOf) {
		ids = append(ids, bodyIDs(part, pointer, inherited, seen)...)
	}
	if schema.Items != nil {
		ids = append(ids, bodyIDs(schema.Items, pointer+"/*", inherited, seen)...)
	}
	for _, name := range sortedKeys(schema.Properties) {
		property := schema.Properties[name]
		field := pointer + "/" + escapePointer(name)
		if kind, ok := idProperty(name, property); ok {
			if kind == "" && name == "id" {
				kind = inherited
			}
			ids = append(ids, IDField{In: "body", Field: field, Kind: kind})
			continue
		}
		ids = append(ids, bodyIDs(property, field, "", seen)...)
	}
	return ids
}

func idProperty(name string, property *openapi3.SchemaRef) (string, bool) {
	if !idLike(name) || property.Value == nil {
		return "", false
	}
	value := property.Value
	switch {
	case value.Type.Includes(openapi3.TypeInteger):
		return refEntity(property), true
	case value.Type.Includes(openapi3.TypeArray) && value.Items != nil &&
		value.Items.Value.Type.Includes(openapi3.TypeInteger):
		if kind := refEntity(value.Items); kind != "" {
			return kind, true
		}
		return refEntity(property), true
	}
	return "", false
}

// refEntity reads x-vv-entity next to a $ref first, then on the schema itself.
func refEntity(ref *openapi3.SchemaRef) string {
	if kind := entity(ref.Extensions); kind != "" {
		return kind
	}
	return entity(ref.Value.Extensions)
}

func idLike(name string) bool {
	return name == "id" || name == "ids" || strings.HasSuffix(name, "Id") || strings.HasSuffix(name, "Ids")
}

func entity(extensions map[string]any) string {
	kind, _ := extensions[entityExtension].(string)
	return kind
}

func escapePointer(name string) string {
	return strings.NewReplacer("~", "~0", "/", "~1").Replace(name)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
