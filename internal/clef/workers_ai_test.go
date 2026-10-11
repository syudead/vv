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

func TestWorkersAISendsRequestToAccountModel(t *testing.T) {
	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/client/v4/accounts/acct-1/ai/run/@cf/cloudflare/clef-flash" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer token-1" {
			t.Errorf("Authorization = %q", auth)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		_, _ = w.Write([]byte(`{"success":true,"errors":[],"result":{"answers":{"tag_1":{"type":"noul","noul":0.91},"tag_2":{"type":"noul","noul":0.02}}}}`))
	}))
	defer server.Close()

	// 設定の Ollama の URL は使わない。
	probabilities, err := NewWorkersAI(server.URL+"/client/v4/", "acct-1", "token-1").Classify(context.Background(), domain.AutoTagRequest{
		Endpoint: "http://127.0.0.1:1",
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
	if _, ok := got["keep_alive"]; ok {
		t.Error("keep_alive was sent to Workers AI")
	}
	state, _ := got["state"].(map[string]any)
	if state["title"] != "嵐山" || state["file_name"] != "竹林.mp4" || state["folder"] != "/media/旅行" {
		t.Errorf("state = %v", got["state"])
	}
	images, _ := got["images"].([]any)
	want := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString([]byte{0xff, 0xd8})
	if len(images) != 1 || images[0] != want {
		t.Errorf("images = %v", got["images"])
	}
	questions, _ := got["questions"].(map[string]any)
	q1, _ := questions["tag_1"].(map[string]any)
	if q1["type"] != "noul" || q1["instructions"] != "Is it travel?" {
		t.Errorf("questions = %v", got["questions"])
	}
}

func TestWorkersAIReportsAPIErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"success":false,"errors":[{"code":5006,"message":"model mismatch"}],"result":null}`))
	}))
	defer server.Close()
	_, err := NewWorkersAI(server.URL, "acct", "token").Classify(context.Background(), domain.AutoTagRequest{
		Model: "clef", Questions: []domain.AutoTagQuestion{{Key: "a", Instructions: "?"}},
	})
	if err == nil || err.Error() != "workers AI answered 400: model mismatch (5006)" {
		t.Fatalf("err = %v", err)
	}
}

func TestWorkersAIDoesNotEchoNonJSONBodies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("<html>gateway</html>"))
	}))
	defer server.Close()
	_, err := NewWorkersAI(server.URL, "acct", "token").Classify(context.Background(), domain.AutoTagRequest{
		Model: "clef", Questions: []domain.AutoTagQuestion{{Key: "a", Instructions: "?"}},
	})
	if err == nil || err.Error() != "workers AI answered 502" {
		t.Fatalf("err = %v", err)
	}
}

// 届かないときの誤りに、アカウント ID を含む URL を写さない。
func TestWorkersAIUnreachableHidesAccountID(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	url := server.URL
	server.Close()
	_, err := NewWorkersAI(url, "secret-account", "token").Classify(context.Background(), domain.AutoTagRequest{
		Model: "clef", Questions: []domain.AutoTagQuestion{{Key: "a", Instructions: "?"}},
	})
	if err == nil || !strings.Contains(err.Error(), "cannot reach Workers AI") || strings.Contains(err.Error(), "secret-account") {
		t.Fatalf("err = %v", err)
	}
}
