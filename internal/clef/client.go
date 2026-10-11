package clef

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// requestTimeout は 1 回の判定の上限時間である。GPU の無い機械では、画像つきの判定に
// 1 件あたり数分かかることがある。
const requestTimeout = 10 * time.Minute

// maxResponseBytes は読む応答の上限である。答えは質問の数に比例する小さな JSON である。
const maxResponseBytes = 1 << 20

// keepAlive は判定のあとに模型を読み込んだままにしておく時間である。待ち行列を続けて
// 処理する間に、模型を読み込み直さない。
const keepAlive = "10m"

// Client は Ollama の判定 API を呼ぶ。NewWorkersAI で作ったものは Cloudflare の
// Workers AI を呼ぶ。ゼロ値は使えず、New か NewWorkersAI で作る。
type Client struct {
	http *http.Client
	// workersAI は Workers AI の問い合わせ先である。nil なら Ollama に問い合わせる。
	workersAI *workersAITarget
}

// New は既定の HTTP クライアント（環境のプロキシ設定に従う）で Client を作る。
func New() *Client {
	return &Client{http: &http.Client{Timeout: requestTimeout}}
}

// NewWithHTTPClient は渡した HTTP クライアントで Client を作る。テストが使う。
func NewWithHTTPClient(client *http.Client) *Client {
	return &Client{http: client}
}

type question struct {
	Type         string `json:"type"`
	Instructions string `json:"instructions"`
}

type subjectState struct {
	Title    string `json:"title"`
	FileName string `json:"file_name"`
	Folder   string `json:"folder"`
}

type requestBody struct {
	Model     string              `json:"model"`
	State     subjectState        `json:"state"`
	Images    []string            `json:"images,omitempty"`
	Questions map[string]question `json:"questions"`
	KeepAlive string              `json:"keep_alive,omitempty"`
}

type answer struct {
	Type string   `json:"type"`
	Noul *float64 `json:"noul"`
}

type responseBody struct {
	Answers map[string]answer `json:"answers"`
	Error   string            `json:"error"`
}

// Classify は質問ごとに「はい」の確率を返す。答えの無い質問は結果に含めない。
// Workers AI に向けた Client（NewWorkersAI）は req.Endpoint を使わない。
func (c *Client) Classify(ctx context.Context, req domain.AutoTagRequest) (map[string]float64, error) {
	if len(req.Questions) == 0 {
		return map[string]float64{}, nil
	}
	if c.workersAI != nil {
		return c.classifyWithWorkersAI(ctx, req)
	}
	body := newRequestBody(req)
	body.KeepAlive = keepAlive
	if len(req.Subject.Thumbnail) > 0 {
		body.Images = []string{base64.StdEncoding.EncodeToString(req.Subject.Thumbnail)}
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("cannot build the classifier request: %w", err)
	}

	url := strings.TrimRight(req.Endpoint, "/") + "/v1/systemone"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot build the classifier request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("cannot reach Ollama at %s: %w", req.Endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("cannot read the classifier response: %w", err)
	}
	var decoded responseBody
	decodeErr := json.Unmarshal(raw, &decoded)
	if resp.StatusCode != http.StatusOK {
		// Ollama が JSON で返した誤りの文だけを伝える。応答の本文そのものは、問い合わせ先が
		// Ollama でないときに別のサービスの中身を画面へ写すことになるので返さない。
		message := strings.TrimSpace(decoded.Error)
		if decodeErr != nil || message == "" {
			return nil, fmt.Errorf("ollama answered %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("ollama answered %d: %s", resp.StatusCode, truncate(message))
	}
	if decodeErr != nil {
		return nil, fmt.Errorf("cannot read the classifier response: %w", decodeErr)
	}
	return probabilitiesOf(decoded.Answers)
}

// newRequestBody は Ollama と Workers AI に共通の本文（模型・状態・質問）を作る。
func newRequestBody(req domain.AutoTagRequest) requestBody {
	body := requestBody{
		Model: req.Model,
		State: subjectState{
			Title:    req.Subject.Title,
			FileName: req.Subject.FileName,
			Folder:   req.Subject.Folder,
		},
		Questions: make(map[string]question, len(req.Questions)),
	}
	for _, q := range req.Questions {
		body.Questions[q.Key] = question{Type: "noul", Instructions: q.Instructions}
	}
	return body
}

// probabilitiesOf は答えから質問ごとの「はい」の確率を取り出す。
func probabilitiesOf(answers map[string]answer) (map[string]float64, error) {
	if answers == nil {
		return nil, errors.New("the classifier response has no answers")
	}
	probabilities := make(map[string]float64, len(answers))
	for key, a := range answers {
		if a.Noul != nil {
			probabilities[key] = *a.Noul
		}
	}
	return probabilities, nil
}

// truncate は画面に出す誤りの文を 300 バイトまでに切る。
func truncate(message string) string {
	if len(message) > 300 {
		return message[:300]
	}
	return message
}
