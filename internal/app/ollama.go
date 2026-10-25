package app

import (
	"context"
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
	// Models lists the model names at the saved endpoint in the provider's order.
	Models(ctx context.Context) ([]string, error)
}

// Provider settings remain independent of feature enablement; see docs/design-docs/ai-layer.md.
type OllamaConnection struct {
	store OllamaSettingsStore
	probe OllamaProbe
	mu    sync.Mutex
	// Optional save hook resumes duplicate pairs waiting for connection settings; see
	// docs/design-docs/duplicate-judging.md.
	saved func(context.Context)
}

func NewOllamaConnection(store OllamaSettingsStore, probe OllamaProbe) *OllamaConnection {
	return &OllamaConnection{store: store, probe: probe}
}

// Register OnSaved once before accepting requests.
func (o *OllamaConnection) OnSaved(saved func(context.Context)) {
	o.saved = saved
}

// Read settings and explicit-save status together, matching duplicate-judging wait rules.
func (o *OllamaConnection) Settings(ctx context.Context) (settings domain.OllamaSettings, saved bool, err error) {
	return o.store.SavedOllamaSettings(ctx)
}

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
	return normalized, nil
}

// Check only saved connection/model presence; features separately verify classification support and
// never supply arbitrary probe URLs.
func (o *OllamaConnection) Check(ctx context.Context) error {
	return o.probe.Check(ctx)
}

// Models lists the names at the saved endpoint only, so a request never chooses where the server connects.
func (o *OllamaConnection) Models(ctx context.Context) ([]string, error) {
	return o.probe.Models(ctx)
}

// AssistantReadiness reads the saved connection without contacting Ollama.
func (o *OllamaConnection) AssistantReadiness(ctx context.Context) (domain.AssistantNotReady, error) {
	settings, saved, err := o.store.SavedOllamaSettings(ctx)
	if err != nil {
		return domain.AssistantReady, err
	}
	return domain.AssistantReadiness(settings, saved), nil
}
