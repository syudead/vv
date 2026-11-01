package domain

import (
	"bytes"
	"encoding/json"
)

// assistantCompactFields are the item fields a list read keeps for the model. A whole Video carries 44 fields of
// URLs, codecs and states; a page of them filled a small model's context, and the model read the same page again.
var assistantCompactFields = map[string][]string{
	"listVideos":        {"id", "title", "durationMs", "addedAt", "favorite", "public", "virtual", "tags", "cardCredits"},
	"listTags":          {"id", "name", "synonyms", "videoCount", "tentative", "kind"},
	"listPeople":        {"id", "name", "aliases", "videoCount", "favorite", "provisional"},
	"listOrganizations": {"id", "name", "aliases", "videoCount", "parent"},
}

// AssistantCompactResult keeps the fields of each item a list read answers that the model works with, and writes a
// nested object that has a name, such as a tag or a credited person, as that name. Other bodies are answered as they
// are.
func AssistantCompactResult(operation string, body []byte) []byte {
	fields, ok := assistantCompactFields[operation]
	if !ok {
		return body
	}
	var page map[string]json.RawMessage
	var items []map[string]json.RawMessage
	if json.Unmarshal(body, &page) != nil || json.Unmarshal(page["items"], &items) != nil {
		return body
	}
	compact := make([]map[string]any, len(items))
	for i, item := range items {
		compact[i] = map[string]any{}
		for _, field := range fields {
			if value, ok := item[field]; ok && string(value) != "null" {
				compact[i][field] = namesOnly(decodeNumbers(value))
			}
		}
	}
	page["items"], _ = json.Marshal(compact) // Decoded JSON values always encode.
	out, _ := json.Marshal(page)
	return out
}

func decodeNumbers(raw json.RawMessage) any {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	_ = decoder.Decode(&value) // raw is a value of an object that already decoded.
	return value
}

// namesOnly writes each object with a string name as its name, in lists and nested objects alike.
func namesOnly(value any) any {
	switch v := value.(type) {
	case map[string]any:
		if name, ok := v["name"].(string); ok {
			return name
		}
		for key, nested := range v {
			v[key] = namesOnly(nested)
		}
	case []any:
		for i, nested := range v {
			v[i] = namesOnly(nested)
		}
	}
	return value
}
