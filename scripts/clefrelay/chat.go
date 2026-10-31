package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"slices"
	"strings"
	"time"
)

type chatModelInfo struct {
	id string
	// tools and vision repeat the function calling and vision properties of the model's Workers AI catalog entry.
	tools  bool
	vision bool
	// userAfterTool is false for a model that rejects a user message right after a tool result, as Mistral does
	// for the images view_images attaches.
	userAfterTool bool
}

// chatModels maps the names /api/tags lists to Workers AI chat models with function calling, so the assistant's
// generation model can be chosen as on a local Ollama. A name starting with @cf/ is sent to Workers AI as it is.
var chatModels = map[string]chatModelInfo{
	"granite4:micro":       {id: "@cf/ibm-granite/granite-4.0-h-micro", tools: true, userAfterTool: true},
	"gemma4:26b":           {id: "@cf/google/gemma-4-26b-a4b-it", tools: true, vision: true, userAfterTool: true},
	"qwen3:30b":            {id: "@cf/qwen/qwen3-30b-a3b-fp8", tools: true, userAfterTool: true},
	"mistral-small3.1:24b": {id: "@cf/mistralai/mistral-small-3.1-24b-instruct", tools: true, vision: true},
	"llama3.3:70b":         {id: "@cf/meta/llama-3.3-70b-instruct-fp8-fast", tools: true, userAfterTool: true},
	"llama4:scout":         {id: "@cf/meta/llama-4-scout-17b-16e-instruct", tools: true, vision: true, userAfterTool: true},
}

// Workers AI answers at most 256 tokens unless told otherwise, which cuts a thinking model's answer off.
const chatMaxTokens = 4096

func chatModelNames() []string {
	names := make([]string, 0, len(chatModels))
	for name := range chatModels {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// chatModel resolves a listed name; a raw @cf/ name is relayed with no known capabilities.
func chatModel(name string) (chatModelInfo, bool) {
	name = strings.TrimSpace(name)
	if strings.HasPrefix(name, "@cf/") {
		return chatModelInfo{id: name, userAfterTool: true}, true
	}
	if info, ok := chatModels[name]; ok {
		return info, true
	}
	info, ok := chatModels[strings.TrimSuffix(name, ":latest")]
	return info, ok
}

// showCapabilities answers /api/show as Ollama does, so the product reads tools and vision from the relay.
func showCapabilities(name string) ([]string, bool) {
	trimmed := strings.TrimSuffix(strings.TrimSpace(name), ":latest")
	if slices.Contains(models, trimmed) {
		return []string{"completion"}, true
	}
	info, ok := chatModel(name)
	if !ok {
		return nil, false
	}
	capabilities := []string{"completion"}
	if info.tools {
		capabilities = append(capabilities, "tools")
	}
	if info.vision {
		capabilities = append(capabilities, "vision")
	}
	return capabilities, true
}

type ollamaToolCall struct {
	Function struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	} `json:"function"`
}

type ollamaChatMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	Images    []string         `json:"images,omitempty"`
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
	ToolName  string           `json:"tool_name,omitempty"`
}

// Request body sent by internal/ollama.
type ollamaChatRequest struct {
	Model    string              `json:"model"`
	Messages []ollamaChatMessage `json:"messages"`
	Tools    []json.RawMessage   `json:"tools"`
}

type workersChatRequest struct {
	Messages  []workersChatMessage `json:"messages"`
	Tools     []json.RawMessage    `json:"tools,omitempty"`
	MaxTokens int                  `json:"max_tokens"`
}

// Workers AI chat models answer either response (a string, or a parsed object) or an OpenAI-style
// choices list; tool calls come in the choice, or at the top level with the arguments as an object.
type workersChatResponse struct {
	Success bool `json:"success"`
	Result  *struct {
		Response json.RawMessage `json:"response"`
		Choices  []struct {
			Message struct {
				Content          *string           `json:"content"`
				ReasoningContent string            `json:"reasoning_content"`
				ToolCalls        []workersToolCall `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		ToolCalls []struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		} `json:"tool_calls"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	} `json:"result"`
	Errors []workersError `json:"errors"`
}

func relayChat(w http.ResponseWriter, r *http.Request, cfg config, client *http.Client) {
	started := time.Now()
	var in ollamaChatRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBodyBytes)).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "cannot read the request: "+err.Error())
		return
	}
	model, ok := chatModel(in.Model)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("model %q is not a chat model on Workers AI; use one of %s",
			in.Model, strings.Join(chatModelNames(), ", ")))
		return
	}
	body, err := workersChatBody(in, model)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	url := fmt.Sprintf("%s/accounts/%s/ai/run/%s", strings.TrimRight(cfg.baseURL, "/"), cfg.account, model.id)
	status, raw, err := postWorkers(r, cfg, client, url, body)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	var out workersChatResponse
	decodeErr := json.Unmarshal(raw, &out)
	if status == http.StatusOK && decodeErr == nil && out.Success && out.Result != nil {
		message := chatAnswer(out)
		log.Printf("clefrelay: %s %d→%d tokens, %s, answer: %s", model.id, out.Result.Usage.PromptTokens,
			out.Result.Usage.CompletionTokens, time.Since(started).Round(time.Millisecond), logLine(message))
		writeJSON(w, http.StatusOK, map[string]any{"model": in.Model, "message": message, "done": true})
		return
	}
	message := errorMessage(status, out.Errors)
	log.Printf("clefrelay: %s failed (%d): %s", model.id, status, message)
	writeError(w, errorStatus(status), message)
}

