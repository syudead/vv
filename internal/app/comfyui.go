package app

import (
	"context"
	"sync"

	"github.com/syudead/vv/internal/domain"
)

// ComfyUISettingsStore は ComfyUI の接続先の保存先である。internal/store の *SettingsStore が満たす。
type ComfyUISettingsStore interface {
	ComfyUISettings(ctx context.Context) (domain.ComfyUISettings, error)
	SaveComfyUISettings(ctx context.Context, settings domain.ComfyUISettings) error
}

// ComfyUIProbe は保存した接続先に ComfyUI（自前か Comfy Cloud）が応えるかを確かめる。
// 画像は作らない。internal/comfyui の *Client が満たす。
type ComfyUIProbe interface {
	Check(ctx context.Context) error
}

// ComfyUIConnection は画像編集の提供元 ComfyUI への接続先の設定と確かめである。サムネイルの
// 編集の機能とは別に持つ（docs/design-docs/ai-layer.md、
// specs/045-comfyui-thumbnails/research.md R-1）。
type ComfyUIConnection struct {
	store ComfyUISettingsStore
	probe ComfyUIProbe
	mu    sync.Mutex
}

// NewComfyUIConnection は接続先の設定と確かめを組み立てる。
func NewComfyUIConnection(store ComfyUISettingsStore, probe ComfyUIProbe) *ComfyUIConnection {
	return &ComfyUIConnection{store: store, probe: probe}
}

// Settings は保存した接続先を返す。何も保存されていなければ Provider が空である。
func (c *ComfyUIConnection) Settings(ctx context.Context) (domain.ComfyUISettings, error) {
	return c.store.ComfyUISettings(ctx)
}

// SaveSettings は保存済みの接続先に update を当てて保存し、保存後の値を返す。キーの
// 残し方と規則は domain.ApplyComfyUISettingsUpdate に従い、合わなければ
// domain.ErrInvalidComfyUISettings を包んだ誤りを返して何も保存しない。読んでから書くまでを
// 排他にし、同時の保存が互いの変更を失わせないようにする。
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

// Check は保存した接続先に ComfyUI が応えるかを確かめる。要求の側が渡した URL へは送らない。
func (c *ComfyUIConnection) Check(ctx context.Context) error {
	return c.probe.Check(ctx)
}
