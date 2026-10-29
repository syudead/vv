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

// generatorThrough connects the product generator through clefrelay to a fake Workers AI handler.
func generatorThrough(t *testing.T, model string, workers http.HandlerFunc) *ollama.Generator {
	t.Helper()
	upstream := httptest.NewServer(workers)
	t.Cleanup(upstream.Close)
	relay := httptest.NewServer(newRelay(config{account: "acct", baseURL: upstream.URL}, upstream.Client()))
	t.Cleanup(relay.Close)
	return ollama.NewGeneratorWithHTTPClient(
		fixedSettings{Endpoint: relay.URL, Model: "clef", GenerationModel: model}, upstream.Client(),
	)
}

func TestRelayChatSendsTheFormatAsAJSONSchemaAndAnswersTheChoice(t *testing.T) {
	var got map[string]json.RawMessage
	var path string
	gen := generatorThrough(t, "llama3.2:3b", func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		_, _ = io.WriteString(w, `{"result":{"choices":[{"message":{"role":"assistant","content":"{\"kind\": \"unsupported\"}"}}],"usage":{"prompt_tokens":10,"completion_tokens":5}},"success":true,"errors":[]}`)
	})
	schema := json.RawMessage(`{"type":"object","properties":{"kind":{"type":"string"}}}`)
	answer, err := gen.Generate(context.Background(), domain.GenerationRequest{Prompt: "request", Schema: schema})
	if err != nil {
		t.Fatal(err)
	}
	if string(answer) != `{"kind":"unsupported"}` {
		t.Errorf("answer = %s", answer)
	}
	if path != "/accounts/acct/ai/run/@cf/meta/llama-3.2-3b-instruct" {
		t.Errorf("path = %q", path)
	}
	var format responseFormat
	if err := json.Unmarshal(got["response_format"], &format); err != nil {
		t.Fatal(err)
	}
	if format.Type != "json_schema" || string(format.JSONSchema) != string(schema) {
		t.Errorf("response_format = %+v", format)
	}
	if _, ok := got["keep_alive"]; ok {
		t.Error("keep_alive was relayed; Workers AI rejects it")
	}
	if string(got["max_tokens"]) != "4096" {
		t.Errorf("max_tokens = %s", got["max_tokens"])
	}
}

// Some models answer the parsed object, formatted over several lines, in response instead of choices.
func TestRelayChatCompactsAParsedResponseObject(t *testing.T) {
	gen := generatorThrough(t, "@cf/meta/llama-3.2-3b-instruct", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "{\"result\":{\"response\":{\n  \"kind\": \"filter\",\n  \"filter\": {\"q\": \"カラオケ\"}\n}},\"success\":true}")
	})
	answer, err := gen.Generate(context.Background(), domain.GenerationRequest{Prompt: "request", Schema: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	if string(answer) != `{"kind":"filter","filter":{"q":"カラオケ"}}` {
		t.Errorf("answer = %s", answer)
	}
}

func TestRelayChatRejectsAModelThatIsNotAChatModel(t *testing.T) {
	gen := generatorThrough(t, "clef", func(http.ResponseWriter, *http.Request) {
		t.Error("the judging model reached the chat endpoint")
	})
	_, err := gen.Generate(context.Background(), domain.GenerationRequest{Prompt: "request", Schema: json.RawMessage(`{}`)})
	if !errors.Is(err, domain.ErrOllamaModel) || !strings.Contains(err.Error(), "llama3.2:3b") {
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
