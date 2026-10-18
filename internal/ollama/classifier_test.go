package ollama

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

type fixedSettings struct {
	settings domain.OllamaSettings
	err      error
}

func (f fixedSettings) OllamaSettings(context.Context) (domain.OllamaSettings, error) {
	return f.settings, f.err
}

func at(url string) fixedSettings {
	return fixedSettings{settings: domain.OllamaSettings{Endpoint: url, Model: "clef"}}
}

func oneQuestion() domain.ClassifierRequest {
	return domain.ClassifierRequest{Questions: []domain.ClassifierQuestion{{Key: "a", Text: "?"}}}
}

func TestClassifySendsSystemOneRequest(t *testing.T) {
	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/systemone" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		_, _ = w.Write([]byte(`{"model":"clef-flash","answers":{"tag_1":{"type":"noul","noul":0.91},"tag_2":{"type":"noul","noul":0.02}}}`))
	}))
	defer server.Close()

	classifier := New(fixedSettings{settings: domain.OllamaSettings{Endpoint: server.URL + "/", Model: "clef-flash"}})
	probabilities, err := classifier.Classify(context.Background(), domain.ClassifierRequest{
		Input: domain.ClassifierInput{
			Facts:  map[string]string{"title": "嵐山", "file_name": "竹林.mp4", "folder": "/media/旅行"},
			Images: [][]byte{{0xff, 0xd8}, {0xff, 0xd9}},
		},
		Questions: []domain.ClassifierQuestion{
			{Key: "tag_1", Text: "Is it travel?"},
			{Key: "tag_2", Text: "Is it cooking?"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if probabilities["tag_1"] != 0.91 || probabilities["tag_2"] != 0.02 {
		t.Fatalf("probabilities = %v", probabilities)
	}
	if got["model"] != "clef-flash" {
		t.Errorf("model = %v", got["model"])
	}
	state, _ := got["state"].(map[string]any)
	if state["title"] != "嵐山" || state["file_name"] != "竹林.mp4" || state["folder"] != "/media/旅行" {
		t.Errorf("state = %v", got["state"])
	}
	images, _ := got["images"].([]any)
	if len(images) != 2 || images[0] != base64.StdEncoding.EncodeToString([]byte{0xff, 0xd8}) ||
		images[1] != base64.StdEncoding.EncodeToString([]byte{0xff, 0xd9}) {
		t.Errorf("images = %v", got["images"])
	}
	questions, _ := got["questions"].(map[string]any)
	q1, _ := questions["tag_1"].(map[string]any)
	if q1["type"] != "noul" || q1["instructions"] != "Is it travel?" {
		t.Errorf("questions = %v", got["questions"])
	}
}

func TestClassifySplitsQuestionsIntoBatches(t *testing.T) {
	var mu sync.Mutex
	var sizes []int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body requestBody
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		sizes = append(sizes, len(body.Questions))
		mu.Unlock()
		answers := map[string]answer{}
		for key := range body.Questions {
			p := 0.5
			answers[key] = answer{Type: "noul", Noul: &p}
		}
		_ = json.NewEncoder(w).Encode(responseBody{Answers: answers})
	}))
	defer server.Close()

	req := domain.ClassifierRequest{}
	for i := range BatchSize*2 + 1 {
		req.Questions = append(req.Questions, domain.ClassifierQuestion{Key: fmt.Sprintf("q%d", i), Text: "?"})
	}
	probabilities, err := New(at(server.URL)).Classify(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(probabilities) != BatchSize*2+1 {
		t.Fatalf("answers = %d", len(probabilities))
	}
	if len(sizes) != 3 || sizes[0] != BatchSize || sizes[1] != BatchSize || sizes[2] != 1 {
		t.Fatalf("batch sizes = %v", sizes)
	}
}

func TestClassifyReadsSettingsEachTime(t *testing.T) {
	_, err := New(fixedSettings{err: errors.New("disk")}).Classify(context.Background(), oneQuestion())
	if err == nil || !strings.Contains(err.Error(), "Ollama settings") {
		t.Fatalf("err = %v", err)
	}
	if got, err := New(fixedSettings{err: errors.New("disk")}).Classify(context.Background(), domain.ClassifierRequest{}); err != nil || len(got) != 0 {
		t.Fatalf("質問の無い要求 = %v, %v", got, err)
	}
}

func TestClassifyOmitsImagesWhenNone(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if _, ok := body["images"]; ok {
			t.Error("images was sent without images")
		}
		_, _ = w.Write([]byte(`{"answers":{}}`))
	}))
	defer server.Close()
	if _, err := New(at(server.URL)).Classify(context.Background(), oneQuestion()); err != nil {
		t.Fatal(err)
	}
}

func TestClassifyReportsOllamaErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"model 'clef' not found"}`))
	}))
	defer server.Close()
	_, err := New(at(server.URL)).Classify(context.Background(), oneQuestion())
	if err == nil || !strings.Contains(err.Error(), "404") || !strings.Contains(err.Error(), "model 'clef' not found") {
		t.Fatalf("err = %v", err)
	}
}

func TestClassifyDoesNotEchoNonOllamaBodies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("<html>secret internal page</html>"))
	}))
	defer server.Close()
	_, err := New(at(server.URL)).Classify(context.Background(), oneQuestion())
	if err == nil || err.Error() != "ollama answered 403" {
		t.Fatalf("err = %v", err)
	}
}

func TestClassifyReportsUnreachableEndpoint(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	url := server.URL
	server.Close()
	_, err := New(at(url)).Classify(context.Background(), oneQuestion())
	if err == nil || !strings.Contains(err.Error(), "cannot reach Ollama") {
		t.Fatalf("err = %v", err)
	}
}