// workersChatBody refuses tools and images a mapped model does not support, with Ollama's wording for tools.
func workersChatBody(in ollamaChatRequest, model chatModelInfo) ([]byte, error) {
	known := !strings.HasPrefix(strings.TrimSpace(in.Model), "@cf/")
	if known && len(in.Tools) > 0 && !model.tools {
		return nil, fmt.Errorf("%s does not support tools", in.Model)
	}
	messages, err := workersMessages(in.Messages, model.userAfterTool)
	if err != nil {
		return nil, err
	}
	if known && !model.vision && slices.ContainsFunc(in.Messages, func(m ollamaChatMessage) bool { return len(m.Images) > 0 }) {
		return nil, fmt.Errorf("%s does not support images; use one of the vision models %s", in.Model,
			strings.Join(visionModelNames(), ", "))
	}
	return json.Marshal(workersChatRequest{
		Messages: messages, Tools: in.Tools, MaxTokens: chatMaxTokens,
	})
}

func visionModelNames() []string {
	return slices.DeleteFunc(chatModelNames(), func(name string) bool { return !chatModels[name].vision })
}

// chatAnswer prefers tool calls over text, since the loop runs the calls before the model answers. An empty answer
// takes the Gemma calls that Workers AI left in the reasoning, when that is all the reasoning holds.
func chatAnswer(out workersChatResponse) ollamaChatMessage {
	message := ollamaChatMessage{Role: "assistant"}
	if len(out.Result.Choices) > 0 {
		for _, call := range out.Result.Choices[0].Message.ToolCalls {
			message.ToolCalls = append(message.ToolCalls, ollamaCalls(call.Function.Name, call.Function.Arguments)...)
		}
	}
	if len(message.ToolCalls) == 0 {
		for _, call := range out.Result.ToolCalls {
			message.ToolCalls = append(message.ToolCalls, ollamaCalls(call.Name, call.Arguments)...)
		}
	}
	if len(message.ToolCalls) == 0 {
		message.Content = chatContent(out)
	}
	if len(message.ToolCalls) == 0 && message.Content == "" && len(out.Result.Choices) > 0 {
		message.ToolCalls = gemmaCalls(out.Result.Choices[0].Message.ReasoningContent)
	}
	return message
}

// ollamaCalls splits arguments that hold several objects one after another into one call each, the form Gemma
// writes for parallel calls of one tool.
func ollamaCalls(name string, arguments json.RawMessage) []ollamaToolCall {
	object := argumentObject(arguments)
	parts := []json.RawMessage{object}
	var text string
	if json.Unmarshal(object, &text) == nil {
		if split := concatenatedObjects(text); len(split) > 1 {
			parts = split
		}
	}
	calls := make([]ollamaToolCall, len(parts))
	for i, part := range parts {
		calls[i].Function.Name = name
		calls[i].Function.Arguments = part
	}
	return calls
}

// concatenatedObjects answers the objects text holds one after another, or nil when it holds anything else.
func concatenatedObjects(text string) []json.RawMessage {
	decoder := json.NewDecoder(strings.NewReader(text))
	var objects []json.RawMessage
	for decoder.More() {
		var object map[string]json.RawMessage
		if decoder.Decode(&object) != nil || object == nil {
			return nil
		}
		encoded, _ := json.Marshal(object) // Decoded JSON always encodes.
		objects = append(objects, encoded)
	}
	return objects
}

// argumentObject turns OpenAI's JSON-encoded argument string, which some models encode twice, into the object
// Ollama returns. Anything that is not an object is passed on as a string for the loop to reject.
func argumentObject(raw json.RawMessage) json.RawMessage {
	value := bytes.TrimSpace(raw)
	for range 2 {
		if len(value) == 0 || value[0] != '"' {
			break
		}
		var text string
		if err := json.Unmarshal(value, &text); err != nil {
			break
		}
		value = bytes.TrimSpace([]byte(text))
	}
	if len(value) == 0 || string(value) == "null" {
		return json.RawMessage(`{}`)
	}
	var compact bytes.Buffer
	if json.Compact(&compact, value) == nil && compact.Bytes()[0] == '{' {
		return compact.Bytes()
	}
	quoted, _ := json.Marshal(string(value))
	return quoted
}

// chatContent takes the answer as text on one line, so the log holds one answer per line.
func chatContent(out workersChatResponse) string {
	var text string
	raw := bytes.TrimSpace(out.Result.Response)
	switch {
	case len(out.Result.Choices) > 0 && out.Result.Choices[0].Message.Content != nil:
		text = *out.Result.Choices[0].Message.Content
	case len(raw) > 0 && raw[0] == '"':
		if err := json.Unmarshal(raw, &text); err != nil {
			text = string(raw)
		}
	case len(raw) > 0 && string(raw) != "null":
		text = string(raw)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, []byte(strings.TrimSpace(text))); err == nil {
		return compact.String()
	}
	return strings.ReplaceAll(strings.TrimSpace(text), "\n", " ")
}

func logLine(message ollamaChatMessage) string {
	if len(message.ToolCalls) == 0 {
		return message.Content
	}
	calls := make([]string, len(message.ToolCalls))
	for i, call := range message.ToolCalls {
		calls[i] = call.Function.Name + string(call.Function.Arguments)
	}
	return "tool calls " + strings.Join(calls, ", ")
}
