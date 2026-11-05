package main

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// dropped keys say nothing the model or the validator uses; x- extensions are dropped separately.
var droppedKeys = []string{"example", "examples", "discriminator", "xml", "externalDocs"}

// unsupported keys would need their subschemas inlined too; the spec uses none of them.
var unsupportedKeys = []string{
	"contains", "patternProperties", "dependentSchemas", "propertyNames", "if", "then", "else", "$defs",
	"unevaluatedItems", "unevaluatedProperties",
}

// inline turns a schema into plain JSON Schema with every $ref replaced by its target; a reference back to a
// schema being inlined becomes an empty schema.
func inline(ref *openapi3.SchemaRef, seen []string) (map[string]any, error) {
	if ref == nil || ref.Value == nil || (ref.Ref != "" && slices.Contains(seen, ref.Ref)) {
		return map[string]any{}, nil
	}
	if ref.Ref != "" {
		seen = append(seen, ref.Ref)
	}
	schema := ref.Value
	encoded, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	if err := json.Unmarshal(encoded, &out); err != nil {
		return nil, err
	}
	for key := range out {
		if strings.HasPrefix(key, "x-") || slices.Contains(droppedKeys, key) {
			delete(out, key)
		}
		if slices.Contains(unsupportedKeys, key) {
			return nil, fmt.Errorf("the keyword %s is not supported by the assistant tool generator", key)
		}
	}
	if err := inlineChildren(out, schema, seen); err != nil {
		return nil, err
	}
	return out, nil
}

func inlineChildren(out map[string]any, schema *openapi3.Schema, seen []string) error {
	for key, refs := range map[string]openapi3.SchemaRefs{
		"allOf": schema.AllOf, "anyOf": schema.AnyOf, "oneOf": schema.OneOf, "prefixItems": schema.PrefixItems,
	} {
		if len(refs) == 0 {
			continue
		}
		list := make([]any, len(refs))
		for i, child := range refs {
			inlined, err := inline(child, seen)
			if err != nil {
				return err
			}
			list[i] = inlined
		}
		out[key] = list
	}
	for key, child := range map[string]*openapi3.SchemaRef{
		"items": schema.Items, "not": schema.Not, "additionalProperties": schema.AdditionalProperties.Schema,
	} {
		if child == nil {
			continue
		}
		inlined, err := inline(child, seen)
		if err != nil {
			return err
		}
		out[key] = inlined
	}
	if len(schema.Properties) == 0 {
		return nil
	}
	properties := map[string]any{}
	for name, child := range schema.Properties {
		inlined, err := inline(child, seen)
		if err != nil {
			return fmt.Errorf("property %s: %w", name, err)
		}
		properties[name] = inlined
	}
	out["properties"] = properties
	return nil
}

// credentialNames are string properties that hold a credential whatever the spec says about them.
var credentialNames = []string{"password", "secret", "token", "apiKey", "credential"}

// reachesSecret reports whether a 2xx JSON response reaches a property marked x-vv-secret, one with format
// password, or a string property with a credential's name.
func reachesSecret(op *openapi3.Operation) (bool, error) {
	for status, response := range op.Responses.Map() {
		if !strings.HasPrefix(status, "2") || response.Value == nil {
			continue
		}
		media := response.Value.Content.Get("application/json")
		if media != nil && secretIn(media.Schema, "", nil) {
			return true, nil
		}
	}
	return false, nil
}

func secretIn(ref *openapi3.SchemaRef, name string, seen []string) bool {
	if ref == nil || ref.Value == nil || (ref.Ref != "" && slices.Contains(seen, ref.Ref)) {
		return false
	}
	if ref.Ref != "" {
		seen = append(seen, ref.Ref)
	}
	schema := ref.Value
	refMarked, _ := ref.Extensions[secretExtension].(bool)
	if marked, _ := schema.Extensions[secretExtension].(bool); marked || refMarked || schema.Format == "password" {
		return true
	}
	if schema.Type.Includes(openapi3.TypeString) && slices.Contains(credentialNames, name) {
		return true
	}
	children := slices.Concat(schema.AllOf, schema.AnyOf, schema.OneOf)
	if schema.Items != nil {
		children = append(children, schema.Items)
	}
	if schema.AdditionalProperties.Schema != nil {
		children = append(children, schema.AdditionalProperties.Schema)
	}
	for _, child := range children {
		if secretIn(child, name, seen) {
			return true
		}
	}
	for property, child := range schema.Properties {
		if secretIn(child, property, seen) {
			return true
		}
	}
	return false
}
