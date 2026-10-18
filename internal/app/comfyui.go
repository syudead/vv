package app

import (
	"context"
	"sync"

	"github.com/syudead/vv/internal/domain"
)

type ComfyUISettingsStore interface {
	ComfyUISettings(ctx context.Context) (domain.ComfyUISettings, error)
	SaveComfyUISettings(ctx context.Context, settings domain.ComfyUISettings) error
}

// Probe connectivity without generating an image.
type ComfyUIProbe interface {
	Check(ctx context.Context) error
}

// Keep provider settings separate from thumbnail editing; see docs/design-docs/ai-layer.md and
// thumbnail-editing.md.
type ComfyUIConnection struct {
	store ComfyUISettingsStore
	probe ComfyUIProbe
	mu    sync.Mutex
}

func NewComfyUIConnection(store ComfyUISettingsStore, probe ComfyUIProbe) *ComfyUIConnection {
	return &ComfyUIConnection{store: store, probe: probe}
}

// An empty Provider means no saved connection.
func (c *ComfyUIConnection) Settings(ctx context.Context) (domain.ComfyUISettings, error) {
	return c.store.ComfyUISettings(ctx)
}

// Serialize read-modify-write to avoid lost concurrent changes; domain validates updates and secret
// retention before any save.
func (c *ComfyUIConnection) SaveSettings(ctx context.Context, update domain.ComfyUISettingsUpdate) (domain.ComfyUISettings, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	current, err := c.store.ComfyUISettings(ctx)
	if err != nil {
		return domain.ComfyUISettings{}, err
	}
	next, err := domain.ApplyComfyUISettingsUpdate(current, update)
	if err != nil {
		return domain.ComfyUISettings{}, err
	}
	if err := c.store.SaveComfyUISettings(ctx, next); err != nil {
		return domain.ComfyUISettings{}, err
	}
	return next, nil
}

// Check only the saved endpoint, never a request-supplied URL.
func (c *ComfyUIConnection) Check(ctx context.Context) error {
	return c.probe.Check(ctx)
}
