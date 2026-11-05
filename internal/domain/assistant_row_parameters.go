package domain

import (
	"encoding/json"
)

// assistantNameItem is a name the server resolves; an object also carries an id from a read result, which is a pin
// that wins only among the entities the name finds.
var assistantNameItem = map[string]any{
	"anyOf": []any{
		map[string]any{"type": "string"},
		map[string]any{
			"type": "object", "required": []string{"name"},
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
				"id":   map[string]any{"type": "integer", "description": "The id a read result gave for this name."},
			},
		},
	},
}

func nameSchema(description string) map[string]any {
	schema := map[string]any{"description": description}
	for key, value := range assistantNameItem {
		schema[key] = value
	}
	return schema
}

func namesSchema(description string) map[string]any {
	return map[string]any{"type": "array", "items": assistantNameItem, "description": description}
}

func typed(kind, description string) map[string]any {
	return map[string]any{"type": kind, "description": description}
}

// assistantArgumentSchemas hold one shared schema and description per argument of the name-based rows.
var assistantArgumentSchemas = map[AssistantArgument]map[string]any{
	AssistantArgName: nameSchema("The name the operation creates, or the playlist or group it adds to, as the " +
		"owner wrote it."),
	AssistantArgNewName: typed("string", "The new name."),
	AssistantArgNames: namesSchema("The entities merged into the target, or the other person of a face " +
		"candidate."),
	AssistantArgTagNames: namesSchema("Tag names. A missing tag to attach is created; one created by an " +
		"earlier step of this request is that step's tag."),
	AssistantArgTagKindName:       nameSchema("A tag kind's name."),
	AssistantArgPersonNames:       namesSchema("People's names."),
	AssistantArgOrganizationNames: namesSchema("Organizations' names."),
	AssistantArgRole: {"type": "string", "description": "The credit role of the people.",
		"enum": []string{"performer", "director", "photographer", "streamer"}},
	AssistantArgSynonym:    typed("string", "A tag synonym."),
	AssistantArgAliases:    {"type": "array", "items": map[string]any{"type": "string"}, "description": "Aliases to add."},
	AssistantArgCode:       typed("string", "A product code."),
	AssistantArgURL:        typed("string", "A URL."),
	AssistantArgDateKind:   {"type": "string", "enum": []string{"release", "recorded", "published"}},
	AssistantArgDate:       typed("string", "YYYY, YYYY-MM or YYYY-MM-DD."),
	AssistantArgEnabled:    typed("boolean", "Turn on (true) or off (false)."),
	AssistantArgNumber:     typed("number", "A number: a position in milliseconds, a probability or an order."),
	AssistantArgPublic:     typed("boolean", "Public (true) or private (false)."),
	AssistantArgModel:      typed("string", "A model name."),
	AssistantArgEncoder:    typed("string", "A video encoder."),
	AssistantArgProvider:   typed("string", "A provider."),
	AssistantArgSort:       typed("string", "A library sort, such as addedDesc or durationDesc."),
	AssistantArgParentName: nameSchema("The parent organization or smart folder."),
	AssistantArgScope:      typed("string", "Which items to run on: missing (the default) or all."),
	AssistantArgFetchID:    typed("string", "The fetchId a fetch tool answered in this request."),
	AssistantArgFields: {"type": "array", "items": map[string]any{"type": "string"},
		"description": "The fetched fields to apply, such as title, tags, code, description, dates, cover, " +
			"people, organizations, urls and externalIds for a video, or image, profile, aliases, " +
			"externalIds and links for a person."},
	AssistantArgAtMs:        typed("integer", "The frame position in milliseconds; leave it out to edit the current thumbnail."),
	AssistantArgInstruction: typed("string", "What to change in the image, in the owner's words."),
}

// assistantFilterSchema is the library filter by names that a target narrows videos by.
var assistantFilterSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"q":             typed("string", "Free text over titles and work information."),
		"favorite":      typed("boolean", "Only favorites."),
		"tags":          map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Tag names."},
		"tagKind":       typed("string", "A tag kind's name."),
		"person":        typed("string", "A person's name."),
		"organization":  typed("string", "An organization's name."),
		"minDurationMs": typed("integer", "Shortest duration in milliseconds."),
		"maxDurationMs": typed("integer", "Longest duration in milliseconds."),
		"addedFrom":     typed("string", "First day added, YYYY-MM-DD."),
		"addedTo":       typed("string", "Last day added, YYYY-MM-DD."),
		"watchState":    map[string]any{"type": "string", "enum": []string{"unwatched", "inProgress", "watched"}},
	},
}

var assistantTargetSchema = map[string]any{
	"type": "object", "required": []string{"kind"},
	"description": "What the operation acts on: selection (the selected videos, or the open video or entity), " +
		"current (what the screen shows), named (names, or videos by title) or filter (the videos matching " +
		"filter). A filter on selection or current narrows them.",
	"properties": map[string]any{
		"kind":   map[string]any{"type": "string", "enum": []string{"selection", "current", "named", "filter"}},
		"names":  namesSchema("For named: the names or video titles."),
		"filter": assistantFilterSchema,
	},
}

// AssistantRowParameters is the JSON Schema of a name-based row's tool arguments: its target when it has one, and
// its arguments. A schema row takes its operation's own schema instead.
func AssistantRowParameters(row AssistantAction) json.RawMessage {
	properties := map[string]any{}
	if row.Target != AssistantTargetNone {
		properties["target"] = assistantTargetSchema
	}
	for _, arg := range row.Arguments {
		if schema, ok := assistantArgumentSchemas[arg]; ok {
			properties[string(arg)] = schema
		}
	}
	encoded, _ := json.Marshal(map[string]any{"type": "object", "properties": properties}) // Plain maps always encode.
	return encoded
}
