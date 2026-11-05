package domain

import (
	"bytes"
	"encoding/json"
)

// AssistantToolResultText is a JSON result as the model reads it, re-encoded so <, > and & stay as written; a body
// that is not JSON is kept as text. Nothing is cut here: the turn fits its prompt to the model's context.
func AssistantToolResultText(body []byte) string {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return string(body)
	}
	var b bytes.Buffer
	encoder := json.NewEncoder(&b)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return string(body)
	}
	return string(bytes.TrimRight(b.Bytes(), "\n"))
}
