package app

import (
	"context"
	"fmt"
	"sync"

	"github.com/syudead/vv/internal/domain"
)

type OllamaSettingsStore interface {
	// Return settings/defaults and saved-presence from one snapshot.
	SavedOllamaSettings(ctx context.Context) (domain.OllamaSettings, bool, error)
	SaveOllamaSettings(ctx context.Context, settings domain.OllamaSettings) error
}

// Check endpoint and model availability without classification.
type OllamaProbe interface {
	Check(ctx context.Context) error
	// Models lists the models at the saved endpoint in the provider's order, with what each can do.
	Models(ctx context.Context) ([]domain.OllamaModel, error)
	// Capabilities reads what model can do at endpoint, which the caller takes from the saved settings.
	Capabilities(ctx context.Context, endpoint, model string) (domain.ModelCapabilities, error)
}

type modelKey struct{ endpoint, model string }

// Provider settings remain independent of feature enablement; see docs/design-docs/ai-layer.md.
type OllamaConnection struct {
	store OllamaSettingsStore
	probe OllamaProbe
	mu    sync.Mutex
	// Optional save hook resumes duplicate pairs waiting for connection settings; see
	// docs/design-docs/duplicate-judging.md.
	saved func(context.Context)
	// capabilities keeps each endpoint and model pair's answer, because readiness is read whenever the panel opens.
	capabilitiesMu sync.Mutex
	capabilities   map[modelKey]domain.ModelCapabilities
}

func NewOllamaConnection(store OllamaSettingsStore, probe OllamaProbe) *OllamaConnection {
	return &OllamaConnection{store: store, probe: probe, capabilities: map[modelKey]domain.ModelCapabilities{}}
}

// Register OnSaved once before accepting requests.
func (o *OllamaConnection) OnSaved(saved func(context.Context)) {
	o.saved = saved
}

// Read settings and explicit-save status together, matching duplicate-judging wait rules.
func (o *OllamaConnection) Settings(ctx context.Context) (settings domain.OllamaSettings, saved bool, err error) {
	return o.store.SavedOllamaSettings(ctx)
}

// SaveSettings also rereads the generation model's capabilities from the stored settings, so the server only
// connects to a saved endpoint; a failed read leaves them to readiness.
func (o *OllamaConnection) SaveSettings(ctx context.Context, settings domain.OllamaSettings) (domain.OllamaSettings, error) {
	normalized, err := domain.NormalizeOllamaSettings(settings)
	if err != nil {
		return domain.OllamaSettings{}, err
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.store.SaveOllamaSettings(ctx, normalized); err != nil {
		return domain.OllamaSettings{}, err
	}
	if o.saved != nil {
		o.saved(ctx)
	}
	if stored, _, err := o.store.SavedOllamaSettings(ctx); err == nil && stored.GenerationModel != "" {
		_, _ = o.readCapabilities(ctx, stored)
	}
	return normalized, nil
}

// Check only saved connection/model presence and the generation model's tools capability; features
// separately verify classification support and never supply arbitrary probe URLs.
func (o *OllamaConnection) Check(ctx context.Context) error {
	if err := o.probe.Check(ctx); err != nil {
		return err
	}
	settings, _, err := o.store.SavedOllamaSettings(ctx)
	if err != nil {
		return fmt.Errorf("cannot read the Ollama settings: %w", err)
	}
	if settings.GenerationModel == "" {
		return nil
	}
	capabilities, err := o.readCapabilities(ctx, settings)
	if err != nil {
		return err
	}
	if !capabilities.Tools {
		return fmt.Errorf("%w: Ollama has %q, but it cannot call tools", domain.ErrGenerationModelWithoutTools,
			settings.GenerationModel)
	}
	return nil
}

// Models lists the models at the saved endpoint only, so a request never chooses where the server connects.
func (o *OllamaConnection) Models(ctx context.Context) ([]domain.OllamaModel, error) {
	return o.probe.Models(ctx)
}

// AssistantReadiness asks Ollama only on the first read for an endpoint and model pair. When Ollama cannot
// be reached then, the assistant counts as ready and the turn reports the connection failure.
func (o *OllamaConnection) AssistantReadiness(ctx context.Context) (domain.AssistantStatus, error) {
	settings, saved, err := o.store.SavedOllamaSettings(ctx)
	if err != nil {
		return domain.AssistantStatus{}, err
	}
	if notReady := domain.AssistantReadiness(settings, saved); notReady != domain.AssistantReady {
		return domain.AssistantStatus{NotReady: notReady}, nil
	}
	key := modelKey{endpoint: settings.Endpoint, model: settings.GenerationModel}
	o.capabilitiesMu.Lock()
	capabilities, known := o.capabilities[key]
	o.capabilitiesMu.Unlock()
	if !known {
		if capabilities, err = o.readCapabilities(ctx, settings); err != nil {
			return domain.AssistantStatus{}, nil
		}
	}
	return domain.AssistantStatusOf(capabilities), nil
}

// readCapabilities replaces the kept answer for the pair, and forgets it when Ollama cannot answer.
func (o *OllamaConnection) readCapabilities(ctx context.Context, settings domain.OllamaSettings) (domain.ModelCapabilities, error) {
	key := modelKey{endpoint: settings.Endpoint, model: settings.GenerationModel}
	capabilities, err := o.probe.Capabilities(ctx, key.endpoint, key.model)
	o.capabilitiesMu.Lock()
	defer o.capabilitiesMu.Unlock()
	if err != nil {
		delete(o.capabilities, key)
		return domain.ModelCapabilities{}, err
	}
	o.capabilities[key] = capabilities
	return capabilities, nil
}
