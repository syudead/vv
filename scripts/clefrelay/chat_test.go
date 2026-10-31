package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/ollama"
)

func TestRelayChatSendsTheModelPathWithoutKeepAlive(t *testing.T) {
	var got map[string]json.RawMessage
	var path string
	chat := chatThrough(t, "llama3.3:70b", func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		_, _ = io.WriteString(w, textAnswer)
	})
	reply, err := chat.Chat(context.Background(), domain.ChatRequest{
		Messages: []domain.ChatMessage{{Role: domain.ChatRoleUser, Content: "request"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if reply.Text != "done" {
		t.Errorf("reply = %+v", reply)
	}
	if path != "/accounts/acct/ai/run/@cf/meta/llama-3.3-70b-instruct-fp8-fast" {
		t.Errorf("path = %q", path)
	}
	if _, ok := got["keep_alive"]; ok {
		t.Error("keep_alive was relayed; Workers AI rejects it")
	}
	if string(got["max_tokens"]) != "4096" {
		t.Errorf("max_tokens = %s", got["max_tokens"])
	}
}

// Some models answer in response instead of choices, as text over several lines.
func TestRelayChatReadsAnAnswerInResponse(t *testing.T) {
	chat := chatThrough(t, "@cf/meta/llama-3.2-3b-instruct", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "{\"result\":{\"response\":\"カラオケの動画は\\n3本です。\"},\"success\":true}")
	})
	reply, err := chat.Chat(context.Background(), domain.ChatRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if reply.Text != "カラオケの動画は 3本です。" {
		t.Errorf("reply = %+v", reply)
	}
}

func TestRelayChatRejectsAModelThatIsNotAChatModel(t *testing.T) {
	chat := chatThrough(t, "clef", func(http.ResponseWriter, *http.Request) {
		t.Error("the judging model reached the chat endpoint")
	})
	_, err := chat.Chat(context.Background(), domain.ChatRequest{})
	if !errors.Is(err, domain.ErrOllamaModel) || !strings.Contains(err.Error(), "llama3.3:70b") {
		t.Fatalf("err = %v", err)
	}
}

func TestRelayListsTheChatModelsForTheConnectionCheck(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("the connection check reached Workers AI")
	}))
	t.Cleanup(upstream.Close)
	relay := httptest.NewServer(newRelay(config{account: "acct", baseURL: upstream.URL}, upstream.Client()))
	t.Cleanup(relay.Close)
	classifier := ollama.New(fixedSettings{Endpoint: relay.URL, Model: "clef", GenerationModel: "llama3.3:70b"})
	if err := classifier.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
}
