package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"image/color"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/ollama"
)

// relayEndpoint serves clefrelay in front of a fake Workers AI handler and returns the relay's URL.
func relayEndpoint(t *testing.T, workers http.HandlerFunc) (string, *http.Client) {
	t.Helper()
	upstream := httptest.NewServer(workers)
	t.Cleanup(upstream.Close)
	relay := httptest.NewServer(newRelay(config{account: "acct", baseURL: upstream.URL}, upstream.Client()))
	t.Cleanup(relay.Close)
	return relay.URL, upstream.Client()
}

// chatThrough connects the product's tool-calling chat through clefrelay to a fake Workers AI handler.
func chatThrough(t *testing.T, model string, workers http.HandlerFunc) *ollama.Chat {
	t.Helper()
	endpoint, client := relayEndpoint(t, workers)
	return ollama.NewChatWithHTTPClient(fixedSettings{Endpoint: endpoint, GenerationModel: model}, client)
}

const textAnswer = `{"result":{"choices":[{"message":{"role":"assistant","content":"done"}}]},"success":true}`

func decodeWorkersChat(t *testing.T, r *http.Request) workersChatRequest {
	t.Helper()
	var got workersChatRequest
	if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestRelayChatSendsEveryToolAsAFunction(t *testing.T) {
	var got workersChatRequest
	chat := chatThrough(t, "llama3.3:70b", func(w http.ResponseWriter, r *http.Request) {
		got = decodeWorkersChat(t, r)
		_, _ = io.WriteString(w, textAnswer)
	})
	_, err := chat.Chat(context.Background(), domain.ChatRequest{
		Messages: []domain.ChatMessage{{Role: domain.ChatRoleUser, Content: "karaoke videos"}},
		Tools: []domain.ToolDefinition{
			{Name: "searchVideos", Description: "Search videos", Parameters: json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}}}`)},
			{Name: "listTags", Description: "List tags", Parameters: json.RawMessage(`{"type":"object"}`)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, raw := range got.Tools {
		var tool struct {
			Type     string `json:"type"`
			Function struct {
				Name       string          `json:"name"`
				Parameters json.RawMessage `json:"parameters"`
			} `json:"function"`
		}
		if err := json.Unmarshal(raw, &tool); err != nil {
			t.Fatal(err)
		}
		if tool.Type != "function" || len(tool.Function.Parameters) == 0 {
			t.Errorf("tool = %s", raw)
		}
		names = append(names, tool.Function.Name)
	}
	if strings.Join(names, ",") != "searchVideos,listTags" {
		t.Errorf("tools = %v", names)
	}
}

// OpenAI-style answers carry the arguments as a JSON string; Ollama's tool_calls carry an object.
func TestRelayChatAnswersAToolCallWithItsArgumentsAsAnObject(t *testing.T) {
	chat := chatThrough(t, "llama3.3:70b", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"result":{"choices":[{"message":{"role":"assistant","content":null,"tool_calls":[{"id":"x","type":"function","function":{"name":"searchVideos","arguments":"{\"q\": \"karaoke\"}"}}]}}],"response":null,"tool_calls":[{"name":"searchVideos","arguments":{"q":"karaoke"}}]},"success":true}`)
	})
	reply, err := chat.Chat(context.Background(), domain.ChatRequest{
		Messages: []domain.ChatMessage{{Role: domain.ChatRoleUser, Content: "karaoke videos"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(reply.ToolCalls) != 1 || reply.ToolCalls[0].Name != "searchVideos" ||
		string(reply.ToolCalls[0].Arguments) != `{"q":"karaoke"}` {
		t.Fatalf("reply = %+v", reply)
	}
}

// Workers AI matches tool results by id and Mistral accepts only nine letters and digits; Ollama matches by name.
func TestRelayChatPairsToolResultsWithTheirCalls(t *testing.T) {
	var got workersChatRequest
	chat := chatThrough(t, "llama3.3:70b", func(w http.ResponseWriter, r *http.Request) {
		got = decodeWorkersChat(t, r)
		_, _ = io.WriteString(w, textAnswer)
	})
	_, err := chat.Chat(context.Background(), domain.ChatRequest{Messages: []domain.ChatMessage{
		{Role: domain.ChatRoleUser, Content: "request"},
		{Role: domain.ChatRoleAssistant, ToolCalls: []domain.ToolCall{
			{Name: "getVideo", Arguments: json.RawMessage(`{"id":1}`)},
			{Name: "listTags", Arguments: json.RawMessage(`{}`)},
		}},
		{Role: domain.ChatRoleTool, ToolName: "listTags", Content: "tags"},
		{Role: domain.ChatRoleTool, ToolName: "getVideo", Content: "video"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	calls := got.Messages[1].ToolCalls
	if len(calls) != 2 || string(calls[0].Function.Arguments) != `"{\"id\":1}"` {
		t.Fatalf("calls = %+v", calls)
	}
	for _, call := range calls {
		if len(call.ID) != 9 || strings.Trim(call.ID, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789") != "" {
			t.Errorf("id %q is not nine letters and digits", call.ID)
		}
	}
	if got.Messages[2].ToolCallID != calls[1].ID || got.Messages[3].ToolCallID != calls[0].ID || calls[0].ID == calls[1].ID {
		t.Errorf("results answer %q and %q, calls are %q and %q",
			got.Messages[2].ToolCallID, got.Messages[3].ToolCallID, calls[0].ID, calls[1].ID)
	}
}

func TestRelayShowsToolsOnlyForFunctionCallingModels(t *testing.T) {
	endpoint, client := relayEndpoint(t, func(http.ResponseWriter, *http.Request) {
		t.Error("/api/show reached Workers AI")
	})
	classifier := ollama.NewWithHTTPClient(fixedSettings{Endpoint: endpoint}, client)
	for model, want := range map[string]domain.ModelCapabilities{
		"clef":         {},
		"llama3.3:70b": {Tools: true},
		"gemma4:26b":   {Tools: true, Vision: true},
	} {
		got, err := classifier.Capabilities(context.Background(), endpoint, model)
		if err != nil || got != want {
			t.Errorf("%s: capabilities = %+v, %v; want %+v", model, got, err, want)
		}
	}
	if _, err := classifier.Capabilities(context.Background(), endpoint, "llama3.2:3b"); err == nil {
		t.Error("a model without function calling is still mapped")
	}
}

func TestRelayChatSendsMessageImagesToAVisionModel(t *testing.T) {
	frame := solidJPEG(t, 32, 18, color.White)
	var got workersChatRequest
	chat := chatThrough(t, "mistral-small3.1:24b", func(w http.ResponseWriter, r *http.Request) {
		got = decodeWorkersChat(t, r)
		_, _ = io.WriteString(w, textAnswer)
	})
	_, err := chat.Chat(context.Background(), domain.ChatRequest{Messages: []domain.ChatMessage{
		{Role: domain.ChatRoleUser, Content: "request"},
		{Role: domain.ChatRoleAssistant, ToolCalls: []domain.ToolCall{{Name: "view_images"}}},
		{Role: domain.ChatRoleTool, ToolName: "view_images", Content: "attached"},
		{Role: domain.ChatRoleUser, Content: "Images from view_images", Images: [][]byte{frame}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	// Mistral rejects a user message right after a tool result, so an assistant message stands between them.
	if len(got.Messages) != 5 || got.Messages[3].Role != "assistant" {
		t.Fatalf("messages = %+v", got.Messages)
	}
	var parts []contentPart
	raw, _ := json.Marshal(got.Messages[4].Content)
	if err := json.Unmarshal(raw, &parts); err != nil {
		t.Fatal(err)
	}
	want := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(frame)
	if len(parts) != 2 || parts[0].Text != "Images from view_images" || parts[1].ImageURL == nil || parts[1].ImageURL.URL != want {
		t.Fatalf("content = %+v", parts)
	}
}
