package ollama

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/syudead/vv/internal/domain"
)

// Generator answers one structured value per call through Ollama's chat API, which enforces the
// JSON schema in format for any chat model; see docs/design-docs/ai-layer.md. Use NewGenerator.
type Generator struct {
	settings SettingsSource
	http     *http.Client
}

// NewGenerator waits as long as a classification, since a CPU host can take minutes to answer.
func NewGenerator(settings SettingsSource) *Generator {
	return &Generator{settings: settings, http: &http.Client{Timeout: requestTimeout}}
}

func NewGeneratorWithHTTPClient(settings SettingsSource, client *http.Client) *Generator {
	return &Generator{settings: settings, http: client}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model     string          `json:"model"`
	Messages  []chatMessage   `json:"messages"`
	Format    json.RawMessage `json:"format"`
	Stream    bool            `json:"stream"`
	KeepAlive string          `json:"keep_alive"`
}

type chatResponse struct {
	Message *chatMessage `json:"message"`
	Error   string       `json:"error"`
}

// Generate reads the saved endpoint and generation model on every call. An answer that is not JSON
// is ErrGenerationUnreadable and is not retried, because a second call doubles a wait of minutes.
func (g *Generator) Generate(ctx context.Context, req domain.GenerationRequest) (json.RawMessage, error) {
	settings, err := g.settings.OllamaSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot read the Ollama settings: %w", err)
	}
	if settings.GenerationModel == "" {
		return nil, kindErrorf(domain.ErrOllamaModel, nil, "no generation model is saved")
	}
	status, raw, err := postJSON(ctx, g.http, settings.Endpoint, "/api/chat", chatRequest{
		Model:     settings.GenerationModel,
		Messages:  []chatMessage{{Role: "user", Content: req.Prompt}},
		Format:    req.Schema,
		Stream:    false,
		KeepAlive: keepAlive,
	})
	if err != nil {
		return nil, err
	}
	var decoded chatResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, kindErrorf(domain.ErrNotOllama, nil, "the server at %s is not Ollama (%s)", settings.Endpoint,
			statusMessage(status, ""))
	}
	if decoded.Error != "" {
		return nil, kindErrorf(domain.ErrOllamaModel, nil, "model %q: %s", settings.GenerationModel,
			statusMessage(status, decoded.Error))
	}
	if status != http.StatusOK || decoded.Message == nil {
		return nil, kindErrorf(domain.ErrNotOllama, nil, "the server at %s is not Ollama (%s)", settings.Endpoint,
			statusMessage(status, ""))
	}
	content := strings.TrimSpace(decoded.Message.Content)
	if !json.Valid([]byte(content)) {
		return nil, kindErrorf(domain.ErrGenerationUnreadable, nil, "model %q answered something that is not JSON",
			settings.GenerationModel)
	}
	return json.RawMessage(content), nil
}
