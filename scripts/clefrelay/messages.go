package main

import (
	"encoding/json"
	"fmt"
)

// workersChatMessage is an OpenAI-style chat message, the form Workers AI function calling and vision models take.
type workersChatMessage struct {
	Role       string            `json:"role"`
	Content    any               `json:"content"`
	ToolCalls  []workersToolCall `json:"tool_calls,omitempty"`
	ToolCallID string            `json:"tool_call_id,omitempty"`
	Name       string            `json:"name,omitempty"`
}

type workersToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name string `json:"name"`
		// Arguments is a JSON-encoded object in a request, and a string or an object in an answer.
		Arguments json.RawMessage `json:"arguments"`
	} `json:"function"`
}

type contentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

type imageURL struct {
	URL string `json:"url"`
}

// workersMessages gives each Ollama tool call an id and each tool result the id of the earliest unanswered call to
// its tool, because Ollama matches results by tool name and order while Workers AI matches them by id. Ids are nine
// letters and digits, the only form Mistral models accept.
func workersMessages(in []ollamaChatMessage, userAfterTool bool) ([]workersChatMessage, error) {
	out := make([]workersChatMessage, len(in))
	var pending []workersToolCall
	for i, m := range in {
		content, err := messageContent(m)
		if err != nil {
			return nil, fmt.Errorf("message %d: %w", i, err)
		}
		out[i] = workersChatMessage{Role: m.Role, Content: content}
		if len(m.ToolCalls) > 0 {
			pending = nil
			for j, call := range m.ToolCalls {
				wire, err := workersCall(fmt.Sprintf("c%04d%04d", i%10000, j%10000), call)
				if err != nil {
					return nil, fmt.Errorf("message %d: %w", i, err)
				}
				out[i].ToolCalls = append(out[i].ToolCalls, wire)
				pending = append(pending, wire)
			}
		}
		if m.Role == "tool" {
			out[i].Name = m.ToolName
			out[i].ToolCallID, pending = answerCall(pending, m.ToolName, i)
		}
	}
	if !userAfterTool {
		return bridgeToolResults(out), nil
	}
	return out, nil
}

// toolResultsRead stands between tool results and a following user message for a model that rejects that order;
// an empty assistant message is rejected too.
const toolResultsRead = "I have read the tool results."

func bridgeToolResults(messages []workersChatMessage) []workersChatMessage {
	out := make([]workersChatMessage, 0, len(messages))
	for i, m := range messages {
		if i > 0 && m.Role == "user" && messages[i-1].Role == "tool" {
			out = append(out, workersChatMessage{Role: "assistant", Content: toolResultsRead})
		}
		out = append(out, m)
	}
	return out
}

func workersCall(id string, call ollamaToolCall) (workersToolCall, error) {
	wire := workersToolCall{ID: id, Type: "function"}
	wire.Function.Name = call.Function.Name
	encoded, err := json.Marshal(string(objectOrEmpty(call.Function.Arguments)))
	if err != nil {
		return workersToolCall{}, err
	}
	wire.Function.Arguments = encoded
	return wire, nil
}

// answerCall falls back to the earliest unanswered call, and to a fresh id when no call is waiting.
func answerCall(pending []workersToolCall, name string, index int) (string, []workersToolCall) {
	if len(pending) == 0 {
		return fmt.Sprintf("r%08d", index%100000000), pending
	}
	at := 0
	for j, call := range pending {
		if call.Function.Name == name {
			at = j
			break
		}
	}
	id := pending[at].ID
	return id, append(pending[:at:at], pending[at+1:]...)
}

// messageContent sends images as data URLs beside the text, tiling more than four as Clef requests do.
func messageContent(m ollamaChatMessage) (any, error) {
	if len(m.Images) == 0 {
		return m.Content, nil
	}
	images, err := workersImages(m.Images)
	if err != nil {
		return nil, err
	}
	parts := []contentPart{{Type: "text", Text: m.Content}}
	for _, image := range images {
		parts = append(parts, contentPart{
			Type:     "image_url",
			ImageURL: &imageURL{URL: "data:" + image.ContentType + ";base64," + image.Base64},
		})
	}
	return parts, nil
}

// objectOrEmpty sends {} for arguments that are not an object, such as the string a model wrote when it put several
// calls in one, because Workers AI refuses the whole conversation over one such call.
func objectOrEmpty(raw json.RawMessage) json.RawMessage {
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil || object == nil {
		return json.RawMessage(`{}`)
	}
	return raw
}
