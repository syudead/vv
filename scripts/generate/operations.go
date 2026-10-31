package main

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// The screen API's operation table, which the peer boundary builds its lists from.
const operationsOut = "internal/httpapi/gen/operations.gen.go"

const jsonMediaType = "application/json"

// operation is one row of the generated table; see the ScreenOperation comment in the output.
type operation struct {
	Method     string
	Path       string
	Response   string
	Streams    bool
	Request    string
	PathParams []string
	BodyIDs    []string
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
	op.BodyIDs, err = d.integerIDFields(schema)
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
				op.Response = schemaName(mapping(media)["schema"])
			}
		}
	}
	return nil
}

// integerIDFields names the body's top-level integer properties called id or ending in Id, and the integer arrays named ids or ending in Ids.
func (d openAPIDoc) integerIDFields(schema any) ([]string, error) {
	resolved, err := d.resolve(schema)
	if err != nil {
		return nil, err
	}
	fields := []string{}
	for name, raw := range mapping(resolved["properties"]) {
		property, err := d.resolve(raw)
		if err != nil {
			return nil, err
		}
		switch {
		case (name == "id" || strings.HasSuffix(name, "Id")) && hasType(property, "integer"):
		case (name == "ids" || strings.HasSuffix(name, "Ids")) && hasType(property, "array"):
			items, err := d.resolve(property["items"])
			if err != nil {
				return nil, err
			}
			if !hasType(items, "integer") {
				continue
			}
		default:
			continue
		}
		fields = append(fields, name)
	}
	slices.Sort(fields)
	return fields, nil
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
	return format.Source(b.Bytes())
}

func stringSlice(values []string) string {
	quoted := make([]string, len(values))
	for i, value := range values {
		quoted[i] = fmt.Sprintf("%q", value)
	}
	return "[]string{" + strings.Join(quoted, ", ") + "}"
}
