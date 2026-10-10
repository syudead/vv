package clef

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

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

	probabilities, err := New().Classify(context.Background(), domain.AutoTagRequest{
		Endpoint: server.URL + "/",
		Model:    "clef-flash",
		Subject:  domain.AutoTagSubject{Title: "嵐山", FileName: "竹林.mp4", Folder: "/media/旅行", Thumbnail: []byte{0xff, 0xd8}},
		Questions: []domain.AutoTagQuestion{
			{Key: "tag_1", Instructions: "Is it travel?"},
			{Key: "tag_2", Instructions: "Is it cooking?"},
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
	if len(images) != 1 || images[0] != base64.StdEncoding.EncodeToString([]byte{0xff, 0xd8}) {
		t.Errorf("images = %v", got["images"])
	}
	questions, _ := got["questions"].(map[string]any)
	q1, _ := questions["tag_1"].(map[string]any)
	if q1["type"] != "noul" || q1["instructions"] != "Is it travel?" {
		t.Errorf("questions = %v", got["questions"])
	}
}

func TestClassifyOmitsImagesWithoutThumbnail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if _, ok := body["images"]; ok {
			t.Error("images was sent without a thumbnail")
		}
		_, _ = w.Write([]byte(`{"answers":{}}`))
	}))
	defer server.Close()
	if _, err := New().Classify(context.Background(), domain.AutoTagRequest{
		Endpoint: server.URL, Model: "clef", Questions: []domain.AutoTagQuestion{{Key: "a", Instructions: "?"}},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestClassifyReportsOllamaErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"model 'clef' not found"}`))
	}))
	defer server.Close()
	_, err := New().Classify(context.Background(), domain.AutoTagRequest{
		Endpoint: server.URL, Model: "clef", Questions: []domain.AutoTagQuestion{{Key: "a", Instructions: "?"}},
	})
	if err == nil || !strings.Contains(err.Error(), "404") || !strings.Contains(err.Error(), "model 'clef' not found") {
		t.Fatalf("err = %v", err)
	}
}

func TestClassifyReportsUnreachableEndpoint(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	url := server.URL
	server.Close()
	_, err := New().Classify(context.Background(), domain.AutoTagRequest{
		Endpoint: url, Model: "clef", Questions: []domain.AutoTagQuestion{{Key: "a", Instructions: "?"}},
	})
	if err == nil || !strings.Contains(err.Error(), "cannot reach Ollama") {
		t.Fatalf("err = %v", err)
	}
}
