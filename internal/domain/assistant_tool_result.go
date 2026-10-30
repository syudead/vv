package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"unicode/utf8"
)

// Bounds of a read result as the model sees it: the context holds 16,384 tokens, and total fields keep the counts.
const (
	AssistantResultItemMax  = 50
	AssistantResultTextMax  = 500
	AssistantResultBytesMax = 24 * 1024
)

// TrimAssistantToolResult cuts arrays to 50 items and strings to 500 characters, marking each cut with the
// original length, and then the whole result to 24 KB. A body that is not JSON is cut as text.
func TrimAssistantToolResult(body []byte) string {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err == nil {
		if encoded, err := marshalResult(trimResultValue(value)); err == nil {
			body = encoded
		}
	}
	if len(body) <= AssistantResultBytesMax {
		return string(body)
	}
	cut := AssistantResultBytesMax
	for cut > 0 && !utf8.RuneStart(body[cut]) {
		cut--
	}
	return fmt.Sprintf("%s…[cut: %d bytes in all]", body[:cut], len(body))
}

func trimResultValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		for key, item := range v {
			v[key] = trimResultValue(item)
		}
		return v
	case []any:
		kept := v
		if len(v) > AssistantResultItemMax {
			kept = append(v[:AssistantResultItemMax:AssistantResultItemMax],
				fmt.Sprintf("[cut: %d items in all]", len(v)))
		}
		for i, item := range kept[:min(len(kept), AssistantResultItemMax)] {
			kept[i] = trimResultValue(item)
		}
		return kept
	case string:
		if utf8.RuneCountInString(v) <= AssistantResultTextMax {
			return v
		}
		runes := []rune(v)
		return fmt.Sprintf("%s…[cut: %d characters in all]", string(runes[:AssistantResultTextMax]), len(runes))
	}
	return value
}

// marshalResult keeps <, > and & as they are, so the model reads titles as written.
func marshalResult(value any) ([]byte, error) {
	var b bytes.Buffer
	encoder := json.NewEncoder(&b)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimRight(b.Bytes(), "\n"), nil
}
