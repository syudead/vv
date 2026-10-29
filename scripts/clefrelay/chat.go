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

// chatModels maps the names /api/tags lists to Workers AI chat models that accept a JSON schema, so the assistant's
// generation model can be chosen as on a local Ollama. A name starting with @cf/ is sent to Workers AI as it is.
var chatModels = map[string]string{
	"llama3.2:3b":          "@cf/meta/llama-3.2-3b-instruct",
	"granite4:micro":       "@cf/ibm-granite/granite-4.0-h-micro",
	"gemma4:26b":           "@cf/google/gemma-4-26b-a4b-it",
	"qwen3:30b":            "@cf/qwen/qwen3-30b-a3b-fp8",
	"mistral-small3.1:24b": "@cf/mistralai/mistral-small-3.1-24b-instruct",
	"llama3.3:70b":         "@cf/meta/llama-3.3-70b-instruct-fp8-fast",
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

func chatModel(name string) (string, bool) {
	name = strings.TrimSpace(name)
	if strings.HasPrefix(name, "@cf/") {
		return name, true
	}
	if id, ok := chatModels[name]; ok {
		return id, true
	}
	id, ok := chatModels[strings.TrimSuffix(name, ":latest")]
	return id, ok
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Request body sent by internal/ollama's generator; format is a JSON schema, the word "json" or absent.
type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []chatMessage   `json:"messages"`
	Format   json.RawMessage `json:"format"`
}

type responseFormat struct {
	Type       string          `json:"type"`
	JSONSchema json.RawMessage `json:"json_schema,omitempty"`
}

type workersChatRequest struct {
	Messages       []chatMessage   `json:"messages"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
	MaxTokens      int             `json:"max_tokens"`
}

// Workers AI chat models answer either response (a string, or the parsed object in JSON mode) or an OpenAI-style
// choices list.
type workersChatResponse struct {
	Success bool `json:"success"`
	Result  *struct {
		Response json.RawMessage `json:"response"`
		Choices  []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	} `json:"result"`
	Errors []workersError `json:"errors"`
}

func chatFormat(format json.RawMessage) *responseFormat {
	trimmed := bytes.TrimSpace(format)
	switch {
	case len(trimmed) == 0 || string(trimmed) == "null":
		return nil
	case trimmed[0] == '{':
		return &responseFormat{Type: "json_schema", JSONSchema: trimmed}
	default:
		return &responseFormat{Type: "json_object"}
	}
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
	body, err := json.Marshal(workersChatRequest{
		Messages: in.Messages, ResponseFormat: chatFormat(in.Format), MaxTokens: chatMaxTokens,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	url := fmt.Sprintf("%s/accounts/%s/ai/run/%s", strings.TrimRight(cfg.baseURL, "/"), cfg.account, model)
	status, raw, err := postWorkers(r, cfg, client, url, body)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	var out workersChatResponse
	decodeErr := json.Unmarshal(raw, &out)
	if status == http.StatusOK && decodeErr == nil && out.Success && out.Result != nil {
		content := chatContent(out)
		log.Printf("clefrelay: %s %d→%d tokens, %s, answer: %s", model, out.Result.Usage.PromptTokens,
			out.Result.Usage.CompletionTokens, time.Since(started).Round(time.Millisecond), content)
		writeJSON(w, http.StatusOK, map[string]any{
			"model": in.Model, "message": chatMessage{Role: "assistant", Content: content}, "done": true,
		})
		return
	}
	message := errorMessage(status, out.Errors)
	log.Printf("clefrelay: %s failed (%d): %s", model, status, message)
	writeError(w, errorStatus(status), message)
}

// chatContent takes the answer as text on one line, so the log holds one answer per line.
func chatContent(out workersChatResponse) string {
	var text string
	raw := bytes.TrimSpace(out.Result.Response)
	switch {
	case len(out.Result.Choices) > 0:
		text = out.Result.Choices[0].Message.Content
	case len(raw) > 0 && raw[0] == '"':
		if err := json.Unmarshal(raw, &text); err != nil {
			text = string(raw)
		}
	default:
		text = string(raw)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, []byte(strings.TrimSpace(text))); err == nil {
		return compact.String()
	}
	return strings.ReplaceAll(text, "\n", " ")
}
