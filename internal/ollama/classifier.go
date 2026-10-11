package ollama

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

// requestTimeout は 1 回の問い合わせの上限時間である。GPU の無い機械では、画像つきの判定に
// 1 回あたり数分かかることがある。
const requestTimeout = 10 * time.Minute

// checkTimeout は接続の確かめの上限時間である。模型の読み込みを含むので長めにとる。
const checkTimeout = 5 * time.Minute

// maxResponseBytes は読む応答の上限である。答えは質問の数に比例する小さな JSON である。
const maxResponseBytes = 1 << 20

// keepAlive は判定のあとに模型を読み込んだままにしておく時間である。続けて判定する間に、
// 模型を読み込み直さない。
const keepAlive = "10m"

// BatchSize は 1 回の問い合わせに載せる質問の数である。要求の質問がこれより多ければ、同じ
// 入力で分けて問い合わせる。/v1/systemone は 64 問まで受け付けるが、clef-flash は 64 問と
// 画像で入力が約 7,400 トークンになり、メモリ 15GB の CPU 機では Ollama の判定のプロセスが
// メモリ不足で落ちた。16 問（約 2,000 トークン）なら収まる（docs/design-docs/ai-layer.md）。
const BatchSize = 16

// SettingsSource は保存された接続先を読む。internal/store の *SettingsStore が満たす。
type SettingsSource interface {
	OllamaSettings(ctx context.Context) (domain.OllamaSettings, error)
}

// Classifier は Ollama の判定 API を呼ぶ。ゼロ値は使えず、New で作る。
type Classifier struct {
	settings SettingsSource
	http     *http.Client
}

// New は既定の HTTP クライアント（環境のプロキシ設定に従う）で Classifier を作る。
func New(settings SettingsSource) *Classifier {
	return &Classifier{settings: settings, http: &http.Client{Timeout: requestTimeout}}
}

// NewWithHTTPClient は渡した HTTP クライアントで Classifier を作る。テストが使う。
func NewWithHTTPClient(settings SettingsSource, client *http.Client) *Classifier {
	return &Classifier{settings: settings, http: client}
}

type question struct {
	Type         string `json:"type"`
	Instructions string `json:"instructions"`
}

type requestBody struct {
	Model     string              `json:"model"`
	State     map[string]string   `json:"state"`
	Images    []string            `json:"images,omitempty"`
	Questions map[string]question `json:"questions"`
	KeepAlive string              `json:"keep_alive"`
}

type answer struct {
	Type string   `json:"type"`
	Noul *float64 `json:"noul"`
}

type responseBody struct {
	Answers map[string]answer `json:"answers"`
	Error   string            `json:"error"`
}

// Classify は質問ごとに「はい」の確率を返す。答えの無い質問は結果に含めない。質問が
// BatchSize より多ければ、同じ入力で BatchSize 問ずつ問い合わせて答えを合わせる。
func (c *Classifier) Classify(ctx context.Context, req domain.ClassifierRequest) (map[string]float64, error) {
	probabilities := map[string]float64{}
	if len(req.Questions) == 0 {
		return probabilities, nil
	}
	settings, err := c.settings.OllamaSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot read the Ollama settings: %w", err)
	}
	var images []string
	for _, image := range req.Input.Images {
		images = append(images, base64.StdEncoding.EncodeToString(image))
	}
	for start := 0; start < len(req.Questions); start += BatchSize {
		batch := req.Questions[start:min(start+BatchSize, len(req.Questions))]
		answers, err := c.post(ctx, settings, req.Input.Facts, images, batch)
		if err != nil {
			return nil, err
		}
		for key, p := range answers {
			probabilities[key] = p
		}
	}
	return probabilities, nil
}

// Check は保存した接続先と模型で短い質問を 1 つ判定させ、使えるかを確かめる。使えなければ
// 理由を包んだ誤りを返す。接続先は保存した設定だけから取る。
func (c *Classifier) Check(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	_, err := c.Classify(ctx, domain.ClassifierRequest{
		Input:     domain.ClassifierInput{Facts: map[string]string{"title": "Connection check"}},
		Questions: []domain.ClassifierQuestion{{Key: "check", Text: "Is this a connection check?"}},
	})
	return err
}

// post は 1 回の問い合わせを送り、質問ごとの確率を返す。
func (c *Classifier) post(ctx context.Context, settings domain.OllamaSettings, facts map[string]string,
	images []string, questions []domain.ClassifierQuestion,
) (map[string]float64, error) {
	if facts == nil {
		facts = map[string]string{}
	}
	body := requestBody{
		Model:     settings.Model,
		State:     facts,
		Images:    images,
		Questions: make(map[string]question, len(questions)),
		KeepAlive: keepAlive,
	}
	for _, q := range questions {
		body.Questions[q.Key] = question{Type: "noul", Instructions: q.Text}
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("cannot build the classifier request: %w", err)
	}

	url := strings.TrimRight(settings.Endpoint, "/") + "/v1/systemone"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(encoded))
	if err != nil {
		return nil, fmt.Errorf("cannot build the classifier request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("cannot reach Ollama at %s: %w", settings.Endpoint, err)
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
		if len(message) > 300 {
			message = message[:300]
		}
		return nil, fmt.Errorf("ollama answered %d: %s", resp.StatusCode, message)
	}
	if decodeErr != nil {
		return nil, fmt.Errorf("cannot read the classifier response: %w", decodeErr)
	}
	if decoded.Answers == nil {
		return nil, errors.New("the classifier response has no answers")
	}
	probabilities := make(map[string]float64, len(decoded.Answers))
	for key, a := range decoded.Answers {
		if a.Noul != nil {
			probabilities[key] = *a.Noul
		}
	}
	return probabilities, nil
}
