package ollama

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/syudead/vv/internal/domain"
)

func solidPNG(t *testing.T, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 16, 9))
	for y := range 9 {
		for x := range 16 {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// decodedImages decodes the base64 images of a request body.
func decodedImages(t *testing.T, raw json.RawMessage) []image.Image {
	t.Helper()
	var encoded []string
	if err := json.Unmarshal(raw, &encoded); err != nil {
		t.Fatalf("images = %s", raw)
	}
	out := make([]image.Image, len(encoded))
	for i, s := range encoded {
		b, _ := base64.StdEncoding.DecodeString(s)
		img, _, err := image.Decode(bytes.NewReader(b))
		if err != nil {
			t.Fatalf("image %d: %v", i, err)
		}
		out[i] = img
	}
	return out
}

// The wire fills the model and keep-alive from the settings, passes state and questions as given, tiles more images
// than Ollama takes into one, and answers the endpoint's JSON as it is.
func TestSystemOneSendsClefsOwnRequestAndAnswersItsJSON(t *testing.T) {
	var got map[string]json.RawMessage
	answer := `{"model":"clef","answers":{"cat":{"type":"noul","noul":0.93}}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/systemone" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(answer))
	}))
	defer server.Close()
	red, blue := solidPNG(t, color.RGBA{R: 255, A: 255}), solidPNG(t, color.RGBA{B: 255, A: 255})
	images := [][]byte{red}
	for range maxImages {
		images = append(images, blue)
	}

	raw, err := New(at(server.URL)).SystemOne(context.Background(), domain.ClefRequest{
		State:     map[string]string{"title": "猫の日"},
		Images:    images,
		Questions: map[string]domain.ClefQuestion{"cat": {Type: "noul", Instructions: "Is a cat visible?"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != answer {
		t.Errorf("answer = %s, want the endpoint's JSON", raw)
	}
	want := map[string]string{
		"model": `"clef"`, "keep_alive": `"10m"`, "state": `{"title":"猫の日"}`,
		"questions": `{"cat":{"type":"noul","instructions":"Is a cat visible?"}}`,
	}
	for key, value := range want {
		if string(got[key]) != value {
			t.Errorf("%s = %s, want %s", key, got[key], value)
		}
	}
	sent := decodedImages(t, got["images"])
	if len(sent) != 1 {
		t.Fatalf("images = %d, want one tiled image", len(sent))
	}
	// Nine images form a 3 x 3 grid of the first image's size, in order.
	if b := sent[0].Bounds(); b.Dx() != 48 || b.Dy() != 27 {
		t.Errorf("tiled size = %v, want 48x27", b.Size())
	}
	if r, _, b, _ := sent[0].At(8, 4).RGBA(); r < 0xc000 || b > 0x4000 {
		t.Error("the first cell is not the first image")
	}
}

func TestSystemOnePassesUpToTheLimitUnchanged(t *testing.T) {
	var got map[string]json.RawMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"answers":{}}`))
	}))
	defer server.Close()
	images := make([][]byte, maxImages)
	for i := range images {
		images[i] = []byte{byte(i)}
	}
	if _, err := New(at(server.URL)).SystemOne(context.Background(), domain.ClefRequest{Images: images}); err != nil {
		t.Fatal(err)
	}
	var encoded []string
	_ = json.Unmarshal(got["images"], &encoded)
	if len(encoded) != maxImages || encoded[1] != base64.StdEncoding.EncodeToString([]byte{1}) {
		t.Errorf("images = %v, want the %d images as they are", encoded, maxImages)
	}
}

func TestSystemOneReturnsTheEndpointsErrorText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"unknown question type \"maybe\""}`))
	}))
	defer server.Close()
	_, err := New(at(server.URL)).SystemOne(context.Background(), domain.ClefRequest{})
	if err == nil || !strings.Contains(err.Error(), `400: unknown question type "maybe"`) {
		t.Errorf("err = %v, want the endpoint's error text", err)
	}
}

// A chat message with more images than Ollama takes carries them as one tiled image.
func TestChatTilesAMessagesImagesOverTheLimit(t *testing.T) {
	var got map[string]json.RawMessage
	url := chatServer(t, http.StatusOK, `{"message":{"role":"assistant","content":"猫です"}}`, &got)
	images := make([][]byte, maxImages+1)
	for i := range images {
		images[i] = solidPNG(t, color.White)
	}
	_, err := NewChat(generating(url)).Chat(context.Background(), domain.ChatRequest{
		Messages: []domain.ChatMessage{{Role: domain.ChatRoleUser, Content: "images", Images: images}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var messages []struct {
		Images json.RawMessage `json:"images"`
	}
	if err := json.Unmarshal(got["messages"], &messages); err != nil || len(messages) != 1 {
		t.Fatalf("messages = %s", got["messages"])
	}
	if sent := decodedImages(t, messages[0].Images); len(sent) != 1 {
		t.Errorf("images = %d, want one tiled image", len(sent))
	}
}
