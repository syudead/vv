package app

import (
	"context"
	"fmt"

	"github.com/syudead/vv/internal/domain"
)

// WorkInfoPluginProxyStore persists plugin proxy settings; implemented by store.SettingsStore.
type WorkInfoPluginProxyStore interface {
	// WorkInfoPluginProxyURL returns an empty string when unset.
	WorkInfoPluginProxyURL(ctx context.Context) (string, error)
	// SaveWorkInfoPluginProxyURL uses an empty string to disable the proxy.
	SaveWorkInfoPluginProxyURL(ctx context.Context, value string) error
}

// WorkInfoPluginProxy rereads storage for each run; settings changes affect the next run while active runs retain their snapshot.
type WorkInfoPluginProxy struct {
	store WorkInfoPluginProxyStore
}

// NewWorkInfoPluginProxy creates proxy settings backed by store.
func NewWorkInfoPluginProxy(store WorkInfoPluginProxyStore) *WorkInfoPluginProxy {
	return &WorkInfoPluginProxy{store: store}
}

// URL returns an empty string when unset.
func (p *WorkInfoPluginProxy) URL(ctx context.Context) (string, error) {
	return p.store.WorkInfoPluginProxyURL(ctx)
}

// SetURL validates with ParseWorkInfoPluginProxyURL before saving; empty disables the proxy. Invalid values return ErrWorkInfoPluginProxyURLInvalid without writes.
func (p *WorkInfoPluginProxy) SetURL(ctx context.Context, raw string) (string, error) {
	if _, err := domain.ParseWorkInfoPluginProxyURL(raw); err != nil {
		return "", err
	}
	if err := p.store.SaveWorkInfoPluginProxyURL(ctx, raw); err != nil {
		return "", err
	}
	return raw, nil
}

// RunNetwork snapshots saved proxy settings; absent/empty means direct access. Invalid persisted values fail closed rather than bypassing the proxy.
func (p *WorkInfoPluginProxy) RunNetwork(ctx context.Context) (domain.WorkInfoRunNetwork, error) {
	raw, err := p.store.WorkInfoPluginProxyURL(ctx)
	if err != nil {
		return domain.WorkInfoRunNetwork{}, err
	}
	proxy, err := domain.ParseWorkInfoPluginProxyURL(raw)
	if err != nil {
		return domain.WorkInfoRunNetwork{}, fmt.Errorf("cannot use the saved work-info plugin proxy: %w", err)
	}
	return domain.WorkInfoRunNetwork{Proxy: proxy}, nil
}
