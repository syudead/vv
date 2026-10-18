package ollama

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
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
// remain distinct.
func (c *Classifier) Check(ctx context.Context) error {
	settings, err := c.settings.OllamaSettings(ctx)
	if err != nil {
		return fmt.Errorf("cannot read the Ollama settings: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()

	var version versionBody
	if err := c.getJSON(ctx, settings.Endpoint, "/api/version", &version); err != nil {
		return err
	}
	if version.Version == "" {
		return fmt.Errorf("the server at %s is not Ollama", settings.Endpoint)
	}
	var tags tagsBody
	if err := c.getJSON(ctx, settings.Endpoint, "/api/tags", &tags); err != nil {
		return err
	}
	for _, m := range tags.Models {
		if sameModel(m.Name, settings.Model) || sameModel(m.Model, settings.Model) {
			return nil
		}
	}
	return fmt.Errorf("ollama %s at %s has no model named %q; pull it first", version.Version, settings.Endpoint, settings.Model)
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
		return fmt.Errorf("cannot reach Ollama at %s: %w", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("the server at %s is not Ollama (%s answered %d)", endpoint, path, resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(out); err != nil {
		return fmt.Errorf("the server at %s is not Ollama (%s is not JSON)", endpoint, path)
	}
	return nil
}
