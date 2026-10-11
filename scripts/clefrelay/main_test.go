package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
	"github.com/syudead/vv/internal/ollama"
)

type fixedSettings domain.OllamaSettings

func (f fixedSettings) OllamaSettings(context.Context) (domain.OllamaSettings, error) {
	return domain.OllamaSettings(f), nil
}

func solidJPEG(t *testing.T, w, h int, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// relayTo は Workers AI の代わりの handler へ中継する clefrelay と、それを Ollama の接続先と
// する製品の Ollama のプロバイダを作る。
func relayTo(t *testing.T, model string, workers http.HandlerFunc) *ollama.Classifier {
	t.Helper()
	upstream := httptest.NewServer(workers)
	t.Cleanup(upstream.Close)
	relay := httptest.NewServer(newRelay(config{account: "acct", baseURL: upstream.URL, token: "tok"}, upstream.Client()))
	t.Cleanup(relay.Close)
	return ollama.New(fixedSettings{Endpoint: relay.URL, Model: model})
}

func TestRelayTranslatesTheOllamaProviderRequestAndAnswer(t *testing.T) {
	thumbnail := solidJPEG(t, 320, 180, color.RGBA{R: 255, A: 255})
	frame := solidJPEG(t, 160, 90, color.RGBA{B: 255, A: 255})
	images := [][]byte{thumbnail}
	for range 8 {
		images = append(images, frame)
	}

	var got map[string]json.RawMessage
	var path, auth string
	classifier := relayTo(t, "clef-flash", func(w http.ResponseWriter, r *http.Request) {
		path, auth = r.URL.Path, r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		_, _ = io.WriteString(w, `{"result":{"model":"clef-flash","answers":{"tag_1":{"type":"noul","noul":0.9},"tag_2":{"type":"noul","noul":0.1}},"usage":{"input_tokens":10,"output_tokens":0}},"success":true,"errors":[],"messages":[]}`)
	})

	probabilities, err := classifier.Classify(context.Background(), domain.ClassifierRequest{
		Input: domain.ClassifierInput{Facts: map[string]string{"title": "Trip"}, Images: images},
		Questions: []domain.ClassifierQuestion{
			{Key: "tag_1", Text: "Does \"cat\" apply?"},
			{Key: "tag_2", Text: "Does \"rocket\" apply?"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if probabilities["tag_1"] != 0.9 || probabilities["tag_2"] != 0.1 || len(probabilities) != 2 {
		t.Fatalf("probabilities = %v", probabilities)
	}
	if path != "/accounts/acct/ai/run/@cf/cloudflare/clef-flash" {
		t.Errorf("path = %q", path)
	}
	if auth != "Bearer tok" {
		t.Errorf("Authorization = %q", auth)
	}
	if _, ok := got["keep_alive"]; ok {
		t.Error("keep_alive was relayed; Workers AI rejects it")
	}
	var sent []workersImage
	if err := json.Unmarshal(got["images"], &sent); err != nil {
		t.Fatal(err)
	}
	if len(sent) != 1 || sent[0].ContentType != "image/jpeg" {
		t.Fatalf("images = %d, want one tiled JPEG", len(sent))
	}
	raw, _ := base64.StdEncoding.DecodeString(sent[0].Base64)
	tiled, err := jpeg.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	// 9 枚は 3×3 に、1 コマは最初の画像（サムネイル）の大きさで並ぶ。
	if b := tiled.Bounds(); b.Dx() != 960 || b.Dy() != 540 {
		t.Errorf("tiled size = %v, want 960x540", b.Size())
	}
	if r, _, b, _ := tiled.At(160, 90).RGBA(); r < 0xc000 || b > 0x4000 {
		t.Error("the first cell is not the thumbnail")
	}
	if r, _, b, _ := tiled.At(880, 500).RGBA(); b < 0xc000 || r > 0x4000 {
		t.Error("the last cell is not a frame")
	}
}

func TestRelayPassesUpToFourImagesUnchanged(t *testing.T) {
	images := [][]byte{solidJPEG(t, 32, 18, color.White), solidJPEG(t, 32, 18, color.Black)}
	var got workersRequest
	classifier := relayTo(t, "clef", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = io.WriteString(w, `{"result":{"answers":{}},"success":true}`)
	})
	if _, err := classifier.Classify(context.Background(), domain.ClassifierRequest{
		Input:     domain.ClassifierInput{Images: images},
		Questions: []domain.ClassifierQuestion{{Key: "q", Text: "?"}},
	}); err != nil {
		t.Fatal(err)
	}
	if len(got.Images) != 2 || got.Images[1].Base64 != base64.StdEncoding.EncodeToString(images[1]) {
		t.Fatalf("images were changed: %d", len(got.Images))
	}
}

func TestRelayPassesTheWorkersAIErrorText(t *testing.T) {
	classifier := relayTo(t, "clef-flash", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = io.WriteString(w, `{"errors":[{"message":"AiError: request exceeds the image count limit","code":5012}],"success":false,"result":{}}`)
	})
	err := classifier.Check(context.Background())
	if err == nil || !strings.Contains(err.Error(), "422") || !strings.Contains(err.Error(), "image count limit") {
		t.Fatalf("err = %v", err)
	}
}

func TestRelayRejectsAModelWorkersAIDoesNotHave(t *testing.T) {
	classifier := relayTo(t, "llama3", func(http.ResponseWriter, *http.Request) {
		t.Error("an unknown model reached Workers AI")
	})
	if err := classifier.Check(context.Background()); err == nil || !strings.Contains(err.Error(), "clef-flash") {
		t.Fatalf("err = %v", err)
	}
}
