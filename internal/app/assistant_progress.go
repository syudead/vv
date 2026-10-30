package app

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/syudead/vv/internal/domain"
)

// progressLabelLimit bounds the ids one progress item names; the panel shows three and a count.
const progressLabelLimit = 20

// progressLabelKinds are the id kinds the vocabulary names.
var progressLabelKinds = map[string]domain.AssistantLabelKind{
	"video": domain.AssistantLabelVideo, "tag": domain.AssistantLabelTag, "tagKind": domain.AssistantLabelTagKind,
	"person": domain.AssistantLabelPerson, "organization": domain.AssistantLabelOrganization,
	"playlist": domain.AssistantLabelPlaylist, "group": domain.AssistantLabelGroup,
	"smartFolder": domain.AssistantLabelSmartFolder,
}

// labels names the entities a call's id arguments point at, as the registry types them; ids that no longer exist
// and kinds the vocabulary does not name are left out, and a lookup failure only drops the labels.
func (l *turnLoop) labels(ctx context.Context, tool domain.AssistantTool, raw json.RawMessage) []domain.AssistantLabel {
	var args map[string]any
	if json.Unmarshal(raw, &args) != nil {
		return nil
	}
	var out []domain.AssistantLabel
	for _, field := range tool.IDs {
		kind, ok := progressLabelKinds[field.Kind]
		if !ok {
			continue
		}
		ids := argumentIDs(args, field)
		if len(ids) == 0 || len(out) >= progressLabelLimit {
			continue
		}
		ids = ids[:min(len(ids), progressLabelLimit-len(out))]
		names, err := l.a.vocabulary.EntityNames(ctx, kind, ids)
		if err != nil {
			continue
		}
		for _, id := range ids {
			if name, ok := names[id]; ok {
				out = append(out, domain.AssistantLabel{Kind: kind, ID: id, Name: name})
			}
		}
	}
	return out
}

// argumentIDs reads the ids of one field: a path parameter by name, or a JSON pointer into the body where "*"
// stands for every array item and a pointer that ends at an array takes each item.
func argumentIDs(args map[string]any, field domain.AssistantIDField) []int64 {
	if field.In == "path" {
		return appendIDs(nil, args[field.Field])
	}
	values := []any{args["body"]}
	for _, segment := range strings.Split(strings.TrimPrefix(field.Field, "/"), "/") {
		var next []any
		for _, value := range values {
			switch v := value.(type) {
			case map[string]any:
				if item, ok := v[segment]; ok {
					next = append(next, item)
				}
			case []any:
				if segment == "*" {
					next = append(next, v...)
				}
			}
		}
		values = next
	}
	var ids []int64
	for _, value := range values {
		ids = appendIDs(ids, value)
	}
	return ids
}

func appendIDs(ids []int64, value any) []int64 {
	switch v := value.(type) {
	case float64:
		if v > 0 && v == float64(int64(v)) {
			ids = append(ids, int64(v))
		}
	case []any:
		for _, item := range v {
			ids = appendIDs(ids, item)
		}
	}
	return ids
}
