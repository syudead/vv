package comfyui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/syudead/vv/internal/domain"
)

const CloudBaseURL = "https://cloud.comfy.org/api"

// Connection checks do not generate images, so use a short timeout.
const checkTimeout = 15 * time.Second

const maxResponseBytes = 1 << 20

// Custom node installations can make /object_info several megabytes.
const maxObjectInfoBytes = 64 << 20

type SettingsSource interface {
	ComfyUISettings(ctx context.Context) (domain.ComfyUISettings, error)
}

// Client requires New; its zero value is unusable.
type Client struct {
	settings SettingsSource
	http     *http.Client
	cloud    string
	// Override diffusion models only for self-hosted ComfyUI; Cloud owns its model inventory.
	diffusionModel string

	// startProgress opens the independently managed progress stream.
	startProgress func(context.Context, *target, string) *progressWatch

	// progressMu protects per-run progress sockets.
	progressMu sync.Mutex
	progress   map[string]*progressWatch
}

type Options struct {
	// Nil uses the default HTTP client.
	HTTPClient *http.Client
	// Empty uses CloudBaseURL.
	CloudBaseURL string
	// DiffusionModel replaces the self-hosted UNETLoader file only, preserving text encoder, VAE, and
	// step count; empty uses the bundled workflow model.
	DiffusionModel string
}

// New uses an HTTP client that honors environment proxy settings.
func New(settings SettingsSource) *Client {
	return NewWithOptions(settings, Options{})
}

func NewWithOptions(settings SettingsSource, opts Options) *Client {
	client := &http.Client{}
	if opts.HTTPClient != nil {
		copied := *opts.HTTPClient
		client = &copied
	}
	// Do not follow redirects with API keys; checks reject redirects and output downloads follow them
	// without the key.
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	cloud := opts.CloudBaseURL
	if cloud == "" {
		cloud = CloudBaseURL
	}
	c := &Client{
		settings: settings, http: client, cloud: strings.TrimRight(cloud, "/"),
		diffusionModel: strings.TrimSpace(opts.DiffusionModel), progress: map[string]*progressWatch{},
	}
	c.startProgress = c.watchProgress
	return c
}

// Only compatible models on self-hosted connections accept overrides.
func (c *Client) diffusionModelFor(conn domain.ComfyUISettings) string {
	if conn.Provider != domain.ComfyUISelfHosted || !presetFor(conn.EffectiveModel()).overridable {
		return ""
	}
	return c.diffusionModel
}

var errAPIKeyRejected = errors.New("the API key was rejected by Comfy Cloud; check the key under Settings")

// Check validates the saved endpoint and required workflow nodes/models; missing prerequisites
// return *domain.ComfyUIMissingError, while connection, identity, and key failures remain distinct.
func (c *Client) Check(ctx context.Context) error {
	settings, err := c.settings.ComfyUISettings(ctx)
	if err != nil {
		return fmt.Errorf("cannot read the ComfyUI settings: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()

	switch settings.Provider {
	case domain.ComfyUISelfHosted:
		var stats systemStats
		name := "ComfyUI at " + settings.URL
		if err := c.getJSON(ctx, settings.URL, "/system_stats", "", name, maxResponseBytes, &stats); err != nil {
			return err
		}
		if !stats.valid() {
			return fmt.Errorf("the server at %s is not ComfyUI (/system_stats has no system information)", settings.URL)
		}
		return c.checkRequired(ctx, settings.URL, "", name, settings)
	case domain.ComfyUICloud:
		if settings.APIKey == "" {
			return domain.ErrComfyUIAPIKeyMissing
		}
		var jobs jobList
		if err := c.getJSON(ctx, c.cloud, "/jobs?limit=1", settings.APIKey, "Comfy Cloud", maxResponseBytes, &jobs); err != nil {
			return err
		}
		if jobs.Jobs == nil {
			return fmt.Errorf("the server at %s is not Comfy Cloud (/jobs has no job list)", c.cloud)
		}
		return c.checkRequired(ctx, c.cloud, settings.APIKey, "Comfy Cloud", settings)
	default:
		return errors.New("no ComfyUI connection is saved; choose a provider and save it first")
	}
}

func (c *Client) checkRequired(ctx context.Context, base, apiKey, name string, settings domain.ComfyUISettings) error {
	required, err := workflowRequirement(settings.EffectiveModel(), c.diffusionModelFor(settings))
	if err != nil {
		return err
	}
	var info objectInfo
	if err := c.getJSON(ctx, base, "/object_info", apiKey, name, maxObjectInfoBytes, &info); err != nil {
		return err
	}
	if items := required.missing(info); len(items) > 0 {
		return &domain.ComfyUIMissingError{Items: items}
	}
	return nil
}

// Recognize ComfyUI by system.os, system.python_version, and the devices array.
type systemStats struct {
	System *struct {
		OS            string `json:"os"`
		PythonVersion string `json:"python_version"`
	} `json:"system"`
	Devices *[]json.RawMessage `json:"devices"`
}

func (s systemStats) valid() bool {
	return s.System != nil && s.System.OS != "" && s.System.PythonVersion != "" && s.Devices != nil
}

// An empty jobs array is a valid list.
type jobList struct {
	Jobs *[]json.RawMessage `json:"jobs"`
}

// getJSON bounds response reads and never includes raw bodies in errors, which could expose content
// from a different service.
func (c *Client) getJSON(ctx context.Context, base, path, apiKey, name string, limit int64, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+path, nil)
	if err != nil {
		return fmt.Errorf("cannot build the request to %s: %w", name, err)
	}
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("cannot reach %s: %w", name, err)
	}
	defer func() { _ = resp.Body.Close() }()
	route := strings.SplitN(path, "?", 2)[0]
	if apiKey != "" && (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden) {
		return errAPIKeyRejected
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("the server at %s is not ComfyUI (%s answered %d)", base, route, resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, limit)).Decode(out); err != nil {
		return fmt.Errorf("the server at %s is not ComfyUI (%s is not JSON)", base, route)
	}
	return nil
}
