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

// Image classification can take minutes on CPU-only hosts.
const requestTimeout = 10 * time.Minute

const maxResponseBytes = 1 << 20

// Keep models loaded between consecutive classifications.
const keepAlive = "10m"

// Batch sixteen questions to fit CPU hosts where sixty-four image-backed questions can exhaust
// memory; see docs/design-docs/ai-layer.md.
const BatchSize = 16

type SettingsSource interface {
	OllamaSettings(ctx context.Context) (domain.OllamaSettings, error)
}

// Classifier requires New; the zero value is unusable.
type Classifier struct {
	settings SettingsSource
	http     *http.Client
}

// New honors environment proxy settings.
func New(settings SettingsSource) *Classifier {
	return &Classifier{settings: settings, http: &http.Client{Timeout: requestTimeout}}
}

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

// Classify returns yes probabilities, omits unanswered questions, and combines BatchSize batches
// using the same input.
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
		// Expose only Ollama's structured error message; raw responses could reveal content from an
		// unrelated service.
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
