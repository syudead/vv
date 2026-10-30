// clefrelay adapts Ollama judging and chat requests to Workers AI for development without a local Ollama server.
// Run CLOUDFLARE_ACCOUNT_ID=<id> go run ./scripts/clefrelay, then select http://127.0.0.1:11434 and clef or clef-flash.
// The assistant's generation model is one of the function calling chat models in chat.go, such as llama3.3:70b;
// /api/show reports which of them take tools and images.
//
// Set CLOUDFLARE_API_TOKEN unless the proxy supplies authentication. Connection checks stay local; use "Test judging" to verify Workers AI.
// The adapter tiles more than four images, drops keep_alive, relays tools and tool calls as OpenAI-style function
// calling, sends a chat format as a JSON schema, logs every chat answer, and converts Workers AI responses and errors
// to Ollama format.
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "image/png" // Decode PNG fixtures and manually submitted images.
)

// Workers AI accepts at most four images per request.
const maxImages = 4

// Nine cells produce about 1,900 x 1,100 pixels, within the 16-megapixel and 4 MiB limits.
const maxCellWidth = 640

// Workers AI normally responds in seconds, well below the product Ollama timeout.
const requestTimeout = 2 * time.Minute

// Limit both request and response bodies.
const maxBodyBytes = 64 << 20

const tileQuality = 85

type config struct {
	addr    string
	account string
	baseURL string
	token   string
}

func main() {
	cfg := config{token: os.Getenv("CLOUDFLARE_API_TOKEN")}
	flag.StringVar(&cfg.addr, "addr", "127.0.0.1:11434", "受け付けるアドレス（Ollama の既定と同じ）")
	flag.StringVar(&cfg.account, "account", os.Getenv("CLOUDFLARE_ACCOUNT_ID"), "Cloudflare の Account ID（既定は CLOUDFLARE_ACCOUNT_ID）")
	flag.StringVar(&cfg.baseURL, "base-url", "https://api.cloudflare.com/client/v4", "Cloudflare API の URL")
	flag.Parse()
	if cfg.account == "" {
		fmt.Fprintln(os.Stderr, "clefrelay: Account ID が無い（-account か CLOUDFLARE_ACCOUNT_ID）")
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	server := &http.Server{
		Addr:              cfg.addr,
		Handler:           newRelay(cfg, &http.Client{Timeout: requestTimeout}),
		ReadHeaderTimeout: 10 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	log.Printf("clefrelay: listening on http://%s, relaying to Workers AI account %s", cfg.addr, cfg.account)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("clefrelay: %v", err)
	}
}

// Request body sent by internal/ollama.
type ollamaRequest struct {
	Model     string          `json:"model"`
	State     json.RawMessage `json:"state"`
	Images    []string        `json:"images"`
	Questions json.RawMessage `json:"questions"`
}

type workersImage struct {
	ContentType string `json:"content_type"`
	Base64      string `json:"base64"`
}

type workersRequest struct {
	Model     string          `json:"model"`
	State     json.RawMessage `json:"state"`
	Images    []workersImage  `json:"images,omitempty"`
	Questions json.RawMessage `json:"questions"`
}

type workersResponse struct {
	Success bool `json:"success"`
	Result  *struct {
		Answers json.RawMessage `json:"answers"`
		Usage   struct {
			InputTokens int `json:"input_tokens"`
		} `json:"usage"`
	} `json:"result"`
	Errors []workersError `json:"errors"`
}

// The product recognizes any nonempty /api/version response as Ollama.
const relayVersion = "0.0.0-clefrelay"

var models = []string{"clef", "clef-flash"}

func newRelay(cfg config, client *http.Client) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/version", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"version": relayVersion})
	})
	mux.HandleFunc("GET /api/tags", func(w http.ResponseWriter, _ *http.Request) {
		var list []map[string]string
		for _, m := range models {
			list = append(list, map[string]string{"name": m + ":latest", "model": m + ":latest"})
		}
		for _, m := range chatModelNames() {
			list = append(list, map[string]string{"name": m, "model": m})
		}
		writeJSON(w, http.StatusOK, map[string]any{"models": list})
	})
	mux.HandleFunc("POST /api/show", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, maxBodyBytes)).Decode(&in); err != nil {
			writeError(w, http.StatusBadRequest, "cannot read the request: "+err.Error())
			return
		}
		capabilities, ok := showCapabilities(in.Model)
		if !ok {
			writeError(w, http.StatusNotFound, fmt.Sprintf("model '%s' not found", in.Model))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"capabilities": capabilities})
	})
	mux.HandleFunc("POST /v1/systemone", func(w http.ResponseWriter, r *http.Request) {
		relay(w, r, cfg, client)
	})
	mux.HandleFunc("POST /api/chat", func(w http.ResponseWriter, r *http.Request) {
		relayChat(w, r, cfg, client)
	})
	return mux
}

