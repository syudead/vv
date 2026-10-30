package httpapi

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/syudead/vv/internal/domain"
)

// assistantToolsJSON is written by task generate from api/openapi.yaml (tools/assistanttools).
//
//go:embed gen/assistant_tools.gen.json
var assistantToolsJSON []byte

// assistantBodyArgument is the tool argument that holds an operation's JSON body.
const assistantBodyArgument = "body"

const (
	responseImage = "image"
	responseMedia = "media"
)

// registryOperation mirrors one operation of the generated registry.
type registryOperation struct {
	ID          string          `json:"operationId"`
	Method      string          `json:"method"`
	Path        string          `json:"path"`
	Summary     string          `json:"summary"`
	Description string          `json:"description"`
	Owner       bool            `json:"owner"`
	Response    string          `json:"response"`
	Secret      bool            `json:"secret"`
	PathParams  []string        `json:"pathParams"`
	Query       []registryQuery `json:"query"`
	Parameters  json.RawMessage `json:"parameters"`
	Body        bool            `json:"body"`
	IDs         []registryID    `json:"ids"`

	schemaOnce sync.Once
	schema     *jsonschema.Resolved
	bodySchema *jsonschema.Resolved
	schemaErr  error
}

type registryQuery struct {
	Name    string `json:"name"`
	Explode bool   `json:"explode"`
}

type registryID struct {
	In    string `json:"in"`
	Field string `json:"field"`
	Kind  string `json:"kind"`
}

type assistantRegistry struct {
	operations []*registryOperation
	byID       map[string]*registryOperation
}

func decodeAssistantRegistry(raw []byte) (*assistantRegistry, error) {
	var file struct {
		Operations []*registryOperation `json:"operations"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("decode the assistant tool registry: %w", err)
	}
	registry := &assistantRegistry{operations: file.Operations, byID: map[string]*registryOperation{}}
	for _, op := range file.Operations {
		registry.byID[op.ID] = op
	}
	return registry, nil
}

// loadAssistantRegistry decodes the embedded file once; a broken file is a build defect that tests catch.
var loadAssistantRegistry = sync.OnceValue(func() *assistantRegistry {
	registry, err := decodeAssistantRegistry(assistantToolsJSON)
	if err != nil {
		panic(err)
	}
	return registry
})

// schemas resolves the arguments schema and its body part once per operation.
func (op *registryOperation) schemas() (args, body *jsonschema.Resolved, err error) {
	op.schemaOnce.Do(func() {
		var schema jsonschema.Schema
		if op.schemaErr = json.Unmarshal(op.Parameters, &schema); op.schemaErr != nil {
			return
		}
		if op.schema, op.schemaErr = schema.Resolve(nil); op.schemaErr != nil {
			return
		}
		if part, ok := schema.Properties[assistantBodyArgument]; ok && op.Body {
			op.bodySchema, op.schemaErr = part.Resolve(nil)
		}
	})
	if op.schemaErr != nil {
		return nil, nil, fmt.Errorf("the schema of %s: %w", op.ID, op.schemaErr)
	}
	return op.schema, op.bodySchema, nil
}

func (op *registryOperation) classify() (domain.AssistantClassification, error) {
	return domain.ClassifyAssistantOperation(op.ID, op.Method, op.Path)
}

func (op *registryOperation) idFields() []domain.AssistantIDField {
	out := make([]domain.AssistantIDField, len(op.IDs))
	for i, id := range op.IDs {
		out[i] = domain.AssistantIDField{In: id.In, Field: id.Field, Kind: id.Kind}
	}
	return out
}

// tools lists every owner operation as a read or excluded tool, and every catalog row as a write tool, by name.
func (r *assistantRegistry) tools() []domain.AssistantTool {
	var tools []domain.AssistantTool
	for _, op := range r.operations {
		if !op.Owner {
			continue
		}
		class, err := op.classify()
		if err != nil || class.Kind == domain.AssistantToolWrite {
			continue
		}
		tool := domain.AssistantTool{
			Name: op.ID, Kind: class.Kind, Operation: op.ID, Summary: op.Summary, Description: op.Description,
			Exclusion: class.Exclusion,
		}
		if class.Kind == domain.AssistantToolRead {
			tool.Parameters, tool.Image, tool.IDs = op.Parameters, op.Response == responseImage, op.idFields()
		}
		tools = append(tools, tool)
	}
	for _, row := range domain.AssistantCatalog() {
		op, ok := r.byID[row.Operations()[0]]
		if !ok {
			continue
		}
		tool := domain.AssistantTool{
			Name: row.ID, Kind: domain.AssistantToolWrite, Operation: row.Operation, Summary: op.Summary,
			Description: op.Description, Row: &row,
		}
		if row.Schema {
			tool.Parameters, tool.IDs = op.Parameters, op.idFields()
		}
		tools = append(tools, tool)
	}
	slices.SortFunc(tools, func(a, b domain.AssistantTool) int { return strings.Compare(a.Name, b.Name) })
	return tools
}

// bodyFields names the properties of the operation's JSON body, which a settings row reads back as current values.
func (op *registryOperation) bodyFields() ([]string, error) {
	var params struct {
		Properties map[string]struct {
			Properties map[string]json.RawMessage `json:"properties"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(op.Parameters, &params); err != nil {
		return nil, fmt.Errorf("the schema of %s: %w", op.ID, err)
	}
	fields := slices.Collect(maps.Keys(params.Properties[assistantBodyArgument].Properties))
	slices.Sort(fields)
	return fields, nil
}
