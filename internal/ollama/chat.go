package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/syudead/vv/internal/domain"
)

// chatContextTokens replaces Ollama's default context of a few thousand tokens, which silently drops the
// oldest messages and with them the tool results an answer cites; see docs/design-docs/ai-layer.md.
const chatContextTokens = 16384

// Chat calls tools through Ollama's chat API with the saved generation model; see
// docs/design-docs/ai-layer.md. Use NewChat.
type Chat struct {
	settings SettingsSource
	http     *http.Client
}

// NewChat waits as long as a classification, since a CPU host can take minutes to answer.
func NewChat(settings SettingsSource) *Chat {
	return &Chat{settings: settings, http: &http.Client{Timeout: requestTimeout}}
}

func NewChatWithHTTPClient(settings SettingsSource, client *http.Client) *Chat {
	return &Chat{settings: settings, http: client}
}

// chatMessage omits Ollama's thinking field, so the model's reasoning never leaves the adapter.
type chatMessage struct {
	Role      string         `json:"role"`
	Content   string         `json:"content"`
	Images    []string       `json:"images,omitempty"`
	ToolCalls []chatToolCall `json:"tool_calls,omitempty"`
	ToolName  string         `json:"tool_name,omitempty"`
}

type chatToolCall struct {
	Function chatFunction `json:"function"`
}

type chatFunction struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type chatTool struct {
	Type     string       `json:"type"`
	Function chatToolSpec `json:"function"`
}

type chatToolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type chatOptions struct {
	NumCtx int `json:"num_ctx"`
}

type chatRequest struct {
	Model     string        `json:"model"`
	Messages  []chatMessage `json:"messages"`
	Tools     []chatTool    `json:"tools,omitempty"`
	Stream    bool          `json:"stream"`
	KeepAlive string        `json:"keep_alive"`
	Options   *chatOptions  `json:"options,omitempty"`
}

type chatResponse struct {
	Message *chatMessage `json:"message"`
	Error   string       `json:"error"`
}

// Chat reads the saved endpoint and generation model on every call. A reply with tool calls is returned
// as calls even when it also has text, since the loop runs the calls before the model answers.
func (c *Chat) Chat(ctx context.Context, req domain.ChatRequest) (domain.ChatReply, error) {
	settings, err := c.settings.OllamaSettings(ctx)
	if err != nil {
		return domain.ChatReply{}, fmt.Errorf("cannot read the Ollama settings: %w", err)
	}
	if settings.GenerationModel == "" {
		return domain.ChatReply{}, kindErrorf(domain.ErrOllamaModel, nil, "no generation model is saved")
	}
	messages, err := wireMessages(req.Messages)
	if err != nil {
		return domain.ChatReply{}, err
	}
	message, err := postChat(ctx, c.http, settings, chatRequest{
		Model:     settings.GenerationModel,
		Messages:  messages,
		Tools:     wireTools(req.Tools),
		Stream:    false,
		KeepAlive: keepAlive,
		Options:   &chatOptions{NumCtx: chatContextTokens},
	})
	if err != nil {
		return domain.ChatReply{}, err
	}
	if len(message.ToolCalls) == 0 {
		return domain.ChatReply{Text: strings.TrimSpace(message.Content)}, nil
	}
	calls := make([]domain.ToolCall, len(message.ToolCalls))
	for i, call := range message.ToolCalls {
		calls[i] = domain.ToolCall{Name: call.Function.Name, Arguments: objectOrEmpty(call.Function.Arguments)}
	}
	return domain.ChatReply{ToolCalls: calls}, nil
}

// postChat keeps only Ollama's own error text, and names the endpoint when the reply is not Ollama's.
func postChat(ctx context.Context, client *http.Client, settings domain.OllamaSettings, body chatRequest) (*chatMessage, error) {
	status, raw, err := postJSON(ctx, client, settings.Endpoint, "/api/chat", body)
	if err != nil {
		return nil, err
	}
	var decoded chatResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, notOllama(settings.Endpoint, status)
	}
	if decoded.Error != "" {
		return nil, kindErrorf(domain.ErrOllamaModel, nil, "model %q: %s", settings.GenerationModel,
			statusMessage(status, decoded.Error))
	}
	if status != http.StatusOK || decoded.Message == nil {
		return nil, notOllama(settings.Endpoint, status)
	}
	return decoded.Message, nil
}

func notOllama(endpoint string, status int) error {
	return kindErrorf(domain.ErrNotOllama, nil, "the server at %s is not Ollama (%s)", endpoint, statusMessage(status, ""))
}

// wireMessages tiles a message's images into one when there are more than Ollama takes, as for Clef.
func wireMessages(messages []domain.ChatMessage) ([]chatMessage, error) {
	out := make([]chatMessage, len(messages))
	for i, m := range messages {
		images, err := encodeImages(m.Images, maxImages)
		if err != nil {
			return nil, fmt.Errorf("cannot tile the images of message %d: %w", i, err)
		}
		out[i] = chatMessage{Role: string(m.Role), Content: m.Content, Images: images, ToolName: m.ToolName}
		for _, call := range m.ToolCalls {
			out[i].ToolCalls = append(out[i].ToolCalls, chatToolCall{
				Function: chatFunction{Name: call.Name, Arguments: objectOrEmpty(call.Arguments)},
			})
		}
	}
	return out, nil
}

func wireTools(tools []domain.ToolDefinition) []chatTool {
	out := make([]chatTool, len(tools))
	for i, tool := range tools {
		out[i] = chatTool{Type: "function", Function: chatToolSpec{
			Name: tool.Name, Description: tool.Description, Parameters: objectOrEmpty(tool.Parameters),
		}}
	}
	return out
}

// objectOrEmpty stands in {} for missing arguments, because Ollama and the loop both expect an object.
func objectOrEmpty(raw json.RawMessage) json.RawMessage {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return json.RawMessage(`{}`)
	}
	return trimmed
}
