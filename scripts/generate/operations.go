package main

import (
	"bytes"
	"fmt"
	"go/format"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// The screen API's operation table, which the peer boundary builds its lists from.
const operationsOut = "internal/httpapi/gen/operations.gen.go"

const (
	jsonMediaType   = "application/json"
	entityExtension = "x-vv-entity"
)

// operation is one row of the generated table; see the ScreenOperation comment in the output.
type operation struct {
	Method     string
	Path       string
	Response   string
	Streams    bool
	Request    string
	PathParams []string
	BodyIDs    []string
	// ResponseIDs are the id fields of the JSON success response.
	ResponseIDs []idField
}

// idField is an id-like integer of a schema: Field is a JSON pointer where "*" stands for every array item, and an
// id array's pointer ends in "/*"; Kind is the x-vv-entity kind, or empty when the spec does not say.
type idField struct {
	Field string
	Kind  string
}

type openAPIDoc map[string]any

var (
	pathParam   = regexp.MustCompile(`\{([^}]+)\}`)
	methodOrder = []string{"get", "put", "post", "delete", "patch"}
)

func writeOperations(root string) error {
	spec, err := os.ReadFile(filepath.Join(root, "api", "openapi.yaml"))
	if err != nil {
		return err
	}
	ops, err := operationTable(spec)
	if err != nil {
		return err
	}
	source, err := renderOperations(ops)
	if err != nil {
		return err
	}
	out := filepath.Join(root, filepath.FromSlash(operationsOut))
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	return os.WriteFile(out, source, 0o644)
}

// operationTable lists every operation in path then method order so the output is stable.
func operationTable(spec []byte) ([]operation, error) {
	// Decode into a plain map: yaml would give nested mappings the named type, which mapping does not accept.
	var raw map[string]any
	if err := yaml.Unmarshal(spec, &raw); err != nil {
		return nil, fmt.Errorf("api/openapi.yaml を読めません: %w", err)
	}
	doc := openAPIDoc(raw)
	paths := mapping(doc["paths"])
	keys := make([]string, 0, len(paths))
	for path := range paths {
		keys = append(keys, path)
	}
	slices.Sort(keys)

	ops := []operation{}
	for _, path := range keys {
		item := mapping(paths[path])
		for _, method := range methodOrder {
			raw, ok := item[method]
			if !ok {
				continue
			}
			op, err := doc.operation(strings.ToUpper(method), path, mapping(raw))
			if err != nil {
				return nil, fmt.Errorf("%s %s: %w", strings.ToUpper(method), path, err)
			}
			ops = append(ops, op)
		}
	}
	return ops, nil
}

func (d openAPIDoc) operation(method, path string, raw map[string]any) (operation, error) {
	op := operation{Method: method, Path: path}
	for i, match := range pathParam.FindAllStringSubmatch(path, -1) {
		if i > 0 {
			op.PathParams = append(op.PathParams, match[1])
		}
	}
	if err := d.successResponse(&op, mapping(raw["responses"])); err != nil {
		return op, err
	}
	body, err := d.resolve(raw["requestBody"])
	if err != nil {
		return op, err
	}
	media, ok := mapping(body["content"])[jsonMediaType]
	if !ok {
		return op, nil
	}
	schema := mapping(media)["schema"]
	op.Request = schemaName(schema)
	ids, err := d.idFields(schema, "", "", nil)
	op.BodyIDs = topLevelNames(ids)
	return op, err
}

// successResponse records the JSON schema of the first 2xx response that has one, and whether any 2xx response carries other bytes.
func (d openAPIDoc) successResponse(op *operation, responses map[string]any) error {
	codes := make([]string, 0, len(responses))
	for code := range responses {
		if strings.HasPrefix(code, "2") {
			codes = append(codes, code)
		}
	}
	slices.Sort(codes)
	for _, code := range codes {
		response, err := d.resolve(responses[code])
		if err != nil {
			return err
		}
		for mediaType, media := range mapping(response["content"]) {
			if mediaType != jsonMediaType {
				op.Streams = true
			} else if op.Response == "" {
				schema := mapping(media)["schema"]
				op.Response = schemaName(schema)
				if op.ResponseIDs, err = d.idFields(schema, "", "", nil); err != nil {
					return err
				}
				if err := requireKinds(op.Response, op.ResponseIDs); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// requireKinds refuses a response id without a kind, since the peer boundary's coverage test reads the kind to
// decide which id a rewritten answer must carry.
func requireKinds(response string, ids []idField) error {
	for _, id := range ids {
		if id.Kind == "" {
			return fmt.Errorf("response %s: no %s names the kind of %s", response, entityExtension, id.Field)
		}
	}
	return nil
}

// idFields walks the schema's objects and arrays for id-like integers (id, ids, *Id, *Ids), as tools/assistanttools
// does for a request body. A kind comes from x-vv-entity on the property, on an id array's items, or on an
// enclosing object, which names the kind of that object's own id.
func (d openAPIDoc) idFields(schema any, pointer, inherited string, seen []string) ([]idField, error) {
	ref, _ := mapping(schema)["$ref"].(string)
	if ref != "" && slices.Contains(seen, ref) {
		return nil, nil
	}
	if ref != "" {
		seen = append(seen, ref)
	}
	resolved, err := d.resolve(schema)
	if err != nil {
		return nil, err
	}
	if kind := refEntity(mapping(schema), resolved); kind != "" {
		inherited = kind
	}
	type nested struct {
		schema             any
		pointer, inherited string
	}
	var walk []nested
	for _, key := range []string{"allOf", "anyOf", "oneOf"} {
		parts, _ := resolved[key].([]any)
		for _, part := range parts {
			walk = append(walk, nested{part, pointer, inherited})
		}
	}
	if items, ok := resolved["items"]; ok {
		walk = append(walk, nested{items, pointer + "/*", inherited})
	}
	var ids []idField
	properties := mapping(resolved["properties"])
	for _, name := range slices.Sorted(maps.Keys(properties)) {
		field := pointer + "/" + strings.NewReplacer("~", "~0", "/", "~1").Replace(name)
		id, isID, err := d.idProperty(name, properties[name])
		switch {
		case err != nil:
			return nil, err
		case !isID:
			walk = append(walk, nested{properties[name], field, ""})
		case id.Kind == "" && name == "id":
			id.Kind = inherited
			fallthrough
		default:
			id.Field = field + id.Field
			ids = append(ids, id)
		}
	}
	for _, next := range walk {
		found, err := d.idFields(next.schema, next.pointer, next.inherited, seen)
		if err != nil {
			return nil, err
		}
		ids = append(ids, found...)
	}
	return ids, nil
}

// idProperty reports whether the property is an id-like integer or integer array, with its kind; Field is "/*" for
// an array and empty otherwise.
func (d openAPIDoc) idProperty(name string, raw any) (idField, bool, error) {
	if name != "id" && name != "ids" && !strings.HasSuffix(name, "Id") && !strings.HasSuffix(name, "Ids") {
		return idField{}, false, nil
	}
	property, err := d.resolve(raw)
	if err != nil {
		return idField{}, false, err
	}
	kind := refEntity(mapping(raw), property)
	switch {
	case hasType(property, "integer"):
		return idField{Kind: kind}, true, nil
	case hasType(property, "array"):
		items, err := d.resolve(property["items"])
		if err != nil || !hasType(items, "integer") {
			return idField{}, false, err
		}
		if itemKind := refEntity(mapping(property["items"]), items); itemKind != "" {
			kind = itemKind
		}
		return idField{Field: "/*", Kind: kind}, true, nil
	}
	return idField{}, false, nil
}

// refEntity reads x-vv-entity next to a $ref first, then on the resolved schema.
func refEntity(node, resolved map[string]any) string {
	if kind, _ := node[entityExtension].(string); kind != "" {
		return kind
	}
	kind, _ := resolved[entityExtension].(string)
	return kind
}

// topLevelNames are the sorted names of the schema's own id properties.
func topLevelNames(ids []idField) []string {
	names := []string{}
	for _, id := range ids {
		name, rest, _ := strings.Cut(strings.TrimPrefix(id.Field, "/"), "/")
		if name != "*" && (rest == "" || rest == "*") && !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

// resolve follows local $ref chains; a missing value resolves to an empty mapping.
func (d openAPIDoc) resolve(value any) (map[string]any, error) {
	node := mapping(value)
	for seen := 0; ; seen++ {
		ref, ok := node["$ref"].(string)
		if !ok {
			return node, nil
		}
		if seen > 16 || !strings.HasPrefix(ref, "#/") {
			return nil, fmt.Errorf("$ref %q を解決できません", ref)
		}
		var target any = map[string]any(d)
		for _, key := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
			next, ok := mapping(target)[key]
			if !ok {
				return nil, fmt.Errorf("$ref %q の先がありません", ref)
			}
			target = next
		}
		node = mapping(target)
	}
}

// schemaName is the component name of a $ref schema, []Name for an array of one, and empty for any other inline schema.
func schemaName(schema any) string {
	node := mapping(schema)
	if ref, ok := node["$ref"].(string); ok {
		return ref[strings.LastIndex(ref, "/")+1:]
	}
	if hasType(node, "array") {
		if item := schemaName(node["items"]); item != "" {
			return "[]" + item
		}
	}
	return ""
}

// hasType accepts both `type: integer` and OpenAPI 3.1's `type: [integer, "null"]`.
func hasType(schema map[string]any, want string) bool {
	switch declared := schema["type"].(type) {
	case string:
		return declared == want
	case []any:
		return slices.Contains(declared, any(want))
	}
	return false
}

func mapping(value any) map[string]any {
	node, _ := value.(map[string]any)
	return node
}

func renderOperations(ops []operation) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString(`// Code generated by scripts/generate from api/openapi.yaml. DO NOT EDIT.

package gen

// ScreenOperation is one operation of the screen API declared in api/openapi.yaml.
type ScreenOperation struct {
	Method string
	// Path is the path pattern with OpenAPI {name} parameters.
	Path string
	// Response is the schema name of the JSON success response, []Name for an array of one, or empty when it has none or declares it inline.
	Response string
	// Streams reports a success response that carries bytes other than JSON.
	Streams bool
	// Request is the schema name of the JSON request body, or empty.
	Request string
	// PathParams are the path parameters after the first, in path order.
	PathParams []string
	// BodyIDs are the request body's top-level integer id fields (id, ids, *Id, *Ids).
	BodyIDs []string
}

// ScreenOperations lists every screen-API operation in path, then method order.
var ScreenOperations = []ScreenOperation{
`)
	for _, op := range ops {
		fmt.Fprintf(&b, "\t{Method: %q, Path: %q", op.Method, op.Path)
		if op.Response != "" {
			fmt.Fprintf(&b, ", Response: %q", op.Response)
		}
		if op.Streams {
			b.WriteString(", Streams: true")
		}
		if op.Request != "" {
			fmt.Fprintf(&b, ", Request: %q", op.Request)
		}
		if len(op.PathParams) > 0 {
			fmt.Fprintf(&b, ", PathParams: %s", stringSlice(op.PathParams))
		}
		if len(op.BodyIDs) > 0 {
			fmt.Fprintf(&b, ", BodyIDs: %s", stringSlice(op.BodyIDs))
		}
		b.WriteString("},\n")
	}
	b.WriteString("}\n")
	renderResponseIDs(&b, ops)
	return format.Source(b.Bytes())
}

func renderResponseIDs(b *bytes.Buffer, ops []operation) {
	responses := map[string][]idField{}
	for _, op := range ops {
		if op.Response != "" && len(op.ResponseIDs) > 0 {
			responses[op.Response] = op.ResponseIDs
		}
	}
	b.WriteString(`
// ScreenID is an id-like integer (id, ids, *Id, *Ids) of a schema.
type ScreenID struct {
	// Field is a JSON pointer where "*" stands for every array item; an id array's pointer ends in "/*".
	Field string
	// Kind is the entity kind x-vv-entity gives the field, its array's items or its enclosing object, or empty.
	Kind string
}

// ScreenResponseIDs lists the id fields of each JSON success response schema that has any, by Response.
var ScreenResponseIDs = map[string][]ScreenID{
`)
	for _, name := range slices.Sorted(maps.Keys(responses)) {
		ids := slices.Clone(responses[name])
		slices.SortFunc(ids, func(a, c idField) int { return strings.Compare(a.Field, c.Field) })
		fmt.Fprintf(b, "\t%q: {\n", name)
		for _, id := range ids {
			fmt.Fprintf(b, "\t\t{Field: %q, Kind: %q},\n", id.Field, id.Kind)
		}
		b.WriteString("\t},\n")
	}
	b.WriteString("}\n")
}

func stringSlice(values []string) string {
	quoted := make([]string, len(values))
	for i, value := range values {
		quoted[i] = fmt.Sprintf("%q", value)
	}
	return "[]string{" + strings.Join(quoted, ", ") + "}"
}
