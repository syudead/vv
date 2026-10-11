// clefrelay は Ollama の判定 API（POST /v1/systemone）を受けて、Cloudflare Workers AI の
// Clef（@cf/cloudflare/clef・@cf/cloudflare/clef-flash）へ中継する開発用の道具である。手順は
// docs/how-to/workers-ai-relay.md にある。
//
// Ollama を動かせない機械（Claude Code on the web のコンテナなど）で、製品の Ollama の
// プロバイダ（internal/ollama）をそのまま使って自動タグ付けを試すためのもので、製品には
// 入れない。Workers AI と Ollama の違いはここで吸収する。
//
//   - 接続の確かめ: GET /api/version と GET /api/tags に、Workers AI の Clef の模型を
//     並べて答える（Workers AI へは問い合わせない）
//   - URL: 模型ごとに /accounts/{id}/ai/run/@cf/cloudflare/{model}
//   - 認証: CLOUDFLARE_API_TOKEN があれば Bearer で付ける（無ければ付けない。プロキシが
//     付ける環境向け）
//   - 画像: 生の base64 は受け付けず、4 枚までである。5 枚以上なら 1 枚に並べる
//   - keep_alive: 余分な入力として拒否されるので落とす
//   - 応答: {"result":{"answers":…},"success":true} を {"answers":…} に、誤りは
//     {"errors":[{"message":…}]} を {"error":…} に直す
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

	_ "image/png" // テストや手で送る PNG も読めるようにする。
)

// maxImages は Workers AI の Clef が 1 回に受け付ける画像の数である。
const maxImages = 4

// maxCellWidth は並べた画像の 1 コマの幅の上限である。9 コマで約 1,900×1,100 になり、
// Workers AI の 1 枚 16 メガピクセル・4 MiB に収まる。
const maxCellWidth = 640

// requestTimeout は Workers AI への 1 回の問い合わせの上限である。製品の Ollama の
// プロバイダは 10 分待つが、Workers AI は通常数秒で答える。
const requestTimeout = 2 * time.Minute

// maxBodyBytes は受け取る要求と読む応答の上限である。
const maxBodyBytes = 64 << 20

// tileQuality は並べた画像を JPEG にするときの品質である。
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

// ollamaRequest は internal/ollama が送る本文である。
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
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// relayVersion は GET /api/version に答える版である。製品の接続の確かめは空でない版を
// Ollama の印とみなす。
const relayVersion = "0.0.0-clefrelay"

// models は Workers AI にある Clef の模型である。
var models = []string{"clef", "clef-flash"}

func newRelay(cfg config, client *http.Client) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/version", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"version": relayVersion})
	})
	mux.HandleFunc("GET /api/tags", func(w http.ResponseWriter, _ *http.Request) {
		list := make([]map[string]string, len(models))
		for i, m := range models {
			list[i] = map[string]string{"name": m + ":latest", "model": m + ":latest"}
		}
		writeJSON(w, http.StatusOK, map[string]any{"models": list})
	})
	mux.HandleFunc("POST /v1/systemone", func(w http.ResponseWriter, r *http.Request) {
		relay(w, r, cfg, client)
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
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.token)
	}
	resp, err := client.Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "cannot reach Workers AI: "+err.Error())
		return
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		writeError(w, http.StatusBadGateway, "cannot read the Workers AI response: "+err.Error())
		return
	}
	var out workersResponse
	decodeErr := json.Unmarshal(raw, &out)

	if resp.StatusCode == http.StatusOK && decodeErr == nil && out.Success && out.Result != nil {
		log.Printf("clefrelay: %s %d questions, %d→%d images, %d tokens, %s",
			model, countKeys(in.Questions), len(in.Images), len(images), out.Result.Usage.InputTokens,
			time.Since(started).Round(time.Millisecond))
		writeJSON(w, http.StatusOK, map[string]any{"model": model, "answers": out.Result.Answers})
		return
	}
	messages := make([]string, 0, len(out.Errors))
	for _, e := range out.Errors {
		messages = append(messages, e.Message)
	}
	message := strings.Join(messages, "; ")
	if message == "" {
		message = fmt.Sprintf("Workers AI answered %d", resp.StatusCode)
	}
	status := resp.StatusCode
	if status < 400 {
		status = http.StatusBadGateway
	}
	log.Printf("clefrelay: %s failed (%d): %s", model, resp.StatusCode, message)
	writeError(w, status, message)
}

// workersImages は Ollama の生の base64 の画像を Workers AI の形にする。5 枚以上なら、
// 順に左上から並べた 1 枚にする。最初の数枚だけに間引くと、動画の後半の場面を落とす。
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

// tile は画像を正方形に近い格子に左上から並べ、JPEG にする。1 コマは最初の画像
// （製品では代表サムネイル）の大きさで、幅は maxCellWidth までに縮める。縦横比の違う
// 画像は 1 コマの中に収めて余白を黒にする。
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

// drawFit は src を縦横比を保って cell の中央に収めて描く。縮めるときは対応する元の
// 画素の平均を取る。
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
