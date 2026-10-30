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
	message, err := postChat(ctx, g.http, settings, chatRequest{
		Model:     settings.GenerationModel,
		Messages:  []chatMessage{{Role: "user", Content: req.Prompt}},
		Format:    req.Schema,
		Stream:    false,
		KeepAlive: keepAlive,
	})
	if err != nil {
		return nil, err
	}
	content := strings.TrimSpace(message.Content)
	if !json.Valid([]byte(content)) {
		return nil, kindErrorf(domain.ErrGenerationUnreadable, nil, "model %q answered something that is not JSON",
			settings.GenerationModel)
	}
	return json.RawMessage(content), nil
}