func relay(w http.ResponseWriter, r *http.Request, cfg config, client *http.Client) {
	started := time.Now()
	var in ollamaRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBodyBytes)).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "cannot read the request: "+err.Error())
		return
	}
	model := strings.TrimSuffix(strings.TrimSpace(in.Model), ":latest")
	if model != "clef" && model != "clef-flash" {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("model %q is not on Workers AI; use clef or clef-flash", in.Model))
		return
	}
	images, err := workersImages(in.Images)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	body, err := json.Marshal(workersRequest{Model: model, State: in.State, Images: images, Questions: in.Questions})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	url := fmt.Sprintf("%s/accounts/%s/ai/run/@cf/cloudflare/%s", strings.TrimRight(cfg.baseURL, "/"), cfg.account, model)
	status, raw, err := postWorkers(r, cfg, client, url, body)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	var out workersResponse
	decodeErr := json.Unmarshal(raw, &out)

	if status == http.StatusOK && decodeErr == nil && out.Success && out.Result != nil {
		log.Printf("clefrelay: %s %d questions, %d→%d images, %d tokens, %s",
			model, countKeys(in.Questions), len(in.Images), len(images), out.Result.Usage.InputTokens,
			time.Since(started).Round(time.Millisecond))
		writeJSON(w, http.StatusOK, map[string]any{"model": model, "answers": out.Result.Answers})
		return
	}
	message := errorMessage(status, out.Errors)
	log.Printf("clefrelay: %s failed (%d): %s", model, status, message)
	writeError(w, errorStatus(status), message)
}

// postWorkers sends one Workers AI request with the relay's credentials and reads its whole answer.
func postWorkers(r *http.Request, cfg config, client *http.Client, url string, body []byte) (int, []byte, error) {
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("cannot reach Workers AI: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return 0, nil, fmt.Errorf("cannot read the Workers AI response: %w", err)
	}
	return resp.StatusCode, raw, nil
}

type workersError struct {
	Message string `json:"message"`
}

func errorMessage(status int, errs []workersError) string {
	messages := make([]string, 0, len(errs))
	for _, e := range errs {
		messages = append(messages, e.Message)
	}
	if message := strings.Join(messages, "; "); message != "" {
		return message
	}
	return fmt.Sprintf("Workers AI answered %d", status)
}

// errorStatus keeps a Workers AI error status and turns an unreadable success into a gateway error.
func errorStatus(status int) int {
	if status < 400 {
		return http.StatusBadGateway
	}
	return status
}

// Tile more than four images instead of discarding later scenes.
func workersImages(encoded []string) ([]workersImage, error) {
	if len(encoded) == 0 {
		return nil, nil
	}
	decoded := make([][]byte, len(encoded))
	for i, s := range encoded {
		b, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return nil, fmt.Errorf("image %d is not base64: %w", i, err)
		}
		decoded[i] = b
	}
	if len(decoded) <= maxImages {
		images := make([]workersImage, len(decoded))
		for i, b := range decoded {
			images[i] = workersImage{ContentType: http.DetectContentType(b), Base64: base64.StdEncoding.EncodeToString(b)}
		}
		return images, nil
	}
	tiled, err := tile(decoded)
	if err != nil {
		return nil, err
	}
	return []workersImage{{ContentType: "image/jpeg", Base64: base64.StdEncoding.EncodeToString(tiled)}}, nil
}

// tile uses a near-square grid with cells based on the first thumbnail, capped at maxCellWidth; differing aspect ratios get black padding.
func tile(images [][]byte) ([]byte, error) {
	decoded := make([]image.Image, len(images))
	for i, b := range images {
		img, _, err := image.Decode(bytes.NewReader(b))
		if err != nil {
			return nil, fmt.Errorf("cannot decode image %d: %w", i, err)
		}
		decoded[i] = img
	}
	first := decoded[0].Bounds()
	cellW, cellH := first.Dx(), first.Dy()
	if cellW > maxCellWidth {
		cellH = cellH * maxCellWidth / cellW
		cellW = maxCellWidth
	}
	cols := int(math.Ceil(math.Sqrt(float64(len(decoded)))))
	rows := (len(decoded) + cols - 1) / cols
	out := image.NewRGBA(image.Rect(0, 0, cols*cellW, rows*cellH))
	for i, img := range decoded {
		cell := image.Rect(0, 0, cellW, cellH).Add(image.Pt((i%cols)*cellW, (i/cols)*cellH))
		drawFit(out, cell, img)
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, out, &jpeg.Options{Quality: tileQuality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// drawFit preserves aspect ratio and averages source pixels when downsampling.
func drawFit(dst *image.RGBA, cell image.Rectangle, src image.Image) {
	sb := src.Bounds()
	scale := math.Min(float64(cell.Dx())/float64(sb.Dx()), float64(cell.Dy())/float64(sb.Dy()))
	w := max(1, int(float64(sb.Dx())*scale))
	h := max(1, int(float64(sb.Dy())*scale))
	origin := cell.Min.Add(image.Pt((cell.Dx()-w)/2, (cell.Dy()-h)/2))
	for y := range h {
		y0 := sb.Min.Y + y*sb.Dy()/h
		y1 := max(y0+1, sb.Min.Y+(y+1)*sb.Dy()/h)
		for x := range w {
			x0 := sb.Min.X + x*sb.Dx()/w
			x1 := max(x0+1, sb.Min.X+(x+1)*sb.Dx()/w)
			var r, g, b, n uint64
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					cr, cg, cb, _ := src.At(sx, sy).RGBA()
					r, g, b, n = r+uint64(cr), g+uint64(cg), b+uint64(cb), n+1
				}
			}
			dst.SetRGBA(origin.X+x, origin.Y+y, color.RGBA{
				R: uint8(r / n >> 8), G: uint8(g / n >> 8), B: uint8(b / n >> 8), A: 0xff,
			})
		}
	}
}

func countKeys(raw json.RawMessage) int {
	var m map[string]json.RawMessage
	_ = json.Unmarshal(raw, &m)
	return len(m)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
