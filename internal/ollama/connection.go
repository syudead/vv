package ollama

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/syudead/vv/internal/domain"
)

// Connection checks do not load models, so use a short deadline.
const checkTimeout = 15 * time.Second

type versionBody struct {
	Version string `json:"version"`
}

type tagsBody struct {
	Models []struct {
		Name  string `json:"name"`
		Model string `json:"model"`
	} `json:"models"`
}

// Check verifies the saved endpoint and model listing without loading/classifying; feature callers
// separately verify classification support, and connectivity, identity, and missing-model failures
// remain distinct. A saved generation model is looked for after the decision model.
func (c *Classifier) Check(ctx context.Context) error {
	settings, err := c.settings.OllamaSettings(ctx)
	if err != nil {
		return fmt.Errorf("cannot read the Ollama settings: %w", err)
	}
	version, tags, err := c.listModels(ctx, settings.Endpoint)
	if err != nil {
		return err
	}
	if !hasModel(tags, settings.Model) {
		return kindErrorf(domain.ErrOllamaModel, nil, "ollama %s at %s has no model named %q; pull it first",
			version, settings.Endpoint, settings.Model)
	}
	if settings.GenerationModel != "" && !hasModel(tags, settings.GenerationModel) {
		return kindErrorf(domain.ErrGenerationModelMissing, nil, "ollama %s at %s has no generation model named %q; pull it first",
			version, settings.Endpoint, settings.GenerationModel)
	}
	return nil
}

// Models lists the models at the saved endpoint in Ollama's order, each with its capabilities, without
// loading any. A model that /api/show refuses is listed with no capabilities.
func (c *Classifier) Models(ctx context.Context) ([]domain.OllamaModel, error) {
	settings, err := c.settings.OllamaSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot read the Ollama settings: %w", err)
	}
	_, tags, err := c.listModels(ctx, settings.Endpoint)
	if err != nil {
		return nil, err
	}
	models := make([]domain.OllamaModel, 0, len(tags.Models))
	for _, m := range tags.Models {
		name := cmp.Or(m.Name, m.Model)
		if name == "" {
			continue
		}
		capabilities, err := c.Capabilities(ctx, settings.Endpoint, name)
		if err != nil && !errors.Is(err, domain.ErrOllamaModel) {
			return nil, err
		}
		models = append(models, domain.OllamaModel{Name: name, Capabilities: capabilities})
	}
	return models, nil
}

type showRequest struct {
	Model string `json:"model"`
}

type showResponse struct {
	Capabilities []string `json:"capabilities"`
	Error        string   `json:"error"`
}

// Capabilities reads tools and vision from /api/show; an Ollama that reports no capabilities has neither.
// The caller passes the saved endpoint, never one from a request.
func (c *Classifier) Capabilities(ctx context.Context, endpoint, model string) (domain.ModelCapabilities, error) {
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	status, raw, err := postJSON(ctx, c.http, endpoint, "/api/show", showRequest{Model: model})
	if err != nil {
		return domain.ModelCapabilities{}, err
	}
	var decoded showResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return domain.ModelCapabilities{}, notOllama(endpoint, status)
	}
	if decoded.Error != "" {
		return domain.ModelCapabilities{}, kindErrorf(domain.ErrOllamaModel, nil, "model %q: %s", model,
			statusMessage(status, decoded.Error))
	}
	if status != http.StatusOK {
		return domain.ModelCapabilities{}, notOllama(endpoint, status)
	}
	return domain.ModelCapabilities{
		Tools:  slices.Contains(decoded.Capabilities, "tools"),
		Vision: slices.Contains(decoded.Capabilities, "vision"),
	}, nil
}

// listModels confirms that the endpoint is Ollama before reading its tags.
func (c *Classifier) listModels(ctx context.Context, endpoint string) (string, tagsBody, error) {
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	var version versionBody
	if err := c.getJSON(ctx, endpoint, "/api/version", &version); err != nil {
		return "", tagsBody{}, err
	}
	if version.Version == "" {
		return "", tagsBody{}, kindErrorf(domain.ErrNotOllama, nil, "the server at %s is not Ollama", endpoint)
	}
	var tags tagsBody
	if err := c.getJSON(ctx, endpoint, "/api/tags", &tags); err != nil {
		return "", tagsBody{}, err
	}
	return version.Version, tags, nil
}

func hasModel(tags tagsBody, want string) bool {
	for _, m := range tags.Models {
		if sameModel(m.Name, want) || sameModel(m.Model, want) {
			return true
		}
	}
	return false
}

// An omitted model tag means :latest.
func sameModel(listed, want string) bool {
	if listed == "" {
		return false
	}
	if listed == want {
		return true
	}
	return !strings.Contains(want, ":") && listed == want+":latest"
}

// Never include raw response bodies in errors because the endpoint may be a different service.
func (c *Classifier) getJSON(ctx context.Context, endpoint, path string, out any) error {
	url := strings.TrimRight(endpoint, "/") + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("cannot build the request to Ollama: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return transportError(endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return kindErrorf(domain.ErrNotOllama, nil, "the server at %s is not Ollama (%s answered %d)", endpoint, path, resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(out); err != nil {
		return kindErrorf(domain.ErrNotOllama, nil, "the server at %s is not Ollama (%s is not JSON)", endpoint, path)
	}
	return nil
}
