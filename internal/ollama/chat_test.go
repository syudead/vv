package ollama

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/syudead/vv/internal/domain"
)

func generating(url string) fixedSettings {
	return fixedSettings{settings: domain.OllamaSettings{Endpoint: url, Model: "clef", GenerationModel: "qwen3:8b"}}
}

// chatServer records the request body and answers reply; /api/show reports a context of 40,960 tokens.
func chatServer(t *testing.T, status int, reply string, got *map[string]json.RawMessage) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/show" {
			_, _ = w.Write([]byte(`{"model_info":{"general.architecture":"qwen3","qwen3.context_length":40960}}`))
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/api/chat" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if got != nil {
			if err := json.NewDecoder(r.Body).Decode(got); err != nil {
				t.Error(err)
			}
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(server.Close)
	return server.URL
}

func TestChatSendsToolsAndReturnsToolCalls(t *testing.T) {
	var got map[string]json.RawMessage
	url := chatServer(t, http.StatusOK, `{"model":"qwen3:8b","message":{"role":"assistant","content":"",
		"thinking":"The owner wants tags.","tool_calls":[{"function":{"name":"listTags","arguments":{"q":"karaoke"}}},
		{"function":{"name":"open_page"}}]},"done":true}`, &got)

	reply, err := NewChat(generating(url)).Chat(context.Background(), domain.ChatRequest{
		Messages: []domain.ChatMessage{
			{Role: domain.ChatRoleSystem, Content: "You are the assistant."},
			{Role: domain.ChatRoleUser, Content: "カラオケ", Images: [][]byte{[]byte("jpeg")}},
			{Role: domain.ChatRoleAssistant, ToolCalls: []domain.ToolCall{{Name: "searchLibrary"}}},
			{Role: domain.ChatRoleTool, Content: `{"items":[]}`, ToolName: "searchLibrary"},
		},
		Tools: []domain.ToolDefinition{{
			Name: "listTags", Description: "List tags", Parameters: json.RawMessage(`{"type":"object"}`),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.ToolCall{
		{Name: "listTags", Arguments: json.RawMessage(`{"q":"karaoke"}`)},
		{Name: "open_page", Arguments: json.RawMessage(`{}`)},
	}
	if len(reply.ToolCalls) != len(want) || reply.Text != "" {
		t.Fatalf("reply = %+v", reply)
	}
	for i, call := range reply.ToolCalls {
		if call.Name != want[i].Name || string(call.Arguments) != string(want[i].Arguments) {
			t.Errorf("call %d = %s %s, want %s %s", i, call.Name, call.Arguments, want[i].Name, want[i].Arguments)
		}
	}

	wantBody := map[string]string{
		"model": `"qwen3:8b"`,
		"messages": `[{"role":"system","content":"You are the assistant."},` +
			`{"role":"user","content":"カラオケ","images":["anBlZw=="]},` +
			`{"role":"assistant","content":"","tool_calls":[{"function":{"name":"searchLibrary","arguments":{}}}]},` +
			`{"role":"tool","content":"{\"items\":[]}","tool_name":"searchLibrary"}]`,
		"tools":      `[{"type":"function","function":{"name":"listTags","description":"List tags","parameters":{"type":"object"}}}]`,
		"stream":     `false`,
		"keep_alive": `"10m"`,
		"options":    `{"num_ctx":40960}`,
	}
	for key, value := range wantBody {
		if string(got[key]) != value {
			t.Errorf("%s = %s, want %s", key, got[key], value)
		}
	}
	if _, ok := got["format"]; ok {
		t.Errorf("format = %s, want none", got["format"])
	}
}

func TestChatReturnsTextWithoutThinking(t *testing.T) {
	url := chatServer(t, http.StatusOK,
		`{"message":{"role":"assistant","content":" タグは 3 件です。 ","thinking":"count them"},"done":true}`, nil)
	reply, err := NewChat(generating(url)).Chat(context.Background(), domain.ChatRequest{
		Messages: []domain.ChatMessage{{Role: domain.ChatRoleUser, Content: "タグはいくつ？"}},
	})
	if err != nil || reply.Text != "タグは 3 件です。" || reply.ToolCalls != nil {
		t.Fatalf("reply = %+v, %v", reply, err)
	}
}

func TestChatReportsHowAnEmptyReplyEnded(t *testing.T) {
	url := chatServer(t, http.StatusOK, `{"message":{"role":"assistant","content":"","thinking":"let me look"},`+
		`"done":true,"done_reason":"length","prompt_eval_count":16000,"eval_count":384}`, nil)
	reply, err := NewChat(generating(url)).Chat(context.Background(), domain.ChatRequest{
		Messages: []domain.ChatMessage{{Role: domain.ChatRoleUser, Content: "何人？"}},
	})
	want := domain.ChatUsage{
		DoneReason: "length", PromptTokens: 16000, OutputTokens: 384, ThinkingChars: 11, ContextTokens: 40960,
	}
	if err != nil || reply.Text != "" || reply.Usage != want {
		t.Fatalf("reply = %+v, %v, want usage %+v", reply, err, want)
	}
}

func TestChatTellsFailuresApart(t *testing.T) {
	expired := func() (context.Context, context.CancelFunc) {
		return context.WithDeadline(context.Background(), time.Unix(0, 0))
	}
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	for _, tc := range []struct {
		name string
		url  string
		ctx  func() (context.Context, context.CancelFunc)
		want error
	}{
		{"error body", chatServer(t, http.StatusBadRequest, `{"error":"qwen2:0.5b does not support tools"}`, nil), nil, domain.ErrOllamaModel},
		{"not Ollama", chatServer(t, http.StatusOK, `<html>secret</html>`, nil), nil, domain.ErrNotOllama},
		{"no message", chatServer(t, http.StatusOK, `{"done":true}`, nil), nil, domain.ErrNotOllama},
		{"timeout", chatServer(t, http.StatusOK, `{}`, nil), expired, domain.ErrOllamaTimeout},
		{"unreachable", closed.URL, nil, domain.ErrOllamaUnreachable},
	} {
		ctx, cancel := context.Background(), func() {}
		if tc.ctx != nil {
			ctx, cancel = tc.ctx()
		}
		_, err := NewChat(generating(tc.url)).Chat(ctx, domain.ChatRequest{})
		cancel()
		for _, kind := range []error{domain.ErrOllamaModel, domain.ErrNotOllama, domain.ErrOllamaTimeout,
			domain.ErrOllamaUnreachable} {
			if want := errors.Is(tc.want, kind); errors.Is(err, kind) != want {
				t.Errorf("%s: err = %v, Is(%v) = %v", tc.name, err, kind, !want)
			}
		}
		if tc.name == "not Ollama" && err != nil && strings.Contains(err.Error(), "secret") {
			t.Errorf("not Ollama: the body leaked into %q", err)
		}
	}
}

func TestChatWithoutGenerationModelSendsNothing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s", r.URL.Path)
	}))
	defer server.Close()
	_, err := NewChat(at(server.URL)).Chat(context.Background(), domain.ChatRequest{})
	if !errors.Is(err, domain.ErrOllamaModel) {
		t.Fatalf("err = %v", err)
	}
}
